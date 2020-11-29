// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package suites

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/integration/common"
)

type rootlessSuite struct{ common.BaseSuite }

func TestRootlessSuite(t *testing.T) {
	common.Skipper(t)
	//t.Parallel() // TODO - tests fail if run in parallel, may be actual race bug
	suite.Run(t, &rootlessSuite{
		BaseSuite: common.BaseSuite{
			Name:        "rootless",
			CreateFlags: []string{"--rootless", "true"},
		},
	})
}

// func (s *rootlessSuite) TestSimpleBuild() {
// 	s.T().Skip("Rootless doesn't support loading to the runtime")
// }
