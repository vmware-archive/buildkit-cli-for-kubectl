// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package imagetools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ParseRefs(t *testing.T) {
	t.Parallel()
	ref, err := parseRef("acme:latest")
	require.NoError(t, err)
	require.NotNil(t, ref)
}
