package main

import (
	"context"
	"fmt"
	"strings"
)

// dockerControl implements ControlPlane using the Docker CLI.
// It requires configured container names and expects the Docker socket to be
// accessible by the executor (locally or via SSH to a Docker host).
type dockerControl struct {
	gameserver  string // container name for the game server
	brokerGame  string // container name for mq-game broker
	brokerAdmin string // container name for mq-admin broker
}

func (c *dockerControl) Name() string { return "docker" }

func (c *dockerControl) GetStatus(_ context.Context, exec Executor) (*BattlegroupStatus, error) {
	if c.gameserver == "" {
		return nil, errNotSupported("docker", "GetStatus (docker_gameserver not configured)")
	}
	out, err := exec.Exec(fmt.Sprintf(
		"docker inspect --format '{{.State.Status}}' %s 2>&1", c.gameserver))
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}
	status := strings.TrimSpace(out)
	return &BattlegroupStatus{
		Name:    c.gameserver,
		Title:   c.gameserver,
		Phase:   status,
		Servers: []ServerRow{},
	}, nil
}

func (c *dockerControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	if c.gameserver == "" {
		return "", errNotSupported("docker", "ExecCommand (docker_gameserver not configured)")
	}
	var dockerCmd string
	switch cmd {
	case "start":
		dockerCmd = fmt.Sprintf("docker start %s 2>&1", c.gameserver)
	case "stop":
		dockerCmd = fmt.Sprintf("docker stop %s 2>&1", c.gameserver)
	case "restart":
		dockerCmd = fmt.Sprintf("docker restart %s 2>&1", c.gameserver)
	default:
		return "", fmt.Errorf("docker control does not support %q", cmd)
	}
	out, err := exec.Exec(dockerCmd)
	if err != nil {
		return out, fmt.Errorf("docker %s: %w — %s", cmd, err, out)
	}
	return out, nil
}

func (c *dockerControl) ListProcesses(_ context.Context, exec Executor) ([]ProcessInfo, string, error) {
	out, err := exec.Exec("docker ps --format '{{.Names}}\\t{{.Status}}' 2>&1")
	if err != nil {
		return nil, "", fmt.Errorf("docker ps: %w", err)
	}
	var procs []ProcessInfo
	for _, line := range splitLines(out) {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		status := ""
		if len(parts) == 2 {
			status = parts[1]
		}
		procs = append(procs, ProcessInfo{Name: parts[0], Status: status})
	}
	return procs, "docker", nil
}

func (c *dockerControl) ListLogSources(_ context.Context, exec Executor) ([]LogSource, error) {
	out, err := exec.Exec("docker ps --format '{{.Names}}' 2>&1")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var sources []LogSource
	for _, line := range splitLines(out) {
		name := strings.TrimSpace(line)
		if name != "" {
			sources = append(sources, LogSource{Namespace: "docker", Name: name})
		}
	}
	return sources, nil
}

func (c *dockerControl) StreamLog(_ context.Context, exec Executor, _, name string) (<-chan string, func(), error) {
	return exec.Stream(fmt.Sprintf("docker logs -f %s 2>&1", name))
}

func (c *dockerControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	if c.gameserver == "" {
		return "", "", errNotSupported("docker", "CaptureJWT (docker_gameserver not configured)")
	}
	existingToken, err := exec.Exec(fmt.Sprintf(
		"docker exec %s env 2>/dev/null | grep FuncomLiveServices__ServiceAuthToken | cut -d= -f2-",
		c.gameserver))
	if err != nil || strings.TrimSpace(existingToken) == "" {
		return "", "", fmt.Errorf("read ServiceAuthToken from container: %w", err)
	}
	return buildCaptureJWT(strings.TrimSpace(existingToken))
}

func (c *dockerControl) ListExchanges(_ context.Context, exec Executor, brokerLabel string) ([]binding, error) {
	container := c.brokerForLabel(brokerLabel)
	if container == "" {
		return nil, errNotSupported("docker", fmt.Sprintf("ListExchanges (%s not configured)", brokerLabel))
	}
	raw, err := exec.Exec(fmt.Sprintf(
		"docker exec %s rabbitmqctl list_exchanges name 2>/dev/null", container))
	if err != nil {
		return nil, err
	}
	return parseExchanges(raw), nil
}

func (c *dockerControl) EnsureCaptureUser(_ context.Context, exec Executor) {
	for _, container := range []string{c.brokerAdmin, c.brokerGame} {
		if container == "" {
			continue
		}
		ensureBrokerViaDockerExec(exec, container)
	}
}

func (c *dockerControl) EvalOnGameBroker(_ context.Context, exec Executor, expr string) (string, error) {
	if c.brokerGame == "" {
		return "", errNotSupported("docker", "EvalOnGameBroker (docker_broker_game not configured)")
	}
	out, err := exec.Exec(fmt.Sprintf(
		"docker exec %s rabbitmqctl eval %s 2>&1",
		c.brokerGame, shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

func (c *dockerControl) ReadDefaultINI(_ context.Context, exec Executor, filename string) string {
	if c.gameserver == "" {
		return ""
	}
	pathOut, err := exec.Exec(fmt.Sprintf(
		"docker exec %s find / -name %s -not -path '*/Saved/*' -not -path '*/proc/*' -not -path '*/sys/*' -not -path '*/dev/*' 2>/dev/null | head -1",
		c.gameserver, shellQuote(filename)))
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(pathOut)
	if p == "" {
		return ""
	}
	content, err := exec.Exec(fmt.Sprintf("docker exec %s cat %s 2>/dev/null", c.gameserver, shellQuote(p)))
	if err != nil {
		return ""
	}
	return content
}

func (c *dockerControl) DiscoverIniDir(_ context.Context, _ Executor) (string, error) {
	return "", fmt.Errorf("docker control plane requires server_ini_dir to be set in config")
}

func (c *dockerControl) brokerForLabel(label string) string {
	switch label {
	case "mq-game":
		return c.brokerGame
	case "mq-admin":
		return c.brokerAdmin
	}
	return ""
}

func ensureBrokerViaDockerExec(exec Executor, container string) {
	base := fmt.Sprintf("docker exec %s", container)

	out, _ := exec.Exec(fmt.Sprintf("%s rabbitmqctl add_user %s %s 2>&1", base, capUser, capPass))
	if !strings.Contains(out, "already exists") {
		fmt.Printf("[capture] [%s] created user %s\n", container, capUser)
	}
	exec.Exec(fmt.Sprintf("%s rabbitmqctl set_permissions -p / %s '.*' '.*' '.*' 2>&1", base, capUser)) //nolint:errcheck
	exec.Exec(fmt.Sprintf(                                                                               //nolint:errcheck
		"%s rabbitmqctl eval 'application:set_env(rabbit, auth_backends, [{rabbit_auth_backend_cache, rabbit_auth_backend_http}, rabbit_auth_backend_internal]).' 2>&1",
		base))
	exec.Exec(fmt.Sprintf( //nolint:errcheck
		"%s rabbitmqctl eval 'application:set_env(rabbitmq_auth_backend_cache, cache_ttl, 86400000).' 2>&1",
		base))
	fmt.Printf("[capture] [%s] auth backends updated\n", container)
}
