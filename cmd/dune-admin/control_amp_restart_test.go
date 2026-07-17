package main

import (
	"context"
	"strings"
	"testing"
)

// TestAmpExecCommand_RestartContainerModeCyclesContainer verifies that under
// containerised AMP, "restart" recycles the whole container rather than calling
// ampinstmgr — proven in-game to be the only action that reaps the
// DuneSandboxServer processes so settings actually apply.
func TestAmpExecCommand_RestartContainerModeCyclesContainer(t *testing.T) {
	t.Parallel()
	exec := &fakeAMPExecutor{out: "AMP_X"}
	c := &ampControl{instance: "Dune01", useContainer: true, container: "AMP_X", ampUser: "amp", containerRuntime: "docker"}
	if _, err := c.ExecCommand(context.Background(), exec, "restart"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !strings.Contains(exec.cmd, "docker restart -t 60 AMP_X") {
		t.Errorf("restart cmd = %q, want 'docker restart -t 60 AMP_X' (generous stop timeout so the heavy container shuts down gracefully instead of wedging on SIGKILL)", exec.cmd)
	}
	if strings.Contains(exec.cmd, "ampinstmgr") {
		t.Errorf("container restart must not use ampinstmgr (does not reap game procs): %q", exec.cmd)
	}
}

// TestAmpExecCommand_RestartContainerModeDefaultsPodman verifies the container
// runtime defaults to podman when unset (backward compatible).
func TestAmpExecCommand_RestartContainerModeDefaultsPodman(t *testing.T) {
	t.Parallel()
	exec := &fakeAMPExecutor{}
	c := &ampControl{instance: "Dune01", useContainer: true, container: "AMP_X", ampUser: "amp"}
	if _, err := c.ExecCommand(context.Background(), exec, "restart"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !strings.Contains(exec.cmd, "podman restart -t 60 AMP_X") {
		t.Errorf("restart cmd = %q, want 'podman restart -t 60 AMP_X'", exec.cmd)
	}
}

// TestAmpExecCommand_RestartNativeModeUsesAmpinstmgr verifies that without a
// container (native AMP), restart keeps the ampinstmgr stop/start cycle.
func TestAmpExecCommand_RestartNativeModeUsesAmpinstmgr(t *testing.T) {
	t.Parallel()
	exec := &fakeAMPExecutor{}
	c := &ampControl{instance: "Dune01", useContainer: false, ampUser: "amp"}
	if _, err := c.ExecCommand(context.Background(), exec, "restart"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !strings.Contains(exec.cmd, "ampinstmgr -q Dune01") || !strings.Contains(exec.cmd, "ampinstmgr -s Dune01") {
		t.Errorf("native restart cmd = %q, want ampinstmgr -q/-s cycle", exec.cmd)
	}
}

// TestAmpExecCommand_RestartContainerModeMissingContainer verifies a clear error
// when container mode is configured without a container name.
func TestAmpExecCommand_RestartContainerModeMissingContainer(t *testing.T) {
	t.Parallel()
	exec := &fakeAMPExecutor{}
	c := &ampControl{instance: "Dune01", useContainer: true, container: "", ampUser: "amp", containerRuntime: "docker"}
	if _, err := c.ExecCommand(context.Background(), exec, "restart"); err == nil {
		t.Fatal("expected error when container name missing in container mode")
	}
}

// TestAmpExecCommand_StartStopUnchanged guards that start/stop still use
// ampinstmgr (only restart was proven to need container recycling).
func TestAmpExecCommand_StartStopUnchanged(t *testing.T) {
	t.Parallel()
	c := &ampControl{instance: "Dune01", useContainer: true, container: "AMP_X", ampUser: "amp", containerRuntime: "docker"}
	for _, tc := range []struct{ cmd, want string }{
		{"start", "ampinstmgr -s Dune01"},
		{"stop", "ampinstmgr -q Dune01"},
	} {
		exec := &fakeAMPExecutor{}
		if _, err := c.ExecCommand(context.Background(), exec, tc.cmd); err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if !strings.Contains(exec.cmd, tc.want) {
			t.Errorf("%s cmd = %q, want %q", tc.cmd, exec.cmd, tc.want)
		}
	}
}
