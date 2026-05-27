package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Executor abstracts where commands run and how TCP connections are made.
// localExecutor runs everything on the same machine; sshExecutor tunnels
// through an SSH connection to a remote host.
type Executor interface {
	Exec(cmd string) (string, error)
	Stream(cmd string) (<-chan string, func(), error)
	PipeToWriter(cmd string, w io.Writer) error
	WriteFile(path string, data io.Reader) error
	Dial(network, addr string) (net.Conn, error)
	Close()
	// Type returns "local" or "ssh" for status reporting.
	Type() string
}

// newExecutor returns an sshExecutor when sshHost is non-empty, otherwise
// a localExecutor. The SSH connection is established immediately; the error
// must be checked before using the executor.
func newExecutor(sshHost, sshUser, sshKeyPath string) (Executor, error) {
	if sshHost == "" {
		return &localExecutor{}, nil
	}
	client, err := dialSSH(sshHost, sshUser, sshKeyPath)
	if err != nil {
		return nil, err
	}
	return &sshExecutor{client: client}, nil
}

// ── SSH executor ──────────────────────────────────────────────────────────────

type sshExecutor struct {
	client *ssh.Client
}

func (e *sshExecutor) Type() string { return "ssh" }

func (e *sshExecutor) Close() {
	if e.client != nil {
		_ = e.client.Close()
	}
}

func (e *sshExecutor) Exec(cmd string) (string, error) {
	sess, err := e.client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close() }()
	out, err := sess.CombinedOutput(cmd)
	return strings.TrimSpace(string(out)), err
}

func (e *sshExecutor) Stream(cmd string) (<-chan string, func(), error) {
	sess, err := e.client.NewSession()
	if err != nil {
		return nil, func() {}, err
	}
	pipe, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, func() {}, err
	}
	if err := sess.Start(cmd); err != nil {
		_ = sess.Close()
		return nil, func() {}, err
	}
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(pipe)
		for sc.Scan() {
			ch <- sc.Text()
		}
		_ = sess.Wait()
	}()
	return ch, func() { _ = sess.Close() }, nil
}

func (e *sshExecutor) PipeToWriter(cmd string, w io.Writer) error {
	sess, err := e.client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()
	sess.Stdout = w
	return sess.Run(cmd)
}

func (e *sshExecutor) WriteFile(path string, data io.Reader) error {
	sess, err := e.client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	if err := sess.Start(fmt.Sprintf("sudo tee %s > /dev/null", shellQuote(path))); err != nil {
		return err
	}
	if _, err := io.Copy(stdin, data); err != nil {
		return err
	}
	_ = stdin.Close()
	return sess.Wait()
}

func (e *sshExecutor) Dial(network, addr string) (net.Conn, error) {
	return e.client.Dial(network, addr)
}

// ── Local executor ────────────────────────────────────────────────────────────

type localExecutor struct{}

func (e *localExecutor) Type() string { return "local" }
func (e *localExecutor) Close()       {}

func (e *localExecutor) Exec(cmd string) (string, error) {
	c := exec.Command("sh", "-c", cmd)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return strings.TrimSpace(buf.String()), err
}

func (e *localExecutor) Stream(cmd string) (<-chan string, func(), error) {
	c := exec.Command("sh", "-c", cmd)
	pipe, err := c.StdoutPipe()
	if err != nil {
		return nil, func() {}, err
	}
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return nil, func() {}, err
	}
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(pipe)
		for sc.Scan() {
			ch <- sc.Text()
		}
		_ = c.Wait()
	}()
	cancel := func() {
		if c.Process != nil {
			_ = c.Process.Kill()
		}
	}
	return ch, cancel, nil
}

func (e *localExecutor) PipeToWriter(cmd string, w io.Writer) error {
	c := exec.Command("sh", "-c", cmd) // #nosec G702 -- all callers build cmd via shellQuote
	c.Stdout = w
	var errBuf bytes.Buffer
	c.Stderr = &errBuf
	return c.Run()
}

func (e *localExecutor) WriteFile(path string, data io.Reader) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644) // #nosec G304 -- path comes from admin config
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, data)
	return err
}

func (e *localExecutor) Dial(network, addr string) (net.Conn, error) {
	return net.Dial(network, addr)
}

// ── SSH dialer (used by newExecutor and setup wizard) ─────────────────────────

func dialSSH(host, user, keyPath string) (*ssh.Client, error) {
	keyData, err := os.ReadFile(keyPath) // #nosec G304 -- keyPath is admin-supplied config
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", keyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}
	client, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 -- private admin tool, known host
	})
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", host, err)
	}
	return client, nil
}
