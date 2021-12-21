package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func Test_runBuild(t *testing.T) {
	t.Parallel()

	err := runBuild(genericclioptions.IOStreams{}, buildOptions{
		squash: true,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "squash currently")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		quiet: true,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "quiet currently")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		platforms: []string{"acme"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown operating system")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		secrets: []string{"foo=baddata"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected key")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		ssh: []string{"someid=bogus-file-does-not-exist"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		outputs: []string{"type=local"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dest is required for local output")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		commonOptions: commonOptions{
			exportPush: true,
			exportLoad: true,
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "push and load")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		outputs: []string{"type=local,dest=out"},
		commonOptions: commonOptions{
			exportPush: true,
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "push and ")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		outputs: []string{"type=local,dest=out"},
		commonOptions: commonOptions{
			exportLoad: true,
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load and ")

	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		cacheFrom: []string{"type="},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type required")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		cacheTo: []string{"type="},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type required")
	err = runBuild(genericclioptions.IOStreams{}, buildOptions{
		allow: []string{"garbage"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entitlement")
}

func Test_listToMap(t *testing.T) {
	res := listToMap([]string{"HOME=bogus"}, false)
	assert.Len(t, res, 1)
	assert.Contains(t, res, "HOME")
	assert.Equal(t, res["HOME"], "bogus")
	res = listToMap([]string{"HOME"}, true)
	assert.Len(t, res, 1)
	assert.Contains(t, res, "HOME")
}

func Test_Complete(t *testing.T) {
	cmd := buildCmd(genericclioptions.IOStreams{}, &rootOptions{})
	err := cmd.ParseFlags([]string{
		"--namespace=somenamespace",
		"--context=foo",
		"--cluster=bar",
		"--kubeconfig=baz",
	})
	assert.NoError(t, err)

	opts := commonKubeOptions{
		configFlags: &genericclioptions.ConfigFlags{},
	}
	err = opts.Complete(cmd, []string{})
	// If no valid contetx is set up, we'll allow this unit-test to pass
	if err != nil {
		if strings.Contains(err.Error(), "no context is currently set") {
			return
		}
	}
	assert.NoError(t, err)
}
