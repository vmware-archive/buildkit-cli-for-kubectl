package proxy

import (
	"context"

	control "github.com/moby/buildkit/api/services/control"
	"github.com/sirupsen/logrus"
)

func (s *server) Solve(ctx context.Context, req *control.SolveRequest) (*control.SolveResponse, error) {
	logrus.Debugf("proxying Solve ref %s session %s", req.Ref, req.Session)

	// Determine if we need to make any hijacking modifications, or let the request pass through unmodified
	if len(req.Exporter) > 0 {
		if req.Exporter == "image" || req.Exporter == "docker" || req.Exporter == "oci" {
			if len(req.ExporterAttrs) > 0 {
				logrus.Infof("detected build with docker/containerd exporter - hijacking")
				err := s.addLocalExporter(ctx, req)
				if err != nil {
					return nil, err
				}
			} // Else it's a local save of the image tar file, don't hijack
		}
	} else {
		logrus.Debugf("build already has exporter - passing through unmodified")
	}
	resp, err := s.buildkitd.Solve(ctx, req)
	logrus.Debugf("solve finished: %#v %v", resp, err)
	return resp, err
}

func (s *server) addLocalExporter(ctx context.Context, req *control.SolveRequest) error {
	session := s.getSession(req.Session)
	session.exporterAttrs = req.ExporterAttrs

	// TODO - any other re-wiring required here?
	return nil
}
