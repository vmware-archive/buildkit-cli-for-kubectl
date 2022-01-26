# TODO - refactor this as a cross-compilation process

ARG BUILDER_BASE=golang:1.17-bullseye
ARG RUNTIME_BASE=debian:bullseye-slim

FROM ${BUILDER_BASE} as build
COPY . /go/src/github.com/vmware-tanzu/buildkit-cli-for-kubectl
WORKDIR /go/src/github.com/vmware-tanzu/buildkit-cli-for-kubectl
ARG BUILDKIT_PROXY_IMAGE
ARG VERSION
RUN go install \
    -ldflags "-extldflags -static -X github.com/vmware-tanzu/buildkit-cli-for-kubectl/version.Version=${VERSION} -X github.com/vmware-tanzu/buildkit-cli-for-kubectl/version.DefaultHelperImage=${BUILDKIT_PROXY_IMAGE}" \
    -tags "osusergo netgo static_build" ./cmd/buildkit-proxy

FROM ${RUNTIME_BASE} as final
COPY --from=build /go/bin/buildkit-proxy /bin/
ENTRYPOINT ["/bin/buildkit-proxy"]