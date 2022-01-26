package proxy

import (
	"context"
	"fmt"
	"io"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type CtrdImporter interface {
	Import(ctx context.Context, reader io.Reader, opts ...containerd.ImportOpt) ([]images.Image, error)
	Close() error
}

func (s *server) containerdConnect(sockPath string) error {
	namespace := "k8s.io" // TODO - this should be driven by inputs/configuration
	client, err := containerd.New(sockPath,
		containerd.WithDefaultNamespace(namespace),
	)
	if err != nil {
		return fmt.Errorf("failed to set up containerd client via proxy: %w", err)
	}
	s.ctrdClient = client
	return nil
}

func (s *server) containerdLoad(ctx context.Context, input io.Reader) error {
	logrus.Debugf("loading image to local containerd runtime")
	imgs, err := s.ctrdClient.Import(ctx, input,

		// TODO - might need to pass through the tagged name with the tag part stripped off
		//containerd.WithImageRefTranslator(archive.FilterRefPrefix(prefix),

		// TODO - might need this too..
		//containerd.WithIndexName(tag),

		// TODO - is this necessary or implicit?  (try without after it's working...)
		containerd.WithAllPlatforms(false),
	)
	if err != nil {
		return errors.Wrap(err, "containerd image Import error")
	}
	for _, img := range imgs {
		image := containerd.NewImage(s.ctrdClient.(*containerd.Client), img)

		// TODO: Show unpack status
		//progress.Write(prog, fmt.Sprintf("unpacking %s (%s)", img.Name, img.Target.Digest), func() error { return nil })
		err = image.Unpack(ctx, "") // Empty snapshotter is default
		if err != nil {
			return errors.Wrap(err, "containerd image Unpack error")
		}
	}
	return nil
}
