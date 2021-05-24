package proxy

import (
	"context"
	"testing"

	"github.com/moby/buildkit/session/secrets"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type secretsProxyTest struct {
	getSecret func(ctx context.Context, in *secrets.GetSecretRequest, opts ...grpc.CallOption) (*secrets.GetSecretResponse, error)
}

func (spt *secretsProxyTest) GetSecret(ctx context.Context, in *secrets.GetSecretRequest, opts ...grpc.CallOption) (*secrets.GetSecretResponse, error) {
	return spt.getSecret(ctx, in, opts...)
}
func TestSecretsProxy(t *testing.T) {
	called := false
	spt := &secretsProxyTest{
		getSecret: func(ctx context.Context, in *secrets.GetSecretRequest, opts ...grpc.CallOption) (*secrets.GetSecretResponse, error) {
			called = true
			return nil, nil
		},
	}
	sp := &secretsProxy{
		c: spt,
	}
	server := grpc.NewServer()
	sp.Register(server)
	_, err := sp.GetSecret(context.Background(), &secrets.GetSecretRequest{})
	require.NoError(t, err)
	require.True(t, called)
}
