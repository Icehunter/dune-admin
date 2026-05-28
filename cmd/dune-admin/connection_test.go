package main

import "testing"

func TestResolveControl(t *testing.T) {
	origControlPlane := controlPlane
	origSSHHost := sshHost
	t.Cleanup(func() {
		controlPlane = origControlPlane
		sshHost = origSSHHost
	})

	controlPlane = "amp"
	sshHost = ""
	if got := resolveControl(); got != "amp" {
		t.Fatalf("expected explicit control plane to win, got %q", got)
	}

	controlPlane = ""
	sshHost = "vm.example:22"
	if got := resolveControl(); got != "kubectl" {
		t.Fatalf("expected ssh host to default control to kubectl, got %q", got)
	}

	controlPlane = ""
	sshHost = ""
	if got := resolveControl(); got != "local" {
		t.Fatalf("expected local default without ssh/control flags, got %q", got)
	}
}
