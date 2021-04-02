// Copyright (C) 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_decodeAuth(t *testing.T) {
	t.Parallel()
	username, password, err := decodeAuth("")
	assert.NoError(t, err)
	assert.Equal(t, username, "")
	assert.Equal(t, password, "")

	username, password, err = decodeAuth("badstring")
	assert.Error(t, err)
	assert.Equal(t, username, "")
	assert.Equal(t, password, "")

	// echo -n "jdoe:supersecret" | base64 -i -
	username, password, err = decodeAuth("amRvZTpzdXBlcnNlY3JldA==")
	assert.NoError(t, err)
	assert.Equal(t, username, "jdoe")
	assert.Equal(t, password, "supersecret")

	// echo -n "baddata" | base64 -i -
	username, password, err = decodeAuth("YmFkZGF0YQ==")
	assert.Error(t, err)
	assert.Equal(t, username, "")
	assert.Equal(t, password, "")
}
