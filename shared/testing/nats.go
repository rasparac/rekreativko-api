package testing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartNATS runs a NATS server (JetStream on, no authorization) in a container
// for the test and returns its URL. The container is removed when the test ends.
func StartNATS(t *testing.T) string {
	t.Helper()

	return startNATS(t, testcontainers.ContainerRequest{Cmd: []string{"-js"}})
}

// StartNATSWithConfig runs a NATS server with the given config file (e.g.
// nats/nats-server.conf, with its users and permissions) and environment (the
// passwords the file reads) and returns its URL.
func StartNATSWithConfig(t *testing.T, configPath string, env map[string]string) string {
	t.Helper()

	return startNATS(t, testcontainers.ContainerRequest{
		Cmd: []string{"-c", "/etc/nats/nats-server.conf"},
		Env: env,
		Files: []testcontainers.ContainerFile{
			{HostFilePath: configPath, ContainerFilePath: "/etc/nats/nats-server.conf", FileMode: 0o644},
		},
	})
}

func startNATS(t *testing.T, req testcontainers.ContainerRequest) string {
	t.Helper()

	req.Image = "nats:2-alpine"
	req.ExposedPorts = []string{"4222/tcp"}
	req.WaitingFor = wait.ForLog("Server is ready").WithStartupTimeout(60 * time.Second)

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "4222/tcp")
	require.NoError(t, err)

	return "nats://" + host + ":" + port.Port()
}
