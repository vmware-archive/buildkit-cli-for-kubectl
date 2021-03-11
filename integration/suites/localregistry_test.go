// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package suites

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/integration/common"

	"golang.org/x/crypto/bcrypt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/cert"
)

const (
	RegistryImageName = "docker.io/registry:2.7"
)

type localRegistrySuite struct {
	suite.Suite
	Name        string
	CreateFlags []string

	ClientSet *kubernetes.Clientset
	Namespace string

	configMapClient v1.ConfigMapInterface
	secretClient    v1.SecretInterface
	podClient       v1.PodInterface
	serviceClient   v1.ServiceInterface
	configMapName   string
	registryName    string
	registryFQDN    string

	skipTeardown bool
}

func (s *localRegistrySuite) SetupSuite() {
	// s.skipTeardown = true
	var err error
	ctx := context.Background()
	s.ClientSet, s.Namespace, err = common.GetKubeClientset()
	require.NoError(s.T(), err, "%s: kube client failed", s.Name)
	s.configMapClient = s.ClientSet.CoreV1().ConfigMaps(s.Namespace)
	s.secretClient = s.ClientSet.CoreV1().Secrets(s.Namespace)
	s.podClient = s.ClientSet.CoreV1().Pods(s.Namespace)
	s.serviceClient = s.ClientSet.CoreV1().Services(s.Namespace)

	s.configMapName = s.Name + "-certs"
	s.registryName = s.Name + "-registry"
	s.registryFQDN = s.registryName + "." + s.Namespace + ".svc.cluster.local"
	username := "jdoe"
	password := "supersecret"

	// Generate TLS certificates
	logrus.Infof("%s: Generating self-signed cert for the registry", s.Name)
	crt, key, err := cert.GenerateSelfSignedCertKey("registry", nil, []string{s.registryFQDN})
	require.NoError(s.T(), err, "%s: self signed cert gen failed", s.Name)

	// Create the htpassword compatible payload for the registry to use
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(s.T(), err, "%s: htpasswd generation failed", s.Name)

	htpass := fmt.Sprintf("%s:%s",
		username,
		hashedPass,
	)

	// Stuff the certs into a configmap (yes, this is technically wrong, a private key
	// should never go in a ConfigMap but rather a Secret)
	cfgMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      s.configMapName,
		},
		BinaryData: map[string][]byte{
			"cert.pem": crt,
			"key.pem":  key,
			"htpasswd": []byte(htpass),
		},
	}
	_, err = s.configMapClient.Create(context.Background(), cfgMap, metav1.CreateOptions{})
	require.NoError(s.T(), err, "%s: create configmap failed", s.Name)

	// Create some registry credentials
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      s.Name, // Use the builder name so that we auto-mount the secret
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(
				fmt.Sprintf(
					`{"auths":{"%s":{"username":"%s","password":"%s"}}}`,
					s.registryFQDN,
					username,
					password,
				),
			),
		},
	}
	_, err = s.secretClient.Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(s.T(), err, "%s: create registry secret failed", s.Name)

	// Start up the local registry
	logrus.Infof("%s: Running local registry pod %s with cert", s.Name, s.registryName)
	_, err = s.podClient.Create(ctx,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: s.registryName,
				Labels: map[string]string{
					"app": s.registryName,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "registry",
						Image:           RegistryImageName,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env: []corev1.EnvVar{
							{
								Name:  "REGISTRY_HTTP_TLS_CERTIFICATE",
								Value: "/certs/cert.pem",
							},
							{
								Name:  "REGISTRY_HTTP_TLS_KEY",
								Value: "/certs/key.pem",
							},
							{
								Name:  "REGISTRY_HTTP_ADDR",
								Value: "0.0.0.0:443",
							},
							{
								Name:  "REGISTRY_AUTH_HTPASSWD_PATH",
								Value: "/certs/htpasswd",
							},
							{
								Name:  "REGISTRY_AUTH_HTPASSWD_REALM",
								Value: "Registry Realm",
							},
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "registry-tls",
								HostPort:      443,
								ContainerPort: 443,
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "certs",
								MountPath: "/certs/",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "certs",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: s.configMapName,
								},
							},
						},
					},
				},
			},
		},
		metav1.CreateOptions{},
	)
	require.NoError(s.T(), err, "%s: create registry pod failed", s.Name)

	logrus.Infof("%s: Creating ClusterIP Service for registry %s", s.Name, s.registryName)
	_, err = s.serviceClient.Create(ctx,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: s.registryName,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "registry-tls",
						Port: 443,
					},
				},
				Selector: map[string]string{
					"app": s.registryName,
				},
				Type: corev1.ServiceTypeClusterIP,
			},
		},
		metav1.CreateOptions{},
	)
	require.NoError(s.T(), err, "%s: create registry service failed", s.Name)

	// Now create the builder with the certs mapped and a config that trusts this local registry
	logrus.Infof("%s: Creating custom builder with registry cert and config %s", s.Name, s.registryName)
	dir, cleanup, err := common.NewBuildContext(map[string]string{
		"buildkitd.toml": fmt.Sprintf(`debug = false
[worker.containerd]
	namespace = "k8s.io"
[registry."%s"]
	ca=["/etc/config/cert.pem"]
`, s.registryFQDN)})
	require.NoError(s.T(), err, "%s: config file create failed", s.Name)

	defer cleanup()

	args := append(
		[]string{
			"--config", filepath.Join(dir, "buildkitd.toml"),
			"--custom-config", s.configMapName,
			s.Name,
		},
		s.CreateFlags...,
	)
	err = common.RunBuildkit("create", args)
	require.NoError(s.T(), err, "%s: builder creation failed", s.Name)
}

func (s *localRegistrySuite) TearDownSuite() {
	if !s.skipTeardown {

		ctx := context.Background()

		// Clean everything up...
		err := s.podClient.Delete(ctx, s.registryName, metav1.DeleteOptions{})
		if err != nil {
			logrus.Warnf("failed to clean up pod %s: %s", s.registryName, err)
		}
		err = s.serviceClient.Delete(ctx, s.registryName, metav1.DeleteOptions{})
		if err != nil {
			logrus.Warnf("failed to clean up service %s: %s", s.registryName, err)
		}
		err = s.configMapClient.Delete(ctx, s.configMapName, metav1.DeleteOptions{})
		if err != nil {
			logrus.Warnf("failed to clean up configMap %s: %s", s.configMapName, err)
		}
		err = s.secretClient.Delete(ctx, s.Name, metav1.DeleteOptions{})
		if err != nil {
			logrus.Warnf("failed to clean up registry secret %s: %s", s.configMapName, err)
		}

		logrus.Infof("%s: Removing builder", s.Name)
		err = common.RunBuildkit("rm", []string{
			s.Name,
		})
		if err != nil {
			logrus.Warnf("failed to clean up builder %s", err)
		}

	}
}

func (s *localRegistrySuite) TestBuildWithPush() {
	logrus.Infof("%s: Registry Push Build", s.Name)

	dir, cleanup, err := common.NewSimpleBuildContext()
	defer cleanup()
	require.NoError(s.T(), err, "Failed to set up temporary build context")
	imageName := s.registryFQDN + "/" + s.Name + "replaceme:latest"
	args := []string{"--progress=plain",
		"--builder", s.Name,
		"--push",
		"--tag", imageName,
		dir,
	}
	err = common.RunBuild(args)
	require.NoError(s.T(), err, "build failed")
	// Note, we can't run the image we just built since it was pushed to the local registry, which isn't ~directly visible to the runtime
}

// TODO add testcase for caching scenarios here

func TestLocalRegistrySuite(t *testing.T) {
	common.Skipper(t)
	// TODO this testcase should be safe to run parallel, but I'm seeing failures in CI that look
	// like containerd runtime concurrency problems.  They don't seem related to this particular change though
	//t.Parallel()
	suite.Run(t, &localRegistrySuite{
		Name: "regtest",
	})
}
