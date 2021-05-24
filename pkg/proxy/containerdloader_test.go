package proxy

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/stretchr/testify/require"
)

type containerdClientMock struct {
	received bytes.Buffer
}

func (ccm *containerdClientMock) Import(ctx context.Context, reader io.Reader, opts ...containerd.ImportOpt) ([]images.Image, error) {
	if reader != nil {
		_, err := io.Copy(&ccm.received, reader)
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}
func (ccm *containerdClientMock) Close() error {
	return nil
}

func TestContainerdLoad(t *testing.T) {
	ctx := context.Background()
	ccm := &containerdClientMock{}
	s := server{
		ctrdClient: ccm,
		sessions:   map[string]*sessionProxies{},
	}
	err := s.containerdLoad(ctx, nil)
	require.NoError(t, err)

	// TODO hits a 10s timeout - maybe tune this to be faster...
	err = s.containerdConnect("dummy-socket-path")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to dial")
}
