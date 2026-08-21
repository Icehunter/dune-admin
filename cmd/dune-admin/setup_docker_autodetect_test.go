package main

import (
	"strconv"
	"strings"
	"testing"
)

// The wizard used to pin the detected container names into docker_gameservers.
// That froze the layout at setup time: the reporter entered Arrakeen after
// finishing setup and the new map server never showed up on the panel, because
// an explicit list short-circuits the runtime image-based discovery that would
// have found it (#311).
//
// Pinning is now opt-in. Pressing Enter records nothing and leaves discovery in
// charge on every poll, which is what makes a map server started later appear
// on its own.

// assertNameList compares resolved container-name lists. The docker tests'
// assertNames works on []dockerGameContainer; these helpers return plain names.
func assertNameList(t *testing.T, label string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func issue311Entries(t *testing.T) []dockerPSEntry {
	t.Helper()
	entries, err := listDockerContainers(dockerScript(issue311DockerPS, nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}
	return entries
}

// TestResolveDockerGameservers_BlankKeepsAutoDetect — the default answer must
// leave docker_gameservers empty. Persisting the three detected names here is
// precisely what hid the fourth map server.
func TestResolveDockerGameservers_BlankKeepsAutoDetect(t *testing.T) {
	t.Parallel()
	got := resolveDockerGameservers("", issue311Entries(t))

	if len(got.pinned) != 0 {
		t.Fatalf("pinned = %v, want nothing pinned so discovery stays live", got.pinned)
	}
	if !got.autoDetect() {
		t.Fatal("blank answer must resolve to auto-detect")
	}
	if !got.usable() {
		t.Fatal("auto-detect found game servers, so the choice is usable")
	}
	// The detected set is still reported, so the operator can see what the
	// runtime will find without it being written to config.
	assertNameList(t, "detected", got.detected, []string{
		"dune-server-deepdesert-1-8", "dune-server-overmap", "dune-server-survival-1",
	})
}

// TestResolveDockerGameservers_ExplicitPin — an operator whose layout defeats
// image detection can still name the containers, and that must win.
func TestResolveDockerGameservers_ExplicitPin(t *testing.T) {
	t.Parallel()
	entries := issue311Entries(t)
	// Indices are 1-based over the listing order (see issue311DockerPS).
	var want []string
	var sel []string
	for i, e := range entries {
		if e.name == "dune-server-overmap" || e.name == "dune-server-survival-1" {
			want = append(want, e.name)
			sel = append(sel, strconv.Itoa(i+1))
		}
	}
	got := resolveDockerGameservers(strings.Join(sel, ","), entries)

	if got.autoDetect() {
		t.Fatal("an explicit selection must not resolve to auto-detect")
	}
	if !got.usable() {
		t.Fatal("an explicit selection is usable")
	}
	assertNameList(t, "pinned", got.pinned, want)
}

// TestResolveDockerGameservers_JunkFallsBackToAutoDetect — a typo must not
// silently pin an empty list and leave the panel showing no game servers.
func TestResolveDockerGameservers_JunkFallsBackToAutoDetect(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"  ", "none", "99", "0", ",,"} {
		got := resolveDockerGameservers(answer, issue311Entries(t))
		if !got.autoDetect() || !got.usable() {
			t.Fatalf("answer %q: got pinned=%v autoDetect=%v usable=%v, want live auto-detect",
				answer, got.pinned, got.autoDetect(), got.usable())
		}
	}
}

// TestResolveDockerGameservers_NothingDetectable — with no game server found
// and none named, the wizard has nothing to fall back on and must say so
// rather than reporting a working setup.
func TestResolveDockerGameservers_NothingDetectable(t *testing.T) {
	t.Parallel()
	entries, err := listDockerContainers(dockerScript(
		"dune-postgres\tigw-postgres:17.4\trunning\n"+
			"portainer_agent\tportainer/agent:2.39.2\trunning\n", nil))
	if err != nil {
		t.Fatalf("listDockerContainers: %v", err)
	}
	got := resolveDockerGameservers("", entries)

	if got.usable() {
		t.Fatal("no containers detected and none pinned must not report a usable selection")
	}
	if len(got.detected) != 0 {
		t.Fatalf("detected = %v, want none", got.detected)
	}
}

// TestSetupDockerContainers_DefaultAnswerLeavesConfigEmpty drives the prompt
// itself: accepting every default must not write docker_gameservers. This is
// the end-to-end form of the bug — the wizard, not the helper, is what wrote
// the frozen list.
func TestSetupDockerContainers_DefaultAnswerLeavesConfigEmpty(t *testing.T) {
	t.Parallel()
	var cfg appConfig
	var failures []string
	ask := func(_, def string) string { return def } // operator presses Enter throughout
	setupDockerContainers(ask,
		func(string) {},
		func(m string) { failures = append(failures, m) },
		&cfg, dockerScript(issue311DockerPS, nil))

	if len(cfg.DockerGameservers) != 0 {
		t.Fatalf("DockerGameservers = %v, want empty so runtime discovery stays live", cfg.DockerGameservers)
	}
	if len(failures) != 0 {
		t.Fatalf("unexpected failure messages: %v", failures)
	}
	// The rest of the discovery must still be offered as defaults.
	if cfg.DockerBrokerGame != "dune-rmq-game" || cfg.DockerBrokerAdmin != "dune-rmq-admin" {
		t.Fatalf("broker defaults = %q / %q, want dune-rmq-game / dune-rmq-admin",
			cfg.DockerBrokerGame, cfg.DockerBrokerAdmin)
	}
	if cfg.DirectorURL == "" {
		t.Fatal("director container is present in the listing, so its URL must be defaulted")
	}
}
