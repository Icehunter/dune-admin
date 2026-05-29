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
	cmdStart         string // e.g. "amp start dune"
	cmdStop          string
	cmdRestart       string
	cmdStatus        string
	controlNamespace string
	brokerExecPrefix string // e.g. "podman exec AMP_MehDune01" — prepended to rabbitmqctl calls
}

func (c *localControl) Name() string { return "local" }

func (c *localControl) kubectlEnabled(exec Executor) bool {
	if c.controlNamespace == "" || exec == nil {
		return false
	}
	_, err := exec.Exec(kubectlCLI(exec) + " version --client >/dev/null 2>&1")
	return err == nil
}

func (c *localControl) kubectlDelegate() *kubectlControl {
	return &kubectlControl{namespace: c.controlNamespace}
}

func (c *localControl) GetStatus(_ context.Context, exec Executor) (*BattlegroupStatus, error) {
	if c.cmdStatus == "" {
		if c.kubectlEnabled(exec) {
			return c.kubectlDelegate().GetStatus(context.Background(), exec)
		}
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
		if c.kubectlEnabled(exec) {
			return c.kubectlDelegate().ExecCommand(context.Background(), exec, cmd)
		}
		return "", errNotSupported("local", fmt.Sprintf("ExecCommand %q (cmd_%s not configured)", cmd, cmd))
	}
	out, err := exec.Exec(shellCmd)
	if err != nil {
		return out, fmt.Errorf("%s command: %w — %s", cmd, err, out)
	}
	return out, nil
}

func (c *localControl) ListProcesses(_ context.Context, exec Executor) ([]ProcessInfo, string, error) {
	if c.kubectlEnabled(exec) {
		return c.kubectlDelegate().ListProcesses(context.Background(), exec)
	}
	return nil, "", errNotSupported("local", "ListProcesses")
}

func (c *localControl) ListLogSources(_ context.Context, exec Executor) ([]LogSource, error) {
	if c.kubectlEnabled(exec) {
		return c.kubectlDelegate().ListLogSources(context.Background(), exec)
	}
	return nil, errNotSupported("local", "ListLogSources")
}

func (c *localControl) StreamLog(_ context.Context, exec Executor, ns, name string) (<-chan string, func(), error) {
	if c.kubectlEnabled(exec) {
		return c.kubectlDelegate().StreamLog(context.Background(), exec, ns, name)
	}
	return nil, func() {}, errNotSupported("local", "StreamLog")
}

func (c *localControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	if c.kubectlEnabled(exec) {
		return c.kubectlDelegate().CaptureJWT(context.Background(), exec)
	}
	return "", "", errNotSupported("local", "CaptureJWT")
}

func (c *localControl) brokerBase() string {
	if c.brokerExecPrefix != "" {
		return c.brokerExecPrefix + " rabbitmqctl"
	}
	return "rabbitmqctl"
}

func (c *localControl) EvalOnGameBroker(ctx context.Context, exec Executor, expr string) (string, error) {
	if c.kubectlEnabled(exec) {
		return c.kubectlDelegate().EvalOnGameBroker(ctx, exec, expr)
	}
	out, err := exec.Exec(fmt.Sprintf("%s eval %s 2>&1", c.brokerBase(), shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

func (c *localControl) ReadDefaultINI(ctx context.Context, exec Executor, filename string) string {
	if c.kubectlEnabled(exec) {
		return c.kubectlDelegate().ReadDefaultINI(ctx, exec, filename)
	}
	return "" // host-path traversal in readDefaultINIContent handles local/Hyper-V
}

func (c *localControl) DiscoverIniDir(_ context.Context, exec Executor) (string, error) {
	if c.kubectlEnabled(exec) {
		ns := c.controlNamespace
		kctl := kubectlCLI(exec)
		// UserSettings live on game-server pods (-sg-), not the bgd deploy pod.
		podOut, err := exec.Exec(fmt.Sprintf(
			"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep -- '-sg-' | head -1",
			kctl, ns,
		))
		if err != nil || strings.TrimSpace(podOut) == "" {
			podOut, err = exec.Exec(fmt.Sprintf(
				"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep bgd | head -1",
				kctl, ns,
			))
		}
		if err != nil || strings.TrimSpace(podOut) == "" {
			return "", fmt.Errorf("could not find game or bgd pod in namespace %s", ns)
		}
		pod := strings.TrimSpace(podOut)
		findCmd := `for d in /home/dune/server/DuneSandbox/Saved/UserSettings /DuneSandbox/Saved/UserSettings /game/DuneSandbox/Saved/UserSettings; do if [ -d "$d" ]; then echo "$d"; exit 0; fi; done; find / -type d -path "*/Saved/UserSettings" 2>/dev/null | head -1`
		dirOut, err := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>/dev/null",
			kctl, ns, pod, shellQuote(findCmd),
		))
		if err != nil || strings.TrimSpace(dirOut) == "" {
			return "", fmt.Errorf("could not auto-discover ini dir in pod %s", pod)
		}
		dir := strings.TrimSpace(dirOut)
		return fmt.Sprintf("k8s://%s/%s%s", ns, pod, dir), nil
	}
	return "", fmt.Errorf("local control plane requires server_ini_dir to be set in config")
}
