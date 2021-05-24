// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

var (
	// TmpDirPattern sets the pattern for temporary files
	TmpDirPattern = "build_cli_tests"
)

// NewBuildContext creates a temporary directory with a set of files
// populated.  The input payload keys represent the filenames, and values
// are the contents of the files.
// Returns the directory path, a cleanup function, and error
// Callers should always call the cleanup, even in the case of an error
func NewBuildContext(payloads map[string]string) (string, func(), error) {
	dir, err := ioutil.TempDir("", TmpDirPattern)
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		os.RemoveAll(dir)
	}
	for filename, contents := range payloads {
		err = ioutil.WriteFile(filepath.Join(dir, filename), []byte(contents), 0644)
		if err != nil {
			return "", cleanup, err
		}
	}
	return dir, cleanup, nil
}

// NewSimpleBuildContext creates a very simple Dockerfile for exercising the builder
func NewSimpleBuildContext() (string, func(), error) {
	proxyImage := os.Getenv("TEST_IMAGE_BASE")
	if proxyImage == "" {
		return "", nil, fmt.Errorf("TEST_IMAGE_BASE env var unset")
	}
	return NewBuildContext(map[string]string{
		"Dockerfile": fmt.Sprintf(`FROM %s
RUN echo "#!/bin/sh" > /run
RUN echo "echo hello world" >> /run && chmod a+x /run
CMD /run
`, proxyImage)})
}
