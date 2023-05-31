module github.com/vmware-tanzu/buildkit-cli-for-kubectl

go 1.14

require (
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.6.19
	github.com/docker/distribution v2.8.2-beta.1+incompatible
	github.com/docker/docker v20.10.24+incompatible
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/moby/buildkit v0.9.3
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799
	github.com/pkg/errors v0.9.1
	github.com/serialx/hashring v0.0.0-20190422032157-8b2912629002
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/grpc v1.47.0
	k8s.io/api v0.23.5
	k8s.io/apimachinery v0.23.5
	k8s.io/cli-runtime v0.23.5
	k8s.io/client-go v0.23.5
)

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

replace github.com/opencontainers/runc => github.com/opencontainers/runc v1.1.2
