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
