package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os/exec"
)

// ampExecutor wraps any Executor with sudo-elevated file writes. The dune-admin
// process typically runs as a non-AMP user (e.g. mehdune) and cannot write
// UserGame.ini directly — that file is owned by the AMP user. WriteFile pipes
// content through `sudo -i -u <ampUser> tee`, which the sudoers grant allows.
//
// All other methods delegate to the inner executor unchanged, so ampExecutor
// works whether the inner executor is a localExecutor or an sshExecutor.
type ampExecutor struct {
	Executor        // inner: *localExecutor or *sshExecutor
	ampUser  string // OS user to write files as (default "amp")
}

func (e *ampExecutor) Type() string { return "amp" }

func (e *ampExecutor) WriteFile(path string, data io.Reader) error {
	if e.ampUser == "" {
		return fmt.Errorf("amp executor requires amp_user to be configured")
	}
	cmd := fmt.Sprintf("sudo -i -u %s tee %s > /dev/null", shellQuote(e.ampUser), shellQuote(path))
	if sshExec, ok := e.Executor.(*sshExecutor); ok {
		sess, err := sshExec.client.NewSession()
		if err != nil {
			return err
		}
		defer func() { _ = sess.Close() }()
		stdin, err := sess.StdinPipe()
		if err != nil {
			return err
		}
		var errBuf bytes.Buffer
		sess.Stderr = &errBuf
		if err := sess.Start(cmd); err != nil {
			return err
		}
		if _, err := io.Copy(stdin, data); err != nil {
			return err
		}
		_ = stdin.Close()
		if err := sess.Wait(); err != nil {
			return fmt.Errorf("sudo tee %s: %w — %s", path, err, errBuf.String())
		}
		return nil
	}
	c := exec.Command("sudo", "-i", "-u", e.ampUser, "tee", path) // #nosec G204,G702 -- args passed as slice (no shell); ampUser and path are admin-supplied config
	c.Stdin = data
	c.Stdout = io.Discard
	var errBuf bytes.Buffer
	c.Stderr = &errBuf
	err := c.Run()
	if err != nil {
		return fmt.Errorf("sudo tee %s: %w — %s", path, err, errBuf.String())
	}
	return nil
}

// Dial delegates to the inner executor so SSH-tunnelled TCP connections work.
func (e *ampExecutor) Dial(network, addr string) (net.Conn, error) {
	return e.Executor.Dial(network, addr)
}
