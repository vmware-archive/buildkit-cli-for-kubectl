// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package manifest

import (
	"fmt"

	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/version"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentOpt struct {
	Namespace              string
	Name                   string
	Image                  string
	Replicas               int
	BuildkitFlags          []string
	Rootless               bool
	Worker                 string
	ContainerdNamespace    string
	ContainerdSockHostPath string
	DockerSockHostPath     string
	ContainerRuntime       string
	CustomConfig           string
}

const (
	containerName = "buildkitd"
	AnnotationKey = "buildkit.mobyproject.org/builder"
)

func labels(opt *DeploymentOpt) map[string]string {
	return map[string]string{
		"app":      opt.Name,
		"runtime":  opt.ContainerRuntime,
		"worker":   opt.Worker,
		"rootless": fmt.Sprintf("%v", opt.Rootless),
	}
}

func annotations(opt *DeploymentOpt) map[string]string {
	return map[string]string{
		AnnotationKey: version.GetVersionString(),
	}
}

func NewDeployment(opt *DeploymentOpt) (*appsv1.Deployment, error) {
	labels := labels(opt)
	annotations := annotations(opt)
	replicas := int32(opt.Replicas)
	privileged := true
	args := opt.BuildkitFlags
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   opt.Namespace,
			Name:        opt.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  containerName,
							Image: opt.Image,
							Args:  args,
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"buildctl", "debug", "workers"},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "buildkitd-config",
									MountPath: "/etc/buildkit/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "buildkitd-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: opt.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if opt.Rootless {
		if err := toRootless(d); err != nil {
			return nil, err
		}
	}

	if opt.Worker == "containerd" {
		if err := toContainerdWorker(d, opt); err != nil {
			return nil, err
		}
	}
	if opt.ContainerRuntime == "docker" && !opt.Rootless {
		if err := addDockerSockMount(d, opt); err != nil {
			return nil, err
		}
	}
	if opt.CustomConfig != "" {
		if err := addCustomConfigMount(d, opt); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func toRootless(d *appsv1.Deployment) error {
	d.Spec.Template.Spec.Containers[0].Args = append(
		d.Spec.Template.Spec.Containers[0].Args,
		"--oci-worker-no-process-sandbox",
	)
	d.Spec.Template.Spec.Containers[0].SecurityContext = nil
	if d.Spec.Template.ObjectMeta.Annotations == nil {
		d.Spec.Template.ObjectMeta.Annotations = make(map[string]string, 2)
	}
	d.Spec.Template.ObjectMeta.Annotations["container.apparmor.security.beta.kubernetes.io/"+containerName] = "unconfined"
	d.Spec.Template.ObjectMeta.Annotations["container.seccomp.security.alpha.kubernetes.io/"+containerName] = "unconfined"
	return nil
}

func toContainerdWorker(d *appsv1.Deployment, opt *DeploymentOpt) error {
	labels := labels(opt)
	buildkitRoot := "/var/lib/buildkit/" + opt.Name
	d.Spec.Template.Spec.Containers[0].Args = append(
		d.Spec.Template.Spec.Containers[0].Args,
		"--oci-worker=false",
		"--containerd-worker=true",
		"--root", buildkitRoot,
	)
	mountPropagationBidirectional := corev1.MountPropagationBidirectional
	d.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		d.Spec.Template.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "containerd-sock",
			MountPath: "/run/containerd/containerd.sock",
		},
		corev1.VolumeMount{
			Name:             "var-lib-buildkit",
			MountPath:        buildkitRoot,
			MountPropagation: &mountPropagationBidirectional,
		},
		corev1.VolumeMount{
			Name:             "var-lib-containerd",
			MountPath:        "/var/lib/containerd",
			MountPropagation: &mountPropagationBidirectional,
		},
		corev1.VolumeMount{
			Name:             "run-containerd",
			MountPath:        "/run/containerd",
			MountPropagation: &mountPropagationBidirectional,
		},
		corev1.VolumeMount{
			Name:             "var-log", // TODO - try narrowing to /var/log/containers
			MountPath:        "/var/log",
			MountPropagation: &mountPropagationBidirectional,
		},
		corev1.VolumeMount{
			Name:             "tmp",
			MountPath:        "/tmp",
			MountPropagation: &mountPropagationBidirectional,
		},
	)
	hostPathSocket := corev1.HostPathSocket
	hostPathDirectory := corev1.HostPathDirectory
	hostPathDirectoryOrCreate := corev1.HostPathDirectoryOrCreate
	d.Spec.Template.Spec.Volumes = append(
		d.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "containerd-sock",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: opt.ContainerdSockHostPath,
					Type: &hostPathSocket,
				},
			},
		},
		// TODO - consider making this ~unique so multiple buildkit builders
		// can co-exist on a single node in a multi-user environment
		corev1.Volume{
			Name: "var-lib-buildkit",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: buildkitRoot,
					Type: &hostPathDirectoryOrCreate,
				}},
		},
		corev1.Volume{
			Name: "var-lib-containerd",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/containerd",
					Type: &hostPathDirectory,
				},
			},
		},
		corev1.Volume{
			Name: "run-containerd",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/containerd",
					Type: &hostPathDirectory,
				},
			},
		},
		corev1.Volume{
			Name: "var-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log",
					Type: &hostPathDirectory,
				},
			},
		},
		corev1.Volume{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/tmp",
					Type: &hostPathDirectory,
				},
			},
		},
	)

	// Spread our builders out on a multi-node cluster
	d.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}

	return nil
}

func addDockerSockMount(d *appsv1.Deployment, opt *DeploymentOpt) error {
	labels := labels(opt)
	d.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		d.Spec.Template.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "docker-sock",
			MountPath: "/run/docker.sock",
		},
	)
	hostPathSocket := corev1.HostPathSocket
	d.Spec.Template.Spec.Volumes = append(
		d.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "docker-sock",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: opt.DockerSockHostPath,
					Type: &hostPathSocket,
				},
			},
		},
	)

	// If we're using the dockerd socket to make images available
	// on the nodes, we want to distribute the workers across the cluster
	// and not let them clump together on a single node
	d.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}

	return nil
}

func addCustomConfigMount(d *appsv1.Deployment, opt *DeploymentOpt) error {
	d.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		d.Spec.Template.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "custom-config",
			MountPath: "/etc/config/",
		},
	)
	d.Spec.Template.Spec.Volumes = append(
		d.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "custom-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: opt.CustomConfig,
					},
				},
			},
		},
	)

	return nil
}

func NewConfigMap(opt *DeploymentOpt, contents []byte) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   opt.Namespace,
			Name:        opt.Name,
			Labels:      labels(opt),
			Annotations: annotations(opt),
		},
		BinaryData: map[string][]byte{
			"buildkitd.toml": contents,
		},
	}
}
