// Copyright (C) 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_toBuildkitExtraHosts(t *testing.T) {
	t.Parallel()
	resp, err := toBuildkitExtraHosts([]string{"foo:127.0.0.1", "bar:8.8.8.8"})
	assert.NoError(t, err)
	assert.Equal(t, resp, "foo=127.0.0.1,bar=8.8.8.8")

	resp, err = toBuildkitExtraHosts([]string{"foo:1234"})
	assert.Error(t, err)
	assert.Equal(t, resp, "")
}
