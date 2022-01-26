package proxy

import (
	"context"
	"io"
	"io/ioutil"

	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type DckrdImporter interface {
	ImageLoad(ctx context.Context, input io.Reader, quiet bool) (types.ImageLoadResponse, error)
}

func (s *server) dockerdConnect(sockPath string) error {
	c, err := dockerclient.NewClientWithOpts(
		dockerclient.WithAPIVersionNegotiation(),
		dockerclient.WithHost("unix://"+s.cfg.DockerdSocketPath),
	)
	if err != nil {
		return errors.Wrapf(err, "unable to connect to docker socket %s", s.cfg.DockerdSocketPath)
	}
	s.dckrdClient = c
	return nil
}

func (s *server) dockerdLoad(ctx context.Context, input io.Reader) error {
	logrus.Debug("loading image to local dockerd runtime")
	resp, err := s.dckrdClient.ImageLoad(ctx, input, false)
	if err != nil {
		return errors.Wrap(err, "failed to begin load image locally")
	}

	// TODO actual progress reporting on image load...
	/*
		prog := mw.WithPrefix("", false)

		close(started[i])

		progress.FromReader(prog, fmt.Sprintf("loading image to docker runtime via pod %s", names[i]), resp.Body)
	*/
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to transfer image locally")
	}
	return nil
}
