// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package commands

import (
	"github.com/moby/buildkit/util/appcontext"
	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type rmOptions struct {
	commonKubeOptions

	builders []string
}

func runRm(streams genericclioptions.IOStreams, in rmOptions) error {
	ctx := appcontext.Context()

	for name, factory := range driver.GetFactories() {
		d, err := driver.GetDriver(ctx, name, factory, in.KubeClientConfig, nil, "", nil, "")
		if err != nil {
			return err
		}
		b, err := d.List(ctx)
		if err != nil {
			return err
		}
		for _, builder := range b {
			for _, deleteMe := range in.builders {
				if builder.Name == deleteMe {
					// TODO this is a bit wonky can could use some refactoring...
					d, err := driver.GetDriver(ctx, deleteMe, factory, in.KubeClientConfig, nil, "", nil, "")
					if err != nil {
						return err
					}
					err = d.Rm(ctx, false)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func rmCmd(streams genericclioptions.IOStreams) *cobra.Command {
	options := rmOptions{
		commonKubeOptions: commonKubeOptions{
			configFlags: genericclioptions.NewConfigFlags(true),
			IOStreams:   streams,
		},
	}

	cmd := &cobra.Command{
		Use:   "rm [NAMES...]",
		Short: "Remove one or more builder instances",
		//Args:  cli.RequiresMaxArgs(1), // TODO support rm for 1 or more builders
		RunE: func(cmd *cobra.Command, args []string) error {
			options.builders = args
			if err := options.Complete(cmd, args); err != nil {
				return err
			}
			if err := options.Validate(); err != nil {
				return err
			}

			return runRm(streams, options)
		},
		SilenceUsage: true,
	}
	options.configFlags.AddFlags(cmd.Flags())

	return cmd
}
