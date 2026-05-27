package main

import (
	"context"
	"fmt"
	"strings"
)

// localControl implements ControlPlane using configurable shell commands.
// Intended for AMP, LGSM, bare-metal, or any environment where the user
// manages the game server through their own tooling.
type localControl struct {
	cmdStart        string // e.g. "amp start dune"
	cmdStop         string
	cmdRestart      string
	cmdStatus       string
	brokerExecPrefix string // e.g. "podman exec AMP_MehDune01" — prepended to rabbitmqctl calls
}

func (c *localControl) Name() string { return "local" }

func (c *localControl) GetStatus(_ context.Context, exec Executor) (*BattlegroupStatus, error) {
	if c.cmdStatus == "" {
		return nil, errNotSupported("local", "GetStatus (cmd_status not configured)")
	}
	out, err := exec.Exec(c.cmdStatus)
	if err != nil {
		return nil, fmt.Errorf("status command: %w — %s", err, out)
	}
	return &BattlegroupStatus{
		Name:    "local",
		Title:   "Local Server",
		Phase:   strings.TrimSpace(out),
		Servers: []ServerRow{},
	}, nil
}

func (c *localControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	var shellCmd string
	switch cmd {
	case "start":
		shellCmd = c.cmdStart
	case "stop":
		shellCmd = c.cmdStop
	case "restart":
		shellCmd = c.cmdRestart
	default:
		return "", fmt.Errorf("local control does not support %q", cmd)
	}
	if shellCmd == "" {
		return "", errNotSupported("local", fmt.Sprintf("ExecCommand %q (cmd_%s not configured)", cmd, cmd))
	}
	out, err := exec.Exec(shellCmd)
	if err != nil {
		return out, fmt.Errorf("%s command: %w — %s", cmd, err, out)
	}
	return out, nil
}

func (c *localControl) ListProcesses(_ context.Context, _ Executor) ([]ProcessInfo, string, error) {
	return nil, "", errNotSupported("local", "ListProcesses")
}

func (c *localControl) ListLogSources(_ context.Context, _ Executor) ([]LogSource, error) {
	return nil, errNotSupported("local", "ListLogSources")
}

func (c *localControl) StreamLog(_ context.Context, _ Executor, _, _ string) (<-chan string, func(), error) {
	return nil, func() {}, errNotSupported("local", "StreamLog")
}

func (c *localControl) CaptureJWT(_ context.Context, _ Executor) (string, string, error) {
	return "", "", errNotSupported("local", "CaptureJWT")
}

func (c *localControl) brokerBase() string {
	if c.brokerExecPrefix != "" {
		return c.brokerExecPrefix + " rabbitmqctl"
	}
	return "rabbitmqctl"
}

func (c *localControl) ListExchanges(_ context.Context, exec Executor, _ string) ([]binding, error) {
	raw, err := exec.Exec(c.brokerBase() + " list_exchanges name 2>/dev/null")
	if err != nil {
		return nil, errNotSupported("local", "ListExchanges (rabbitmqctl not available)")
	}
	return parseExchanges(raw), nil
}

func (c *localControl) EnsureCaptureUser(_ context.Context, exec Executor) {
	base := c.brokerBase()
	out, _ := exec.Exec(fmt.Sprintf("%s add_user %s %s 2>&1", base, capUser, capPass))
	if !strings.Contains(out, "already exists") {
		fmt.Printf("[capture] [local] created user %s\n", capUser)
	}
	exec.Exec(fmt.Sprintf("%s set_permissions -p / %s '.*' '.*' '.*' 2>&1", base, capUser)) //nolint:errcheck
	exec.Exec(fmt.Sprintf(                                                                    //nolint:errcheck
		"%s eval 'application:set_env(rabbit, auth_backends, [{rabbit_auth_backend_cache, rabbit_auth_backend_http}, rabbit_auth_backend_internal]).' 2>&1",
		base))
	exec.Exec(fmt.Sprintf( //nolint:errcheck
		"%s eval 'application:set_env(rabbitmq_auth_backend_cache, cache_ttl, 86400000).' 2>&1",
		base))
	fmt.Println("[capture] [local] auth backends updated")
}

func (c *localControl) EvalOnGameBroker(_ context.Context, exec Executor, expr string) (string, error) {
	out, err := exec.Exec(fmt.Sprintf("%s eval %s 2>&1", c.brokerBase(), shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

func (c *localControl) ReadDefaultINI(_ context.Context, _ Executor, _ string) string {
	return "" // host-path traversal in readDefaultINIContent handles local/Hyper-V
}

func (c *localControl) DiscoverIniDir(_ context.Context, _ Executor) (string, error) {
	return "", fmt.Errorf("local control plane requires server_ini_dir to be set in config")
}
