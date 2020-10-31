// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/version"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func runVersion(streams genericclioptions.IOStreams) error {
	fmt.Println(version.Version, version.Revision)
	return nil
}

func versionCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information ",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(streams)
		},
	}
	return cmd
}
