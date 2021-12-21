// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package platformutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Parse(t *testing.T) {
	t.Parallel()
	platforms, err := Parse([]string{
		"linux/armv7",
		"linux/amd64,linux/arm64",
	})
	assert.NoError(t, err)
	assert.NotNil(t, platforms)
	assert.Len(t, platforms, 3)

	res := FormatInGroups(platforms)
	assert.Len(t, res, 3)
	res = Format(platforms)
	assert.Len(t, res, 3)
	res = Format(nil)
	assert.Len(t, res, 0)

	platforms, err = Parse([]string{
		"local",
		"local,local",
	})
	assert.NoError(t, err)
	assert.Len(t, platforms, 3)
	deduped := Dedupe(platforms)
	assert.Len(t, deduped, 1)

	_, err = Parse([]string{
		"linux/amd64,acme",
	})
	assert.Error(t, err)
}
