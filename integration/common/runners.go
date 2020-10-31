// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	commands "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/cmd"

	// Import the kubernetes driver so we can exercise its code paths
	_ "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes"
)

func RunBuild(args []string) error {
	flags := pflag.NewFlagSet("kubectl-build", pflag.ExitOnError)
	pflag.CommandLine = flags
	finalArgs := append(
		[]string{"--kubeconfig", os.Getenv("TEST_KUBECONFIG")},
		args...,
	)

	// TODO do we want to capture the output someplace else?
	root := commands.NewRootBuildCmd(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	root.SetArgs(finalArgs)
	logrus.Infof("Build: %v", finalArgs)

	return root.Execute()
}

func RunBuildkit(command string, args []string) error {
	flags := pflag.NewFlagSet("kubectl-buildkit", pflag.ExitOnError)
	pflag.CommandLine = flags
	finalArgs := append(
		[]string{command, "--kubeconfig", os.Getenv("TEST_KUBECONFIG")},
		args...,
	)
	logrus.Infof("CMD: %v", finalArgs)

	root := commands.NewRootCmd(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	root.SetArgs(finalArgs)

	return root.Execute()
}
