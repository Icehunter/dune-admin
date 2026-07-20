package main

import "testing"

// preserveServerConnectionFields keeps connection fields the client left blank on
// an update from wiping the persisted value. ssh_mode is the motivating case: a
// client (or an older UI build) that omits it must not silently reset a "command"
// server back to the library default. An explicit value always wins.
func TestPreserveServerConnectionFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		nextMode string
		oldMode  string
		want     string
	}{
		{"blank next keeps old command", "", "command", "command"},
		{"blank next keeps old library", "", "library", "library"},
		{"blank next, blank old stays blank", "", "", ""},
		{"explicit library overrides command", "library", "command", "library"},
		{"explicit command kept when old blank", "command", "", "command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &ServerConfig{SSHMode: tt.nextMode}
			preserveServerConnectionFields(next, ServerConfig{SSHMode: tt.oldMode})
			if next.SSHMode != tt.want {
				t.Errorf("SSHMode = %q, want %q", next.SSHMode, tt.want)
			}
		})
	}
}

// A PUT that omits control (an older/mismatched UI build, or a form that didn't
// populate the field) must NOT wipe a configured "amp" (or "kubectl"/"docker")
// server back to blank — which resolves to the "local" control plane and breaks
// GetStatus with "local control plane does not support GetStatus". Blank means
// "not sent, keep what's stored"; an explicit value always wins so a deliberate
// control-plane switch is still honored. ControlNamespace travels with Control.
func TestPreserveServerConnectionFields_Control(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		nextCtrl string
		oldCtrl  string
		wantCtrl string
		nextNS   string
		oldNS    string
		wantNS   string
	}{
		{"blank next keeps old amp", "", "amp", "amp", "", "", ""},
		{"blank next keeps old kubectl + ns", "", "kubectl", "kubectl", "", "dune", "dune"},
		{"blank next, blank old stays local-default blank", "", "", "", "", "", ""},
		{"explicit local overrides amp", "local", "amp", "local", "", "", ""},
		{"explicit amp kept when old blank", "amp", "", "amp", "", "", ""},
		{"blank ns keeps old ns", "kubectl", "kubectl", "kubectl", "", "prod", "prod"},
		{"explicit ns overrides old ns", "kubectl", "kubectl", "kubectl", "staging", "prod", "staging"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &ServerConfig{Control: tt.nextCtrl, ControlNamespace: tt.nextNS}
			preserveServerConnectionFields(next, ServerConfig{Control: tt.oldCtrl, ControlNamespace: tt.oldNS})
			if next.Control != tt.wantCtrl {
				t.Errorf("Control = %q, want %q", next.Control, tt.wantCtrl)
			}
			if next.ControlNamespace != tt.wantNS {
				t.Errorf("ControlNamespace = %q, want %q", next.ControlNamespace, tt.wantNS)
			}
		})
	}
}
