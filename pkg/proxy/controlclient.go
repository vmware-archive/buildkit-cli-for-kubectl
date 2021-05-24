package proxy

import (
	"context"
	"fmt"

	control "github.com/moby/buildkit/api/services/control"
	"google.golang.org/grpc"
)

// Wrapper to expose the ControlClient interface
//
// This is needed only so we can leverage the grpchijack.Dial routine
// which takes a ControlClient as input, and only calls the Session API
// to get access to the stream.
type sessionWrapper struct {
	sc control.Control_SessionClient
}

func (sw *sessionWrapper) Session(ctx context.Context, opts ...grpc.CallOption) (control.Control_SessionClient, error) {
	return sw.sc, nil
}

// The rest of the ControlClient functions are unnecessary in this implementation

func (sw *sessionWrapper) DiskUsage(context.Context, *control.DiskUsageRequest, ...grpc.CallOption) (*control.DiskUsageResponse, error) {
	return nil, fmt.Errorf("unimplemented")
}
func (sw *sessionWrapper) Prune(context.Context, *control.PruneRequest, ...grpc.CallOption) (control.Control_PruneClient, error) {
	return nil, fmt.Errorf("unimplemented")
}
func (sw *sessionWrapper) Solve(context.Context, *control.SolveRequest, ...grpc.CallOption) (*control.SolveResponse, error) {
	return nil, fmt.Errorf("unimplemented")
}
func (sw *sessionWrapper) Status(context.Context, *control.StatusRequest, ...grpc.CallOption) (control.Control_StatusClient, error) {
	return nil, fmt.Errorf("unimplemented")
}
func (sw *sessionWrapper) ListWorkers(context.Context, *control.ListWorkersRequest, ...grpc.CallOption) (*control.ListWorkersResponse, error) {
	return nil, fmt.Errorf("unimplemented")
}
