// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"fmt"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	Name        string
	CreateFlags []string
}

func (s *BaseSuite) SetupTest() {
	logrus.Infof("%s: Setting up builder", s.Name)
	args := append(
		[]string{
			s.Name,
		},
		s.CreateFlags...,
	)
	err := RunBuildkit("create", args)
	require.NoError(s.T(), err, "%s: builder create failed", s.Name)
}

func (s *BaseSuite) TearDownTest() {
	logrus.Infof("%s: Removing builder", s.Name)
	err := RunBuildkit("rm", []string{
		s.Name,
	})
	require.NoError(s.T(), err, "%s: builder rm failed", s.Name)
}

func (s *BaseSuite) TestSimpleBuild() {
	logrus.Infof("%s: Simple Build", s.Name)

	dir, cleanup, err := NewSimpleBuildContext()
	defer cleanup()
	require.NoError(s.T(), err, "Failed to set up temporary build context")
	args := []string{}
	if s.Name != "buildkit" { // TODO wire up the default name variable
		args = append(
			args,
			"--builder", s.Name,
		)
	}
	args = append(
		args,
		"--tag", s.Name+"replaceme:latest",
		dir,
	)
	err = RunBuild(args)
	if isRootlessCreate(s.CreateFlags) {
		require.Error(s.T(), err)
		require.Contains(s.T(), err.Error(), "please specify")
	} else {
		require.NoError(s.T(), err, "build failed")
	}
}

func isRootlessCreate(flags []string) bool {
	for _, flag := range flags {
		if strings.Contains(flag, "rootless") {
			return true
		}
	}
	return false
}

func (s *BaseSuite) TestLocalOutputTarBuild() {
	logrus.Infof("%s: Local Output Tar Build", s.Name)

	dir, cleanup, err := NewSimpleBuildContext()
	defer cleanup()
	require.NoError(s.T(), err, "Failed to set up temporary build context")
	args := []string{}
	if s.Name != "buildkit" { // TODO wire up the default name variable
		args = append(
			args,
			"--builder", s.Name,
		)
	}
	args = append(
		args,
		"--tag", s.Name+"replaceme:latest",
		fmt.Sprintf("--output=type=tar,dest=%s", path.Join(dir, "out.tar")),
		dir,
	)
	err = RunBuild(args)
	require.NoError(s.T(), err, "build failed")
	// TODO - consider inspecting the out.tar for validity...
}
