// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
)

func Test_GetDefaultFactory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	factory, err := driver.GetDefaultFactory(ctx, false)
	require.NoError(t, err)
	require.NotNil(t, factory)
}

func Test_initDriverFromConfig(t *testing.T) {
	d := &Driver{
		InitConfig: driver.InitConfig{
			DriverOpts: map[string]string{
				"image":                "image",
				"proxy-image":          "proxy-image",
				"namespace":            "namespace",
				"replicas":             "2",
				"rootless":             "false",
				"loadbalance":          "random",
				"worker":               "containerd",
				"containerd-namespace": "containerd-namespace",
				"containerd-sock":      "containerd-sock",
				"docker-sock":          "docker-sock",
				"runtime":              "containerd",
				"custom-config":        "custom-config",
				"env":                  "a=1;b=2",
			},
		},
	}

	err := d.initDriverFromConfig()
	assert.NoError(t, err)

	// A few error cases
	d.InitConfig.DriverOpts["rootless"] = "maybe"
	err = d.initDriverFromConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ParseBool")
	d.InitConfig.DriverOpts["rootless"] = "false"
	d.InitConfig.DriverOpts["replicas"] = "nope"
	err = d.initDriverFromConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Atoi")
	d.InitConfig.DriverOpts["replicas"] = "2"
	d.InitConfig.DriverOpts["loadbalance"] = "never"
	err = d.initDriverFromConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid loadbalance")
	d.InitConfig.DriverOpts["loadbalance"] = "random"
	d.InitConfig.DriverOpts["worker"] = "never"
	err = d.initDriverFromConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid worker")
	d.InitConfig.DriverOpts["worker"] = "containerd"
	d.InitConfig.DriverOpts["runtime"] = "never"
	err = d.initDriverFromConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid runtime")
	d.InitConfig.DriverOpts["runtime"] = "docker"
	d.InitConfig.DriverOpts["wrong"] = "input"
	err = d.initDriverFromConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid driver option")

}
