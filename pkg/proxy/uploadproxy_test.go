package proxy

import (
	"bytes"
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/moby/buildkit/session/upload"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type uploadClientMock struct {
	// Note: overloading 2 interfaces for testing purposes
	results *bytes.Buffer
	done    chan (error)
	grpc.ClientStream
}

func (fsc *uploadClientMock) Pull(ctx context.Context, opts ...grpc.CallOption) (upload.Upload_PullClient, error) {
	return fsc, nil
}

func (fsc *uploadClientMock) Send(*upload.BytesMessage) error {
	return nil
}
func (fsc *uploadClientMock) Recv() (*upload.BytesMessage, error) {
	return nil, nil
}
func (fsc *uploadClientMock) SendMsg(m interface{}) error {
	bm := m.(*upload.BytesMessage)
	_, err := fsc.results.Write(bm.Data)
	return err
}
func (fsc *uploadClientMock) RecvMsg(m interface{}) error {
	err := <-fsc.done // never fired
	return err
}

type uploadServerMock struct {
	ctx  context.Context
	data *bytes.Buffer
	grpc.ServerStream
}

func (*uploadServerMock) Send(*upload.BytesMessage) error {
	return nil
}
func (*uploadServerMock) Recv() (*upload.BytesMessage, error) {
	return nil, nil
}
func (fss *uploadServerMock) Context() context.Context {
	return fss.ctx
}
func (fss *uploadServerMock) RecvMsg(m interface{}) error {
	bm := m.(*upload.BytesMessage)
	if len(bm.Data) == 0 {
		bm.Data = make([]byte, 32*1024)
	}
	_, err := fss.data.Read(bm.Data)
	return err
}

func TestUploadProxy(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	expected := randSeq(1024 * 1024)
	buf := bytes.NewBufferString(expected)

	fsc := &uploadClientMock{
		results: &bytes.Buffer{},
	}
	fsp := &uploadProxy{
		c: fsc,
	}
	md := metadata.New(map[string]string{
		"foo": "someval",
	})
	fss := &uploadServerMock{
		ctx:  metadata.NewIncomingContext(context.Background(), md),
		data: buf,
	}
	server := grpc.NewServer()
	fsp.Register(server)

	err := fsp.Pull(fss)
	require.NoError(t, err)
	res := fsc.results.String()
	require.Equal(t, len(expected), len(res))
	require.Equal(t, expected, res)
}
