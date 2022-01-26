// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/stretchr/testify/assert"
)

func Test_createTempDockerfile(t *testing.T) {
	t.Parallel()
	dfContent := "FROM scratch"
	dockerfile := bytes.NewBufferString(dfContent)
	d, err := createTempDockerfile(dockerfile)
	assert.NoError(t, err)
	data, err := ioutil.ReadFile(filepath.Join(d, "Dockerfile"))
	assert.NoError(t, err)
	assert.Equal(t, dfContent, string(data))
	os.RemoveAll(d)
}

func Test_LoadInputs(t *testing.T) {
	inp := Inputs{}
	target := &client.SolveOpt{
		LocalDirs:      map[string]string{},
		FrontendAttrs:  map[string]string{},
		FrontendInputs: map[string]llb.State{},
	}
	_, err := LoadInputs(inp, target)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "please specify build context")

	inp.ContextPath = "-"
	inp.DockerfilePath = "-"
	_, err = LoadInputs(inp, target)
	assert.Error(t, err)
	assert.Equal(t, err, errStdinConflict)

	inp.InStream = bytes.NewBuffer([]byte{0x1F, 0x8B, 0x08})
	inp.DockerfilePath = "dummy"
	f, err := LoadInputs(inp, target)
	assert.NoError(t, err)
	assert.Len(t, target.Session, 1)
	f()

	inp.InStream = bytes.NewBufferString("FROM scratch")
	inp.DockerfilePath = "dummy"
	_, err = LoadInputs(inp, target)
	assert.Error(t, err)
	assert.Equal(t, err, errDockerfileConflict)

	inp.DockerfilePath = ""
	f, err = LoadInputs(inp, target)
	assert.NoError(t, err)
	f()

	inp.ContextPath = "git@github.com:vmware-tanzu/buildkit-cli-for-kubectl.git"
	inp.DockerfilePath = "-"
	_, err = LoadInputs(inp, target)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Dockerfile from stdin is not supported with remote contexts")

	inp.DockerfilePath = ""
	f, err = LoadInputs(inp, target)
	assert.NoError(t, err)
	f()

	inp.ContextPath = "./"
	inp.DockerfilePath = "-"
	f, err = LoadInputs(inp, target)
	assert.NoError(t, err)
	f()

	inp.DockerfilePath = "some/path/to/Dockerfile"
	f, err = LoadInputs(inp, target)
	assert.NoError(t, err)
	f()

	inp.ContextPath = "bogus/path/does/not/exist"
	_, err = LoadInputs(inp, target)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to prepare context")

}
