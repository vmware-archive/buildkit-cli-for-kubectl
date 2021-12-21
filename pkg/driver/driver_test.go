package driver

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/moby/buildkit/session"
	"github.com/stretchr/testify/assert"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/imagetools"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"
)

func TestFailFast(t *testing.T) {
	t.Parallel()
	assert.True(t, failFast(fmt.Errorf("Failed to pull image")))
	assert.True(t, failFast(fmt.Errorf("Error: ErrImagePull")))
	assert.False(t, failFast(fmt.Errorf("retry me")))
	assert.False(t, failFast(nil))
}

func TestStatus(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Inactive.String(), "inactive")
	assert.Equal(t, Starting.String(), "starting")
	assert.Equal(t, Running.String(), "running")
	assert.Equal(t, Stopping.String(), "stopping")
	assert.Equal(t, Stopped.String(), "stopped")
	assert.Equal(t, (Stopped + Starting).String(), "unknown")
}

func TestBoot(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Boot(ctx, &testDriver{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")

	_, err = Boot(context.Background(), &testDriver{
		infoErr: fmt.Errorf("Info not implemented"),
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Info not implemented")

	_, err = Boot(context.Background(), &testDriver{
		infoRes: &Info{
			Status: Inactive,
		},
		infoErr:      nil,
		bootstrapErr: fmt.Errorf("Failed to pull image"),
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to pull image")

}

type testDriver struct {
	infoRes      *Info
	infoErr      error
	bootstrapErr error
}

func (d *testDriver) Factory() Factory {
	return nil
}
func (d *testDriver) Bootstrap(context.Context, progress.Logger) error {
	return d.bootstrapErr
}
func (d *testDriver) Info(context.Context) (*Info, error) {
	return d.infoRes, d.infoErr
}
func (d *testDriver) Stop(ctx context.Context, force bool) error {
	return fmt.Errorf("not implemented")
}
func (d *testDriver) Rm(ctx context.Context, force bool) error {
	return fmt.Errorf("not implemented")
}
func (d *testDriver) Clients(ctx context.Context) (*BuilderClients, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *testDriver) Features() map[Feature]bool {
	return nil
}
func (d *testDriver) List(ctx context.Context) ([]Builder, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *testDriver) RuntimeSockProxy(ctx context.Context, name string) (net.Conn, error) {
	return nil, fmt.Errorf("not implemented")
}
func (d *testDriver) GetVersion(ctx context.Context) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (d *testDriver) GetAuthWrapper(string) imagetools.Auth {
	return nil
}
func (d *testDriver) GetAuthProvider(secretName string, stderr io.Writer) session.Attachable {
	return nil
}
func (d *testDriver) GetAuthHintMessage() string {
	return "not implemented"
}
