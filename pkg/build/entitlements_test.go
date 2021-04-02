// Copyright (C) 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseEntitlements(t *testing.T) {
	t.Parallel()
	resp, err := ParseEntitlements([]string{})
	assert.NoError(t, err)
	assert.Len(t, resp, 0)

	resp, err = ParseEntitlements([]string{"garbage"})
	assert.Error(t, err)
	assert.Nil(t, resp)

	resp, err = ParseEntitlements([]string{"security.insecure", "network.host"})
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
}
