package main

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

type fakeAMPExecutor struct {
	out string
	err error
	cmd string
}

// fnExecutor routes each Exec call through a provided function, allowing
// tests to return different output for different commands.
type fnExecutor struct {
	fn func(cmd string) (string, error)
}

func (f *fnExecutor) Exec(cmd string) (string, error) { return f.fn(cmd) }
func (f *fnExecutor) Stream(string) (<-chan string, func(), error) {
	return nil, func() {}, nil
}
func (f *fnExecutor) PipeToWriter(string, io.Writer) error { return nil }
func (f *fnExecutor) WriteFile(string, io.Reader) error    { return nil }
func (f *fnExecutor) Dial(string, string) (net.Conn, error) {
	return nil, nil
}
func (f *fnExecutor) Close()       {}
func (f *fnExecutor) Type() string { return "local" }

func (f *fakeAMPExecutor) Exec(cmd string) (string, error) {
	f.cmd = cmd
	return f.out, f.err
}
func (f *fakeAMPExecutor) Stream(string) (<-chan string, func(), error) {
	return nil, func() {}, nil
}
func (f *fakeAMPExecutor) PipeToWriter(string, io.Writer) error { return nil }
func (f *fakeAMPExecutor) WriteFile(string, io.Reader) error    { return nil }
func (f *fakeAMPExecutor) Dial(string, string) (net.Conn, error) {
	return nil, nil
}
func (f *fakeAMPExecutor) Close()       {}
func (f *fakeAMPExecutor) Type() string { return "local" }

func TestParseAMPGameProcess(t *testing.T) {
	t.Parallel()

	line := "123 /srv/DuneSandboxServer-Linux-Shipping DuneSandbox HaggaBasinS -Port=7777 -PartitionIndex=3"
	got, ok := parseAMPGameProcess(line)
	if !ok {
		t.Fatalf("expected line to parse")
	}
	if got.pid != 123 || got.mapName != "HaggaBasinS" || got.port != 7777 || got.partition != 3 {
		t.Fatalf("unexpected parsed process: %+v", got)
	}

	if _, ok := parseAMPGameProcess("garbage"); ok {
		t.Fatalf("expected malformed line to be rejected")
	}
}

func TestListGameProcesses(t *testing.T) {
	t.Parallel()

	ctrl := &ampControl{}
	exec := &fakeAMPExecutor{
		out: "100 one DuneSandbox MapA -Port=7001 -PartitionIndex=1\nbad\n200 two DuneSandbox MapB -Port=7002",
	}
	procs, err := ctrl.listGameProcesses(exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("expected 2 parsed processes, got %d", len(procs))
	}
	if procs[0].pid != 100 || procs[0].mapName != "MapA" || procs[0].port != 7001 || procs[0].partition != 1 {
		t.Fatalf("unexpected first process: %+v", procs[0])
	}
	if procs[1].pid != 200 || procs[1].mapName != "MapB" || procs[1].port != 7002 || procs[1].partition != 0 {
		t.Fatalf("unexpected second process: %+v", procs[1])
	}
	if exec.cmd == "" {
		t.Fatalf("expected process listing command to be executed")
	}
}

func TestListGameProcesses_EmptyOnExecErrorWithoutOutput(t *testing.T) {
	t.Parallel()

	ctrl := &ampControl{}
	exec := &fakeAMPExecutor{err: errors.New("ps failed")}
	procs, err := ctrl.listGameProcesses(exec)
	if err != nil {
		t.Fatalf("expected no error when exec fails without output, got %v", err)
	}
	if len(procs) != 0 {
		t.Fatalf("expected empty process list, got %+v", procs)
	}
}

func TestListGameProcesses_NoContainer(t *testing.T) {
	t.Parallel()

	ctrl := &ampControl{useContainer: false}
	exec := &fakeAMPExecutor{out: ""}
	_, _ = ctrl.listGameProcesses(exec)
	if strings.Contains(exec.cmd, " exec ") {
		t.Fatalf("expected no container wrapping for useContainer=false, got cmd: %q", exec.cmd)
	}
	if !strings.Contains(exec.cmd, "DuneSandboxServer") {
		t.Fatalf("expected ps command for DuneSandboxServer, got: %q", exec.cmd)
	}
}

func TestListGameProcesses_WithContainer(t *testing.T) {
	t.Parallel()

	ctrl := &ampControl{
		useContainer:     true,
		container:        "AMP_Dune01",
		ampUser:          "amp",
		containerRuntime: "podman",
	}
	exec := &fakeAMPExecutor{out: ""}
	_, _ = ctrl.listGameProcesses(exec)
	if !strings.Contains(exec.cmd, "podman exec AMP_Dune01") {
		t.Fatalf("expected podman exec wrapping, got cmd: %q", exec.cmd)
	}
	if !strings.Contains(exec.cmd, "DuneSandboxServer") {
		t.Fatalf("expected ps command inside wrapper, got: %q", exec.cmd)
	}
}

// TestAmpDiscoverIniDir_PrefersUE5SavedPath verifies that when
// ue5-saved/UserSettings/UserGame.ini exists (install.sh layout),
// DiscoverIniDir returns that sub-directory rather than the base state dir.
func TestAmpDiscoverIniDir_PrefersUE5SavedPath(t *testing.T) {
	t.Parallel()

	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "ue5-saved/UserSettings") {
			return "yes\n", nil
		}
		return "no\n", nil
	}}
	ctrl := &ampControl{instance: "TestInst", ampUser: "amp"}

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/home/amp/.ampdata/instances/TestInst/duneawakening/server/state/ue5-saved/UserSettings"
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

// TestAmpDiscoverIniDir_FallsBackToState verifies that when ue5-saved/UserSettings
// does not have a UserGame.ini, DiscoverIniDir returns the base state directory.
func TestAmpDiscoverIniDir_FallsBackToState(t *testing.T) {
	t.Parallel()

	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		return "no\n", nil
	}}
	ctrl := &ampControl{instance: "TestInst", ampUser: "amp"}

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/home/amp/.ampdata/instances/TestInst/duneawakening/server/state"
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

// TestAmpDiscoverIniDir_ExplicitConfigSkipsProbe verifies that when server_ini_dir
// is explicitly configured, DiscoverIniDir returns it without probing.
func TestAmpDiscoverIniDir_ExplicitConfigSkipsProbe(t *testing.T) {
	t.Parallel()

	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		t.Error("executor must not be called when iniDir is explicitly configured")
		return "", nil
	}}
	ctrl := &ampControl{iniDir: "/custom/ini/dir"}

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/custom/ini/dir" {
		t.Errorf("got %q, want %q", dir, "/custom/ini/dir")
	}
}

// TestAmpRuntimeCLI_DefaultsToPodman verifies the container-runtime selector
// defaults to podman (backward compatible) and honours an explicit docker.
func TestAmpRuntimeCLI_DefaultsToPodman(t *testing.T) {
	t.Parallel()
	if got := (&ampControl{}).runtimeCLI(); got != "podman" {
		t.Errorf("empty containerRuntime: got %q, want podman", got)
	}
	if got := (&ampControl{containerRuntime: "docker"}).runtimeCLI(); got != "docker" {
		t.Errorf("explicit docker: got %q, want docker", got)
	}
	if got := (&ampControl{containerRuntime: "podman"}).runtimeCLI(); got != "podman" {
		t.Errorf("explicit podman: got %q, want podman", got)
	}
}

// TestAmpWrapInContainer_RuntimeSelection verifies wrapInContainer emits the
// configured container runtime as `<rt> exec` in container mode, defaulting to
// podman when unset.
func TestAmpWrapInContainer_RuntimeSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		runtime    string
		wantSub    string
		notWantSub string
	}{
		{"default empty -> podman", "", "podman exec AMP_X", "docker"},
		{"explicit podman", "podman", "podman exec AMP_X", "docker"},
		{"docker", "docker", "docker exec AMP_X", "podman"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ampControl{ampUser: "amp", container: "AMP_X", useContainer: true, containerRuntime: tt.runtime}
			got := c.wrapInContainer("ls /tmp")
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("wrapInContainer = %q, want substring %q", got, tt.wantSub)
			}
			if tt.notWantSub != "" && strings.Contains(got, tt.notWantSub) {
				t.Errorf("wrapInContainer = %q, must not contain %q", got, tt.notWantSub)
			}
		})
	}
}

// TestAmpWrapInContainer_NativeIgnoresRuntime verifies native mode never wraps
// in a container runtime even when one is configured.
func TestAmpWrapInContainer_NativeIgnoresRuntime(t *testing.T) {
	t.Parallel()
	c := &ampControl{ampUser: "amp", useContainer: false, containerRuntime: "docker"}
	got := c.wrapInContainer("ls")
	if strings.Contains(got, "docker") || strings.Contains(got, "podman") || strings.Contains(got, "exec") {
		t.Errorf("native wrapInContainer must not reference a container runtime: %q", got)
	}
}

// TestAmpBuildRabbitmqctl_RuntimeSelection verifies the rabbitmqctl trampoline
// is wrapped in the configured container runtime.
func TestAmpBuildRabbitmqctl_RuntimeSelection(t *testing.T) {
	t.Parallel()
	c := &ampControl{ampUser: "amp", container: "AMP_X", useContainer: true, containerRuntime: "docker"}
	cmd := c.buildRabbitmqctl("mq-admin", "status")
	if !strings.Contains(cmd, "docker exec AMP_X") {
		t.Errorf("buildRabbitmqctl = %q, want 'docker exec AMP_X'", cmd)
	}
	if strings.Contains(cmd, "podman") {
		t.Errorf("buildRabbitmqctl must not reference podman when runtime=docker: %q", cmd)
	}
}

// TestAmpListLogSources_UsesConfiguredRuntime is an end-to-end check that the
// runtime selection flows through a real ControlPlane method to the executor.
func TestAmpListLogSources_UsesConfiguredRuntime(t *testing.T) {
	t.Parallel()
	exec := &fakeAMPExecutor{out: "game.log\nserver.log\n"}
	c := &ampControl{container: "AMP_X", ampUser: "amp", logPath: "/AMP/duneawakening/logs", useContainer: true, containerRuntime: "docker"}
	if _, err := c.ListLogSources(context.Background(), exec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(exec.cmd, "docker exec AMP_X") {
		t.Errorf("ListLogSources cmd = %q, want 'docker exec AMP_X'", exec.cmd)
	}
}
