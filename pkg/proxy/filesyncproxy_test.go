package proxy

import (
	"bytes"
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/moby/buildkit/session/filesync"
	"github.com/stretchr/testify/require"
	"github.com/tonistiigi/fsutil/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fileSyncClientMock struct {
	// Note: overloading 2 interfaces for testing purposes
	results *bytes.Buffer
	done    chan (error)
	grpc.ClientStream
}

func (fsc *fileSyncClientMock) DiffCopy(ctx context.Context, opts ...grpc.CallOption) (filesync.FileSync_DiffCopyClient, error) {
	return fsc, nil
}
func (fsc *fileSyncClientMock) TarStream(ctx context.Context, opts ...grpc.CallOption) (filesync.FileSync_TarStreamClient, error) {
	return fsc, nil
}

func (fsc *fileSyncClientMock) Send(*types.Packet) error {
	return nil
}
func (fsc *fileSyncClientMock) Recv() (*types.Packet, error) {
	return nil, nil
}
func (fsc *fileSyncClientMock) SendMsg(m interface{}) error {
	p := m.(*types.Packet)
	_, err := fsc.results.Write(p.Data)
	return err
}
func (fsc *fileSyncClientMock) RecvMsg(m interface{}) error {
	err := <-fsc.done // never fired
	return err
}

type fileSyncServerMock struct {
	ctx  context.Context
	data *bytes.Buffer
	grpc.ServerStream
}

func (*fileSyncServerMock) Send(*types.Packet) error {
	return nil
}
func (*fileSyncServerMock) Recv() (*types.Packet, error) {
	return nil, nil
}
func (fss *fileSyncServerMock) Context() context.Context {
	return fss.ctx
}
func (fss *fileSyncServerMock) RecvMsg(m interface{}) error {
	p := m.(*types.Packet)
	if len(p.Data) == 0 {
		p.Data = make([]byte, 32*1024)
	}
	_, err := fss.data.Read(p.Data)
	return err
}

func TestFileSyncProxy(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	expected := randSeq(1024 * 1024)
	buf := bytes.NewBufferString(expected)

	fsc := &fileSyncClientMock{
		results: &bytes.Buffer{},
	}
	fsp := &fileSyncProxy{
		c: fsc,
	}
	md := metadata.New(map[string]string{
		"foo": "someval",
	})
	fss := &fileSyncServerMock{
		ctx:  metadata.NewIncomingContext(context.Background(), md),
		data: buf,
	}
	server := grpc.NewServer()
	fsp.Register(server)

	err := fsp.DiffCopy(fss)
	require.NoError(t, err)
	res := fsc.results.String()
	require.Equal(t, len(expected), len(res))
	require.Equal(t, expected, res)

	err = fsp.TarStream(fss)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not yet implemented")
}
