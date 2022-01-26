// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	commands "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/cmd"

	// Import the kubernetes driver so we can exercise its code paths
	_ "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes"
)

// RunBuildStreams can override in/out/err streams if the output needs to be evaluated
// if unset, stdin/stdout/stderr will be used
type RunBuildStreams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func RunBuild(args []string, streams RunBuildStreams) error {
	flags := pflag.NewFlagSet("kubectl-build", pflag.ExitOnError)
	pflag.CommandLine = flags
	if streams.In == nil {
		streams.In = os.Stdin
	}
	if streams.Out == nil {
		streams.Out = os.Stdout
	}
	if streams.Err == nil {
		streams.Err = os.Stderr
	}

	// TODO do we want to capture the output someplace else?
	root := commands.NewRootBuildCmd(genericclioptions.IOStreams{In: streams.In, Out: streams.Out, ErrOut: streams.Err})
	root.SetArgs(args)
	logrus.Infof("Build: %v", args)

	// BUG! - there's some flakiness in the layers beneath us - we'll retry up to 3 times for lease failures inside buildkit
	// example: "Error: failed to solve: failed to compute cache key: lease does not exist: not found"
	limit := 3
	var err error
	for i := 0; i < limit; i++ {
		err = root.Execute()
		if err != nil && strings.Contains(err.Error(), "failed to compute cache key") {
			logrus.Warnf("Hit flaky error in buildkit: %d of %d - %s", i+1, limit, err)
			continue
		}
		break
	}
	return err
}

func RunBuildkit(command string, args []string, streams RunBuildStreams) error {
	flags := pflag.NewFlagSet("kubectl-buildkit", pflag.ExitOnError)
	pflag.CommandLine = flags
	finalArgs := append(
		[]string{command},
		args...,
	)

	if altBuildKitImage := os.Getenv("TEST_ALT_BUILDKIT_IMAGE"); altBuildKitImage != "" {
		isCreate := false
		hasRootless := false
		hasImage := false
		for _, arg := range finalArgs {
			if strings.Contains(arg, "--rootless") {
				hasRootless = true
			} else if strings.Contains(arg, "--image") {
				hasImage = true
			} else if arg == "create" {
				isCreate = true
			}
		}
		if isCreate && !hasImage {
			if hasRootless {
				finalArgs = append(finalArgs, "--image", altBuildKitImage+"-rootless")
			} else {
				finalArgs = append(finalArgs, "--image", altBuildKitImage)
			}
		}
	}
	logrus.Infof("CMD: %v", finalArgs)
	if streams.In == nil {
		streams.In = os.Stdin
	}
	if streams.Out == nil {
		streams.Out = os.Stdout
	}
	if streams.Err == nil {
		streams.Err = os.Stderr
	}

	root := commands.NewRootCmd(genericclioptions.IOStreams{In: streams.In, Out: streams.Out, ErrOut: streams.Err})
	root.SetArgs(finalArgs)

	return root.Execute()
}
