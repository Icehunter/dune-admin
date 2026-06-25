package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

// Dial mirrors localExecutor: a real direct dial. The director HTTP client now
// routes through the executor, so GetStatus tests that hit a loopback httptest
// server need a functioning Dial here.
func (f *fakeAMPExecutor) Dial(network, addr string) (net.Conn, error) {
	return net.Dial(network, addr)
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

func TestAmpPgDumpRestoreCommands(t *testing.T) {
	t.Parallel()
	c := &ampControl{
		container:        "AMP_DuneTest01",
		ampUser:          "amp",
		containerRuntime: "docker",
		pgBin:            "/AMP/duneawakening/extracted/postgres/usr/local/bin",
		pgLib:            "/pg/lib:/db/lib",
	}
	conn := dbConn{Host: "127.0.0.1", Port: 15432, User: "dune", Pass: "secret", Name: "dune"}

	dump := c.pgDumpCommand(conn, "/home/test/db-backups/x.dump")
	for _, want := range []string{
		"sudo -i -u amp", "docker exec", "AMP_DuneTest01",
		"PGPASSWORD=", "LD_LIBRARY_PATH=", "/pg/lib:/db/lib",
		"pg_dump", "-Fc", "-h ", "127.0.0.1", "-p 15432", "-U ", "-d ",
		"> ", "/home/test/db-backups/x.dump",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("pgDumpCommand missing %q in:\n%s", want, dump)
		}
	}

	restore := c.pgRestoreCommand(conn, "/home/test/db-backups/x.dump")
	for _, want := range []string{
		"docker exec -i", "pg_restore", "--clean", "--if-exists", "-d ",
		"< ", "/home/test/db-backups/x.dump",
	} {
		if !strings.Contains(restore, want) {
			t.Errorf("pgRestoreCommand missing %q in:\n%s", want, restore)
		}
	}
}

func TestParseProcessAges(t *testing.T) {
	t.Parallel()

	// `ps -o pid=,etimes=` emits two whitespace-separated columns (pid, elapsed
	// seconds), typically with leading padding. We only care about a pid→seconds
	// map; malformed lines are skipped, not fatal.
	out := "  123     3600\n456 90\nbad line here\n   789  0\n"
	got := parseProcessAges(out)

	want := map[int]int{123: 3600, 456: 90, 789: 0}
	if len(got) != len(want) {
		t.Fatalf("parseProcessAges len = %d, want %d (%+v)", len(got), len(want), got)
	}
	for pid, age := range want {
		if got[pid] != age {
			t.Fatalf("parseProcessAges[%d] = %d, want %d", pid, got[pid], age)
		}
	}

	if len(parseProcessAges("")) != 0 {
		t.Fatalf("expected empty input to yield empty map")
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
	_, err := ctrl.listGameProcesses(exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	_, err := ctrl.listGameProcesses(exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(exec.cmd, "podman exec AMP_Dune01") {
		t.Fatalf("expected podman exec wrapping, got cmd: %q", exec.cmd)
	}
	if !strings.Contains(exec.cmd, "DuneSandboxServer") {
		t.Fatalf("expected ps command inside wrapper, got: %q", exec.cmd)
	}
}

func TestListGameProcesses_WithContainer_MissingContainerName(t *testing.T) {
	t.Parallel()

	ctrl := &ampControl{useContainer: true, container: "", ampUser: "amp"}
	exec := &fakeAMPExecutor{out: ""}
	_, err := ctrl.listGameProcesses(exec)
	if err == nil {
		t.Fatal("expected error when useContainer=true but container name is empty")
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

// TestAmpDiscoverIniDir_ExplicitConfig_PrefersUE5SubDir verifies that when
// server_ini_dir is set to a base state directory and ue5-saved/UserSettings
// contains UserGame.ini, DiscoverIniDir returns the ue5-saved subdirectory.
func TestAmpDiscoverIniDir_ExplicitConfig_PrefersUE5SubDir(t *testing.T) {
	t.Parallel()

	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "ue5-saved/UserSettings") {
			return "yes\n", nil
		}
		return "no\n", nil
	}}
	ctrl := &ampControl{iniDir: "/custom/state"}

	dir, err := ctrl.DiscoverIniDir(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/custom/state/ue5-saved/UserSettings"
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

// TestAmpDiscoverIniDir_ExplicitConfig_FallsBackToConfigured verifies that when
// server_ini_dir is set and ue5-saved/UserSettings has no UserGame.ini, the
// configured path is returned as-is.
func TestAmpDiscoverIniDir_ExplicitConfig_FallsBackToConfigured(t *testing.T) {
	t.Parallel()

	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		return "no\n", nil
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

// TestDirectorConfigPath_LandsInStateDir is the core regression for #171: the
// director_config.ini lives in the instance $STATE dir (alongside
// UserOverrides.ini), regardless of whether the resolved INI dir is the base
// state dir (server_ini_dir documented form) or its ue5-saved/UserSettings
// subdirectory. The old derivation did Dir(Dir(iniDir)), which overshot by two
// levels when iniDir was already $STATE, yielding …/duneawakening/director_config.ini
// (a path that does not exist) and the opaque "could not read director config".
func TestDirectorConfigPath_LandsInStateDir(t *testing.T) {
	t.Parallel()
	const stateDir = "/home/amp/.ampdata/instances/Dune01/duneawakening/server/state"
	want := stateDir + "/director_config.ini"

	tests := []struct {
		name   string
		iniDir string
		ue5Has bool // does <base>/ue5-saved/UserSettings/UserGame.ini exist?
	}{
		{"base state dir, no ue5 subdir", stateDir, false},
		{"base state dir, ue5 subdir present", stateDir, true},
		{"configured as ue5 subdir", stateDir + "/ue5-saved/UserSettings", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fnExecutor{fn: func(cmd string) (string, error) {
				if strings.Contains(cmd, "UserGame.ini") {
					if tt.ue5Has {
						return "yes\n", nil
					}
					return "no\n", nil
				}
				return "no\n", nil
			}}
			ctrl := &ampControl{iniDir: tt.iniDir, ampUser: "amp", instance: "Dune01"}
			got, err := ctrl.directorConfigPath(exec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != want {
				t.Errorf("directorConfigPath = %q, want %q", got, want)
			}
		})
	}
}

// TestReadDirectorConfig_FallsBackToPlainCat is the regression for #171: the
// Director config tab showed "could not read director config / exit status 1"
// because the read only ever tried `sudo -i -u amp cat`, which the narrow AMP
// sudoers (ampinstmgr/podman/tee only) refuses. When dune-admin runs AS the amp
// user it owns the 0700 host file, so a plain `cat` succeeds and no sudo is
// needed — the read must try that.
func TestReadDirectorConfig_FallsBackToPlainCat(t *testing.T) {
	t.Parallel()
	var sawPlainCat bool
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		trimmed := strings.TrimSpace(cmd)
		switch {
		case strings.Contains(cmd, "ue5-saved/UserSettings"):
			return "no\n", nil // DiscoverIniDir probe → use configured iniDir
		case strings.HasPrefix(trimmed, "sudo "):
			return "", errors.New("exit status 1") // sudoers won't grant cat
		case strings.HasPrefix(trimmed, "cat "):
			sawPlainCat = true
			return "[Director]\nfoo=bar\n", nil
		}
		return "", nil
	}}
	ctrl := &ampControl{iniDir: "/custom/state", ampUser: "amp"}

	_, content, err := ctrl.readDirectorConfig(exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawPlainCat {
		t.Fatalf("expected a plain `cat` read to be attempted")
	}
	if !strings.Contains(content, "foo=bar") {
		t.Errorf("expected director content, got %q", content)
	}
}

// TestReadDirectorConfig_FallsBackToSudoCat covers the split-user deployment:
// the service user does not own the file (plain cat fails), but the admin has
// granted /usr/bin/cat so `sudo -i -u amp cat` succeeds.
func TestReadDirectorConfig_FallsBackToSudoCat(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		trimmed := strings.TrimSpace(cmd)
		switch {
		case strings.Contains(cmd, "ue5-saved/UserSettings"):
			return "no\n", nil
		case strings.HasPrefix(trimmed, "sudo "):
			return "[Director]\nfoo=bar\n", nil
		case strings.HasPrefix(trimmed, "cat "):
			return "", errors.New("exit status 1") // not the owner
		}
		return "", nil
	}}
	ctrl := &ampControl{iniDir: "/custom/state", ampUser: "amp"}

	_, content, err := ctrl.readDirectorConfig(exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "foo=bar") {
		t.Errorf("expected director content, got %q", content)
	}
}

// TestReadDirectorConfig_AllReadsFailIsActionable verifies that when neither a
// plain nor a sudo cat can read the file (split-user deployment without a cat
// grant), the wrapped error names the remedy so it shows up in director.log.
func TestReadDirectorConfig_AllReadsFailIsActionable(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "ue5-saved/UserSettings") {
			return "no\n", nil
		}
		return "", errors.New("exit status 1")
	}}
	ctrl := &ampControl{iniDir: "/custom/state", ampUser: "amp"}

	_, _, err := ctrl.readDirectorConfig(exec)
	if err == nil {
		t.Fatalf("expected an error when every read strategy fails")
	}
	if !strings.Contains(err.Error(), "cat") {
		t.Errorf("error should point the admin at the cat grant / amp user, got %q", err)
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

// directorBattlegroupJSON mirrors the structurally-relevant subset of the
// Battlegroup Director's /v0/battlegroup response: a single-server map, a
// dimension map (sharded under serversByDimension), and an instanced map.
// Each leaf server carries a "partition" object with partitionId,
// dimensionIndex and label — the fields GetStatus enriches rows with.
const directorBattlegroupJSON = `{
  "bgTitle": "Test BG",
  "singleServerMaps": {
    "Overmap": {
      "cfg": {"playerHardCap": 60},
      "gamePort": 7794,
      "numPlayersInGame": 5,
      "numPlayersInQueue": 2,
      "serverPlayerHardCap": -1,
      "partition": {"partitionId": 2, "dimensionIndex": 0, "label": "Overland"}
    }
  },
  "dimensionMaps": {
    "DeepDesert_1": {
      "cfg": null,
      "webOverrideCfg": null,
      "serversByDimension": {
        "0": {"gamePort": 7799, "numPlayersInGame": 2, "numPlayersInQueue": 0, "serverPlayerHardCap": -1, "cfg": {"playerHardCap": 80}, "partition": {"partitionId": 8, "dimensionIndex": 0, "label": "DeepDesert_0"}},
        "1": {"gamePort": 7800, "numPlayersInGame": 0, "numPlayersInQueue": 1, "serverPlayerHardCap": 40, "cfg": {"playerHardCap": 80}, "partition": {"partitionId": 143, "dimensionIndex": 1, "label": "DeepDesert_1"}}
      }
    }
  },
  "instancedMaps": {
    "SH_Arrakeen": {
      "instances": {
        "inst-a": {"gamePort": 7792, "numPlayersInGame": 7, "numPlayersInQueue": null, "serverPlayerHardCap": -1, "cfg": {"playerHardCap": 80}, "partition": {"partitionId": 3, "dimensionIndex": 0, "label": "Arrakeen_0"}}
      }
    }
  }
}`

// psLineFor builds a synthetic `ps`-style game-server line for a map/port/partition.
func psLineFor(pid int, mapName string, port, partition int) string {
	return fmt.Sprintf(
		"%d /x/DuneSandboxServer-Linux-Shipping DuneSandbox %s -Port=%d -PartitionIndex=%d",
		pid, mapName, port, partition)
}

// TestAmpGetStatus_EnrichesDimensionFromDirector verifies that GetStatus joins
// each ps-derived partition to the director's dimensionIndex and label, walking
// single-server, dimension, and instanced map categories alike.
func TestAmpGetStatus_EnrichesDimensionFromDirector(t *testing.T) {
	// Not parallel: GetStatus reads the globalDB package global, which other
	// parallel tests mutate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/battlegroup" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, directorBattlegroupJSON)
	}))
	defer srv.Close()

	psOut := strings.Join([]string{
		psLineFor(1001, "Overmap", 7794, 2),
		psLineFor(1002, "DeepDesert_1", 7799, 8),
		psLineFor(1003, "DeepDesert_1", 7800, 143),
		psLineFor(1004, "SH_Arrakeen", 7792, 3),
	}, "\n")

	c := &ampControl{container: "AMP_X", useContainer: false, directorURL: srv.URL}
	status, err := c.GetStatus(context.Background(), &fakeAMPExecutor{out: psOut})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	want := map[int]struct {
		dim     int
		sietch  string
		players int
		cap     int
		queue   int
	}{
		2:   {0, "Overland", 5, 60, 2},     // serverPlayerHardCap -1 → cfg cap 60
		8:   {0, "DeepDesert_0", 2, 80, 0}, // cfg cap 80
		143: {1, "DeepDesert_1", 0, 40, 1}, // serverPlayerHardCap 40 overrides cfg 80
		3:   {0, "Arrakeen_0", 7, 80, 0},   // queue null → 0
	}
	if len(status.Servers) != len(want) {
		t.Fatalf("got %d servers, want %d", len(status.Servers), len(want))
	}
	for _, row := range status.Servers {
		exp, ok := want[row.Partition]
		if !ok {
			t.Fatalf("unexpected partition %d", row.Partition)
		}
		if row.Dimension != exp.dim {
			t.Errorf("partition %d: dimension = %d, want %d", row.Partition, row.Dimension, exp.dim)
		}
		if row.Sietch != exp.sietch {
			t.Errorf("partition %d: sietch = %q, want %q", row.Partition, row.Sietch, exp.sietch)
		}
		if row.Players != exp.players {
			t.Errorf("partition %d: players = %d, want %d", row.Partition, row.Players, exp.players)
		}
		if row.PlayerHardCap != exp.cap {
			t.Errorf("partition %d: playerHardCap = %d, want %d", row.Partition, row.PlayerHardCap, exp.cap)
		}
		if row.Queue != exp.queue {
			t.Errorf("partition %d: queue = %d, want %d", row.Partition, row.Queue, exp.queue)
		}
	}
}

// TestAmpGetStatus_NoDirectorURL verifies that with no director configured,
// GetStatus still returns rows from ps with dimension left at zero (current
// behaviour) and makes no HTTP call.
func TestAmpGetStatus_NoDirectorURL(t *testing.T) {
	// Not parallel: GetStatus reads the globalDB package global.
	psOut := psLineFor(2001, "Overmap", 7794, 2)
	c := &ampControl{container: "AMP_X", useContainer: false} // directorURL empty
	status, err := c.GetStatus(context.Background(), &fakeAMPExecutor{out: psOut})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(status.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(status.Servers))
	}
	if status.Servers[0].Partition != 2 || status.Servers[0].Dimension != 0 {
		t.Errorf("row = %+v, want partition 2 dimension 0", status.Servers[0])
	}
}

// TestAmpGetStatus_DirectorUnreachable_FallsBack verifies that a transport
// failure to the director is non-fatal: rows are still returned from ps with
// dimension left at zero.
func TestAmpGetStatus_DirectorUnreachable_FallsBack(t *testing.T) {
	// Not parallel: GetStatus reads the globalDB package global.
	// Closed server: take a listener address then immediately close it.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := srv.URL
	srv.Close()

	psOut := psLineFor(3001, "Overmap", 7794, 2)
	c := &ampControl{container: "AMP_X", useContainer: false, directorURL: deadURL}
	status, err := c.GetStatus(context.Background(), &fakeAMPExecutor{out: psOut})
	if err != nil {
		t.Fatalf("GetStatus should not fail on director error: %v", err)
	}
	if len(status.Servers) != 1 || status.Servers[0].Dimension != 0 {
		t.Fatalf("expected 1 row with dimension 0, got %+v", status.Servers)
	}
}

// TestCollectPartitions_WalksNestedAndIgnoresNull verifies the recursive walker
// records partitions from arbitrary nesting and ignores null/non-object
// "partition" values.
func TestCollectPartitions_WalksNestedAndIgnoresNull(t *testing.T) {
	t.Parallel()

	var raw map[string]any
	if err := json.Unmarshal([]byte(directorBattlegroupJSON), &raw); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	out := map[int]partitionMeta{}
	collectPartitions(raw, out)

	for id, want := range map[int]partitionMeta{
		2:   {dimension: 0, label: "Overland", players: 5, playerHardCap: 60, queue: 2},
		8:   {dimension: 0, label: "DeepDesert_0", players: 2, playerHardCap: 80, queue: 0},
		143: {dimension: 1, label: "DeepDesert_1", players: 0, playerHardCap: 40, queue: 1},
		3:   {dimension: 0, label: "Arrakeen_0", players: 7, playerHardCap: 80, queue: 0},
	} {
		got, ok := out[id]
		if !ok {
			t.Errorf("partition %d missing", id)
			continue
		}
		if got != want {
			t.Errorf("partition %d = %+v, want %+v", id, got, want)
		}
	}
	if len(out) != 4 {
		t.Errorf("collected %d partitions, want 4: %+v", len(out), out)
	}
}
