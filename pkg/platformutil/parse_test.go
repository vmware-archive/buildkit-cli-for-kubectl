// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package platformutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Parse(t *testing.T) {
	t.Parallel()
	platforms, err := Parse([]string{
		"linux/armv7",
		"linux/amd64,linux/arm64",
	})
	require.NoError(t, err)
	require.NotNil(t, platforms)
	require.Len(t, platforms, 3)
}
