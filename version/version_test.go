// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GetVersionString(t *testing.T) {
	t.Parallel()
	ver := GetVersionString()
	require.Contains(t, ver, Version)
}
