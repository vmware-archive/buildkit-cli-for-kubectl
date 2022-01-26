// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package driver

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/imagetools"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"
	pb "github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/proxy/proto"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/store"
)

// TODO - Will we want any other drivers, or is this driver abstraction overkill?
//        Perhaps a low-level containerd driver might make sense, but wiring that up
//        and accessing via kubectl CLI plugin seems strange.

var ErrNotRunning = errors.Errorf("driver not running")
var ErrNotConnecting = errors.Errorf("driver not connecting")

type Status int

const (
	Inactive Status = iota
	Starting
	Running
	Stopping
	Stopped
)

// RandSleep sleeps a random amount of time between zero and maxMS milliseconds
func RandSleep(maxMS int64) {
	sleepTime, err := rand.Int(rand.Reader, big.NewInt(maxMS))
	if err == nil {
		time.Sleep(time.Duration(sleepTime.Int64()) * time.Millisecond)
	} else {
		logrus.Debugf("failed to get random number: %s", err)
		time.Sleep(time.Duration(maxMS) * time.Millisecond)
	}
}

func (s Status) String() string {
	switch s {
	case Inactive:
		return "inactive"
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Stopped:
		return "stopped"
	}
	return "unknown"
}

type Info struct {
	Status Status
	// DynamicNodes must be empty if the actual nodes are statically listed in the store
	DynamicNodes []store.Node
}

type Driver interface {
	Factory() Factory
	Bootstrap(context.Context, progress.Logger) error
	Info(context.Context) (*Info, error)
	Stop(ctx context.Context, force bool) error
	Rm(ctx context.Context, force bool) error
	Clients(ctx context.Context) (*BuilderClients, error)
	Features() map[Feature]bool
	List(ctx context.Context) ([]Builder, error)
	RuntimeSockProxy(ctx context.Context, name string) (net.Conn, error)
	GetVersion(ctx context.Context) (string, error)

	// TODO - do we really need both?  Seems like some cleanup needed here...
	GetAuthWrapper(string) imagetools.Auth
	GetAuthProvider(secretName string, stderr io.Writer) session.Attachable
	GetAuthHintMessage() string
}

type Builder struct {
	Name   string
	Driver string
	Nodes  []Node

	// TODO consider adding these for a verbose listing
	//Flags      []string
	//ConfigFile string
	//DriverOpts map[string]string
}

type Node struct {
	Name      string
	Status    string
	Platforms []specs.Platform
}

type BuilderClients struct {
	// The builder on the chosen node/pod
	ChosenNode NodeClient

	// If multiple builders present, clients to the other builders (excluding the chosen pod)
	OtherNodes []NodeClient
}
type NodeClient struct {
	NodeName       string
	ClusterAddr    string
	BuildKitClient *client.Client
	ProxyClient    *pb.ProxyClient // nil if running in rootless mode
}

func Boot(ctx context.Context, d Driver, pw progress.Writer) (*BuilderClients, error) {
	err := fmt.Errorf("timeout before starting")
	var info *Info
	var results *BuilderClients
	for err != nil {
		select {
		case <-ctx.Done():
			return nil, errors.Wrap(err, "timed out trying to bootstrap builder")
		default:
		}

		info, err = d.Info(ctx)
		if err != nil {
			return nil, err
		}

		if info.Status != Running {
			if err = d.Bootstrap(ctx, func(s *client.SolveStatus) {
				if pw != nil {
					pw.Status() <- s
				}
			}); err != nil {
				if failFast(err) {
					return nil, err
				}
				// Possibly another CLI running in parallel - random sleep then retry
				RandSleep(100)
				continue
			}
		}
		results, err = d.Clients(ctx)
		if err != nil {
			if failFast(err) {
				return nil, err
			}
			RandSleep(1000)
		}
	}
	return results, nil
}

// Return true for failure cases that we don't want to retry for
func failFast(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "Failed to pull image") {
		return true
	} else if strings.Contains(msg, "Error: ErrImagePull") {
		return true
	}
	// TODO - over time add other fail-fast scenarios here to bypass our retry logic for transient errors
	return false
}
