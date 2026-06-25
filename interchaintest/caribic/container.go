package caribic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/strangelove-ventures/interchaintest/v8/dockerutil"
)

const (
	ImageTag = "injectivelabs/injective-caribic:latest"
	RepoDir  = "/opt/cardano-ibc-incubator"
)

type Container struct {
	client      *client.Client
	containerID string
}

func NewContainer(
	t *testing.T,
	ctx context.Context,
	dockerClient *client.Client,
	injectiveRepoRoot string,
	workDir string,
) (*Container, error) {
	t.Helper()

	resp, err := dockerClient.ContainerCreate(
		ctx,
		&container.Config{
			Image: ImageTag,
			Cmd:   []string{"tail", "-f", "/dev/null"},
			Env: []string{
				"DOCKER_HOST=unix:///var/run/docker.sock",
				"INJECTIVE_CORE_REPO=" + injectiveRepoRoot,
				"CARIBIC_WORKDIR=" + workDir,
				"HOME=" + filepath.Join(workDir, "home"),
			},
		},
		&container.HostConfig{
			NetworkMode: "host",
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: "/var/run/docker.sock",
					Target: "/var/run/docker.sock",
				},
				{
					Type:     mount.TypeBind,
					Source:   injectiveRepoRoot,
					Target:   injectiveRepoRoot,
					ReadOnly: true,
				},
				{
					Type:   mount.TypeBind,
					Source: workDir,
					Target: workDir,
				},
			},
		},
		nil,
		nil,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create caribic container: %w", err)
	}

	if err := dockerutil.StartContainer(ctx, dockerClient, resp.ID); err != nil {
		return nil, fmt.Errorf("failed to start caribic container: %w", err)
	}

	t.Logf("Caribic container started: %s", resp.ID)

	return &Container{
		client:      dockerClient,
		containerID: resp.ID,
	}, nil
}

func (c *Container) Exec(ctx context.Context, cmd []string) (stdout, stderr string, err error) {
	return c.ExecWithWriters(ctx, cmd, nil, nil)
}

func (c *Container) ExecWithWriters(
	ctx context.Context,
	cmd []string,
	stdoutWriter io.Writer,
	stderrWriter io.Writer,
) (stdout, stderr string, err error) {
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.client.ContainerExecCreate(ctx, c.containerID, execConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to create exec: %w", err)
	}

	resp, err := c.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer resp.Close()

	var (
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
	)

	stdoutTarget := io.Writer(&stdoutBuf)
	if stdoutWriter != nil {
		stdoutTarget = io.MultiWriter(&stdoutBuf, stdoutWriter)
	}
	stderrTarget := io.Writer(&stderrBuf)
	if stderrWriter != nil {
		stderrTarget = io.MultiWriter(&stderrBuf, stderrWriter)
	}

	if _, err = stdcopy.StdCopy(stdoutTarget, stderrTarget, resp.Reader); err != nil {
		return "", "", fmt.Errorf("failed to read output: %w", err)
	}

	inspectResp, err := c.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("failed to inspect exec: %w", err)
	}

	if inspectResp.ExitCode != 0 {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command exited with code %d", inspectResp.ExitCode)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

func (c *Container) Cleanup(ctx context.Context) error {
	if err := c.client.ContainerStop(ctx, c.containerID, container.StopOptions{}); err != nil {
		return err
	}
	return c.client.ContainerRemove(ctx, c.containerID, container.RemoveOptions{})
}
