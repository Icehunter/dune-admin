package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
)

// ampExecutor wraps localExecutor with sudo-elevated file writes. The dune-admin
// process typically runs as a non-AMP user (e.g. mehdune) and cannot write
// UserGame.ini directly — that file is owned by the AMP user. WriteFile pipes
// content through `sudo -i -u <ampUser> tee`, which the sudoers grant allows.
//
// Exec, Stream, PipeToWriter, and Dial inherit from localExecutor unchanged.
type ampExecutor struct {
	*localExecutor
	ampUser string // OS user to write files as (default "amp")
}

func (e *ampExecutor) Type() string { return "amp" }

func (e *ampExecutor) WriteFile(path string, data io.Reader) error {
	if e.ampUser == "" {
		return fmt.Errorf("amp executor requires amp_user to be configured")
	}
	// Buffer the payload so stdin to `tee` is a fixed reader. Sizes here are
	// INI files, capped well below memory concerns.
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, data); err != nil {
		return fmt.Errorf("read payload: %w", err)
	}
	c := exec.Command("sudo", "-i", "-u", e.ampUser, "tee", path) // #nosec G204,G702 -- args passed as slice (no shell); ampUser and path are admin-supplied config
	c.Stdin = &buf
	c.Stdout = io.Discard
	var errBuf bytes.Buffer
	c.Stderr = &errBuf
	if err := c.Run(); err != nil {
		return fmt.Errorf("sudo tee %s: %w — %s", path, err, errBuf.String())
	}
	return nil
}
