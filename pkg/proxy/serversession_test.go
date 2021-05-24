package proxy

import (
	"context"
	"fmt"
	"testing"

	control "github.com/moby/buildkit/api/services/control"
	moby_buildkit_v1 "github.com/moby/buildkit/api/services/control"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type sessionServerMock struct {
	ctx context.Context
	grpc.ServerStream
}

func (ssm *sessionServerMock) Send(in *control.BytesMessage) error {
	return fmt.Errorf("sessionServerMock send error")
}
func (ssm *sessionServerMock) Recv() (*control.BytesMessage, error) {
	return nil, fmt.Errorf("sessionServerMock send error")
}
func (ssm *sessionServerMock) SendMsg(m interface{}) error {
	return fmt.Errorf("sessionServeMock SendMsg error")
}
func (ssm *sessionServerMock) RecvMsg(m interface{}) error {
	return fmt.Errorf("sessionServeMock RecvMsg error")
}

func (ssm *sessionServerMock) Context() context.Context {
	return ssm.ctx
}

type sessionClientMock struct {
	grpc.ClientStream
}

func (csm *sessionClientMock) Send(in *control.BytesMessage) error {
	return fmt.Errorf("sessionClientMock send error")
}
func (csm *sessionClientMock) Recv() (*control.BytesMessage, error) {
	return nil, fmt.Errorf("sessionClientMock send error")
}
func (csm *sessionClientMock) SendMsg(m interface{}) error {
	return fmt.Errorf("sessionClientMock SendMsg error")
}
func (csm *sessionClientMock) RecvMsg(m interface{}) error {
	return fmt.Errorf("sessionClientMock RecvMsg error")
}
func (csm *sessionClientMock) CloseSend() error {
	return fmt.Errorf("sessionClientMock CloseSend error")
}

func TestSessionSetup(t *testing.T) {
	ctx := context.Background()
	c := &buildkitControlClientMock{
		sessionFn: func(ctx context.Context, opts ...grpc.CallOption) (moby_buildkit_v1.Control_SessionClient, error) {
			return nil, fmt.Errorf("buildkit session setup error")
		},
	}
	s := server{
		buildkitd: c,
		sessions:  map[string]*sessionProxies{},
	}
	ssm := &sessionServerMock{
		ctx: ctx,
	}
	err := s.Session(ssm)
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed session name header")

	md := metadata.New(map[string]string{
		headerSessionName: "val1",
	})
	ssm.ctx = metadata.NewIncomingContext(context.Background(), md)
	err = s.Session(ssm)
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed session key header")

	md.Append(headerSessionSharedKey, "val2")
	ssm.ctx = metadata.NewIncomingContext(context.Background(), md)
	err = s.Session(ssm)
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed session ID header")

	md.Append(headerSessionID, "sessionid")
	ssm.ctx = metadata.NewIncomingContext(context.Background(), md)
	err = s.Session(ssm)
	require.Error(t, err)
	require.Contains(t, err.Error(), "buildkit session setup error")

	csm := &sessionClientMock{}
	c.sessionFn = func(ctx context.Context, opts ...grpc.CallOption) (moby_buildkit_v1.Control_SessionClient, error) {
		return csm, nil
	}
	err = s.Session(ssm)
	// Note: the session definitely isn't a viable session given the errors we return
	// in the mocks so this might turn out to be fragile
	require.NoError(t, err)
}
