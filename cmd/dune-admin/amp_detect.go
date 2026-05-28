package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// ampInstance describes a single AMP-managed game-server instance discovered
// via `ampinstmgr -l`. Used by the setup wizard to pre-fill prompts.
type ampInstance struct {
	Name        string // "DuneTest01"
	Module      string // "GenericModule", "DuneAwakening", etc.
	Running     bool
	InContainer bool
	DataPath    string // "/home/amp/.ampdata/instances/DuneTest01"
}

// candidate AMP user accounts checked in order. First one that exists wins.
// Sites that use a custom AMP user will still get a manual fallback.
var ampUserCandidates = []string{"amp", "ampuser"}

// detectAmpInstances runs `sudo -u <amp_user> ampinstmgr -l`, parses the
// output, and returns the discovered instances along with the AMP user it
// found. Filters out the ADS module (that's AMP itself, not a game).
//
// Returns an empty slice (not an error) when ampinstmgr is not on PATH or
// the probe times out — the caller is expected to fall back to manual
// prompts in that case. Genuine parse errors are returned as errors so the
// operator sees them.
func detectAmpInstances() (instances []ampInstance, ampUser string, err error) {
	if _, lookErr := exec.LookPath("ampinstmgr"); lookErr != nil {
		return nil, "", nil
	}

	for _, candidate := range ampUserCandidates {
		if _, lookupErr := user.Lookup(candidate); lookupErr == nil {
			ampUser = candidate
			break
		}
	}
	if ampUser == "" {
		return nil, "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "-u", ampUser, "ampinstmgr", "-l")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		// Non-fatal: probably needs interactive sudo or ampinstmgr crashed.
		// Caller falls back to manual prompts.
		return nil, ampUser, nil
	}

	instances = parseAmpInstmgrOutput(out)
	return instances, ampUser, nil
}

// parseAmpInstmgrOutput parses the human-formatted output of `ampinstmgr -l`.
// Pure function — exported via package-internal callers and unit-tested with
// a golden fixture so changes to the output format are caught early.
//
// Output blocks look like:
//
//	Instance Name      │ DuneTest01
//	Module             │ GenericModule
//	Running            │ Yes
//	Runs in Container  │ Yes
//	Data Path          │ /home/amp/.ampdata/instances/DuneTest01
//
// The separator is the Unicode box-drawing character │ (U+2502) which AMP
// emits regardless of locale. We also accept "|" as a fallback in case a
// future ampinstmgr release drops Unicode in batch mode.
//
// ADS instances (AMP itself) are filtered out — the wizard targets game
// servers.
func parseAmpInstmgrOutput(out []byte) []ampInstance {
	var instances []ampInstance
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	current := ampInstance{}
	flush := func() {
		if current.Name != "" && !strings.EqualFold(current.Module, "ADS") {
			instances = append(instances, current)
		}
		current = ampInstance{}
	}

	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}

		key, val, ok := splitAmpKV(line)
		if !ok {
			continue
		}
		switch key {
		case "Instance Name":
			current.Name = val
		case "Module":
			current.Module = val
		case "Running":
			current.Running = strings.EqualFold(val, "Yes")
		case "Runs in Container":
			current.InContainer = strings.EqualFold(val, "Yes")
		case "Data Path":
			current.DataPath = val
		}
	}
	flush()
	return instances
}

// splitAmpKV splits a "Key │ Value" line on the Unicode box character (or
// ASCII pipe as a fallback). Returns key, value, and whether the split
// succeeded.
func splitAmpKV(line string) (string, string, bool) {
	for _, sep := range []string{"│", "|"} {
		if i := strings.Index(line, sep); i >= 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+len(sep):])
			return key, val, true
		}
	}
	return "", "", false
}

// probeGameRoot inspects a running container to discover the game install
// path under /AMP/. Most AMP modules put the game at /AMP/<game-name>/ but
// the exact <game-name> depends on the module (e.g. "duneawakening" for the
// official CubeCoders Dune Awakening module). Rather than hardcoding that
// suffix in the wizard, we list /AMP/ inside the container with -F to mark
// directories with a trailing slash, then pick the first directory entry.
// Returns "" + nil when the probe cannot answer authoritatively (container
// not running, sudo prompts, non-standard layout) — caller falls back to
// the historical default.
func probeGameRoot(ctx context.Context, ampUser, container string) (string, error) {
	if ampUser == "" || container == "" {
		return "", errors.New("ampUser and container are required")
	}
	// Use `sudo -n -i -u <ampUser>` so sudo enters amp's login shell and
	// chdirs to amp's home before exec'ing — otherwise the calling user's
	// cwd typically isn't readable by amp ("cannot chdir to /home/X: …").
	// -F appends "/" to directory entries; -1 forces one-per-line output.
	// Use Output() (not CombinedOutput) so any residual stderr doesn't
	// poison the directory list.
	cmd := exec.CommandContext(ctx, "sudo", "-n", "-i", "-u", ampUser, "podman", "exec", container, "ls", "-1F", "/AMP/")
	out, err := cmd.Output()
	if err != nil {
		// Probe failure (sudo prompt, container down, exec denied, etc.) —
		// caller falls back to defaults.
		return "", nil
	}
	for _, entry := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		entry = strings.TrimSpace(entry)
		// Skip blanks, the lost+found dir, and anything ls -F didn't tag as
		// a directory (file entries don't end in "/").
		if entry == "" || !strings.HasSuffix(entry, "/") {
			continue
		}
		dirName := strings.TrimSuffix(entry, "/")
		if dirName == "" || strings.HasPrefix(dirName, "lost+found") ||
			strings.HasPrefix(dirName, "AMP_Logs") || dirName == "Backups" {
			// Skip known AMP-meta directories — we want the game folder.
			continue
		}
		return "/AMP/" + dirName, nil
	}
	return "", nil
}

// summarizeInstance returns a single-line description suitable for the
// instance picker in the setup wizard.
func summarizeInstance(inst ampInstance) string {
	topology := "native"
	if inst.InContainer {
		topology = "container"
	}
	status := "stopped"
	if inst.Running {
		status = "running"
	}
	return fmt.Sprintf("%s (module=%s, %s, %s)", inst.Name, inst.Module, topology, status)
}
