// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package version

import "fmt"

var (
	// Package is filled at linking time
	//Package = "TBD/buildkit-cli"

	// Version holds the complete version number. Filled in at linking time.
	Version = "0.0.0+unknown"

	// Revision is filled with the VCS (e.g. git) revision being used to build
	// the program at linking time.
	Revision = ""
)

func GetVersionString() string {
	if Revision != "" {
		return fmt.Sprintf("%s-%s", Version, Revision)
	}
	return Version
}
