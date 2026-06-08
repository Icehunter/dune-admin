package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// These tests cover docker game-server discovery for the Battlegroup view.
// They reuse fnExecutor from control_amp_test.go (same package).

func TestDockerListGameProcesses(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{gameserver: "dune-gs"}
	var gotCmd string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		gotCmd = cmd
		return "100 /x/DuneSandboxServer-Linux-Shipping DuneSandbox MapA -Port=7001 -PartitionIndex=1\n" +
			"bad\n" +
			"200 /x/DuneSandboxServer-Linux-Shipping DuneSandbox MapB -Port=7002", nil
	}}

	procs, err := ctrl.listGameProcesses(exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("expected 2 parsed processes, got %d", len(procs))
	}
	if procs[0].mapName != "MapA" || procs[0].port != 7001 || procs[0].partition != 1 {
		t.Errorf("unexpected first process: %+v", procs[0])
	}
	if procs[1].mapName != "MapB" || procs[1].partition != 0 {
		t.Errorf("unexpected second process: %+v", procs[1])
	}
	if !strings.Contains(gotCmd, "docker exec dune-gs") {
		t.Errorf("expected command to exec inside the gameserver container, got: %q", gotCmd)
	}
	if !strings.Contains(gotCmd, "DuneSandboxServer-Linux-Shipping") {
		t.Errorf("expected ps grep for DuneSandboxServer, got: %q", gotCmd)
	}
}

func TestDockerListGameProcesses_ErrorWhenGameserverUnset(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	if _, err := ctrl.listGameProcesses(exec); err == nil {
		t.Fatal("expected error when docker_gameserver is unset")
	}
}

func TestDockerListGameProcesses_EmptyOnExecErrorWithoutOutput(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{gameserver: "dune-gs"}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", errors.New("ps failed") }}
	procs, err := ctrl.listGameProcesses(exec)
	if err != nil {
		t.Fatalf("expected no error when exec fails without output, got %v", err)
	}
	if len(procs) != 0 {
		t.Fatalf("expected empty process list, got %+v", procs)
	}
}

func TestDockerGetStatus_PopulatesServersFromProcesses(t *testing.T) {
	ctrl := &dockerControl{gameserver: "dune-gs"}
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "inspect"):
			return "running\n", nil
		case strings.Contains(cmd, "DuneSandboxServer"):
			return "100 /x/DuneSandboxServer-Linux-Shipping DuneSandbox MapA -Port=7001 -PartitionIndex=1\n" +
				"200 /x/DuneSandboxServer-Linux-Shipping DuneSandbox MapB -Port=7002 -PartitionIndex=2", nil
		}
		return "", nil
	}}

	st, err := ctrl.GetStatus(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Name != "dune-gs" {
		t.Errorf("Name = %q, want dune-gs", st.Name)
	}
	if st.Phase != "running" {
		t.Errorf("Phase = %q, want running (the container state)", st.Phase)
	}
	if len(st.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(st.Servers))
	}
	if st.Servers[0].Map != "MapA" || st.Servers[0].Partition != 1 {
		t.Errorf("server[0] = %+v, want MapA partition 1", st.Servers[0])
	}
	if !st.Servers[0].Ready || st.Servers[0].Phase != "Running" {
		t.Errorf("server[0] should be Ready and Phase=Running, got %+v", st.Servers[0])
	}
	if st.Servers[1].Map != "MapB" || st.Servers[1].Partition != 2 {
		t.Errorf("server[1] = %+v, want MapB partition 2", st.Servers[1])
	}
}

func TestDockerGetStatus_ErrorWhenGameserverUnset(t *testing.T) {
	ctrl := &dockerControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	if _, err := ctrl.GetStatus(context.Background(), exec); err == nil {
		t.Fatal("expected error when docker_gameserver is unset")
	}
}

func TestDockerGetStatus_EmptyServersWhenNoProcesses(t *testing.T) {
	ctrl := &dockerControl{gameserver: "dune-gs"}
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "inspect") {
			return "running", nil
		}
		return "", nil // no game-server processes
	}}

	st, err := ctrl.GetStatus(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Servers == nil {
		t.Fatal("Servers must be a non-nil empty slice so it serialises as [] not null")
	}
	if len(st.Servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(st.Servers))
	}
}

// ── DiscoverIniDir (Server Settings) ──────────────────────────────────────────

func TestDockerDiscoverIniDir_ConfiguredBase_FindsUserSettings(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{gameserver: "dune-gs", iniDir: "/srv/state"}
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "/srv/state") && strings.Contains(cmd, "UserSettings/UserGame.ini") {
			return "/srv/state/ue5-saved/UserSettings/UserGame.ini\n", nil
		}
		return "", nil
	}}

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/srv/state/ue5-saved/UserSettings" {
		t.Errorf("got %q, want /srv/state/ue5-saved/UserSettings", dir)
	}
}

// Layout-agnostic auto-discovery: no server_ini_dir configured, so the directory
// is found by probing the container's mount sources (covers bind mounts AND
// named volumes, whose host source is /var/lib/docker/volumes/<name>/_data).
func TestDockerDiscoverIniDir_AutoDiscoverFromMounts(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{gameserver: "dune-gs"}
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "docker inspect") && strings.Contains(cmd, "Mounts"):
			return "/var/lib/docker/volumes/dune_state/_data\n/etc/localtime\n", nil
		case strings.Contains(cmd, "dune_state") && strings.Contains(cmd, "UserSettings/UserGame.ini"):
			return "/var/lib/docker/volumes/dune_state/_data/ue5-saved/UserSettings/UserGame.ini", nil
		}
		return "", nil
	}}

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/var/lib/docker/volumes/dune_state/_data/ue5-saved/UserSettings"
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestDockerDiscoverIniDir_ConfiguredBase_FallsBackToConfigured(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{gameserver: "dune-gs", iniDir: "/srv/state"}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }} // nothing found, no mounts

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/srv/state" {
		t.Errorf("got %q, want /srv/state (configured fallback when probe is inconclusive)", dir)
	}
}

func TestDockerDiscoverIniDir_NotFound_Errors(t *testing.T) {
	t.Parallel()

	ctrl := &dockerControl{gameserver: "dune-gs"} // no server_ini_dir, nothing discoverable
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}

	if _, err := ctrl.DiscoverIniDir(context.Background(), exec); err == nil {
		t.Fatal("expected error when no UserGame.ini found and no server_ini_dir configured")
	}
}
