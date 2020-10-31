// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package driver

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"k8s.io/client-go/tools/clientcmd"
)

type Factory interface {
	Name() string
	Usage() string
	Priority(context.Context) int
	New(ctx context.Context, cfg InitConfig) (Driver, error)
	AllowsInstances() bool
}

type BuildkitConfig struct {
	// Entitlements []string
	// Rootless bool
}

type InitConfig struct {
	// This object needs updates to be generic for different drivers
	Name string
	//DockerAPI        dockerclient.APIClient
	KubeClientConfig clientcmd.ClientConfig
	BuildkitFlags    []string
	ConfigFile       string
	DriverOpts       map[string]string
	// ContextPathHash can be used for determining pods in the driver instance
	ContextPathHash string
}

var drivers map[string]Factory

func Register(f Factory) {
	if drivers == nil {
		drivers = map[string]Factory{}
	}
	drivers[f.Name()] = f
}

func GetDefaultFactory(ctx context.Context, instanceRequired bool) (Factory, error) {
	if len(drivers) == 0 {
		return nil, errors.Errorf("no drivers available")
	}
	type p struct {
		f        Factory
		priority int
	}
	dd := make([]p, 0, len(drivers))
	for _, f := range drivers {
		if instanceRequired && !f.AllowsInstances() {
			continue
		}
		dd = append(dd, p{f: f, priority: f.Priority(ctx)})
	}
	sort.Slice(dd, func(i, j int) bool {
		return dd[i].priority < dd[j].priority
	})
	return dd[0].f, nil
}

func GetFactory(name string, instanceRequired bool) Factory {
	validNames := make([]string, len(drivers))
	i := int(0)
	for _, f := range drivers {
		i++
		if instanceRequired && !f.AllowsInstances() {
			continue
		}
		if f.Name() == name {
			return f
		}
		validNames[i] = f.Name()
	}
	return nil
}

func GetDriver(ctx context.Context, name string, f Factory, kubeClientConfig clientcmd.ClientConfig /*kcc clientcmd.ClientConfig,*/, flags []string, config string, do map[string]string, contextPathHash string) (Driver, error) {
	ic := InitConfig{
		KubeClientConfig: kubeClientConfig,
		Name:             name,
		BuildkitFlags:    flags,
		ConfigFile:       config,
		DriverOpts:       do,
		ContextPathHash:  contextPathHash,
	}
	if f == nil {
		var err error
		f, err = GetDefaultFactory(ctx, false)
		if err != nil {
			return nil, err
		}
	}
	return f.New(ctx, ic)
}

func GetFactories() map[string]Factory {
	return drivers
}
