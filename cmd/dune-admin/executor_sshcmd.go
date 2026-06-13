package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// controlOpts returns the OpenSSH connection-sharing options for the given
// GOOS. ControlMaster multiplexes many channels over one authenticated
// connection — replicating the single-client behaviour of sshExecutor. The
// %C token lets ssh hash the connection parameters into the socket name,
// avoiding the ~104-char ControlPath length limit. Windows has no support for
// this, so it returns nil and every invocation opens its own connection.
func controlOpts(goos string) []string {
	if goos == "windows" {
		return nil
	}
	path := filepath.Join(os.TempDir(), "dune-admin-cm-%C")
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + path,
		"-o", "ControlPersist=60s",
	}
}

// sshExecArgs builds the args to run a remote command:
//
//	<base> <target> -- <remoteCmd>
//
// The "--" stops ssh option parsing; remoteCmd is handed to the remote shell
// as a single word, matching sshExecutor's session.Run(cmd) semantics.
func sshExecArgs(base []string, target, remoteCmd string) []string {
	args := append([]string{}, base...)
	return append(args, target, "--", remoteCmd)
}

// sshDialArgs builds the args to open a stdio TCP tunnel:
//
//	<base> -W <addr> <target>
//
// -W makes ssh forward its stdin/stdout to addr (the ProxyCommand mechanism),
// so ProxyJump chains in ~/.ssh/config are applied automatically.
func sshDialArgs(base []string, target, addr string) []string {
	args := append([]string{}, base...)
	return append(args, "-W", addr, target)
}

// sshAddr is a minimal net.Addr for stdioConn endpoints.
type sshAddr struct {
	network string
	addr    string
}

func (a sshAddr) Network() string { return a.network }
func (a sshAddr) String() string  { return a.addr }

// stdioConn adapts the stdin/stdout of an `ssh -W` process to net.Conn.
// Read pulls from the process stdout, Write pushes to its stdin. Deadline
// methods are no-ops: pgx/amqp set deadlines, but the connection is bounded by
// the consumer's context and the ssh process lifetime instead. Documented
// limitation, acceptable for this admin tool.
type stdioConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	local  sshAddr
	remote sshAddr
	once   sync.Once
}

func (c *stdioConn) Read(b []byte) (int, error)  { return c.stdout.Read(b) }
func (c *stdioConn) Write(b []byte) (int, error) { return c.stdin.Write(b) }

func (c *stdioConn) Close() error {
	c.once.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
	})
	return nil
}

func (c *stdioConn) LocalAddr() net.Addr              { return c.local }
func (c *stdioConn) RemoteAddr() net.Addr             { return c.remote }
func (c *stdioConn) SetDeadline(time.Time) error      { return nil }
func (c *stdioConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stdioConn) SetWriteDeadline(time.Time) error { return nil }

// sshCommandExecutor implements Executor by shelling out to the OS ssh client.
type sshCommandExecutor struct {
	target  string   // ssh target: "user@host" or a ~/.ssh/config alias
	base    []string // connection-level args shared by every invocation
	control []string // ControlMaster opts (retained for Close's -O exit)
}

// buildSSHCommandExecutor assembles the executor from config without doing any
// I/O beyond stat-ing the key file. base = BatchMode + optional -p +
// ControlMaster opts + optional -i + extra opts. -i is added only when the key
// path is set AND the file exists, so command mode falls back to
// ssh-agent/~/.ssh/config when no key is configured (the whole point: no
// private key handed to the program).
func buildSSHCommandExecutor(sshHost, sshUser, sshKeyPath, extraOpts string) *sshCommandExecutor {
	host := sshHost
	base := []string{"-o", "BatchMode=yes"}
	if h, p, err := net.SplitHostPort(sshHost); err == nil {
		host = h
		base = append(base, "-p", p)
	}
	control := controlOpts(runtime.GOOS)
	base = append(base, control...)
	if sshKeyPath != "" {
		if _, err := os.Stat(sshKeyPath); err == nil {
			base = append(base, "-i", sshKeyPath)
		}
	}
	base = append(base, strings.Fields(extraOpts)...)

	target := host
	if sshUser != "" && !strings.Contains(host, "@") {
		target = sshUser + "@" + host
	}
	return &sshCommandExecutor{target: target, base: base, control: control}
}

func (e *sshCommandExecutor) Type() string { return "ssh" }

// newSSHCommandExecutor builds the executor and probes reachability, eagerly
// establishing the ControlMaster so a bad config fails fast (mirrors dialSSH).
func newSSHCommandExecutor(sshHost, sshUser, sshKeyPath, extraOpts string) (Executor, error) {
	e := buildSSHCommandExecutor(sshHost, sshUser, sshKeyPath, extraOpts)
	if _, err := e.Exec("true"); err != nil {
		return nil, fmt.Errorf("ssh command probe %s: %w", e.target, err)
	}
	return e, nil
}

func (e *sshCommandExecutor) Exec(cmd string) (string, error) {
	c := exec.Command("ssh", sshExecArgs(e.base, e.target, cmd)...) // #nosec G204,G702 -- args from admin config, not user input
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return strings.TrimSpace(buf.String()), err
}

func (e *sshCommandExecutor) Stream(cmd string) (<-chan string, func(), error) {
	c := exec.Command("ssh", sshExecArgs(e.base, e.target, cmd)...) // #nosec G204,G702 -- args from admin config, not user input
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

func (e *sshCommandExecutor) PipeToWriter(cmd string, w io.Writer) error {
	c := exec.Command("ssh", sshExecArgs(e.base, e.target, cmd)...) // #nosec G204,G702 -- args from admin config, not user input
	c.Stdout = w
	var errBuf bytes.Buffer
	c.Stderr = &errBuf
	if err := c.Run(); err != nil {
		if stderr := strings.TrimSpace(errBuf.String()); stderr != "" {
			return fmt.Errorf("%w: %s", err, stderr)
		}
		return err
	}
	return nil
}

func (e *sshCommandExecutor) WriteFile(path string, data io.Reader) error {
	remote := fmt.Sprintf("sudo tee %s > /dev/null", shellQuote(path))
	c := exec.Command("ssh", sshExecArgs(e.base, e.target, remote)...) // #nosec G204,G702 -- args from admin config, not user input
	stdin, err := c.StdinPipe()
	if err != nil {
		return err
	}
	if err := c.Start(); err != nil {
		return err
	}
	if _, err := io.Copy(stdin, data); err != nil {
		_ = stdin.Close()
		_ = c.Wait()
		return err
	}
	_ = stdin.Close()
	return c.Wait()
}

func (e *sshCommandExecutor) Dial(network, addr string) (net.Conn, error) {
	c := exec.Command("ssh", sshDialArgs(e.base, e.target, addr)...) // #nosec G204,G702 -- args from admin config, not user input
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &stdioConn{
		cmd:    c,
		stdin:  stdin,
		stdout: stdout,
		local:  sshAddr{network: network, addr: "ssh-stdio"},
		remote: sshAddr{network: network, addr: addr},
	}, nil
}

// Close tears down the ControlMaster (Unix). No-op when control is nil
// (Windows or no master), since each invocation owned its own connection.
func (e *sshCommandExecutor) Close() {
	if len(e.control) == 0 {
		return
	}
	args := append([]string{}, e.base...)
	args = append(args, "-O", "exit", e.target)
	_ = exec.Command("ssh", args...).Run() // #nosec G204,G702 -- args from admin config, not user input
}
