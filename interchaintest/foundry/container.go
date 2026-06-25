package foundry

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/strangelove-ventures/interchaintest/v8/dockerutil"
)

type Container struct {
	client      *client.Client
	containerID string
	networkID   string
}

// NewFoundryContainer creates and starts a container
func NewFoundryContainer(
	t *testing.T,
	ctx context.Context,
	dockerClient *client.Client,
	networkID string,
) (*Container, error) {
	deployerImageTag := "injectivelabs/injective-foundry-deployer:local"

	// Create container
	resp, err := dockerClient.ContainerCreate(
		ctx,
		&container.Config{
			Image: deployerImageTag,
			Cmd:   []string{"tail", "-f", "/dev/null"},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkID: {},
			},
		},
		nil,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployer container: %w", err)
	}

	// Start container
	if err := dockerutil.StartContainer(ctx, dockerClient, resp.ID); err != nil {
		return nil, fmt.Errorf("failed to start deployer container: %w", err)
	}

	t.Logf("Deployer container started: %s", resp.ID)

	return &Container{
		client:      dockerClient,
		containerID: resp.ID,
		networkID:   networkID,
	}, nil
}

// Exec runs a command in the container
func (dc *Container) Exec(ctx context.Context, cmd []string) (stdout, stderr string, err error) {
	// Create exec instance
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := dc.client.ContainerExecCreate(ctx, dc.containerID, execConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to create exec: %w", err)
	}

	// Attach to exec
	resp, err := dc.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer resp.Close()

	var (
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
	)

	// Read output
	if _, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader); err != nil {
		return "", "", fmt.Errorf("failed to read output: %w", err)
	}

	// Check exit code
	inspectResp, err := dc.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("failed to inspect exec: %w", err)
	}

	if inspectResp.ExitCode != 0 {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command exited with code %d", inspectResp.ExitCode)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// WriteFile writes content to a file inside the container
func (dc *Container) WriteFile(ctx context.Context, path string, content string) error {
	// Use bash -c with heredoc to write the file
	cmd := []string{
		"bash", "-c",
		fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", path, content),
	}

	_, _, err := dc.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// Cleanup stops and removes the deployer container
func (dc *Container) Cleanup(ctx context.Context) error {
	if err := dc.client.ContainerStop(ctx, dc.containerID, container.StopOptions{}); err != nil {
		return err
	}
	return dc.client.ContainerRemove(ctx, dc.containerID, container.RemoveOptions{})
}
