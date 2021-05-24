package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	pb "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/proxy/proto"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Note: this code does not currently send detailed progress reporting.
// In most cases, nodes within a cluster will have generally consistent
// network performance.  This code will stream at the slowest consumers speed.
// In the future, we may want to perform more buffering to streamline
// the data transfer, and send per-node progress reporting through the log
// channel back to the CLI.

// LocalImageLoader is used to hijack the image load to the local runtime and/or remote runtimes
type LocalImageLoader interface {
	HijackRequired(sessionID string) bool
	// TODO need to figure out progress reporting wiring...
	ImageLoad(ctx context.Context, input io.Reader) error
}

func (s *server) HijackRequired(sessionID string) bool {
	session := s.getSession(sessionID)
	return len(session.exporterAttrs) > 0
}

func (s *server) ImageLoad(ctx context.Context, input io.Reader) error {
	var localInput io.Reader
	g, ctx := errgroup.WithContext(ctx)
	s.replicationLock.Lock()
	if len(s.remotes) > 0 {
		logrus.Debugf("replicating image load to %d nodes", len(s.remotes))
		pipeWriters := []*io.PipeWriter{}
		writers := []io.Writer{}
		pr, pw := io.Pipe()
		writers = append(writers, pw)
		pipeWriters = append(pipeWriters, pw)
		localInput = pr

		for _, remoteNode := range s.remotes {
			// We should not see unhealthy nodes in this list of remote nodes
			// so we'll consider failures to connect as a hard failure
			// If this becomes problematic, improve health reporting for pods, or
			// soften this algorithm to skip non-responsive remote nodes

			ctx2, cancel := context.WithTimeout(ctx, 20*time.Second) // TODO configurable timeout?
			grpcConn, err := grpc.DialContext(ctx2,
				fmt.Sprintf("%s:%d", remoteNode.Addr, PortNumber),
				grpc.WithInsecure(),
				grpc.WithBlock(),
			)
			cancel()
			if err != nil {
				return errors.Wrapf(err, "failed to connect to remote builder %s", remoteNode.Addr)
			}

			nodeClient := pb.NewImageLoaderClient(grpcConn)

			// Wire up the key in the grpc metadata
			md := metadata.New(map[string]string{
				"key": remoteNode.Key,
			})
			loadClient, err := nodeClient.Load(metadata.NewOutgoingContext(ctx, md))
			if err != nil {
				return errors.Wrapf(err, "failed to set up Load client to builder %s", remoteNode.Addr)
			}

			pr, pw := io.Pipe()
			writers = append(writers, pw)
			pipeWriters = append(pipeWriters, pw)
			node := remoteNode
			g.Go(func() error {
				logrus.Debugf("sending image to remote builder at %s", node.Addr)
				buf := make([]byte, 32*2014)
				var err error
				for {
					select {
					case <-ctx.Done():
						return nil
					default:
					}

					var n int
					n, err = pr.Read(buf)
					if err != nil {
						break
					}
					msg := &pb.BytesMessage{
						Data: buf[0:n],
					}
					err = loadClient.Send(msg)
					if err != nil {
						break
					}
				}
				_, err2 := loadClient.CloseAndRecv()
				if err2 != nil && err2 != io.EOF {
					logrus.Warnf("failed to close remote loader %s: %s", node.Addr, err2)
				}
				if err == io.EOF {
					return nil
				} else if err != nil {
					return errors.Wrapf(err, "failed to transfer image to %s", node.Addr)
				}
				return nil
			})
		}
		// Note: MultiWriter stops on first failure, so we might consider a custom impl to better support partial failures
		writer := io.MultiWriter(writers...)
		g.Go(func() error {
			_, err := io.Copy(writer, input)
			for _, w := range pipeWriters {
				err2 := w.Close()
				if err2 != nil {
					logrus.Warnf("failed to close pipewriter: %s", err2)
				}
			}
			return err
		})
	} else {
		localInput = input
	}
	s.replicationLock.Unlock()

	g.Go(func() error {
		if s.cfg.ContainerdSocketPath != "" {
			return s.containerdLoad(ctx, localInput)
		}
		return s.dockerdLoad(ctx, localInput)
	})
	err := g.Wait()
	return err
}

func (s *server) Listen(ctx context.Context, req *pb.ListenRequest) (*pb.ListenResponse, error) {
	s.replicationLock.Lock()
	defer s.replicationLock.Unlock()
	if s.listenRefCount == 0 {
		logrus.Infof("starting TCP based listen on port %d", PortNumber)
		lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", PortNumber))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start gRPC listener on port %d", PortNumber)
		}
		s.grpcServer = grpc.NewServer()
		pb.RegisterImageLoaderServer(s.grpcServer, s)
		go func() {
			if err := s.grpcServer.Serve(lis); err != nil {
				logrus.Errorf("failed to serve: %v", err)
			}
		}()
	}
	s.listenRefCount++
	return &pb.ListenResponse{Key: s.listenKey}, nil
}

func (s *server) StopListen(ctx context.Context, req *pb.ListenResponse) (*pb.StopListenResponse, error) {
	s.replicationLock.Lock()
	defer s.replicationLock.Unlock()
	if s.listenRefCount == 0 {
		return nil, fmt.Errorf("internal error - zero refcount on StopListen")
	}
	s.listenRefCount--
	if s.listenRefCount == 0 {
		s.grpcServer.Stop()
		// TODO Any other cleanups to perform here?
	}

	return &pb.StopListenResponse{}, nil
}

func (s *server) Replicate(ctx context.Context, req *pb.ReplicateRequest) (*pb.ReplicateResponse, error) {
	logrus.Debugf("setting up replication to %d remote nodes", len(req.Nodes))
	s.replicationLock.Lock()
	defer s.replicationLock.Unlock()

	for _, node := range req.Nodes {
		found := false
		for i, test := range s.remotes {
			if test.Addr == node.Addr {
				if test.Key != node.Key {
					logrus.Warnf("key drift detected for node %s - updating key", node.Addr)
					s.remotes[i].Key = node.Key
				}
				found = true
				break
			}
		}
		if !found {
			s.remotes = append(s.remotes, *node)
		}
	}
	return &pb.ReplicateResponse{}, nil
}

func (s *server) Load(srv pb.ImageLoader_LoadServer) error {
	ctx := srv.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	keys := md.Get("key")
	if len(keys) != 1 {
		logrus.Info("received image from remote builder without key, rejecting")
		return fmt.Errorf("valid key must be specified in request metadata")
	}
	logrus.Debugf("proxying files from build context: %v", md)
	if keys[0] != s.listenKey {
		logrus.Info("received image from remote builder with invalid key, rejecting")
		return fmt.Errorf("invalid key sent from remote")
	}

	logrus.Debug("receiving image from remote builder with valid key")

	// Looks like a good request, start up the pipe for loading
	pr, pw := io.Pipe()
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			msg, err := srv.Recv()
			if err != nil && err == io.EOF {
				pw.Close()
				return nil
			} else if err != nil {
				_ = pw.CloseWithError(err)
				return errors.Wrap(err, "failed to read from remote sender")
			}
			_, err = pw.Write(msg.Data)
			if err != nil {
				return errors.Wrap(err, "failed to write to pipe for local runtime")
			}
		}
	})
	g.Go(func() error {
		if s.cfg.ContainerdSocketPath != "" {
			return s.containerdLoad(ctx, pr)
		}
		return s.dockerdLoad(ctx, pr)
	})
	err := g.Wait()
	return err
}
