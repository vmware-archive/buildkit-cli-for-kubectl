// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package version

var (
	// Package is filled at linking time
	//Package = "TBD/buildkit-cli"

	// Version holds the complete version number. Filled in at linking time.
	Version = "v0.0.0+unknown"
)

func GetVersionString() string {
	return Version
}
