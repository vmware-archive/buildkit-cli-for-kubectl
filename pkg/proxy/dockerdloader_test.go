package proxy

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

type dockerdClientMock struct {
	received bytes.Buffer
}

func (dcm *dockerdClientMock) ImageLoad(ctx context.Context, input io.Reader, quiet bool) (types.ImageLoadResponse, error) {
	if input != nil {
		_, err := io.Copy(&dcm.received, input)
		if err != nil {
			return types.ImageLoadResponse{}, err
		}
	}
	return types.ImageLoadResponse{
		Body: ioutil.NopCloser(&bytes.Buffer{}),
	}, nil
}

func TestDockerdLoad(t *testing.T) {
	ctx := context.Background()
	dcm := &dockerdClientMock{}
	s := server{
		dckrdClient: dcm,
		sessions:    map[string]*sessionProxies{},
	}
	err := s.dockerdLoad(ctx, nil)
	require.NoError(t, err)

	err = s.dockerdConnect("dummy-socket-path")
	// Note: docker client connect doesn't validate socket
	require.NoError(t, err)
}
