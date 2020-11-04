// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/bkimage"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver/kubernetes"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/google/shlex"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	DefaultDriver = "kubernetes"
)

type createOptions struct {
	name                string
	image               string
	runtime             string
	containerdSock      string
	containerdNamespace string
	dockerSock          string
	replicas            int
	rootless            bool
	loadbalance         string
	worker              string
	driver              string
	platform            []string
	flags               string
	configFile          string
	progress            string
}

func runCreate(streams genericclioptions.IOStreams, in createOptions, rootOpts *rootOptions) error {
	ctx := appcontext.Context()

	if in.name == "default" {
		return errors.Errorf("default is a reserved name and cannot be used to identify builder instance")
	}

	driverFactory := driver.GetFactory(DefaultDriver, true)
	if driverFactory == nil {
		return errors.Errorf("failed to find driver %q", in.driver)
	}

	var flags []string
	var err error
	if in.flags != "" {
		flags, err = shlex.Split(in.flags)
		if err != nil {
			return errors.Wrap(err, "failed to parse buildkit flags")
		}
	}

	// TODO: consider swapping this out and passing the createOptions directly instead of
	//       using a hashmap
	driverOpts := map[string]string{
		"image":                in.image,
		"replicas":             strconv.Itoa(in.replicas),
		"rootless":             strconv.FormatBool(in.rootless),
		"loadbalance":          in.loadbalance,
		"worker":               in.worker,
		"containerd-namespace": in.containerdNamespace,
		"containerd-sock":      in.containerdSock,
		"docker-sock":          in.dockerSock,
		"runtime":              in.runtime,
	}

	d, err := driver.GetDriver(ctx, in.name, driverFactory, rootOpts.KubeClientConfig, flags, in.configFile, driverOpts, "" /*contextPathHash*/)
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

func createCmd(streams genericclioptions.IOStreams, rootOpts *rootOptions) *cobra.Command {
	options := createOptions{}

	var driverUsage []string
	for _, f := range driver.GetFactories() {
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
			if err := rootOpts.Complete(cmd, args); err != nil {
				return err
			}
			if err := rootOpts.Validate(); err != nil {
				return err
			}
			return runCreate(streams, options, rootOpts)
		},
		SilenceUsage: true,
	}

	flags := cmd.Flags()

	flags.StringVar(&options.flags, "buildkitd-flags", "", "Flags for buildkitd daemon")
	flags.StringVar(&options.configFile, "config", "", "BuildKit config file")
	flags.StringArrayVar(&options.platform, "platform", []string{}, "Fixed platforms for current node")
	flags.StringVar(&options.progress, "progress", "auto", "Set type of progress output [auto, plain, tty]. Use plain to show container output")
	flags.StringVar(&options.image, "image", bkimage.DefaultImage, "Specify an alternate buildkit image")
	flags.StringVar(&options.runtime, "runtime", "auto", "Container runtime used by cluster [auto, docker, containerd]")
	flags.StringVar(&options.containerdSock, "containerd-sock", kubernetes.DefaultContainerdSockPath, "Path to the containerd.sock on the host")
	flags.StringVar(&options.containerdNamespace, "containerd-namespace", kubernetes.DefaultContainerdNamespace, "Containerd namespace to build images in")
	flags.StringVar(&options.dockerSock, "docker-sock", kubernetes.DefaultDockerSockPath, "Path to the docker.sock on the host")
	flags.IntVar(&options.replicas, "replicas", 1, "BuildKit deployment replica count")
	flags.BoolVar(&options.rootless, "rootless", false, "Run in rootless mode")
	flags.StringVar(&options.loadbalance, "loadbalance", "random", "Load balancing strategy [random, sticky]")
	flags.StringVar(&options.worker, "worker", "auto", "Worker backend [auto, runc, containerd]")

	return cmd
}
