// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pkg/errors"
)

var errNoContext = fmt.Errorf("no context is currently set, use %q to select a new one", "kubectl config use-context <context>")

func ExactArgs(count int) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == count {
			return nil
		}
		return errors.Errorf(
			"%q requires exactly %d %s.\nSee '%s --help'.\n\nUsage:  %s\n\n%s",
			cmd.CommandPath(),
			count,
			"argument(s)",
			cmd.CommandPath(),
			cmd.UseLine(),
			cmd.Short,
		)
	}
}
