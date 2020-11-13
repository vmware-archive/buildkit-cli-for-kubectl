

# Multi-architecture Images

## Why is multi-arch important?

Multi-architecture container images allow you to have images for other operating systems and
system architectures that are referenced _by the same tag_. An example would be if you want to
pull the `alpine:3.12` image and you have a 64 bit Intel linux server, and a 64 bit ARM EC2 instance.
Because the `alpine` image supports multiple architectures, you can pull it on both systems and get
the right image for your platform. There is no need to try to guess a unique namespace, repository
name, or tag for either image.

Building these multi-architecture images can be fairly daunting. Unless you are using BuildKit
you need to build each image separately, stitch each of them together, and then push all of
the pieces along with a _manifest list_ to a container registry. This requires a lot of highly
customized build infrastructure, and can be very complex to set up.

Architectures:

| architecture  | bits   | notes                   |
|:--------------|:-------|:------------------------|
| linux/amd64   | 64 bit | Intel/AMD 64 bit        |
| linux/386     | 32 bit | Intel 32 bit            |
| linux/arm/v6  | 32 bit | RPi                     |
| linux/arm/v7  | 32 bit | RPi 2/RPi 3             |
| linux/arm/v8  | 32 bit |                         |
| linux/arm64   | 64 bit | 64 bit ARM              |
| linux/ppc64le | 64 bit | IBM Power Architecture  |
| linux/s390x   | 64 bit | IBM z/Architecture      |
| windows/amd64 | 64 bit | Windows                 |


### BuildKit Multi-architecture operating modes

BuildKit supports several different ways of building container images of different architectures and platforms,
each with different strengths and weaknesses. At present we only support building in
_cross-compilation_ mode, however, the other modes will be supported in the future.


  1. Cross-compilation mode (supported)

Cross-compilation is the fastest way to build a multi-architecture image, however it can be difficult
to set up. It depends upon whether your compiler supports cross-compilation, as well as the complexity
of your project. For something such as a simple statically compiled Go build, this can work quite well,
but if you're building a C or C++ application, and you're using dynamically compiled binaries, it may
be quite a bit trickier.

Speed of the build really comes down to the number of nodes in the cluster, as well as the speed of those
nodes. It's also easier to set up the cluster, since all of your nodes can be of the same type.

  2. Mixed cluster mode (not yet supported)

Mixed cluster mode will pick the correct node for a given architecture and will natively compile images
for that architecture. You will need to have nodes of each architecture for this to work, which can be
difficult with more rare architectures.

Speed of the build is comparable to doing cross-compilation, and it's typically easier to get binaries
to compile correctly.

  3. QEMU "Emulation" mode (not yet supported)

QEMU emulates each of the architectures on a single architecture type (linux/amd64). Since this is full
emulation, it can be quite slow to build everything. This mode is useful if it's too difficult to
get your build to cross-compile and you don't have access to machines to build natively.


## Using a registry

The Docker runtime doesn't support multi-architecture images natively. It's only possible to pull
the part of a multi-architecture image which is _native to your platform_, and you cannot hold
images for _other_ platforms in your local image cache. To handle a multi-architecture image after you
have built it, you need to push it to a container registry.

To push to a registry, you're going to first need to create a _registry secret_. Do this with the command:

`kubectl create secret docker-registry reg-secret --docker-server=my_registry --docker-username=my_user --docker-password=$MY_PASSWORD`

You will need to set the `docker-server` to the hostname of the registry that you are using, and the
`docker-username` and `docker-password` settings to your login credentials for that registry. If possible,
try to use an API key for the registry so that you don't expose your plain text credentials. When you
are doing the build, you can pass the flag `--registry-secret=reg-secret` which will tell the builder
to use that secret to push the image.

As a convenience, if you are only ever pushing to a single registry, it's possible to skip specifying the
`--registry-secret` flag to `kubectl build` by naming the secret the same name as the builder. The default builder is
called `buildkit`, so if you create a secret named the same thing, it will save you a few extra
keystrokes when building your image.

## Configuring your Dockerfile

Configuring the Dockerfile can be somewhat difficult to set up, and will be different depending on how
you compile your application. BuildKit uses multi-stage Dockerfiles to create the build, and expects
certain stages to exist to create images for a specific platform. Here's an example `Dockerfile.cross`
from our [example repository](https://github.com/pdevine/pants).

```
FROM --platform=$BUILDPLATFORM golang:1.11-alpine AS builder
RUN apk add --no-cache git
RUN go get github.com/pdevine/go-asciisprite
WORKDIR /project
COPY *.go ./

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o pants pants.go

FROM scratch AS release-linux
COPY --from=builder /project/pants /pants
ENTRYPOINT ["/pants"]

FROM mcr.microsoft.com/windows/nanoserver:1809 AS release-windows
COPY --from=builder /project/pants /pants.exe
ENTRYPOINT ["\\pants.exe"]

FROM release-$TARGETOS
```

The initial _builder_ stage is used to cross-compile the `pants.go` binary for each
platform, and then the uses the `release-windows` and `release-linux` to create images for
every windows and linux platform, regardless of the architecture. To build the image, you
will use the command:

```
$ kubectl build ./ -t <server>/<namespace>/<repositor>:<tag> -f Dockerfile.cross --registry-secret my-secret --platform=linux/386,linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64,windows/amd64 --push
```

Once this has built all of the images, it will push everything to the container registry and image tag that you specified in
`-t <server>/<namespace>/<repositor>:<tag>`. To make this work with your own images, you will need
to adapt the Dockerfile to allow you to cross-compile correctly.

