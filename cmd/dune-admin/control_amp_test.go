package main

import (
	"errors"
	"io"
	"net"
	"testing"
)

type fakeAMPExecutor struct {
	out string
	err error
	cmd string
}

func (f *fakeAMPExecutor) Exec(cmd string) (string, error) {
	f.cmd = cmd
	return f.out, f.err
}
func (f *fakeAMPExecutor) Stream(string) (<-chan string, func(), error) {
	return nil, func() {}, nil
}
func (f *fakeAMPExecutor) PipeToWriter(string, io.Writer) error { return nil }
func (f *fakeAMPExecutor) WriteFile(string, io.Reader) error    { return nil }
func (f *fakeAMPExecutor) Dial(string, string) (net.Conn, error) {
	return nil, nil
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
