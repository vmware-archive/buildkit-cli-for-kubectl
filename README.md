# BuildKit CLI for kubectl

BuildKit CLI for kubectl is a tool for building OCI and Docker images with your kubernetes cluster.
It replaces the `docker build` command to let you quickly and easily build your single and
multi-architecture container images.

![Pants Cast](./docs/pants-cast.svg)

## Features

### Drop in replacement for `docker build`

The BuildKit CLI for kubectl replaces the `docker build` command with `kubectl build` to build
images in your kubernetes cluster, instead of on a single node. Your Dockerfile will be parsed
the same way as with the existing `docker build` command, and build flags should feel almost
the same.

### Uses containerd or docker runtime environments

Regardless of whether your Kubernetes cluster is using pure [containerd](https://containerd.io) or
[docker](https://docker.com), the builder will be able to build OCI compatible images. These
images can be used inside of your cluster, or pushed to an image registry for distribution.

### Works in numerous kubernetes environments

The BuildKit builder should work in most Kubernetes environments. We tested it with:

  * [vSphere Tanzu](./docs/installing.md#vmware-vsphere-tanzu)
  * Amazon EKS
  * Minikube
  * [VMware Fusion](./docs/installing.md#vmware-fusion)
  * Docker Desktop

You should use Kubernetes version 1.14 or greater.


### Supports building multi-architecture images

If you need to support building multi-architecture images, the builder can create those by
cross-compiling your binaries. Multi-architecture images allow you to create images _with the same tag_
so that when they are pulled, the correct image for the architecture will be used. This includes
architectures such as:

 * linux/amd64 (Intel based 64 bit images)
 * linux/386 (Intel based 32 bit images)
 * linux/arm/v7 (ARM based 32 bit images - Raspberry Pi)
 * linux/arm64 (ARM based 64 bit images)
 * windows/amd64 (Intel based 64 bit Windows images)

Future versions will include native builds in mixed clusters, as well as being able to
emulate architectures when it's difficult to cross-compile.

## Getting started

### Installing the tarball

Head over to https://github.com/vmware-tanzu/buildkit-cli-for-kubectl/releases and download the `tgz` asset for your platform.

Once downloaded, extract it into your path.  For example, on MacOS
```sh
cat darwin-*.tgz | tar -C /usr/local/bin -xvf -
```

Test your install with
```sh
kubectl build --help
```

### Building from source

Check out our [contributing](./CONTRIBUTING.md) guide for instructions on setting up your environment and build instructions

### Changing contexts

If you're using more than one kubernetes environment, switch to the context you wish to use with
the buildkit builder.

```
kubectl config get-contexts
kubectl config use-context <context name>
```

### Creating a Kubernetes Registry Secret and Pushing

If you're going to push a newly created image to a container registry, you will need to store your
container registry credentials. This can be done either by passing a secret through a multi-stage
Dockerfile, or with a *registry secret* inside of Kubernetes.  Although either method can store your
plaintext password, you should consider creating an API key in your registry to be more secure.

To create a registry secret inside of Kubernetes, use the command:

```
read -s REG_SECRET
kubectl create secret docker-registry mysecret --docker-server='<registry hostname>' --docker-username=<user name> --docker-password=$REG_SECRET
kubectl build --push --registry-secret mysecret -t <registry hostname>/<namespace>/<repo>:<tag> -f Dockerfile ./
```


## Contributing

Check out our [contributing](./CONTRIBUTING.md) for guidance on how to help contribute to the project.
