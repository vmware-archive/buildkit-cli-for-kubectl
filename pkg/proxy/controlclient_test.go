package proxy

import (
	"context"
	"testing"

	control "github.com/moby/buildkit/api/services/control"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type controlSessionMock struct {
	grpc.ClientStream
}

func (*controlSessionMock) Send(*control.BytesMessage) error {
	return nil
}
func (*controlSessionMock) Recv() (*control.BytesMessage, error) {
	return nil, nil
}

func TestSessionWrapper(t *testing.T) {
	ctx := context.Background()
	csm := &controlSessionMock{}
	sw := &sessionWrapper{
		sc: csm,
	}
	res, err := sw.Session(ctx)
	require.NoError(t, err)
	require.Equal(t, csm, res)
	_, err = sw.DiskUsage(ctx, &control.DiskUsageRequest{})
	require.Error(t, err)
	_, err = sw.Prune(ctx, &control.PruneRequest{})
	require.Error(t, err)
	_, err = sw.Solve(ctx, &control.SolveRequest{})
	require.Error(t, err)
	_, err = sw.Status(ctx, &control.StatusRequest{})
	require.Error(t, err)
	_, err = sw.ListWorkers(ctx, &control.ListWorkersRequest{})
	require.Error(t, err)
}
