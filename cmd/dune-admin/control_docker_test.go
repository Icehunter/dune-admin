package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The container/image names below are taken verbatim from issue #311 — a stock
// redblinks dune-docker install. Three containers share the plain
// `seabass-server` image and are the real game servers; the gateway and
// text-router carry distinct images and must never be treated as game servers.
const issue311DockerPS = "dune-autoscaler\tdune-orchestrator:dev\trunning\n" +
	"dune-director\tdune-director-compat:2048594-0-shipping\trunning\n" +
	"dune-postgres\tregistry.funcom.com/funcom/self-hosting/igw-postgres:17.4-alpine-fc-13\trunning\n" +
	"dune-rmq-admin\tregistry.funcom.com/funcom/self-hosting/seabass-server-rabbitmq:2048594-0-shipping\trunning\n" +
	"dune-rmq-game\tregistry.funcom.com/funcom/self-hosting/seabass-server-rabbitmq:2048594-0-shipping\trunning\n" +
	"dune-server-deepdesert-1-8\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n" +
	"dune-server-gateway\tregistry.funcom.com/funcom/self-hosting/seabass-server-gateway:2048594-0-shipping\trunning\n" +
	"dune-server-overmap\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n" +
	"dune-server-survival-1\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n" +
	"dune-text-router\tregistry.funcom.com/funcom/self-hosting/seabass-server-text-router:2048594-0-shipping\trunning\n" +
	"portainer_agent\tportainer/agent:2.39.2\trunning\n"

// dialingExecutor is an fnExecutor that really dials. The director HTTP client
// is routed through the executor, so GetStatus tests that hit a loopback
// httptest server need a working Dial (fnExecutor's returns nil).
type dialingExecutor struct{ *fnExecutor }

func (d *dialingExecutor) Dial(network, addr string) (net.Conn, error) {
	return net.Dial(network, addr)
}

// dockerInspectOutput reproduces `docker inspect --format '{{.Name}}\t{{json .Args}}'`
// over several containers at once: one line each, name prefixed with a slash.
// Only containers actually named in the command are echoed back.
func dockerInspectOutput(cmd string, args map[string]string) string {
	var b strings.Builder
	for name, a := range args {
		if strings.Contains(cmd, name) {
			fmt.Fprintf(&b, "/%s\t%s\n", name, a)
		}
	}
	return b.String()
}

// dockerScript routes `docker ps` to a fixed listing and `docker inspect` to
// per-container args, so discovery can be driven end to end.
func dockerScript(psOut string, args map[string]string) *dialingExecutor {
	return &dialingExecutor{&fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "docker ps"):
			return psOut, nil
		case strings.Contains(cmd, "docker inspect"):
			return dockerInspectOutput(cmd, args), nil
		}
		return "", nil
	}}}
}

func containerNames(cs []dockerGameContainer) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.name)
	}
	return out
}

func assertNames(t *testing.T, got []dockerGameContainer, want ...string) {
	t.Helper()
	names := containerNames(got)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("containers = %v, want %v", names, want)
	}
}

// Auto-detection must key off the image, not the container name: a
// `dune-server-` name prefix would wrongly sweep in dune-server-gateway.
func TestDockerDiscover_AutoDetectByImage(t *testing.T) {
	c := &dockerControl{}
	got, err := c.discoverGameContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("discoverGameContainers: %v", err)
	}
	assertNames(t, got, "dune-server-deepdesert-1-8", "dune-server-overmap", "dune-server-survival-1")
}

func TestDockerDiscover_ExplicitListOverridesAutoDetect(t *testing.T) {
	c := &dockerControl{gameservers: []string{"custom-a", "custom-b"}}
	got, err := c.discoverGameContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("discoverGameContainers: %v", err)
	}
	assertNames(t, got, "custom-a", "custom-b")
}

// Back-compat: an existing config with the singular docker_gameserver key must
// keep behaving exactly as before.
func TestDockerDiscover_SingleGameserverBackCompat(t *testing.T) {
	c := &dockerControl{gameserver: "dune-gameserver"}
	got, err := c.discoverGameContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("discoverGameContainers: %v", err)
	}
	assertNames(t, got, "dune-gameserver")
}

// When no image matches (a non-Funcom registry, a retagged image), fall back to
// the dune-server- name prefix rather than discovering nothing.
func TestDockerDiscover_NamePrefixFallback(t *testing.T) {
	ps := "dune-server-overmap\tmyreg/custom-build:latest\trunning\n" +
		"dune-postgres\tpostgres:17\trunning\n"
	c := &dockerControl{}
	got, err := c.discoverGameContainers(dockerScript(ps, nil))
	if err != nil {
		t.Fatalf("discoverGameContainers: %v", err)
	}
	assertNames(t, got, "dune-server-overmap")
}

func TestDockerDiscover_ParsesPartitionAndPort(t *testing.T) {
	args := map[string]string{
		"dune-server-survival-1": `["-PartitionIndex=1","-Port=7777"]`,
	}
	c := &dockerControl{gameservers: []string{"dune-server-survival-1"}}
	got, err := c.discoverGameContainers(dockerScript(issue311DockerPS, args))
	if err != nil {
		t.Fatalf("discoverGameContainers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d containers, want 1", len(got))
	}
	if got[0].partition != 1 {
		t.Errorf("partition = %d, want 1", got[0].partition)
	}
	if got[0].port != 7777 {
		t.Errorf("port = %d, want 7777", got[0].port)
	}
}

// A container whose args carry no -PartitionIndex must still yield a row —
// otherwise the table goes empty again, which is the whole bug in #311.
func TestDockerGetStatus_RowEvenWithoutArgs(t *testing.T) {
	c := &dockerControl{}
	st, err := c.GetStatus(context.Background(), dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 3 {
		t.Fatalf("got %d servers, want 3", len(st.Servers))
	}
	for _, s := range st.Servers {
		if s.Map == "" {
			t.Error("ServerRow.Map is empty; want container-derived name")
		}
	}
}

func TestDockerGetStatus_ReadyTracksContainerState(t *testing.T) {
	ps := "dune-server-overmap\tseabass-server:1\trunning\n" +
		"dune-server-survival-1\tseabass-server:1\texited\n"
	c := &dockerControl{}
	st, err := c.GetStatus(context.Background(), dockerScript(ps, nil))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(st.Servers))
	}
	byMap := map[string]ServerRow{}
	for _, s := range st.Servers {
		byMap[s.Map] = s
	}
	if !byMap["overmap"].Ready {
		t.Error("running container: Ready = false, want true")
	}
	if byMap["survival-1"].Ready {
		t.Error("exited container: Ready = true, want false")
	}
}

// #311 also showed "Database: —" because dockerControl never set the field.
func TestDockerGetStatus_SetsDatabasePhase(t *testing.T) {
	c := &dockerControl{}
	st, err := c.GetStatus(context.Background(), dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Database == "" {
		t.Error("Database is empty; want a phase string")
	}
}

func TestDockerGetStatus_DirectorEnrichment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"servers":[{"partition":{"partitionId":1,"dimensionIndex":2,"label":"Sietch Tabr"},
			"numPlayersInGame":7,"numPlayersInQueue":3}]}`)
	}))
	defer srv.Close()

	args := map[string]string{"dune-server-survival-1": `["-PartitionIndex=1"]`}
	c := &dockerControl{gameservers: []string{"dune-server-survival-1"}, directorURL: srv.URL}
	st, err := c.GetStatus(context.Background(), dockerScript(issue311DockerPS, args))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(st.Servers))
	}
	got := st.Servers[0]
	if got.Dimension != 2 {
		t.Errorf("Dimension = %d, want 2", got.Dimension)
	}
	if got.Players != 7 {
		t.Errorf("Players = %d, want 7", got.Players)
	}
	if got.Queue != 3 {
		t.Errorf("Queue = %d, want 3", got.Queue)
	}
	if got.Sietch != "Sietch Tabr" {
		t.Errorf("Sietch = %q, want %q", got.Sietch, "Sietch Tabr")
	}
}

// Director enrichment is best-effort: an unreachable director must not blank
// out the server table.
func TestDockerGetStatus_UnreachableDirectorStillReturnsRows(t *testing.T) {
	c := &dockerControl{directorURL: "http://127.0.0.1:1"}
	st, err := c.GetStatus(context.Background(), dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 3 {
		t.Errorf("got %d servers, want 3", len(st.Servers))
	}
}

func TestDockerGetStatus_NoContainersIsNotAnError(t *testing.T) {
	c := &dockerControl{}
	st, err := c.GetStatus(context.Background(), dockerScript("", nil))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 0 {
		t.Errorf("got %d servers, want 0", len(st.Servers))
	}
}

// The core lifecycle bug: restart must cycle every game container, not one.
func TestDockerExecCommand_RestartsEveryContainer(t *testing.T) {
	var restarted []string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "docker ps") {
			return issue311DockerPS, nil
		}
		if strings.Contains(cmd, "docker restart") {
			for _, n := range []string{"dune-server-deepdesert-1-8", "dune-server-overmap", "dune-server-survival-1"} {
				if strings.Contains(cmd, n) {
					restarted = append(restarted, n)
				}
			}
		}
		return "", nil
	}}
	c := &dockerControl{}
	if _, err := c.ExecCommand(context.Background(), exec, "restart"); err != nil {
		t.Fatalf("ExecCommand: %v", err)
	}
	if len(restarted) != 3 {
		t.Errorf("restarted %v, want all 3 game containers", restarted)
	}
}

// One failing container must surface an error without silently skipping the rest.
func TestDockerExecCommand_PartialFailureSurfacesError(t *testing.T) {
	var attempted int
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "docker ps") {
			return issue311DockerPS, nil
		}
		if strings.Contains(cmd, "docker stop") {
			attempted++
			if strings.Contains(cmd, "dune-server-overmap") {
				return "no such container", fmt.Errorf("exit status 1")
			}
		}
		return "", nil
	}}
	c := &dockerControl{}
	_, err := c.ExecCommand(context.Background(), exec, "stop")
	if err == nil {
		t.Fatal("ExecCommand: want error when a container fails, got nil")
	}
	if attempted != 3 {
		t.Errorf("attempted %d containers, want 3 (failure must not abort the rest)", attempted)
	}
}

func TestDockerRestartPartition_TargetsOnlyThatContainer(t *testing.T) {
	var restarted []string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "docker ps"):
			return issue311DockerPS, nil
		case strings.Contains(cmd, "docker inspect"):
			return dockerInspectOutput(cmd, map[string]string{
				"dune-server-survival-1": `["-PartitionIndex=1"]`,
				"dune-server-overmap":    `["-PartitionIndex=2"]`,
			}), nil
		case strings.Contains(cmd, "docker restart"):
			for _, n := range []string{"dune-server-deepdesert-1-8", "dune-server-overmap", "dune-server-survival-1"} {
				if strings.Contains(cmd, n) {
					restarted = append(restarted, n)
				}
			}
		}
		return "", nil
	}}
	c := &dockerControl{}
	if _, err := c.RestartPartition(context.Background(), exec, 2); err != nil {
		t.Fatalf("RestartPartition: %v", err)
	}
	if len(restarted) != 1 || restarted[0] != "dune-server-overmap" {
		t.Errorf("restarted %v, want only dune-server-overmap", restarted)
	}
}

func TestDockerRestartPartition_UnknownPartitionErrors(t *testing.T) {
	c := &dockerControl{}
	_, err := c.RestartPartition(context.Background(), dockerScript(issue311DockerPS, nil), 99)
	if err == nil {
		t.Fatal("RestartPartition: want error for unknown partition, got nil")
	}
}

// ── setup wizard helpers ──────────────────────────────────────────────────────

func TestParseDockerPS_SkipsBlankAndMalformed(t *testing.T) {
	entries, err := listDockerContainers(dockerScript("a\timg\trunning\n\n\nb\timg2\texited\n", nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[1].name != "b" || entries[1].state != "exited" {
		t.Errorf("entries[1] = %+v, want {b img2 exited}", entries[1])
	}
}

// The wizard offers the auto-detected game servers as the default selection so
// the operator confirms rather than guessing a name (#311).
func TestDefaultGameserverSelection(t *testing.T) {
	entries, err := listDockerContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}
	got := defaultGameserverSelection(entries)
	// deepdesert=6, overmap=8, survival=9 in the 1-based issue #311 listing.
	if got != "6,8,9" {
		t.Errorf("defaultGameserverSelection = %q, want %q", got, "6,8,9")
	}
}

func TestParseIndexSelection(t *testing.T) {
	entries := []dockerPSEntry{{name: "a"}, {name: "b"}, {name: "c"}}
	cases := []struct {
		in   string
		want string
	}{
		{"1,3", "a,c"},
		{" 2 , 1 ", "b,a"},
		{"2", "b"},
		{"", ""},
		{"0,4,9", ""}, // out of range indices are dropped
		{"x,1", "a"},  // junk is dropped
		{"1,1", "a"},  // duplicates collapse
	}
	for _, tc := range cases {
		got := strings.Join(parseIndexSelection(tc.in, entries), ",")
		if got != tc.want {
			t.Errorf("parseIndexSelection(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetectBrokerContainers(t *testing.T) {
	entries, err := listDockerContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}
	game, admin := detectBrokerContainers(entries)
	if game != "dune-rmq-game" {
		t.Errorf("game broker = %q, want dune-rmq-game", game)
	}
	if admin != "dune-rmq-admin" {
		t.Errorf("admin broker = %q, want dune-rmq-admin", admin)
	}
}

func TestDetectDirectorContainer(t *testing.T) {
	entries, err := listDockerContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}
	if got := detectDirectorContainer(entries); got != "dune-director" {
		t.Errorf("director = %q, want dune-director", got)
	}
	if got := detectDirectorContainer(nil); got != "" {
		t.Errorf("director = %q, want empty for no containers", got)
	}
}
