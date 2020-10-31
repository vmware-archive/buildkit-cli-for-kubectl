// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"os"
	"testing"
)

// Skipper will skip this test if we're not in integration mode
func Skipper(t *testing.T) {
	if _, found := os.LookupEnv("TEST_KUBECONFIG"); !found {
		t.Skip("Skipping integration tests without TEST_KUBECONFIG set")
	}
}
