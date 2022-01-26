// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package progress

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/moby/buildkit/client"
	"github.com/stretchr/testify/assert"
)

type writer struct {
	status func() chan *client.SolveStatus
}

func (w *writer) Done() <-chan struct{} {
	// unused
	return make(<-chan struct{})
}
func (w *writer) Err() error {
	return fmt.Errorf("unused")
}
func (w *writer) Status() chan *client.SolveStatus {
	return w.status()
}
func Test_FromReader(t *testing.T) {
	t.Parallel()
	ch := make(chan *client.SolveStatus, 1)
	w := &writer{
		status: func() chan *client.SolveStatus {
			return ch
		},
	}
	name := "foo"
	rc := bytes.NewBufferString("some data")
	go FromReader(w, name, rc)
	ss1 := <-ch
	assert.Len(t, ss1.Vertexes, 1)
	assert.Equal(t, ss1.Vertexes[0].Name, name)
	assert.NotNil(t, ss1.Vertexes[0].Started)
	ss2 := <-ch
	assert.Len(t, ss2.Vertexes, 1)
	assert.Equal(t, ss2.Vertexes[0].Name, name)
	assert.NotNil(t, ss2.Vertexes[0].Completed)

}

func Test_Write(t *testing.T) {
	t.Parallel()
	ch := make(chan *client.SolveStatus, 1)
	w := &writer{
		status: func() chan *client.SolveStatus {
			return ch
		},
	}
	name := "foo"
	go Write(w, name, func() error { return nil })
	ss1 := <-ch
	assert.Len(t, ss1.Vertexes, 1)
	assert.Equal(t, ss1.Vertexes[0].Name, name)
	assert.NotNil(t, ss1.Vertexes[0].Started)
	ss2 := <-ch
	assert.Len(t, ss2.Vertexes, 1)
	assert.Equal(t, ss2.Vertexes[0].Name, name)
	assert.NotNil(t, ss2.Vertexes[0].Completed)
}
