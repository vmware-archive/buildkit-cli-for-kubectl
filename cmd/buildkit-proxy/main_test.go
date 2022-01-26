package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoMain(t *testing.T) {
	ctx := context.Background()
	err := doMain(ctx)
	require.NoError(t, err)
}
