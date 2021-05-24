package proxy

import (
	"fmt"
	"io"

	"github.com/moby/buildkit/session/filesync"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tonistiigi/fsutil/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The FileSync proxy routes requests from buildkitd through to the CLI
//
// This is used to retrieve files from the build context
//
// Note: while FileSync and FileSend are nearly identical, they are subtly different
//       so while it might be possible to DRY them out, the nuances would get even more confusing
type fileSyncProxy struct {
	// Connection to the client to send requests through to...
	c filesync.FileSyncClient
}

func (fsp *fileSyncProxy) Register(server *grpc.Server) {
	filesync.RegisterFileSyncServer(server, fsp)
}

func (fsp *fileSyncProxy) DiffCopy(buildkitStream filesync.FileSync_DiffCopyServer) error {
	ctx := buildkitStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	logrus.Debugf("proxying files from build context: %v", md)
	ctx = metadata.NewOutgoingContext(ctx, md)

	clientStream, err := fsp.c.DiffCopy(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to establish DiffCopy to CLI")
	}
	finished := make(chan error)

	go func() {
		// Note: different APIs have different payloads, so we can't DRY this out easily...
		p := types.Packet{
			Data: make([]byte, 32*1024),
		}
		var err error
		for err == nil {
			if err = buildkitStream.RecvMsg(&p); err == nil {
				err = clientStream.SendMsg(&p)
			}
		}
		finished <- err
	}()

	go func() {
		p := types.Packet{
			Data: make([]byte, 32*1024),
		}
		var err error
		for err == nil {
			if err = clientStream.RecvMsg(&p); err == nil {
				err = buildkitStream.SendMsg(&p)
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

func (fsp *fileSyncProxy) TarStream(buildkitStream filesync.FileSync_TarStreamServer) error {
	// Based on code inspection of the BuildKit repo, this API appears to be unused.
	// We'll leave it unimplemented for now, and if this error pops up, we'll add
	// an implementation.
	ctx := buildkitStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	logrus.Warningf("the TarStream API is not yet implemented for the proxy.  Request metadata: %#v", md)
	return fmt.Errorf("the TarStream API is not yet implemented for the proxy.  Request metadata: %#v", md)
}
