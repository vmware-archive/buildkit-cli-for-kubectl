// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package kubernetes

import (
	"context"
	"testing"

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
