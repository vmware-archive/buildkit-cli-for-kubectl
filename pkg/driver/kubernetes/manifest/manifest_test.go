// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewDeployment(t *testing.T) {
	t.Parallel()
	opt := &DeploymentOpt{ContainerRuntime: "docker"}
	deployment, err := NewDeployment(opt)
	require.NoError(t, err)
	require.NotNil(t, deployment)

	opt.ContainerRuntime = "containerd"
	deployment, err = NewDeployment(opt)
	require.NoError(t, err)
	require.NotNil(t, deployment)
}
