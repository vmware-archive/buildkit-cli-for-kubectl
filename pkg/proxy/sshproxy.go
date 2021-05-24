package proxy

import (
	"context"
	"io"

	"github.com/moby/buildkit/session/sshforward"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The SSH proxy routes requests from buildkitd through to the CLI
type sshProxy struct {
	c sshforward.SSHClient
}

func (sp *sshProxy) Register(server *grpc.Server) {
	sshforward.RegisterSSHServer(server, sp)
}

func (sp *sshProxy) CheckAgent(ctx context.Context, in *sshforward.CheckAgentRequest) (*sshforward.CheckAgentResponse, error) {
	logrus.Debugf("proxying SSH CheckAgent %v", in)
	return sp.c.CheckAgent(ctx, in)
}
func (sp *sshProxy) ForwardAgent(buildkitStream sshforward.SSH_ForwardAgentServer) error {
	ctx := buildkitStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	logrus.Debugf("proxying SSH forward: %v", md)
	ctx = metadata.NewOutgoingContext(ctx, md)

	clientStream, err := sp.c.ForwardAgent(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to establish SSH ForwardAgent to CLI")
	}
	finished := make(chan error)

	go func() {
		// Note: different APIs have different payloads, so we can't DRY this out easily...
		bm := sshforward.BytesMessage{
			Data: make([]byte, 32*1024),
		}
		var err error
		for err == nil {
			if err = buildkitStream.RecvMsg(&bm); err == nil {
				err = clientStream.SendMsg(&bm)
			}
		}
		finished <- err
	}()

	go func() {
		bm := sshforward.BytesMessage{
			Data: make([]byte, 32*1024),
		}
		var err error
		for err == nil {
			if err = clientStream.RecvMsg(&bm); err == nil {
				err = buildkitStream.SendMsg(&bm)
			}
		}
		finished <- err
	}()

	err = <-finished
	if err == io.EOF {
		return nil
	}
	return err
}
