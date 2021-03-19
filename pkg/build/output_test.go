// Copyright (C) 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseOutputs(t *testing.T) {
	t.Parallel()
	resp, err := ParseOutputs([]string{})
	assert.NoError(t, err)
	assert.Len(t, resp, 0)

	resp, err = ParseOutputs([]string{"-"})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)

	resp, err = ParseOutputs([]string{"somepath"})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)

	resp, err = ParseOutputs([]string{"type=local,dest=out"})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)

	filename := "/tmp/bktestout.tar"
	resp, err = ParseOutputs([]string{"type=tar,dest=" + filename})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)
	defer func() {
		os.Remove(filename)
	}()

	resp, err = ParseOutputs([]string{"type=local"})
	assert.Error(t, err)
	assert.Len(t, resp, 0)

	resp, err = ParseOutputs([]string{"type=oci,dest=."})
	assert.Error(t, err)
	assert.Len(t, resp, 0)

	resp, err = ParseOutputs([]string{"type=registry"})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)

	resp, err = ParseOutputs([]string{"type=docker"})
	assert.NoError(t, err)
	assert.Len(t, resp, 1)

	resp, err = ParseOutputs([]string{"bad=string,extra=foo"})
	assert.Error(t, err)
	assert.Len(t, resp, 0)
}
