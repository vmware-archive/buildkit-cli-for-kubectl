# Copyright (C) 2020 VMware, Inc.
# SPDX-License-Identifier: Apache-2.0

VERSION?=dev
TEST_TIMEOUT=600s
TEST_PARALLELISM=3

# Set this to your favorite proxy cache if you experience rate limiting problems with Docker Hub.
# Remember to include the prefix for library images
DOCKER_HUB_LIBRARY_PROXY_CACHE?=
export DOCKER_HUB_LIBRARY_PROXY_CACHE
# TODO wire this up so it's passed into the build of the binaries instead of replicating the hardcoded string
BUILDKIT_PROXY_IMAGE=ghcr.io/vmware-tanzu/buildkit-proxy
TEST_IMAGE_BASE=$(DOCKER_HUB_LIBRARY_PROXY_CACHE)busybox
BUILDER_BASE?=$(DOCKER_HUB_LIBRARY_PROXY_CACHE)golang:1.17-bullseye
RUNTIME_BASE?=$(DOCKER_HUB_LIBRARY_PROXY_CACHE)debian:bullseye-slim


export TEST_IMAGE_BASE 
BUILD_CMD=kubectl buildkit build
PUSH_CMD=docker push

# Verify Go in PATH
ifeq (, $(shell which go))
$(error You must install Go to build - https://golang.org/dl/ )
endif

INSTALL_DIR=/usr/local/bin
BIN_DIR=./bin
NATIVE_ARCH=$(shell uname | tr A-Z a-z)

GOARCH=amd64
export GOARCH
CI_OSES=linux darwin windows
CI_BUILD_TARGETS=$(foreach os,$(CI_OSES),\
	$(if $(filter windows,$(os)),\
		$(BIN_DIR)/$(os)/kubectl-buildkit.exe $(BIN_DIR)/$(os)/kubectl-build.exe,\
		$(BIN_DIR)/$(os)/kubectl-buildkit $(BIN_DIR)/$(os)/kubectl-build) \
	)
CI_ARCHIVES=$(foreach os,$(CI_OSES),$(BIN_DIR)/$(os).tgz)

GO_MOD_NAME=github.com/vmware-tanzu/buildkit-cli-for-kubectl
GO_DEPS=$(foreach dir,$(shell go list -deps -f '{{.Dir}}' ./cmd/kubectl-buildkit ./cmd/kubectl-build ./cmd/buildkit-proxy),$(wildcard $(dir)/*.go)) Makefile
REVISION=$(shell git describe --match 'v[0-9]*' --always --dirty --tags)
GO_FLAGS=-ldflags "-X $(GO_MOD_NAME)/version.Version=${VERSION} -X $(GO_MOD_NAME)/version.DefaultHelperImage=$(BUILDKIT_PROXY_IMAGE)" -mod=vendor
GO_COVER_FLAGS=-cover -coverpkg=./... -covermode=count
UNIT_TEST_PACKAGES=$(shell go list ./... | grep -v "/integration/")
COVERAGE_FILTERS=grep -v "\.pb\.go" | grep -v "/integration/"

.PHONY: help
help:
	@echo "To install the CLI plugin locall run 'make build && sudo make install'"
	@echo "(Note: You must have Go installed on your host to compile)"

.PHONY: clean
clean:
	-rm -rf $(BIN_DIR) cover*.out cover*.html

.PHONY: build
build: $(BIN_DIR)/$(NATIVE_ARCH)/kubectl-buildkit $(BIN_DIR)/$(NATIVE_ARCH)/kubectl-build $(BIN_DIR)/$(NATIVE_ARCH)/buildkit-proxy

$(BIN_DIR)/%/kubectl-buildkit $(BIN_DIR)/%/kubectl-buildkit.exe: $(GO_DEPS)
	GOOS=$* go build $(GO_FLAGS) -o $@ ./cmd/kubectl-buildkit

$(BIN_DIR)/%/kubectl-build $(BIN_DIR)/%/kubectl-build.exe: $(GO_DEPS)
	GOOS=$* go build $(GO_FLAGS) -o $@  ./cmd/kubectl-build

$(BIN_DIR)/%/buildkit-proxy $(BIN_DIR)/%/buildkit-proxy.exe: $(GO_DEPS)
	GOOS=$* go build $(GO_FLAGS) -o $@  ./cmd/buildkit-proxy

.PHONY: image
image:
	$(BUILD_CMD) -t $(BUILDKIT_PROXY_IMAGE):$(VERSION) \
		--build-arg BUILDKIT_PROXY_IMAGE=$(BUILDKIT_PROXY_IMAGE) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILDER_BASE=$(BUILDER_BASE) \
		--build-arg RUNTIME_BASE=$(RUNTIME_BASE) \
		-f ./builder/Dockerfile .

# TODO refine so this can support native kubectl build/save
.PHONY: save-image
save-image:
	@mkdir -p $(BIN_DIR)
	docker save $(BUILDKIT_PROXY_IMAGE):$(VERSION) > $(BIN_DIR)/buildkit_proxy_image.tar

.PHONY: push
push:
	$(PUSH_CMD) $(BUILDKIT_PROXY_IMAGE):$(VERSION)

install: $(BIN_DIR)/$(NATIVE_ARCH)/kubectl-buildkit $(BIN_DIR)/$(NATIVE_ARCH)/kubectl-build
	cp $(BIN_DIR)/$(NATIVE_ARCH)/kubectl-buildkit $(BIN_DIR)/$(NATIVE_ARCH)/kubectl-build $(INSTALL_DIR)

print-%:
	@echo '$*=$($*)'

.PHONY: build-ci
build-ci: $(CI_BUILD_TARGETS)

.PHONY: dist
dist: $(CI_BUILD_TARGETS) $(CI_ARCHIVES) image save-image

$(BIN_DIR)/%.tgz: $(BIN_DIR)/%/*
	cd $(BIN_DIR)/$* && tar -czvf ../$*.tgz kubectl-*

.PHONY: generate
generate:
	go generate ./...


.PHONY: test
test:
	go test $(GO_FLAGS) \
		-parallel $(TEST_PARALLELISM) \
		$(TEST_FLAGS)  \
		$(GO_COVER_FLAGS) -coverprofile=./cover-unit-full.out \
		$(UNIT_TEST_PACKAGES)
	cat ./cover-unit-full.out | $(COVERAGE_FILTERS) > ./cover-unit.out
	rm -f ./cover-unit-full.out


.PHONY: integration
integration:
	@echo "Running integration tests"
	@kubectl config get-contexts
	go test -timeout $(TEST_TIMEOUT) $(GO_FLAGS) \
		-parallel $(TEST_PARALLELISM) \
		$(TEST_FLAGS)  \
		$(GO_COVER_FLAGS) -coverprofile=./cover-int-full.out \
		./integration/...
	cat ./cover-int-full.out | $(COVERAGE_FILTERS) > ./cover-int.out
	rm -f ./cover-int-full.out
	go tool cover -html=./cover-int.out -o ./cover-int.html

.PHONY: coverage
coverage: cover.html

cover.html: cover-int.out cover-unit.out
	cat cover-int.out > cover.out
	tail +2 cover-unit.out | $(COVERAGE_FILTERS) >> cover.out
	go tool cover -html=./cover.out -o ./cover.html
	go tool cover -func cover.out | grep total:
	open ./cover.html

.PHONY: lint
lint:
	golangci-lint run
