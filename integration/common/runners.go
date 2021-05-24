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
	finalArgs := append(
		[]string{"--kubeconfig", os.Getenv("TEST_KUBECONFIG")},
		args...,
	)
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
	root.SetArgs(finalArgs)
	logrus.Infof("Build: %v", finalArgs)

	return root.Execute()
}

func RunBuildkit(command string, args []string, streams RunBuildStreams) error {
	flags := pflag.NewFlagSet("kubectl-buildkit", pflag.ExitOnError)
	pflag.CommandLine = flags
	finalArgs := append(
		[]string{command, "--kubeconfig", os.Getenv("TEST_KUBECONFIG")},
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
