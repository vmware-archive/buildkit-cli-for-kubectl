// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package store

import (
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

type Node struct {
	Name       string
	Endpoint   string
	Platforms  []specs.Platform
	Flags      []string
	ConfigFile string
	DriverOpts map[string]string
}
