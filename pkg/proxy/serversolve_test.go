package proxy

import (
	"context"
	"fmt"
	"testing"

	control "github.com/moby/buildkit/api/services/control"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestSolve(t *testing.T) {
	solveCalled := false
	c := &buildkitControlClientMock{
		solveFn: func(ctx context.Context, in *control.SolveRequest, opts ...grpc.CallOption) (*control.SolveResponse, error) {
			solveCalled = true
			return nil, fmt.Errorf("expected error")
		},
	}
	s := server{
		buildkitd: c,
		sessions:  map[string]*sessionProxies{},
	}
	req := &control.SolveRequest{
		Exporter: "oci",
		ExporterAttrs: map[string]string{
			"foo": "bar",
		},
		Session: "sessionid",
	}
	_, err := s.Solve(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected error")
	require.True(t, solveCalled)

}
