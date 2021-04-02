// Copyright (C) 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseSSHSpecs(t *testing.T) {
	t.Parallel()
	resp, err := ParseSSHSpecs([]string{})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	resp, err = ParseSSHSpecs([]string{"someid=bogus-file-does-not-exist"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
	assert.Nil(t, resp)
}
