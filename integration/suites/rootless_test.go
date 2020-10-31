// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package suites

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/integration/common"
)

type RootlessSuite struct{ common.BaseSuite }

func TestRootlessSuite(t *testing.T) {
	t.Skip("Skipping rootless due to bug! - should disable local storage mode automatically")
	common.Skipper(t)
	//t.Parallel() // TODO - tests fail if run in parallel, may be actual race bug
	suite.Run(t, &RootlessSuite{
		BaseSuite: common.BaseSuite{
			Name:        "rootless",
			CreateFlags: []string{"--driver-opt", "rootless=true"},
		},
	})
}
