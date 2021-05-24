package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	control "github.com/moby/buildkit/api/services/control"
	"github.com/sirupsen/logrus"
	pb "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/proxy/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// Replicated from the session package as they're unexported
	headerSessionID        = "X-Docker-Expose-Session-Uuid"
	headerSessionName      = "X-Docker-Expose-Session-Name"
	headerSessionSharedKey = "X-Docker-Expose-Session-Sharedkey"
	// headerSessionMethod    = "X-Docker-Expose-Session-Grpc-Method" // unused

	// Replicated from the filesync package as they're unexported
	keyExporterMetaPrefix = "exporter-md-"
)

type Runtime int

const (
	ContainerdRuntime Runtime = 0
	DockerdRuntime    Runtime = 1
)

func (r Runtime) String() string {
	if r == ContainerdRuntime {
		return "containerd"
	}
	return "dockerd"
}

type ServerConfig struct {
	BuildkitdSocketPath  string
	ContainerdSocketPath string
	DockerdSocketPath    string
	HelperSocketPath     string
}

func (cfg ServerConfig) Validate() error {
	if cfg.BuildkitdSocketPath == "" {
		return fmt.Errorf("buildkitd sock path must be specified")
	}
	if cfg.HelperSocketPath == "" {
		return fmt.Errorf("proxy sock path must be specified")
	}
	if (cfg.ContainerdSocketPath == "" && cfg.DockerdSocketPath == "") || (cfg.ContainerdSocketPath != "" && cfg.DockerdSocketPath != "") {
		return fmt.Errorf("you must specify exactly one of containerd or dockerd runtime socket paths")
	}
	return nil
}

func (cfg ServerConfig) Runtime() Runtime {
	if cfg.ContainerdSocketPath != "" {
		return ContainerdRuntime
	}
	return DockerdRuntime
}

type sessionProxies struct {
	// Note: only contains the ones we do interesting hijacking with
	fileSendProxy *fileSendProxy

	exporterAttrs map[string]string
}
type server struct {
	cfg       ServerConfig
	buildkitd control.ControlClient

	ctrdClient  CtrdImporter
	dckrdClient DckrdImporter

	sessionsLock sync.Mutex
	sessions     map[string]*sessionProxies

	replicationLock sync.Mutex
	listenKey       string
	listenRefCount  uint // the number of active listen requests
	remotes         []pb.Node

	proxyGrpcServer *grpc.Server
	grpcServer      *grpc.Server
}

type Server interface {
	Serve(context.Context) error
}

func NewProxy(ctx context.Context, cfg ServerConfig) (Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// Create our proxy connection to the real buildkitd
	buildkitd, err := getBuildkitClient(cfg)
	if err != nil {
		return nil, err
	}
	uuid, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}
	srv := grpc.NewServer()
	s := &server{
		cfg:             cfg,
		buildkitd:       buildkitd,
		sessions:        map[string]*sessionProxies{},
		listenKey:       uuid.String(),
		proxyGrpcServer: srv,
	}
	control.RegisterControlServer(srv, s)
	pb.RegisterProxyServer(srv, s)
	// Connect to the local container runtime
	if err := s.connectToRuntime(); err != nil {
		return nil, err
	}
	return s, nil
}

func getBuildkitClient(cfg ServerConfig) (control.ControlClient, error) {
	logrus.Debugf("dialing buildkit socket: %s", cfg.BuildkitdSocketPath)
	grpcConn, err := grpc.Dial(
		cfg.BuildkitdSocketPath,
		grpc.WithInsecure(),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", addr)
		}),
	)
	if err != nil {
		return nil, err
	}
	buildkitd := control.NewControlClient(grpcConn)
	return buildkitd, nil
}

func (s *server) connectToRuntime() error {
	runtime := s.cfg.Runtime()
	logrus.Infof("Starting BuildKit proxy for %s runtime on %s", runtime, s.cfg.HelperSocketPath)
	switch runtime {
	case DockerdRuntime:
		err := s.dockerdConnect(s.cfg.DockerdSocketPath)
		if err != nil {
			return err
		}
	case ContainerdRuntime:
		err := s.containerdConnect(s.cfg.ContainerdSocketPath)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unrecognized runtime")
	}
	return nil
}

func (s *server) waitForBuildkitHealth(ctx context.Context) {
	logrus.Info("checking for buildkitd health")
	for {
		select {
		case <-ctx.Done():
			logrus.Warn("timed out waiting for buildkit to become health")
			return
		default:
		}

		resp, err := s.buildkitd.ListWorkers(ctx, &control.ListWorkersRequest{})
		if err == nil {
			logrus.Infof("buildkitd workers detected: %#v", resp.Record)
			break
		}
		logrus.Debugf("buildkitd not ready yet: %s", err)
		time.Sleep(500 * time.Millisecond) // TODO - backoff maybe?
	}
}

func (s *server) Serve(ctx context.Context) error {
	lis, err := net.Listen("unix", s.cfg.HelperSocketPath)
	if err != nil {
		return err
	}
	if s.ctrdClient != nil {
		defer s.ctrdClient.Close()
	}
	// Make sure we can talk to buildkitd and it's responding.
	s.waitForBuildkitHealth(ctx)
	logrus.Info("proxy is ready")

	return s.proxyGrpcServer.Serve(lis)
}

func (s *server) DiskUsage(ctx context.Context, req *control.DiskUsageRequest) (*control.DiskUsageResponse, error) {
	logrus.Debugf("proxying DiskUsage %v", req.Filter)
	return s.buildkitd.DiskUsage(ctx, req)
}
func (s *server) ListWorkers(ctx context.Context, req *control.ListWorkersRequest) (*control.ListWorkersResponse, error) {
	logrus.Debugf("proxying ListWorkers %v", req.Filter)
	return s.buildkitd.ListWorkers(ctx, req)
}

func (s *server) Prune(req *control.PruneRequest, clientStream control.Control_PruneServer) error {
	// TODO: this is currently untested as we lack a CLI UX to poke at it
	// https://github.com/vmware-tanzu/buildkit-cli-for-kubectl/issues/100
	logrus.Debugf("proxying Prune request: %#v", req)
	ctx := clientStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	buildkitStream, err := s.buildkitd.Prune(ctx, req)
	if err != nil {
		return err
	}
	p := control.UsageRecord{}
	for err == nil {
		if err = buildkitStream.RecvMsg(&p); err == nil {
			err = clientStream.SendMsg(&p)
		}
	}
	if err == io.EOF {
		return nil
	}
	return err
}

func (s *server) Status(req *control.StatusRequest, clientStream control.Control_StatusServer) error {
	logrus.Debugf("proxying Status for %s", req.Ref)
	ctx := clientStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)
	buildkitStream, err := s.buildkitd.Status(ctx, req)
	if err != nil {
		return err
	}
	p := control.StatusResponse{}
	// Note:  If we want to wire up more fine-grain status reporting on the transfer status
	//        this is where it will belong.  Refactor this to use a channel so that messages
	//        can flow from both buildkit and the proxy interleaved out to the client
	//        More investigation is required to figure out how to get the StatusResponse
	//        messages set up properly - do the vertex IDs have to match something "real" or
	//        can something synthetic be cobbled together in a way that renders well in the CLI?
	for err == nil {
		if err = buildkitStream.RecvMsg(&p); err == nil {
			err = clientStream.SendMsg(&p)
		}
	}
	if err == io.EOF {
		return nil
	}
	return err
}

func (s *server) getSession(sessionID string) *sessionProxies {
	s.sessionsLock.Lock()
	defer s.sessionsLock.Unlock()

	sp, found := s.sessions[sessionID]
	if !found {
		sp = &sessionProxies{}
		s.sessions[sessionID] = sp
	}
	return sp

}
