package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/proxy"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/version"
)

func main() {
	if err := doMain(context.Background()); err != nil {
		os.Exit(1)
	}
}

// TODO - explore wiring up a unit test wrapper so we can gather proxy code coverage in integration tests

func doMain(ctx context.Context) error {
	pflag.CommandLine = pflag.NewFlagSet("buildkit-proxy", pflag.ExitOnError)
	cfg := &proxy.ServerConfig{}
	var debug bool

	root := &cobra.Command{
		Use:   "buildkit-proxy CMD [OPTIONS]",
		Short: "Run the BuildKit proxy gRPC service",
		//SilenceUsage: true,
	}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "run the gRPC server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if debug {
				logrus.SetLevel(logrus.DebugLevel)
			}
			srv, err := proxy.NewProxy(ctx, *cfg)
			if err != nil {
				return err
			}
			return srv.Serve(ctx)
		},
		SilenceUsage: true,
	}
	flags := cmd.Flags()
	flags.StringVar(&cfg.BuildkitdSocketPath, "buildkitd", "/run/buildkit/buildkitd.sock", "Specify the buildkitd socket path")
	flags.StringVar(&cfg.ContainerdSocketPath, "containerd", "", "Connect to local containerd with the specified socket path")
	flags.StringVar(&cfg.DockerdSocketPath, "dockerd", "", "Connect to local dockerd with the specified socket path")
	flags.StringVar(&cfg.HelperSocketPath, "listen", "/run/buildkit/buildkit-proxy.sock", "Socket path for this proxy to listen on")
	flags.BoolVar(&debug, "debug", false, "enable debug level logging")

	root.AddCommand(cmd,
		&cobra.Command{
			Use:   "version",
			Short: "Show version information ",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("%s\n", version.GetProxyImage())
				return nil
			},
		})
	return root.Execute()
}
