package proxy

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/moby/buildkit/session/sshforward"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type SSHClientMock struct {
	// Note: overloading 2 interfaces for testing purposes
	results            *bytes.Buffer
	done               chan (error)
	checkAgentResponse *sshforward.CheckAgentResponse
	checkAgentErr      error
	grpc.ClientStream
}

func (fsc *SSHClientMock) CheckAgent(ctx context.Context, in *sshforward.CheckAgentRequest, opts ...grpc.CallOption) (*sshforward.CheckAgentResponse, error) {
	return fsc.checkAgentResponse, fsc.checkAgentErr
}
func (fsc *SSHClientMock) ForwardAgent(ctx context.Context, opts ...grpc.CallOption) (sshforward.SSH_ForwardAgentClient, error) {
	return fsc, nil
}

func (fsc *SSHClientMock) Send(*sshforward.BytesMessage) error {
	return nil
}
func (fsc *SSHClientMock) Recv() (*sshforward.BytesMessage, error) {
	return nil, nil
}
func (fsc *SSHClientMock) SendMsg(m interface{}) error {
	bm := m.(*sshforward.BytesMessage)
	_, err := fsc.results.Write(bm.Data)
	return err
}
func (fsc *SSHClientMock) RecvMsg(m interface{}) error {
	err := <-fsc.done // never fired
	return err
}

type SSHServerMock struct {
	ctx  context.Context
	data *bytes.Buffer
	grpc.ServerStream
}

func (*SSHServerMock) Send(*sshforward.BytesMessage) error {
	return nil
}
func (*SSHServerMock) Recv() (*sshforward.BytesMessage, error) {
	return nil, nil
}
func (fss *SSHServerMock) Context() context.Context {
	return fss.ctx
}
func (fss *SSHServerMock) RecvMsg(m interface{}) error {
	bm := m.(*sshforward.BytesMessage)
	if len(bm.Data) == 0 {
		bm.Data = make([]byte, 32*1024)
	}
	_, err := fss.data.Read(bm.Data)
	return err
}

func TestSSHProxy(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	expected := randSeq(1024 * 1024)
	buf := bytes.NewBufferString(expected)

	fsc := &SSHClientMock{
		results:       &bytes.Buffer{},
		checkAgentErr: fmt.Errorf("expected error"),
	}
	fsp := &sshProxy{
		c: fsc,
	}
	md := metadata.New(map[string]string{
		"foo": "someval",
	})
	fss := &SSHServerMock{
		ctx:  metadata.NewIncomingContext(context.Background(), md),
		data: buf,
	}
	server := grpc.NewServer()
	fsp.Register(server)

	err := fsp.ForwardAgent(fss)
	require.NoError(t, err)
	res := fsc.results.String()
	require.Equal(t, len(expected), len(res))
	require.Equal(t, expected, res)

	_, err = fsp.CheckAgent(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected error")
}
