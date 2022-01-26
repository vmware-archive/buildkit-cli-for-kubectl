package proxy

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/moby/buildkit/session/filesync"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fileSendClientMock struct {
	// Note: overloading 2 interfaces for testing purposes
	results *bytes.Buffer
	done    chan (error)
	grpc.ClientStream
}

func (fsc *fileSendClientMock) DiffCopy(ctx context.Context, opts ...grpc.CallOption) (filesync.FileSend_DiffCopyClient, error) {
	return fsc, nil
}

func (fsc *fileSendClientMock) Send(*filesync.BytesMessage) error {
	return nil
}
func (fsc *fileSendClientMock) Recv() (*filesync.BytesMessage, error) {
	return nil, nil
}
func (fsc *fileSendClientMock) SendMsg(m interface{}) error {
	bm := m.(*filesync.BytesMessage)
	_, err := fsc.results.Write(bm.Data)
	return err
}
func (fsc *fileSendClientMock) RecvMsg(m interface{}) error {
	err := <-fsc.done // never fired
	return err
}

type fileSendServerMock struct {
	ctx  context.Context
	data *bytes.Buffer
	grpc.ServerStream
}

func (*fileSendServerMock) Send(*filesync.BytesMessage) error {
	return nil
}
func (*fileSendServerMock) Recv() (*filesync.BytesMessage, error) {
	return nil, nil
}
func (fss *fileSendServerMock) Context() context.Context {
	return fss.ctx
}
func (fss *fileSendServerMock) RecvMsg(m interface{}) error {
	bm := m.(*filesync.BytesMessage)
	if len(bm.Data) == 0 {
		bm.Data = make([]byte, 32*1024)
	}
	_, err := fss.data.Read(bm.Data)
	return err
}

type localImageLoaderMock struct {
	results *bytes.Buffer
}

func (*localImageLoaderMock) HijackRequired(sessionID string) bool {
	return true
}
func (lil *localImageLoaderMock) ImageLoad(ctx context.Context, input io.Reader) error {
	_, err := io.Copy(lil.results, input)
	return err
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func TestFileSendProxy(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	expected := randSeq(1024 * 1024)
	buf := bytes.NewBufferString(expected)

	fsc := &fileSendClientMock{
		results: &bytes.Buffer{},
	}
	lil := &localImageLoaderMock{
		results: &bytes.Buffer{},
	}
	fsp := &fileSendProxy{
		c:         fsc,
		sessionID: "session_id_here",
		loader:    lil,
	}
	md := metadata.New(map[string]string{
		keyExporterMetaPrefix + "foo": "someval",
	})
	fss := &fileSendServerMock{
		ctx:  metadata.NewIncomingContext(context.Background(), md),
		data: buf,
	}
	server := grpc.NewServer()
	fsp.Register(server)

	err := fsp.DiffCopy(fss)
	require.NoError(t, err)
	require.Equal(t, expected, lil.results.String())

	// Now reset and run again without hijacking, results go to fsc
	fss.ctx = context.Background()
	fss.data = bytes.NewBufferString(expected)
	err = fsp.DiffCopy(fss)
	require.NoError(t, err)
	res := fsc.results.String()
	require.Equal(t, len(expected), len(res))
	require.Equal(t, expected, res)
}
