package proxy

import (
	"io"

	"github.com/moby/buildkit/session/upload"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The Upload proxy routes requests from buildkitd through to the CLI
type uploadProxy struct {
	c upload.UploadClient
}

func (up *uploadProxy) Register(server *grpc.Server) {
	upload.RegisterUploadServer(server, up)
}

func (up *uploadProxy) Pull(buildkitStream upload.Upload_PullServer) error {
	ctx := buildkitStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	logrus.Debugf("proxying Upload Pull: %#v", md)
	ctx = metadata.NewOutgoingContext(ctx, md)

	clientStream, err := up.c.Pull(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to establish Upload Pull to CLI")
	}
	finished := make(chan error)

	go func() {
		// Note: different APIs have different payloads, so we can't DRY this out easily...
		bm := upload.BytesMessage{
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
		bm := upload.BytesMessage{
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
