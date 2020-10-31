// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"

	"github.com/spf13/pflag"
	commands "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/cmd"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	_ "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-build", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := commands.NewRootBuildCmd(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
