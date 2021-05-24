package proxy

import (
	"context"

	"github.com/moby/buildkit/session/auth"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// The Auth proxy routes requests from buildkitd through to the CLI
//
// TODO - in the future we might want to consider supporting a flow
//        where credentials can be retrieved from within the cluster
//        possibly via mounted secrets in the builder pod
//        and only if those aren't found, then send the request through
//        to the CLI
type authProxy struct {
	// Connection to the client to send requests through to...
	c auth.AuthClient
}

func (ap *authProxy) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, ap)
}

func (ap *authProxy) Credentials(ctx context.Context, in *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	logrus.Debugf("proxying Auth Credentials request for %s", in.Host)
	return ap.c.Credentials(ctx, in)
}

// TODO - to actually implement these properly, use buildkit/session/autrh/authprovider/authprovider.go for inspiration
func (ap *authProxy) FetchToken(ctx context.Context, req *auth.FetchTokenRequest) (*auth.FetchTokenResponse, error) {
	return ap.c.FetchToken(ctx, req)
}
func (ap *authProxy) GetTokenAuthority(ctx context.Context, req *auth.GetTokenAuthorityRequest) (*auth.GetTokenAuthorityResponse, error) {
	return ap.c.GetTokenAuthority(ctx, req)
}
func (ap *authProxy) VerifyTokenAuthority(ctx context.Context, req *auth.VerifyTokenAuthorityRequest) (*auth.VerifyTokenAuthorityResponse, error) {
	return ap.c.VerifyTokenAuthority(ctx, req)
}
