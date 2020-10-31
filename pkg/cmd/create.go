// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package commands

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/google/shlex"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type createOptions struct {
	commonKubeOptions

	name       string
	driver     string
	platform   []string
	flags      string
	configFile string
	driverOpts []string
	progress   string
}

func runCreate(streams genericclioptions.IOStreams, in createOptions) error {
	ctx := appcontext.Context()

	if in.name == "default" {
		return errors.Errorf("default is a reserved name and cannot be used to identify builder instance")
	}

	var driverFactory driver.Factory
	var err error
	if in.driver == "" {
		driverFactory, err = driver.GetDefaultFactory(ctx, true)
		if err != nil {
			return err
		}
		if driverFactory == nil {
			return errors.Errorf("no valid drivers found")
		}
	} else {
		driverFactory = driver.GetFactory(in.driver, true)
		if driverFactory == nil {
			return errors.Errorf("failed to find driver %q", in.driver)
		}
	}

	var flags []string
	if in.flags != "" {
		flags, err = shlex.Split(in.flags)
		if err != nil {
			return errors.Wrap(err, "failed to parse buildkit flags")
		}
	}
	driverOptsMap, err := csvToMap(in.driverOpts)
	if err != nil {
		return err
	}
	d, err := driver.GetDriver(ctx, in.name, driverFactory, in.KubeClientConfig, flags, in.configFile, driverOptsMap, "" /*contextPathHash*/)
	if err != nil {
		return err
	}
	pw := progress.NewPrinter(ctx, os.Stderr, in.progress)
	_, _, err = driver.Boot(ctx, d, pw)
	if err != nil {
		return err
	}
	fmt.Printf("Created %s builder %s\n", driverFactory.Name(), in.name)
	return nil
}

func createCmd(streams genericclioptions.IOStreams) *cobra.Command {
	options := createOptions{
		commonKubeOptions: commonKubeOptions{
			configFlags: genericclioptions.NewConfigFlags(true),
			IOStreams:   streams,
		},
	}

	var drivers []string
	var driverUsage []string
	for s, f := range driver.GetFactories() {
		drivers = append(drivers, s)
		driverUsage = append(driverUsage, f.Usage())
	}

	cmd := &cobra.Command{
		Use:   "create [OPTIONS] [NAME]",
		Short: "Create a new builder instance",
		Long: `Create a new builder instance

Driver Specific Usage:
` + strings.Join(driverUsage, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				options.name = args[0]
			}
			if err := options.Complete(cmd, args); err != nil {
				return err
			}
			if err := options.Validate(); err != nil {
				return err
			}
			return runCreate(streams, options)
		},
		SilenceUsage: true,
	}

	flags := cmd.Flags()

	flags.StringVar(&options.driver, "driver", "", fmt.Sprintf("Driver to use (available: %v)", drivers))
	flags.StringVar(&options.flags, "buildkitd-flags", "", "Flags for buildkitd daemon")
	flags.StringVar(&options.configFile, "config", "", "BuildKit config file")
	flags.StringArrayVar(&options.platform, "platform", []string{}, "Fixed platforms for current node")
	flags.StringArrayVar(&options.driverOpts, "driver-opt", []string{}, "Options for the driver")
	flags.StringVar(&options.progress, "progress", "auto", "Set type of progress output (auto, plain, tty). Use plain to show container output")

	options.configFlags.AddFlags(flags)

	return cmd
}

func csvToMap(in []string) (map[string]string, error) {
	m := make(map[string]string, len(in))
	for _, s := range in {
		csvReader := csv.NewReader(strings.NewReader(s))
		fields, err := csvReader.Read()
		if err != nil {
			return nil, err
		}
		for _, v := range fields {
			p := strings.SplitN(v, "=", 2)
			if len(p) != 2 {
				return nil, errors.Errorf("invalid value %q, expecting k=v", v)
			}
			m[p[0]] = p[1]
		}
	}
	return m, nil
}
