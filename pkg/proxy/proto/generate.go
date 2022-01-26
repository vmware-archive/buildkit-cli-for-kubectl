package vmware_tanzu_buildkit_cli_for_kubectl_proxy_v1 //nolint:golint

//go:generate protoc -I=. -I=../../../vendor -I=../../../../../../ --gogofaster_out=plugins=grpc:. proxy.proto

// Deps:
// go get github.com/gogo/protobuf/proto
// go get github.com/gogo/protobuf/protoc-gen-gogofaster
// go get github.com/gogo/protobuf/gogoproto
