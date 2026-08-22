package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A docker install whose game-server args carry no -PartitionIndex parses every
// container to partition 0 (parseAMPArgInt returns 0 on a miss). A
// partition-keyed restart then matches the first container in the list and
// restarts the wrong map, silently. That was latent while the Battlegroup tab
// hid the per-map button on docker; unhiding it made it reachable.
//
// The target now carries the map, which on docker is the container identity, so
// the row the operator clicked is the container that cycles.

// dockerPSNoPartitionArgs is the reporter's layout with args that expose a port
// but no partition index — the case that collapses every row to 0.
const dockerPSNoPartitionArgs = "dune-server-deepdesert-1-8\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n" +
	"dune-server-overmap\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n" +
	"dune-server-survival-1\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n"

// recordingDockerExec is dockerScript plus a record of every container name a
// `docker restart` was issued against, so a test can assert that exactly one
// container cycled — and which.
func recordingDockerExec(psOut string, args map[string]string, ran *[]string) *dialingExecutor {
	script := dockerScript(psOut, args)
	inner := script.fn
	return &dialingExecutor{&fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "docker restart") {
			for _, line := range strings.Split(psOut, "\n") {
				name, _, found := strings.Cut(strings.TrimSpace(line), "\t")
				if found && name != "" && strings.Contains(cmd, name) {
					*ran = append(*ran, cmd)
				}
			}
			return "", nil
		}
		return inner(cmd)
	}}}
}

// TestDockerRestartTarget_MapWinsOverAmbiguousPartition — with three rows all
// parsing to partition 0, the map must decide which container restarts.
func TestDockerRestartTarget_MapWinsOverAmbiguousPartition(t *testing.T) {
	t.Parallel()
	var ran []string
	exec := recordingDockerExec(dockerPSNoPartitionArgs, nil, &ran)
	c := &dockerControl{}

	// Every row reports partition 0; only the map separates them.
	if _, err := c.RestartPartition(context.Background(), exec,
		restartTarget{Partition: 0, Map: "overmap"}); err != nil {
		t.Fatalf("RestartPartition: %v", err)
	}

	if len(ran) != 1 {
		t.Fatalf("restarted %d containers, want exactly 1: %v", len(ran), ran)
	}
	if !strings.Contains(ran[0], "dune-server-overmap") {
		t.Fatalf("restarted %q, want the container for the overmap row", ran[0])
	}
}

// TestDockerRestartTarget_AmbiguousPartitionWithoutMapErrors — refusing beats
// guessing. Restarting an arbitrary map is worse than saying which rows collide.
func TestDockerRestartTarget_AmbiguousPartitionWithoutMapErrors(t *testing.T) {
	t.Parallel()
	var ran []string
	exec := recordingDockerExec(dockerPSNoPartitionArgs, nil, &ran)
	c := &dockerControl{}

	_, err := c.RestartPartition(context.Background(), exec, restartTarget{Partition: 0})
	if err == nil {
		t.Fatal("want an error when three containers share partition 0 and no map is given")
	}
	if len(ran) != 0 {
		t.Fatalf("nothing may be restarted when the target is ambiguous, ran: %v", ran)
	}
	// The message has to name the collision, or the operator cannot act on it.
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %q, want it to say the target is ambiguous", err)
	}
}

// TestDockerRestartTarget_PartitionStillWorksWhenUnique — the normal path,
// where args do carry -PartitionIndex, must be unchanged.
func TestDockerRestartTarget_PartitionStillWorksWhenUnique(t *testing.T) {
	t.Parallel()
	var ran []string
	args := map[string]string{
		"dune-server-overmap":        "-PartitionIndex=1 -Port=7777",
		"dune-server-survival-1":     "-PartitionIndex=2 -Port=7778",
		"dune-server-deepdesert-1-8": "-PartitionIndex=3 -Port=7779",
	}
	exec := recordingDockerExec(dockerPSNoPartitionArgs, args, &ran)
	c := &dockerControl{}

	if _, err := c.RestartPartition(context.Background(), exec, restartTarget{Partition: 2}); err != nil {
		t.Fatalf("RestartPartition: %v", err)
	}
	if len(ran) != 1 || !strings.Contains(ran[0], "dune-server-survival-1") {
		t.Fatalf("restarted %v, want only dune-server-survival-1", ran)
	}
}

// TestDockerRestartTarget_UnknownMapErrors — a map that matches nothing must
// not silently fall through to a partition match.
func TestDockerRestartTarget_UnknownMapErrors(t *testing.T) {
	t.Parallel()
	var ran []string
	exec := recordingDockerExec(dockerPSNoPartitionArgs, nil, &ran)
	c := &dockerControl{}

	if _, err := c.RestartPartition(context.Background(), exec,
		restartTarget{Partition: 0, Map: "arrakeen"}); err == nil {
		t.Fatal("want an error for a map with no matching container")
	}
	if len(ran) != 0 {
		t.Fatalf("nothing may be restarted for an unknown map, ran: %v", ran)
	}
}

// firstContainer feeds CaptureJWT and ReadDefaultINI, which docker exec into
// the container. Discovery lists with `docker ps -a`, so the first entry can be
// exited while another map is up — and exec'ing a stopped container fails for
// no reason. Prefer a running one.
func TestDockerFirstContainer_PrefersRunningContainer(t *testing.T) {
	t.Parallel()
	ps := "dune-server-deepdesert-1-8\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\texited\n" +
		"dune-server-overmap\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n"
	c := &dockerControl{}

	got, err := c.firstContainer(dockerScript(ps, nil))
	if err != nil {
		t.Fatalf("firstContainer: %v", err)
	}
	if got != "dune-server-overmap" {
		t.Fatalf("firstContainer = %q, want the running container", got)
	}
}

// All stopped is still a usable answer: the caller's docker exec will fail with
// docker's own message, which beats inventing one here.
func TestDockerFirstContainer_FallsBackWhenNoneRunning(t *testing.T) {
	t.Parallel()
	ps := "dune-server-overmap\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\texited\n"
	c := &dockerControl{}

	got, err := c.firstContainer(dockerScript(ps, nil))
	if err != nil {
		t.Fatalf("firstContainer: %v", err)
	}
	if got != "dune-server-overmap" {
		t.Fatalf("firstContainer = %q, want the only container even though it is stopped", got)
	}
}

// The name-prefix fallback only runs when no image matches seabass-server
// (retagged or privately built images). It must carry the same exclusions the
// image match gives for free, or it sweeps in the gateway and text-router —
// which is exactly why detection keys off the image in the first place.
func TestDockerSelectGameContainers_PrefixFallbackExcludesSupportContainers(t *testing.T) {
	t.Parallel()
	ps := "dune-server-gateway\tmyregistry/private-gateway:1\trunning\n" +
		"dune-server-overmap\tmyregistry/private-dune:1\trunning\n" +
		"dune-server-survival-1\tmyregistry/private-dune:1\trunning\n" +
		"dune-server-text-router\tmyregistry/private-text-router:1\trunning\n"
	entries, err := listDockerContainers(dockerScript(ps, nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}

	var got []string
	for _, e := range selectGameContainers(entries) {
		got = append(got, e.name)
	}
	want := []string{"dune-server-overmap", "dune-server-survival-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("prefix fallback selected %v, want %v", got, want)
	}
}

// Director metadata is keyed by partition, so the partition-0 collapse hits
// GetStatus too: every row would take partition 0's players, dimension and
// label and display them as its own. Refusing the restart but still showing
// three maps with identical, wrong player counts fixes half the bug.
//
// Enrichment is applied only where the partition identifies exactly one
// container. A blank column is honest; a fabricated one is not.
func TestDockerGetStatus_SkipsDirectorEnrichmentWhenPartitionsCollide(t *testing.T) {
	t.Parallel()
	dir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"servers":[{"partition":{"partitionId":0,"dimensionIndex":4,"label":"Hagga Basin"},
			"numPlayersInGame":37,"numPlayersInQueue":9}]}`)
	}))
	defer dir.Close()

	// No -PartitionIndex in any container's args, so all three parse to 0.
	c := &dockerControl{directorURL: dir.URL}
	st, err := c.GetStatus(context.Background(), dockerScript(dockerPSNoPartitionArgs, nil))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 3 {
		t.Fatalf("got %d rows, want 3", len(st.Servers))
	}
	for _, row := range st.Servers {
		if row.Players != 0 || row.Dimension != 0 || row.Queue != 0 || row.Sietch != "" {
			t.Errorf("row %q got enriched from an ambiguous partition: %+v", row.Map, row)
		}
	}
}

// The normal path must keep enriching: unique partitions still get their
// players, dimension and label.
func TestDockerGetStatus_EnrichesWhenPartitionsAreUnique(t *testing.T) {
	t.Parallel()
	dir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"servers":[{"partition":{"partitionId":2,"dimensionIndex":4,"label":"Hagga Basin"},
			"numPlayersInGame":37,"numPlayersInQueue":9}]}`)
	}))
	defer dir.Close()

	args := map[string]string{
		"dune-server-overmap":        `["-PartitionIndex=1"]`,
		"dune-server-survival-1":     `["-PartitionIndex=2"]`,
		"dune-server-deepdesert-1-8": `["-PartitionIndex=3"]`,
	}
	c := &dockerControl{directorURL: dir.URL}
	st, err := c.GetStatus(context.Background(), dockerScript(dockerPSNoPartitionArgs, args))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	var found bool
	for _, row := range st.Servers {
		if row.Partition != 2 {
			continue
		}
		found = true
		if row.Players != 37 || row.Dimension != 4 || row.Queue != 9 || row.Sietch != "Hagga Basin" {
			t.Fatalf("partition 2 row not enriched: %+v", row)
		}
	}
	if !found {
		t.Fatal("no row reported partition 2")
	}
}

// A name in docker_gameservers that docker does not know about is deliberately
// still listed, so the operator sees the stale entry rather than wondering
// where their container went. But it has no args to parse, so it lands on
// partition 0 — and if it counts toward the collision math it manufactures a
// false ambiguity that suppresses enrichment for a real partition-0 container
// and can be matched by a partition-keyed restart.
//
// A container docker cannot see has no partition identity. It stays in the
// table; it stays out of the arithmetic.
func TestDockerGetStatus_StaleConfiguredNameDoesNotFakeACollision(t *testing.T) {
	t.Parallel()
	dir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"servers":[{"partition":{"partitionId":0,"dimensionIndex":4,"label":"Hagga Basin"},
			"numPlayersInGame":37,"numPlayersInQueue":9}]}`)
	}))
	defer dir.Close()

	ps := "dune-server-overmap\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n"
	c := &dockerControl{
		// "dune-server-gone" was removed since setup pinned it.
		gameservers: []string{"dune-server-overmap", "dune-server-gone"},
		directorURL: dir.URL,
	}
	st, err := c.GetStatus(context.Background(), dockerScript(ps, map[string]string{
		"dune-server-overmap": `["-PartitionIndex=0"]`,
	}))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(st.Servers) != 2 {
		t.Fatalf("got %d rows, want 2 — the stale name must stay visible", len(st.Servers))
	}
	var overmap *ServerRow
	for i := range st.Servers {
		if st.Servers[i].Map == "overmap" {
			overmap = &st.Servers[i]
		}
	}
	if overmap == nil {
		t.Fatal("no overmap row")
	}
	// Partition 0 is unique among containers docker can actually see.
	if overmap.Players != 37 || overmap.Dimension != 4 || overmap.Sietch != "Hagga Basin" {
		t.Fatalf("overmap lost its enrichment to a phantom collision: %+v", *overmap)
	}
}

// The same phantom must not be restartable by partition — `docker restart` on a
// container that does not exist is a guaranteed failure, and picking it over a
// real container is worse than picking nothing.
func TestDockerRestartTarget_StaleConfiguredNameIsNotAPartitionMatch(t *testing.T) {
	t.Parallel()
	ps := "dune-server-overmap\tregistry.funcom.com/funcom/self-hosting/seabass-server:2048594-0-shipping\trunning\n"
	var ran []string
	exec := recordingDockerExec(ps, map[string]string{
		"dune-server-overmap": `["-PartitionIndex=0"]`,
	}, &ran)
	c := &dockerControl{gameservers: []string{"dune-server-gone", "dune-server-overmap"}}

	if _, err := c.RestartPartition(context.Background(), exec, restartTarget{Partition: 0}); err != nil {
		t.Fatalf("RestartPartition: %v", err)
	}
	if len(ran) != 1 || !strings.Contains(ran[0], "dune-server-overmap") {
		t.Fatalf("restarted %v, want only the container docker knows about", ran)
	}
}
