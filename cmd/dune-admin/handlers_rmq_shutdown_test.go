package main

import (
	"context"
	"testing"
	"time"
)

// TestShutdownVerb maps the broadcast shutdown type to the control-plane verb:
// "Restart" cycles the server, every other window (Maintenance/Update) stops it.
func TestShutdownVerb(t *testing.T) {
	tests := []struct {
		shutdownType string
		want         string
	}{
		{"Restart", "restart"},
		{"restart", "restart"},
		{"Maintenance", "stop"},
		{"Update", "stop"},
		{"", "stop"},
	}
	for _, tt := range tests {
		if got := shutdownVerb(tt.shutdownType); got != tt.want {
			t.Errorf("shutdownVerb(%q) = %q, want %q", tt.shutdownType, got, tt.want)
		}
	}
}

// TestFireBroadcastShutdown_InvokesExecCommand verifies the broadcast shutdown
// actually drives the control plane (#205 — previously it only announced and the
// server kept running).
func TestFireBroadcastShutdown_InvokesExecCommand(t *testing.T) {
	ctrl := &recordingControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}

	fireBroadcastShutdown(context.Background(), "restart", ctrl, exec)

	if len(ctrl.execCmds) != 1 || ctrl.execCmds[0] != "restart" {
		t.Fatalf("expected ExecCommand(\"restart\"), got %v", ctrl.execCmds)
	}
}

// TestFireBroadcastShutdown_NoControlPlane is a no-op (and must not panic) when
// the control plane isn't connected.
func TestFireBroadcastShutdown_NoControlPlane(t *testing.T) {
	fireBroadcastShutdown(context.Background(), "restart", nil, nil) // must not panic
}

// TestScheduleAndCancelBroadcastShutdown verifies the armed control-plane action
// can be cancelled (a "Cancel" broadcast must abort the pending restart/stop) and
// that the pending state — exposed via /status for UI rehydration — tracks it.
func TestScheduleAndCancelBroadcastShutdown(t *testing.T) {
	t.Cleanup(cancelBroadcastShutdownExec)

	if _, pending := pendingBroadcastShutdown(); pending {
		t.Fatal("expected no pending shutdown before scheduling")
	}

	scheduleBroadcastShutdownExec(time.Hour, "restart", nil, nil)
	shutdownExecMu.Lock()
	armed := shutdownExecTimer != nil
	shutdownExecMu.Unlock()
	if !armed {
		t.Fatal("expected a timer to be armed after scheduling")
	}
	at, pending := pendingBroadcastShutdown()
	if !pending || at <= 0 {
		t.Fatalf("expected pending=true with a fire time, got pending=%v at=%d", pending, at)
	}

	cancelBroadcastShutdownExec()
	shutdownExecMu.Lock()
	cleared := shutdownExecTimer == nil
	shutdownExecMu.Unlock()
	if !cleared {
		t.Fatal("expected the timer to be cleared after cancel")
	}
	if _, pending := pendingBroadcastShutdown(); pending {
		t.Fatal("expected no pending shutdown after cancel")
	}
}
