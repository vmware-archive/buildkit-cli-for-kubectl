// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package suites

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/integration/common"
	"golang.org/x/sync/errgroup"

	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const ParallelDefaultBuildCount = 3

type parallelDefaultSuite struct {
	suite.Suite
	Name        string
	CreateFlags []string

	ClientSet *kubernetes.Clientset
	Namespace string

	configMapClient v1.ConfigMapInterface
}

func (s *parallelDefaultSuite) SetupSuite() {
	var err error
	s.ClientSet, s.Namespace, err = common.GetKubeClientset()
	require.NoError(s.T(), err, "%s: kube client failed", s.Name)
	s.configMapClient = s.ClientSet.CoreV1().ConfigMaps(s.Namespace)
}

func (s *parallelDefaultSuite) TearDownSuite() {
	common.LogBuilderLogs(context.Background(), s.Name, s.Namespace, s.ClientSet)
	logrus.Infof("%s: Removing builder", s.Name)
	err := common.RunBuildkit("rm", []string{
		s.Name,
	}, common.RunBuildStreams{})
	require.NoError(s.T(), err, "%s: builder rm failed", s.Name)
}

func (s *parallelDefaultSuite) TestParallelDefaultBuilds() {
	logrus.Infof("%s: Parallel %d Build", s.Name, ParallelDefaultBuildCount)

	dirs := make([]string, ParallelDefaultBuildCount)
	ctx := context.Background()

	// Create the contexts before threading
	for i := 0; i < ParallelDefaultBuildCount; i++ {
		dir, cleanup, err := common.NewSimpleBuildContext()
		dirs[i] = dir
		require.NoError(s.T(), err, "Failed to set up temporary build context")
		defer cleanup()
	}
	nodeNames, err := common.GetBuilderNodes(ctx, s.Name, s.Namespace, s.ClientSet)
	require.NoError(s.T(), err, "failed to get builder node names")
	g, ctx := errgroup.WithContext(ctx)

	for j := 0; j < ParallelDefaultBuildCount; j++ {
		i := j
		g.Go(func() error {
			imageName := fmt.Sprintf("dummy.acme.com/pbuild:%d", i)
			args := []string{
				"--progress=plain",
				"--tag", imageName,
				dirs[i],
			}
			err := common.RunBuild(args, common.RunBuildStreams{})
			if err != nil {
				return err
			}

			for _, nodeName := range nodeNames {
				err = common.RunSimpleBuildImageAsPod(
					ctx,
					fmt.Sprintf("%s-testbuiltimage-%d", s.Name, i),
					imageName,
					s.Namespace,
					nodeName,
					s.ClientSet,
				)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
	err = g.Wait()
	require.NoError(s.T(), err, "build/run failed")
}

func TestParallelDefaultBuildSuite(t *testing.T) {
	common.Skipper(t)
	// We don't parallelize with other tests, since we use the default builder name
	suite.Run(t, &parallelDefaultSuite{
		Name:        "buildkit",
		CreateFlags: []string{"--buildkitd-flags=--debug"},
	})
}
