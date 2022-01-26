package proxy

import (
	"context"
	"fmt"
	"testing"

	"github.com/moby/buildkit/session/auth"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type authClientTest struct {
	credentials func(ctx context.Context, in *auth.CredentialsRequest, opts ...grpc.CallOption) (*auth.CredentialsResponse, error)
}

func (act *authClientTest) Credentials(ctx context.Context, in *auth.CredentialsRequest, opts ...grpc.CallOption) (*auth.CredentialsResponse, error) {
	return act.credentials(ctx, in, opts...)
}
func (act *authClientTest) FetchToken(ctx context.Context, req *auth.FetchTokenRequest, opts ...grpc.CallOption) (*auth.FetchTokenResponse, error) {
	return nil, fmt.Errorf("not mocked")
}
func (act *authClientTest) GetTokenAuthority(ctx context.Context, req *auth.GetTokenAuthorityRequest, opts ...grpc.CallOption) (*auth.GetTokenAuthorityResponse, error) {
	return nil, fmt.Errorf("not mocked")
}
func (act *authClientTest) VerifyTokenAuthority(ctx context.Context, req *auth.VerifyTokenAuthorityRequest, opts ...grpc.CallOption) (*auth.VerifyTokenAuthorityResponse, error) {
	return nil, fmt.Errorf("not mocked")
}

func TestAuthProxy(t *testing.T) {
	called := false
	act := &authClientTest{
		credentials: func(ctx context.Context, in *auth.CredentialsRequest, opts ...grpc.CallOption) (*auth.CredentialsResponse, error) {
			called = true
			return nil, nil
		},
	}
	ap := &authProxy{
		c: act,
	}
	server := grpc.NewServer()
	ap.Register(server)
	_, err := ap.Credentials(context.Background(), &auth.CredentialsRequest{})
	require.NoError(t, err)
	require.True(t, called)
}
