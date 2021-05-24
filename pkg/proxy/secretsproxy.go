package proxy

import (
	"context"

	"github.com/moby/buildkit/session/secrets"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// The Secrets proxy routes requests from buildkitd through to the CLI
//
// TODO - in the future we might want to consider supporting a flow
//        where secrets can be retrieved from within the cluster
//        possibly via mounted secrets in the builder pod
//        and only if those aren't found, then send the request through
//        to the CLI
type secretsProxy struct {
	c secrets.SecretsClient
}

func (sp *secretsProxy) Register(server *grpc.Server) {
	secrets.RegisterSecretsServer(server, sp)
}

func (sp *secretsProxy) GetSecret(ctx context.Context, in *secrets.GetSecretRequest) (*secrets.GetSecretResponse, error) {
	logrus.Debugf("proxying Secrets %v", in)
	// TODO - consider being "smarter" and possibly support secrets loading from within the cluster?
	return sp.c.GetSecret(ctx, in)
}
