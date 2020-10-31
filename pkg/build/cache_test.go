// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_isRefOnlyFormat(t *testing.T) {
	t.Parallel()
	resp := isRefOnlyFormat([]string{})
	require.True(t, resp)
	resp = isRefOnlyFormat([]string{"foo"})
	require.True(t, resp)
	resp = isRefOnlyFormat([]string{"foo=bar"})
	require.False(t, resp)
}
