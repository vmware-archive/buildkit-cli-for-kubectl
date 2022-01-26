// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type BaseSuite struct {
	suite.Suite
	Name            string
	CreateFlags     []string
	SkipSetupCreate bool

	ClientSet *kubernetes.Clientset
	Namespace string
}

func (s *BaseSuite) SetupSuite() {
	var err error
	s.ClientSet, s.Namespace, err = GetKubeClientset()
	require.NoError(s.T(), err, "%s: kube client failed", s.Name)

	if !s.SkipSetupCreate {
		logrus.Infof("%s: Setting up builder", s.Name)
		nodes, err := GetNodes(context.Background(), s.ClientSet)
		require.NoError(s.T(), err, "%s: get nodes failed", s.Name)

		args := append(
			[]string{
				s.Name,
			},
			s.CreateFlags...,
		)
		if len(nodes) > 1 {
			hasReplicas := false
			for _, arg := range args {
				if strings.Contains(arg, "--replica") {
					hasReplicas = true
				}
			}
			if !hasReplicas {
				args = append(args, fmt.Sprintf("--replicas=%d", len(nodes)))
			}
		}
		err = RunBuildkit("create", args, RunBuildStreams{})
		require.NoError(s.T(), err, "%s: builder create failed", s.Name)
	}
}

func (s *BaseSuite) TearDownSuite() {
	LogBuilderLogs(context.Background(), s.Name, s.Namespace, s.ClientSet)
	logrus.Infof("%s: Removing builder", s.Name)
	err := RunBuildkit("rm", []string{
		s.Name,
	}, RunBuildStreams{})
	require.NoError(s.T(), err, "%s: builder rm failed", s.Name)
	configMapClient := s.ClientSet.CoreV1().ConfigMaps(s.Namespace)
	_, err = configMapClient.Get(context.Background(), s.Name, metav1.GetOptions{})
	require.Error(s.T(), err, "config map wasn't cleaned up")
	require.Contains(s.T(), err.Error(), "not found")
}

func (s *BaseSuite) TestSimpleBuild() {
	logrus.Infof("%s: Simple Build", s.Name)
	ctx := context.Background()

	dir, cleanup, err := NewSimpleBuildContext()
	defer cleanup()
	require.NoError(s.T(), err, "Failed to set up temporary build context")
	args := []string{"--progress=plain"}
	if s.Name != "buildkit" { // TODO wire up the default name variable
		args = append(
			args,
			"--builder", s.Name,
		)
	}
	imageName := "dummy.acme.com/" + s.Name + "replaceme:latest"
	args = append(
		args,
		"--tag", imageName,
		dir,
	)
	err = RunBuild(args, RunBuildStreams{})
	if isRootlessCreate(s.CreateFlags) {
		require.Error(s.T(), err)
		require.Contains(s.T(), err.Error(), "please specify")
	} else {
		require.NoError(s.T(), err, "build failed")
	}
	nodeNames, err := GetBuilderNodes(ctx, s.Name, s.Namespace, s.ClientSet)
	require.NoError(s.T(), err, "failed to get builder node names")
	for _, nodeName := range nodeNames {
		err = RunSimpleBuildImageAsPod(ctx, s.Name+"-testbuiltimage", imageName, s.Namespace, nodeName, s.ClientSet)
		require.NoError(s.T(), err, "failed to start pod with image")
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
	args := []string{"--progress=plain"}
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
	err = RunBuild(args, RunBuildStreams{})
	require.NoError(s.T(), err, "build failed")
	// TODO - consider inspecting the out.tar for validity...
}

func (s *BaseSuite) TestLs() {
	buf := &bytes.Buffer{}
	err := RunBuildkit("ls", []string{}, RunBuildStreams{Out: buf})
	require.NoError(s.T(), err, "%s: ls failed", s.Name)
	lines := buf.String()
	require.Contains(s.T(), lines, s.Name)

	err = RunBuildkit("ls", []string{"dummy"}, RunBuildStreams{})
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "requires exactly")
}

func (s *BaseSuite) TestBuildWithSecret() {
	logrus.Infof("%s: Build with Secret", s.Name)
	proxyImage := GetTestImageBase()

	dir, cleanup, err := NewBuildContext(map[string]string{
		"Dockerfile": fmt.Sprintf(`# syntax=docker/dockerfile:experimental
FROM %s
RUN --mount=type=secret,id=mysecret cat /run/secrets/mysecret && [ "$(cat /run/secrets/mysecret)" = "supersecret" ]
`, proxyImage),
		"mysecret.txt": "supersecret",
	})

	defer cleanup()
	require.NoError(s.T(), err, "Failed to set up temporary build context")
	args := []string{"--progress=plain"}
	if s.Name != "buildkit" { // TODO wire up the default name variable
		args = append(
			args,
			"--builder", s.Name,
		)
	}
	if isRootlessCreate(s.CreateFlags) {
		args = append(args,
			fmt.Sprintf("--output=type=tar,dest=%s", path.Join(dir, "out.tar")),
		)
	}
	imageName := "dummy.acme.com/" + s.Name + "sectest:latest"
	args = append(
		args,
		"--secret", fmt.Sprintf("id=mysecret,src=%s", path.Join(dir, "mysecret.txt")),
		"--tag", imageName,
		dir,
	)

	err = RunBuild(args, RunBuildStreams{})

	// TODO - there's a bug here somewhere, but it's a flaky scenario... needs more investigation
	if err != nil && strings.Contains(err.Error(), "lease does not exist") {
		logrus.Warningf("IGNORING FLAKY TEST ERROR: %s", err)
		return
	}

	require.NoError(s.T(), err, "build failed")
}

func (s *BaseSuite) TestBuildWithSSHKey() {
	// TODO - figure out root cause of this flaky error
	// rpc error: code = Unknown desc = failed to solve with frontend dockerfile.v0: failed to solve with frontend gateway.v0: rpc error: code = Unknown desc = unable to lease content: lease does not exist: not found
	s.T().Skip("Skipping SSH test as it's too flaky")

	logrus.Infof("%s: Build with SSH key", s.Name)
	proxyImage := GetTestImageBase()

	// Note: if the key is not valid, the socket will not be created, so simply checking for
	// its existence is sufficient to verify the ssh plumbing is working.
	dir, cleanup, err := NewBuildContext(map[string]string{
		"Dockerfile": fmt.Sprintf(`# syntax=docker/dockerfile:experimental
FROM %s
RUN --mount=type=ssh,id=myssh echo "SSH_AUTH_SOCK=${SSH_AUTH_SOCK}" && ls -l ${SSH_AUTH_SOCK} && [ -S ${SSH_AUTH_SOCK} ]
`, proxyImage),
	})
	require.NoError(s.T(), err, "Failed to create temporary build context")
	defer cleanup()

	fullKeyPath := path.Join(dir, "id_rsa")

	// Note: we could check in a static dummy private key to speed this up
	// but we'd undoubtedly get nagged by scanners thinking we've
	// checked in a real private key into the repo, so we just generate
	// it every time.
	cmd := exec.Command("ssh-keygen", "-f", fullKeyPath, "-t", "rsa", "-N", "", "-C", "dummy_test_key")
	err = cmd.Run()
	if err != nil {
		s.T().Skipf("skipping SSH test as ssh-keygen failed: %s", err)
	}

	require.NoError(s.T(), err, "Failed to set up temporary build context")
	args := []string{"--progress=plain"}
	if s.Name != "buildkit" { // TODO wire up the default name variable
		args = append(
			args,
			"--builder", s.Name,
		)
	}
	if isRootlessCreate(s.CreateFlags) {
		args = append(args,
			fmt.Sprintf("--output=type=tar,dest=%s", path.Join(dir, "out.tar")),
		)
	}
	imageName := "dummy.acme.com/" + s.Name + "sshtest:latest"
	args = append(
		args,
		"--ssh", "myssh="+path.Join(dir, "id_rsa"),
		"--tag", imageName,
		dir,
	)

	err = RunBuild(args, RunBuildStreams{})

	// TODO - there's a bug here somewhere, but it's a flaky scenario... needs more investigation
	if err != nil && strings.Contains(err.Error(), "lease does not exist") {
		logrus.Warningf("IGNORING FLAKY TEST ERROR: %s", err)
		return
	}

	require.NoError(s.T(), err, "build failed")
}

func GetTestImageBase() string {
	base := os.Getenv("TEST_IMAGE_BASE")
	if base == "" {
		base = "busybox"
	}
	return base
}
