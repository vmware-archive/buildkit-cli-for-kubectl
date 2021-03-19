// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isRefOnlyFormat(t *testing.T) {
	t.Parallel()
	resp := isRefOnlyFormat([]string{})
	assert.True(t, resp)
	resp = isRefOnlyFormat([]string{"foo"})
	assert.True(t, resp)
	resp = isRefOnlyFormat([]string{"foo=bar"})
	assert.False(t, resp)
}

func Test_ParseCacheEntry(t *testing.T) {
	t.Parallel()
	resp, err := ParseCacheEntry([]string{"foo"})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)
}
