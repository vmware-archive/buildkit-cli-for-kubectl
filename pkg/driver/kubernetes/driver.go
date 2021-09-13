// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/pkg/errors"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/execconn"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/manifest"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/podchooser"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/imagetools"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/store"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	DriverName = "kubernetes"
)

const (
	// valid values for driver-opt loadbalance
	LoadbalanceRandom = "random"
	LoadbalanceSticky = "sticky"

	// valid values for driver-opt worker
	WorkerContainerd           = "containerd"
	WorkerRunc                 = "runc"
	DefaultContainerdNamespace = "k8s.io"
	DefaultContainerdSockPath  = "/run/containerd/containerd.sock"
	DefaultDockerSockPath      = "/var/run/docker.sock"

	//DefaultContainerRuntime    = "containerd" // TODO figure out a way to autodiscover this if not specified for better UX
	DefaultContainerRuntime = "docker" // Temporary since most kubernetes clusters are still Docker based today...

	// TODO - consider adding other default values here to aid users in fine-tuning by editing the configmap post deployment
	DefaultConfigFileTemplate = `# Default buildkitd configuration.  Use --config <path/to/file> to override during create
debug = false
[worker.containerd]
  namespace = "{{ .ContainerdNamespace }}"
`
)

type Driver struct {
	driver.InitConfig
	factory              driver.Factory
	minReplicas          int
	deployment           *appsv1.Deployment
	configMap            *corev1.ConfigMap
	clientset            *kubernetes.Clientset
	deploymentClient     clientappsv1.DeploymentInterface
	replicaSetClient     clientappsv1.ReplicaSetInterface
	podClient            clientcorev1.PodInterface
	configMapClient      clientcorev1.ConfigMapInterface
	secretClient         clientcorev1.SecretInterface
	podChooser           podchooser.PodChooser
	eventClient          clientcorev1.EventInterface
	userSpecifiedRuntime bool
	userSpecifiedConfig  bool
	namespace            string
	loadbalance          string
	authHintMessage      string
}

func (d *Driver) Bootstrap(ctx context.Context, l progress.Logger) error {
	return progress.Wrap("[internal] booting buildkit", l, func(sub progress.SubLogger) error {
		return sub.Wrap(
			fmt.Sprintf("waiting for %d pods to be ready for %s", d.minReplicas, d.deployment.Name),
			func() error {
				if err := d.wait(ctx, sub); err != nil {
					return err
				}
				return nil
			})
	})
}

func isChildOf(objectMeta metav1.ObjectMeta, parentUID string) bool {
	for _, ownerRef := range objectMeta.OwnerReferences {
		if string(ownerRef.UID) == parentUID {
			return true
		}
	}
	return false
}

// Idempotently create and wait for the builder to start up
func (d *Driver) wait(ctx context.Context, sub progress.SubLogger) error {
	// Create the config map first
	err := d.createConfigMap(ctx, sub)
	if err != nil {
		return err
	}

	// Now try to converge to a running builder
	return d.createBuilder(ctx, sub, d.userSpecifiedRuntime)
}

func (d *Driver) Info(ctx context.Context) (*driver.Info, error) {
	depl, err := d.deploymentClient.Get(ctx, d.deployment.Name, metav1.GetOptions{})
	if err != nil {
		// TODO: return err if err != ErrNotFound
		return &driver.Info{
			Status: driver.Inactive,
		}, nil
	}
	if depl.Status.ReadyReplicas <= 0 {
		return &driver.Info{
			Status: driver.Stopped,
		}, nil
	}
	pods, err := podchooser.ListRunningPods(ctx, d.podClient, depl)
	if err != nil {
		return nil, err
	}
	var dynNodes []store.Node
	for _, p := range pods {
		node := store.Node{
			Name: p.Name,
			// Other fields are unset (TODO: detect real platforms)
		}
		dynNodes = append(dynNodes, node)
	}
	return &driver.Info{
		Status:       driver.Running,
		DynamicNodes: dynNodes,
	}, nil
}

func (d *Driver) Stop(ctx context.Context, force bool) error {
	// future version may scale the replicas to zero here
	return nil
}

func (d *Driver) Rm(ctx context.Context, force bool) error {
	if err := d.deploymentClient.Delete(ctx, d.deployment.Name, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "error while calling deploymentClient.Delete for %q", d.deployment.Name)
	}
	// TODO - consider checking for our expected labels and preserve pre-existing ConfigMaps
	if err := d.configMapClient.Delete(ctx, d.configMap.Name, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "error while calling configMapClient.Delete for %q", d.configMap.Name)
	}
	return nil
}

func (d *Driver) Clients(ctx context.Context) (*driver.BuilderClients, error) {
	restClient := d.clientset.CoreV1().RESTClient()
	restClientConfig, err := d.KubeClientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	pod, otherPods, err := d.podChooser.ChoosePod(ctx)
	if err != nil {
		return nil, err
	}
	if len(pod.Spec.Containers) == 0 {
		return nil, errors.Errorf("pod %s does not have any container", pod.Name)
	}
	chosenNode, err := buildNodeClient(ctx, pod, restClient, restClientConfig)
	if err != nil {
		return nil, err
	}
	res := &driver.BuilderClients{
		ChosenNode: *chosenNode,
		OtherNodes: []driver.NodeClient{},
	}
	for _, pod := range otherPods {
		otherNode, err := buildNodeClient(ctx, pod, restClient, restClientConfig)
		// TODO - consider allowing partial failure if a node is down but others are available...
		if err != nil {
			return nil, err
		}
		res.OtherNodes = append(res.OtherNodes, *otherNode)
	}
	return res, err
}

func buildNodeClient(ctx context.Context, pod *corev1.Pod, restClient rest.Interface, restClientConfig *rest.Config) (*driver.NodeClient, error) {
	containerName := pod.Spec.Containers[0].Name
	cmd := []string{"buildctl", "dial-stdio"}
	nodeClient := &driver.NodeClient{
		NodeName:    pod.Name,
		ClusterAddr: pod.Status.PodIP,
	}
	conn, err := execconn.ExecConn(restClient, restClientConfig,
		pod.Namespace, pod.Name, containerName, cmd)
	if err != nil {
		return nil, err
	}

	buildkitClient, err := client.New(ctx, "", client.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return conn, nil
	}))
	if err != nil {
		return nil, err
	}
	nodeClient.BuildKitClient = buildkitClient
	return nodeClient, nil
}

// TODO debugging concurrent access problems
// type wrappedConn struct {
// 	net.Conn
// 	name string
// }
//
// func (wc wrappedConn) Close() error {
// 	return wc.Conn.Close()
// }

func (d *Driver) RuntimeSockProxy(ctx context.Context, name string) (net.Conn, error) {
	restClient := d.clientset.CoreV1().RESTClient()
	restClientConfig, err := d.InitConfig.KubeClientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	pods, err := podchooser.ListRunningPods(ctx, d.podClient, d.deployment)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods {
		// TODO - this should really be node name based not pod name based
		if pod.Name != name {
			continue
		}

		if len(pod.Spec.Containers) == 0 {
			return nil, errors.Errorf("pod %s does not have any container", pod.Name)
		}
		containerName := pod.Spec.Containers[0].Name

		runtime := pod.ObjectMeta.Labels["runtime"]
		var sockPath string
		switch runtime {
		case "containerd":
			sockPath = "unix://" + DefaultContainerdSockPath
		case "docker":
			sockPath = "unix://" + DefaultDockerSockPath
		default:
			return nil, fmt.Errorf("unexpected runtime label (%v) on pod (%s)", runtime, pod.Name)
		}
		cmd := []string{"buildctl", "--addr", sockPath, "dial-stdio"}
		return execconn.ExecConn(restClient, restClientConfig,
			pod.Namespace, pod.Name, containerName, cmd)
	}

	return nil, fmt.Errorf("no available builder pods for %s", name)

}

func (d *Driver) GetVersion(ctx context.Context) (string, error) {
	restClient := d.clientset.CoreV1().RESTClient()
	restClientConfig, err := d.KubeClientConfig.ClientConfig()
	if err != nil {
		return "", err
	}
	pod, _, err := d.podChooser.ChoosePod(ctx)
	if err != nil {
		return "", err
	}
	if len(pod.Spec.Containers) == 0 {
		return "", errors.Errorf("pod %s does not have any container", pod.Name)
	}
	containerName := pod.Spec.Containers[0].Name
	cmd := []string{"buildkitd", "--version"}
	buf := &bytes.Buffer{}

	req := restClient.
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	req.Timeout(10 * time.Second)
	u := req.URL()
	exec, err := remotecommand.NewSPDYExecutor(restClientConfig, "POST", u)
	if err != nil {
		return "", err
	}
	serr := exec.Stream(remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if serr != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), err
}

func (d *Driver) List(ctx context.Context) ([]driver.Builder, error) {
	var builders []driver.Builder
	depls, err := d.deploymentClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup builder deployments")
	}
	for _, depl := range depls.Items {
		// Check for the builkit annotation, else skip
		if _, found := depl.ObjectMeta.Annotations[manifest.AnnotationKey]; !found {
			continue
		}
		builder := driver.Builder{
			Name:   depl.ObjectMeta.Name,
			Driver: DriverName,
		}
		pods, err := podchooser.ListRunningPods(ctx, d.podClient, &depl)
		if err != nil {
			return nil, err
		}
		for _, p := range pods {
			node := driver.Node{
				// TODO this isn't ideal - Need a good way to translate between pod name and node hostname
				//Name:   p.Spec.NodeName,
				//Name:   p.Status.HostIP,
				Name:   p.Name,
				Status: p.Status.Message, // TODO - this seems to be blank, need to look at kubectl CLI magic...
				// Other fields are unset (TODO: detect real platforms)
			}
			builder.Nodes = append(builder.Nodes, node)
		}
		builders = append(builders, builder)
	}
	return builders, nil
}

func (d *Driver) Factory() driver.Factory {
	return d.factory
}

func isRootless(rootless string) bool {
	b, err := strconv.ParseBool(rootless)
	if err == nil {
		return b
	}
	// TODO consider logging failed parse error message
	return false
}

func (d *Driver) Features() map[driver.Feature]bool {
	res := map[driver.Feature]bool{
		driver.OCIExporter:        true,
		driver.DockerExporter:     false,
		driver.ContainerdExporter: false,
		driver.CacheExport:        true,
		driver.MultiPlatform:      true, // Untested (needs multiple Driver instances)
	}
	// Query the pod to figure out the runtime
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pod, _, err := d.podChooser.ChoosePod(ctx)
	if err == nil && len(pod.Spec.Containers) > 0 && !isRootless(pod.ObjectMeta.Labels["rootless"]) {
		switch pod.ObjectMeta.Labels["runtime"] {
		case "containerd":
			res[driver.ContainerdExporter] = true
		case "docker":
			res[driver.DockerExporter] = true
		default:
			// TODO consider logging warning for unrecognized runtime...
		}
	}
	return res
}

func (d *Driver) GetAuthWrapper(secretName string) imagetools.Auth {
	if secretName == "" {
		secretName = buildxNameToDeploymentName(d.InitConfig.Name)
	}

	return &authProvider{
		driver: d,
		name:   secretName,
	}
}
