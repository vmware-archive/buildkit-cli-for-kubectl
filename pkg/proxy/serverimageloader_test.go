package proxy

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pb "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/proxy/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type loadServerMock struct {
	ctx  context.Context
	data *bytes.Buffer
	grpc.ServerStream
}

func (*loadServerMock) SendAndClose(*pb.LoadResponse) error {
	return nil
}
func (ls *loadServerMock) Recv() (*pb.BytesMessage, error) {
	bm := &pb.BytesMessage{
		Data: make([]byte, 32*1024),
	}
	_, err := ls.data.Read(bm.Data)
	return bm, err
}
func (ls *loadServerMock) Context() context.Context {
	return ls.ctx
}

func TestImageLoadRoutines(t *testing.T) {
	ctx := context.Background()
	c := &buildkitControlClientMock{}
	s := server{
		buildkitd: c,
		sessions:  map[string]*sessionProxies{},
		listenKey: "listenkey",
	}
	res := s.HijackRequired("sessionid")
	require.False(t, res)

	_, err := s.StopListen(ctx, &pb.ListenResponse{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "zero refcount")

	// ImageLoad without remotes
	ccm := &containerdClientMock{}
	dcm := &dockerdClientMock{}
	s.ctrdClient = ccm
	s.dckrdClient = dcm
	err = s.ImageLoad(ctx, &bytes.Buffer{})
	require.NoError(t, err)
	s.cfg.ContainerdSocketPath = "dummy-socket-path"
	err = s.ImageLoad(ctx, &bytes.Buffer{})
	require.NoError(t, err)

	_, err = s.Replicate(ctx, &pb.ReplicateRequest{
		Nodes: []*pb.Node{
			{
				Key:  "key1",
				Addr: "addr1",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, s.remotes, 1)
	_, err = s.Replicate(ctx, &pb.ReplicateRequest{
		Nodes: []*pb.Node{
			{
				Key:  "key3",
				Addr: "addr1",
			},
			{
				Key:  "key2",
				Addr: "addr2",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, s.remotes, 2)
	require.Equal(t, s.remotes[0].Key, "key3")

	var resp *pb.ListenResponse
	// TODO consider hardening for in-use sockets, and alter the PortNumber
	resp, err = s.Listen(ctx, &pb.ListenRequest{})
	require.NoError(t, err)
	require.Equal(t, s.listenKey, resp.Key)

	_, err = s.StopListen(ctx, resp)
	require.NoError(t, err)

	expected := randSeq(1024 * 1024)
	buf := bytes.NewBufferString(expected)
	md := metadata.New(map[string]string{
		"key": s.listenKey,
	})
	ls := &loadServerMock{
		ctx:  metadata.NewIncomingContext(context.Background(), md),
		data: buf,
	}
	err = s.Load(ls)
	require.NoError(t, err)

	received := ccm.received.String()
	require.Equal(t, len(expected), len(received))
	require.Equal(t, expected, received)

	ls.ctx = context.Background()
	err = s.Load(ls)
	require.Error(t, err)
	require.Contains(t, err.Error(), "key must be specified")

	md = metadata.New(map[string]string{
		"key": "invalid-value",
	})
	ls.ctx = metadata.NewIncomingContext(context.Background(), md)
	err = s.Load(ls)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid key")
}
