// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package progress

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewPrinter(t *testing.T) {
	t.Parallel()
	writer := NewPrinter(context.Background(), os.Stderr, "")
	require.NotNil(t, writer)
}
