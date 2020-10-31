// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/bkimage"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/manifest"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/podchooser"

	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
)

const prioritySupported = 40

// const priorityUnsupported = 80

func init() {
	driver.Register(&factory{})
}

type factory struct {
}

func (*factory) Name() string {
	return DriverName
}

func (*factory) Usage() string {
	// Report supported options
	prefix := "  "
	var usage bytes.Buffer
	fmt.Fprintf(&usage, "%skubernetes driver usage:\n", prefix)
	fmt.Fprintf(&usage, "%s  Builtkitd configuration is stored in a ConfigMap by the same name as the builder\n", prefix)
	fmt.Fprintf(&usage, "%s  If no '--config' is specified, a default config will be generated\n", prefix)
	fmt.Fprintf(&usage, "%s  When containerd worker is used, host mounts to support containerd are added\n", prefix)

	fmt.Fprintf(&usage, "%skubernetes driver options:\n", prefix)
	w := tabwriter.NewWriter(&usage, 0, 0, 1, ' ', 0)
	fmt.Fprintf(w, "%s  image\tspecify an alternate buildkit image (default %s)\n", prefix, bkimage.DefaultImage)
	fmt.Fprintf(w, "%s  namespace\tkubernetes namespace to use (defaults to your kubeconfig default)\n", prefix)
	fmt.Fprintf(w, "%s  runtime\tcontainer runtime used by cluster (defaults to %s)\n", prefix, DefaultContainerRuntime)
	fmt.Fprintf(w, "%s  containerd-sock\tpath to the containerd.sock on the host\n", prefix)
	fmt.Fprintf(w, "%s  containerd-namespace\tcontainerd namespace to build images in (defaults to %s)\n", prefix, DefaultContainerdNamespace)
	fmt.Fprintf(w, "%s  replicas\tbuildkit deployment replica count (default 1)\n", prefix)
	fmt.Fprintf(w, "%s  rootless\trun in rootless mode (default is root mode)\n", prefix)
	fmt.Fprintf(w, "%s  loadbalance\tspecify strategy (%s or %s - default %s)\n", prefix, LoadbalanceRandom, LoadbalanceSticky, LoadbalanceSticky)
	fmt.Fprintf(w, "%s  worker\tspecify worker back-end (%s or %s - default %s)\n", prefix, WorkerRunc, WorkerContainerd, WorkerContainerd)
	w.Flush()

	// TODO add more options to fine tune things...

	return usage.String()
}

func (*factory) Priority(ctx context.Context) int {

	return prioritySupported
}

func (f *factory) New(ctx context.Context, cfg driver.InitConfig) (driver.Driver, error) {
	if cfg.KubeClientConfig == nil {
		return nil, errors.Errorf("%s driver requires kubernetes API access", DriverName)
	}
	namespace, _, err := cfg.KubeClientConfig.Namespace()
	if err != nil {
		return nil, errors.Wrap(err, "cannot determine Kubernetes namespace, specify manually")
	}
	restClientConfig, err := cfg.KubeClientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(restClientConfig)
	if err != nil {
		return nil, err
	}
	d := &Driver{
		factory:     f,
		InitConfig:  cfg,
		clientset:   clientset,
		namespace:   namespace,
		loadbalance: LoadbalanceSticky,
	}

	err = d.initDriverFromConfig()
	if err != nil {
		return nil, err
	}

	d.deploymentClient = clientset.AppsV1().Deployments(d.namespace)
	d.replicaSetClient = clientset.AppsV1().ReplicaSets(d.namespace)
	d.podClient = clientset.CoreV1().Pods(d.namespace)
	d.eventClient = clientset.CoreV1().Events(d.namespace)
	d.configMapClient = clientset.CoreV1().ConfigMaps(d.namespace)
	d.secretClient = clientset.CoreV1().Secrets(d.namespace)

	switch d.loadbalance {
	case LoadbalanceSticky:
		d.podChooser = &podchooser.StickyPodChooser{
			Key:        cfg.ContextPathHash,
			PodClient:  d.podClient,
			Deployment: d.deployment,
		}
	case LoadbalanceRandom:
		d.podChooser = &podchooser.RandomPodChooser{
			PodClient:  d.podClient,
			Deployment: d.deployment,
		}
	}

	return d, nil
}

func (d *Driver) initDriverFromConfig() error {
	cfg := d.InitConfig
	deploymentName := buildxNameToDeploymentName(cfg.Name)

	deploymentOpt := &manifest.DeploymentOpt{
		Name:                   deploymentName,
		Image:                  bkimage.DefaultImage,
		Replicas:               1,
		BuildkitFlags:          cfg.BuildkitFlags,
		Rootless:               false,
		ContainerRuntime:       DefaultContainerRuntime,
		ContainerdSockHostPath: DefaultContainerdSockPath,
		ContainerdNamespace:    DefaultContainerdNamespace,
		DockerSockHostPath:     DefaultDockerSockPath,
	}

	imageOverride := ""
	var err error
	for k, v := range cfg.DriverOpts {
		switch k {
		case "image":
			imageOverride = v
		case "namespace":
			d.namespace = v
		case "replicas":
			deploymentOpt.Replicas, err = strconv.Atoi(v)
			if err != nil {
				return err
			}
		case "rootless":
			deploymentOpt.Rootless, err = strconv.ParseBool(v)
			if err != nil {
				return err
			}
			deploymentOpt.Image = bkimage.DefaultRootlessImage
		case "loadbalance":
			switch v {
			case LoadbalanceSticky:
			case LoadbalanceRandom:
			default:
				return errors.Errorf("invalid loadbalance %q", v)
			}
			d.loadbalance = v
		case "worker":
			switch v {
			case WorkerContainerd:
			case WorkerRunc:
			default:
				return errors.Errorf("invalid worker %q", v)
			}
			deploymentOpt.Worker = v
		case "containerd-namespace":
			deploymentOpt.ContainerdNamespace = v
		case "containerd-sock":
			deploymentOpt.ContainerdSockHostPath = v
		case "docker-sock":
			deploymentOpt.DockerSockHostPath = v
		case "runtime":
			switch v {
			case "docker":
			case "containerd":
			default:
				return errors.Errorf("invalid runtime %q", v)
			}
			deploymentOpt.ContainerRuntime = v
			d.userSpecifiedRuntime = true
		default:
			return errors.Errorf("invalid driver option %s for driver %s", k, DriverName)
		}
	}

	// Wire up defaults based on the chosen runtime
	if deploymentOpt.ContainerRuntime == "containerd" && deploymentOpt.Worker == "" {
		deploymentOpt.Worker = WorkerContainerd
	}
	if deploymentOpt.ContainerRuntime == "docker" && deploymentOpt.Worker == "" {
		// This makes sense as long as Docker hasn't shipped a newer version of containerd bundled
		// into the stable engine packaging.  In the future, we may want to default to containerd
		deploymentOpt.Worker = WorkerRunc
	}

	// Sanity check settings for incompatibilities
	if deploymentOpt.Rootless && deploymentOpt.Worker == WorkerContainerd {
		return fmt.Errorf("containerd worker does not support rootless mode - use 'runc' worker")
	}

	// TODO consider warning that in rootless mode you can't auto-load the images into the runtime (push or local only)

	if imageOverride != "" {
		deploymentOpt.Image = imageOverride
	}
	d.deployment, err = manifest.NewDeployment(deploymentOpt)
	if err != nil {
		return err
	}
	d.minReplicas = deploymentOpt.Replicas

	if cfg.ConfigFile == "" {
		// TODO might want to do substitution after parsing with the buildkitd.LoadFile instead of template...
		tmpl, err := template.New("config").Parse(DefaultConfigFileTemplate)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, deploymentOpt)
		if err != nil {
			return err
		}
		d.configMap = manifest.NewConfigMap(deploymentOpt, buf.Bytes())
	} else {
		data, err := ioutil.ReadFile(cfg.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}
		// TODO - parse the config file with buildkit/cmd/buildkitd.LoadFile(path)
		//        and make sure things get wired up properly, and/or error out if the
		//        user tries to set properties that should be in the config file
		d.configMap = manifest.NewConfigMap(deploymentOpt, data)
	}

	return nil
}

func (f *factory) AllowsInstances() bool {
	return true
}

// buildxNameToDeploymentName converts buildx name to Kubernetes Deployment name.
//
// eg. "buildx_buildkit_loving_mendeleev0" -> "loving-mendeleev0"
func buildxNameToDeploymentName(bx string) string {
	// TODO: commands.util.go should not pass "buildx_buildkit_" prefix to drivers
	s := bx
	if strings.HasPrefix(s, "buildx_buildkit_") {
		s = strings.TrimPrefix(s, "buildx_buildkit_")
	}
	s = strings.ReplaceAll(s, "_", "-")
	if s == "kubernetes" {
		// Having the default name of the deployment for buildkit called "kubernetes" is confusing, use something better
		return "buildkit"
	}
	if s == "" {
		return "buildkit"
	}
	return s
}
