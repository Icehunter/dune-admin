package main

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestAppConfigSSHModeRoundTrip(t *testing.T) {
	in := []byte("ssh_mode: command\nssh_extra_opts: \"-o StrictHostKeyChecking=accept-new\"\n")
	var cfg appConfig
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.SSHMode != "command" {
		t.Errorf("SSHMode = %q, want %q", cfg.SSHMode, "command")
	}
	if cfg.SSHExtraOpts != "-o StrictHostKeyChecking=accept-new" {
		t.Errorf("SSHExtraOpts = %q", cfg.SSHExtraOpts)
	}
}

func TestControlOpts(t *testing.T) {
	if got := controlOpts("windows"); got != nil {
		t.Errorf("windows controlOpts = %v, want nil", got)
	}
	got := controlOpts("linux")
	joined := strings.Join(got, " ")
	for _, want := range []string{"ControlMaster=auto", "ControlPersist=60s", "ControlPath="} {
		if !strings.Contains(joined, want) {
			t.Errorf("linux controlOpts %q missing %q", joined, want)
		}
	}
}

func TestSSHExecArgs(t *testing.T) {
	base := []string{"-o", "BatchMode=yes"}
	got := sshExecArgs(base, "dune@vm-dune-01", "echo hi")
	want := []string{"-o", "BatchMode=yes", "dune@vm-dune-01", "--", "echo hi"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sshExecArgs = %v, want %v", got, want)
	}
}

func TestSSHDialArgs(t *testing.T) {
	base := []string{"-o", "BatchMode=yes"}
	got := sshDialArgs(base, "vm-dune-01", "10.0.0.5:5432")
	want := []string{"-o", "BatchMode=yes", "-W", "10.0.0.5:5432", "vm-dune-01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sshDialArgs = %v, want %v", got, want)
	}
}

func TestSSHExecArgsDoesNotAliasBase(t *testing.T) {
	base := []string{"-o", "BatchMode=yes"}
	_ = sshExecArgs(base, "t", "c")
	if len(base) != 2 {
		t.Errorf("base was mutated: %v", base)
	}
}

func TestStdioConnReadWrite(t *testing.T) {
	rIn, wIn := io.Pipe()   // stands in for the process stdin
	rOut, wOut := io.Pipe() // stands in for the process stdout
	c := &stdioConn{
		stdin:  wIn,
		stdout: rOut,
		local:  sshAddr{network: "tcp", addr: "ssh-stdio"},
		remote: sshAddr{network: "tcp", addr: "10.0.0.5:5432"},
	}

	// Write goes to stdin pipe.
	go func() {
		buf := make([]byte, 5)
		_, _ = io.ReadFull(rIn, buf)
		_, _ = wOut.Write(buf) // echo back onto stdout pipe
	}()
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, 5)
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("read = %q, want hello", got)
	}

	// net.Conn surface.
	var _ net.Conn = c
	if c.RemoteAddr().String() != "10.0.0.5:5432" {
		t.Errorf("RemoteAddr = %q", c.RemoteAddr())
	}
	if err := c.SetDeadline(time.Now()); err != nil {
		t.Errorf("SetDeadline = %v, want nil", err)
	}
}

func TestStdioConnCloseIdempotent(t *testing.T) {
	_, wIn := io.Pipe()
	rOut, _ := io.Pipe()
	c := &stdioConn{stdin: wIn, stdout: rOut}
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestBuildSSHCommandExecutorTarget(t *testing.T) {
	e := buildSSHCommandExecutor("vm-dune-01", "dune", "", "")
	if e.target != "dune@vm-dune-01" {
		t.Errorf("target = %q, want dune@vm-dune-01", e.target)
	}
	// alias already contains user → not double-prefixed
	e2 := buildSSHCommandExecutor("dune@vm-dune-01", "dune", "", "")
	if e2.target != "dune@vm-dune-01" {
		t.Errorf("target = %q", e2.target)
	}
}

func TestBuildSSHCommandExecutorPort(t *testing.T) {
	e := buildSSHCommandExecutor("192.168.33.65:2222", "dune", "", "")
	joined := strings.Join(e.base, " ")
	if !strings.Contains(joined, "-p 2222") {
		t.Errorf("base %q missing -p 2222", joined)
	}
	if e.target != "dune@192.168.33.65" {
		t.Errorf("target = %q, want dune@192.168.33.65", e.target)
	}
}

func TestBuildSSHCommandExecutorIdentityOnlyIfExists(t *testing.T) {
	// Non-existent key path must NOT add -i (ssh would otherwise fail).
	e := buildSSHCommandExecutor("vm-dune-01", "dune", "/no/such/key", "")
	if strings.Contains(strings.Join(e.base, " "), "-i") {
		t.Errorf("non-existent key added -i: %v", e.base)
	}
	// Existing key path adds -i.
	f := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(f, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	e2 := buildSSHCommandExecutor("vm-dune-01", "dune", f, "")
	if !strings.Contains(strings.Join(e2.base, " "), "-i "+f) {
		t.Errorf("existing key missing -i: %v", e2.base)
	}
}

func TestBuildSSHCommandExecutorExtraOpts(t *testing.T) {
	e := buildSSHCommandExecutor("vm-dune-01", "dune", "", "-o StrictHostKeyChecking=accept-new")
	joined := strings.Join(e.base, " ")
	if !strings.Contains(joined, "StrictHostKeyChecking=accept-new") {
		t.Errorf("base %q missing extra opt", joined)
	}
	if !strings.Contains(joined, "BatchMode=yes") {
		t.Errorf("base %q missing BatchMode", joined)
	}
}

func TestNewExecutorEmptyHostIsLocal(t *testing.T) {
	for _, mode := range []string{"", "library", "command"} {
		e, err := newExecutor("", "", "", mode, "")
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if _, ok := e.(*localExecutor); !ok {
			t.Errorf("mode %q: got %T, want *localExecutor", mode, e)
		}
	}
}

func TestSSHConnected(t *testing.T) {
	if sshConnected(nil) {
		t.Error("nil executor must report not connected")
	}
	if sshConnected(&localExecutor{}) {
		t.Error("local executor must report ssh not connected")
	}
	if !sshConnected(&sshCommandExecutor{}) {
		t.Error("ssh command executor must report connected")
	}
}

// Integration: requires a reachable ssh target. Run with:
//
//	SSH_CMD_TARGET=vm-dune-01 go test -run TestSSHCommandExecutorIntegration ./...
func TestSSHCommandExecutorIntegration(t *testing.T) {
	target := os.Getenv("SSH_CMD_TARGET")
	if target == "" {
		t.Skip("set SSH_CMD_TARGET to run the ssh command-executor integration test")
	}
	e, err := newSSHCommandExecutor(target, "", "", "")
	if err != nil {
		t.Fatalf("newSSHCommandExecutor: %v", err)
	}
	defer e.Close()
	out, err := e.Exec("echo dune-ok")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if out != "dune-ok" {
		t.Errorf("Exec output = %q, want dune-ok", out)
	}
}
