// Copyright (C) 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseSecretSpecs(t *testing.T) {
	t.Parallel()
	resp, err := ParseSecretSpecs([]string{})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	resp, err = ParseSecretSpecs([]string{"type=bogus"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported secret type")
	assert.Nil(t, resp)
	resp, err = ParseSecretSpecs([]string{"bogus=bogus"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected key")
	assert.Nil(t, resp)
	resp, err = ParseSecretSpecs([]string{"id=mysecret,src=/local/secret/path/doesnt/exist"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
	assert.Nil(t, resp)
}
