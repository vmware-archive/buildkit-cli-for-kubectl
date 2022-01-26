package proxy

import (
	"io"
	"strings"

	"github.com/moby/buildkit/session/filesync"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The FileSend proxy comes into play when BuildKit is sending payloads back to the client
//
// This is where we hijack the requests so we can load the resulting image into the local runtime
// and send to other nodes in the cluster
//
// Note: while FileSync and FileSend are nearly identical, they are subtly different
//       so while it might be possible to DRY them out, the nuances would get even more confusing
type fileSendProxy struct {
	c         filesync.FileSendClient
	sessionID string
	loader    LocalImageLoader
}

func (fsp *fileSendProxy) Register(server *grpc.Server) {
	filesync.RegisterFileSendServer(server, fsp)
}

func (fsp *fileSendProxy) DiffCopy(buildkitStream filesync.FileSend_DiffCopyServer) error {
	ctx := buildkitStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	exporterMetadata := map[string][]string{}

	// Extract any Exporter metadata and strip off the prefix
	for key := range md {
		if strings.HasPrefix(key, keyExporterMetaPrefix) {
			exporterMetadata[strings.TrimPrefix(key, keyExporterMetaPrefix)] = md[key]
		}
	}
	if len(exporterMetadata) > 0 && fsp.loader.HijackRequired(fsp.sessionID) {
		logrus.Debugf("detected a FileSend requiring hijack")
		// Note: the return flow from client->server is unused
		pr, pw := io.Pipe()
		go func() {
			bm := filesync.BytesMessage{
				Data: make([]byte, 32*1024),
			}
			var err error
			for err == nil {
				if err = buildkitStream.RecvMsg(&bm); err == nil {
					_, err = pw.Write(bm.Data)
				}
			}
			if err == nil || err == io.EOF {
				pw.Close()
			} else {
				_ = pw.CloseWithError(err)
			}
		}()
		return fsp.loader.ImageLoad(ctx, pr)
	}

	ctx = metadata.NewOutgoingContext(ctx, md)

	clientStream, err := fsp.c.DiffCopy(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to establish DiffCopy to CLI")
	}
	finished := make(chan error)

	go func() {
		// Note: different APIs have different payloads, so we can't DRY this out easily...
		bm := filesync.BytesMessage{
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
		bm := filesync.BytesMessage{
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
