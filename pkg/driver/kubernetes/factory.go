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
	"text/template"

	"github.com/pkg/errors"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/manifest"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes/podchooser"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/version"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // register GCP auth provider
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
		Image:                  version.DefaultImage,
		ProxyImage:             version.GetProxyImage(),
		Replicas:               1,
		BuildkitFlags:          cfg.BuildkitFlags,
		Rootless:               false,
		ContainerRuntime:       DefaultContainerRuntime,
		ContainerdSockHostPath: DefaultContainerdSockPath,
		ContainerdNamespace:    DefaultContainerdNamespace,
		DockerSockHostPath:     DefaultDockerSockPath,
		Environments:           make(map[string]string),
	}

	imageOverride := ""
	var err error
	for k, v := range cfg.DriverOpts {
		switch k {
		case "image":
			imageOverride = v
		case "proxy-image":
			deploymentOpt.ProxyImage = v
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
			if deploymentOpt.Rootless {
				deploymentOpt.Image = version.DefaultRootlessImage
			}
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
			case "auto":
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
			case "auto":
				d.userSpecifiedRuntime = false
			case "docker":
				d.userSpecifiedRuntime = true
			case "containerd":
				d.userSpecifiedRuntime = true
			default:
				return errors.Errorf("invalid runtime %q", v)
			}
			deploymentOpt.ContainerRuntime = v
		case "custom-config":
			deploymentOpt.CustomConfig = v
		case "env":
			// Split over comma for multiple key/value
			for _, item := range strings.Split(v, ";") {
				m := strings.SplitN(item, "=", 2)
				if len(m) == 2 {
					deploymentOpt.Environments[m[0]] = m[1]
				}
			}
		default:
			return errors.Errorf("invalid driver option %s for driver %s", k, DriverName)
		}
	}

	if deploymentOpt.ContainerRuntime == "auto" {
		deploymentOpt.ContainerRuntime = DefaultContainerRuntime
	}

	// Wire up defaults based on the chosen runtime
	if deploymentOpt.ContainerRuntime == "containerd" && deploymentOpt.Worker == "auto" {
		deploymentOpt.Worker = WorkerContainerd
	}
	if deploymentOpt.ContainerRuntime == "docker" && deploymentOpt.Worker == "auto" {
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
		d.userSpecifiedConfig = true
	}
	return nil
}

func (f *factory) AllowsInstances() bool {
	return true
}

// buildxNameToDeploymentName converts buildx name to Kubernetes Deployment name.
func buildxNameToDeploymentName(bx string) string {
	if bx == "" {
		return "buildkit"
	}
	return bx
}
