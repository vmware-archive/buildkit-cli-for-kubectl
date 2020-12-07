// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package suites

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/integration/common"
)

type RandomLBSuite struct{ common.BaseSuite }

// TODO - add some conditional test cases to varify random scheduling is actually working

func TestRandomLBSuite(t *testing.T) {
	common.Skipper(t)
	t.Parallel()
	suite.Run(t, &RandomLBSuite{
		BaseSuite: common.BaseSuite{
			Name:        "randomlb",
			CreateFlags: []string{"--loadbalance", "random"},
		},
	})
}
