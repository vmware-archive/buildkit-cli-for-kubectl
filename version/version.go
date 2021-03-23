// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package version

var (
	// Package is filled at linking time
	//Package = "TBD/buildkit-cli"

	// Version holds the complete version number. Filled in at linking time.
	Version = "v0.0.0+unknown"

	// DefaultImage hols the primary build image we use
	DefaultImage = "docker.io/moby/buildkit:buildx-stable-1"

	// DefaultRootlessImage holds the rootless default image
	DefaultRootlessImage = "docker.io/moby/buildkit:buildx-stable-1-rootless"
)

func GetVersionString() string {
	return Version
}
