package proxy

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	control "github.com/moby/buildkit/api/services/control"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type buildkitControlClientMock struct {
	diskUsageFn   func(ctx context.Context, in *control.DiskUsageRequest, opts ...grpc.CallOption) (*control.DiskUsageResponse, error)
	pruneFn       func(ctx context.Context, in *control.PruneRequest, opts ...grpc.CallOption) (control.Control_PruneClient, error)
	solveFn       func(ctx context.Context, in *control.SolveRequest, opts ...grpc.CallOption) (*control.SolveResponse, error)
	statusFn      func(ctx context.Context, in *control.StatusRequest, opts ...grpc.CallOption) (control.Control_StatusClient, error)
	sessionFn     func(ctx context.Context, opts ...grpc.CallOption) (control.Control_SessionClient, error)
	listWorkersFn func(ctx context.Context, in *control.ListWorkersRequest, opts ...grpc.CallOption) (*control.ListWorkersResponse, error)
}

func (c *buildkitControlClientMock) DiskUsage(ctx context.Context, in *control.DiskUsageRequest, opts ...grpc.CallOption) (*control.DiskUsageResponse, error) {
	return c.diskUsageFn(ctx, in, opts...)
}
func (c *buildkitControlClientMock) Prune(ctx context.Context, in *control.PruneRequest, opts ...grpc.CallOption) (control.Control_PruneClient, error) {
	return c.pruneFn(ctx, in, opts...)
}
func (c *buildkitControlClientMock) Solve(ctx context.Context, in *control.SolveRequest, opts ...grpc.CallOption) (*control.SolveResponse, error) {
	return c.solveFn(ctx, in, opts...)
}
func (c *buildkitControlClientMock) Status(ctx context.Context, in *control.StatusRequest, opts ...grpc.CallOption) (control.Control_StatusClient, error) {
	return c.statusFn(ctx, in, opts...)
}
func (c *buildkitControlClientMock) Session(ctx context.Context, opts ...grpc.CallOption) (control.Control_SessionClient, error) {
	return c.sessionFn(ctx, opts...)
}
func (c *buildkitControlClientMock) ListWorkers(ctx context.Context, in *control.ListWorkersRequest, opts ...grpc.CallOption) (*control.ListWorkersResponse, error) {
	return c.listWorkersFn(ctx, in, opts...)
}

type pruneServerMock struct {
	ctx context.Context
	grpc.ServerStream
}

func (psm *pruneServerMock) Send(in *control.UsageRecord) error {
	return fmt.Errorf("prunServerMock send error")
}

func (psm *pruneServerMock) Context() context.Context {
	return psm.ctx
}

type pruneClientMock struct {
	grpc.ClientStream
}

func (pcm *pruneClientMock) Recv() (*control.UsageRecord, error) {
	return nil, io.EOF
}

func (pcm *pruneClientMock) RecvMsg(m interface{}) error {
	return io.EOF
}
func (pcm *pruneClientMock) CloseSend() error {
	return nil
}

type statusServerMock struct {
	ctx context.Context
	grpc.ServerStream
}

func (ssm *statusServerMock) Send(in *control.StatusResponse) error {
	return fmt.Errorf("prunServerMock send error")
}

func (ssm *statusServerMock) Context() context.Context {
	return ssm.ctx
}

type statusClientMock struct {
	grpc.ClientStream
}

func (scm *statusClientMock) Recv() (*control.StatusResponse, error) {
	return nil, io.EOF
}

func (scm *statusClientMock) RecvMsg(m interface{}) error {
	return io.EOF
}
func (scm *statusClientMock) CloseSend() error {
	return nil
}

func TestSimpleProxies(t *testing.T) {
	ctx := context.Background()
	diskUsageCalled := false
	listWorkersCalled := false
	pruneCalled := false
	statusCalled := false
	pcm := &pruneClientMock{}
	scm := &statusClientMock{}
	c := &buildkitControlClientMock{
		diskUsageFn: func(ctx context.Context, in *control.DiskUsageRequest, opts ...grpc.CallOption) (*control.DiskUsageResponse, error) {
			diskUsageCalled = true
			return nil, fmt.Errorf("expected du error")
		},
		listWorkersFn: func(ctx context.Context, in *control.ListWorkersRequest, opts ...grpc.CallOption) (*control.ListWorkersResponse, error) {
			listWorkersCalled = true
			return nil, fmt.Errorf("expected lw error")
		},
		pruneFn: func(ctx context.Context, in *control.PruneRequest, opts ...grpc.CallOption) (control.Control_PruneClient, error) {
			pruneCalled = true
			return pcm, nil
		},
		statusFn: func(ctx context.Context, in *control.StatusRequest, opts ...grpc.CallOption) (control.Control_StatusClient, error) {
			statusCalled = true
			return scm, nil
		},
	}
	s := server{
		buildkitd: c,
		sessions:  map[string]*sessionProxies{},
	}
	_, err := s.DiskUsage(ctx, &control.DiskUsageRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected du error")
	require.True(t, diskUsageCalled)
	_, err = s.ListWorkers(ctx, &control.ListWorkersRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected lw error")
	require.True(t, listWorkersCalled)

	psm := &pruneServerMock{
		ctx: context.Background(),
	}
	err = s.Prune(&control.PruneRequest{}, psm)
	require.NoError(t, err)
	require.True(t, pruneCalled)

	ssm := &statusServerMock{
		ctx: context.Background(),
	}
	err = s.Status(&control.StatusRequest{}, ssm)
	require.NoError(t, err)
	require.True(t, statusCalled)
}

func TestServingFuncs(t *testing.T) {
	cfg := ServerConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "buildkitd sock path must")
	cfg.BuildkitdSocketPath = "dummy-sock"
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxy sock path must")
	cfg.HelperSocketPath = "listen-dummy-sock"
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "you must specify")
	cfg.ContainerdSocketPath = "ctrd-dummy-sock-path"
	err = cfg.Validate()
	require.NoError(t, err)

	_, err = getBuildkitClient(cfg)
	require.NoError(t, err)

	s := &server{
		cfg: ServerConfig{
			DockerdSocketPath: "dummy-dockerd-sock-path",
		},
	}
	err = s.connectToRuntime()
	require.NoError(t, err)
	require.NotNil(t, s.dckrdClient)

	c := &buildkitControlClientMock{
		listWorkersFn: func(ctx context.Context, in *control.ListWorkersRequest, opts ...grpc.CallOption) (*control.ListWorkersResponse, error) {
			return &control.ListWorkersResponse{}, nil
		},
	}
	s.buildkitd = c
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	s.waitForBuildkitHealth(ctx)
}
