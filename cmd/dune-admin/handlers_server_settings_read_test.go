package main

import (
	"errors"
	"strings"
	"testing"
)

// setGlobalExecutor swaps globalExecutor for the duration of a test and
// restores it via t.Cleanup. Uses fnExecutor (defined in control_amp_test.go).
func setGlobalExecutor(t *testing.T, fn func(cmd string) (string, error)) {
	t.Helper()
	orig := globalExecutor
	globalExecutor = &fnExecutor{fn: fn}
	t.Cleanup(func() { globalExecutor = orig })
}

// TestReadINIContent_PlainCatFirst verifies that readINIContent uses plain cat
// before attempting sudo cat. When plain cat succeeds, sudo must not be called.
func TestReadINIContent_PlainCatFirst(t *testing.T) {
	var calls []string
	setGlobalExecutor(t, func(cmd string) (string, error) {
		calls = append(calls, cmd)
		return "[section]\nkey=value\n", nil
	})

	content := readINIContent("/path/to/UserGame.ini", globalControl, globalExecutor)

	if content == "" {
		t.Fatal("expected content, got empty string")
	}
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 exec call, got %d: %v", len(calls), calls)
	}
	if strings.Contains(calls[0], "sudo") {
		t.Errorf("plain cat should be tried first, got: %q", calls[0])
	}
}

// TestReadINIContent_FallsBackToSudoCat verifies that when plain cat fails
// (e.g. permission denied on a file not owned by the service user), readINIContent
// retries with sudo cat — supporting non-AMP deployments where sudo is available.
func TestReadINIContent_FallsBackToSudoCat(t *testing.T) {
	setGlobalExecutor(t, func(cmd string) (string, error) {
		if strings.Contains(cmd, "sudo") {
			return "[section]\nkey=sudovalue\n", nil
		}
		return "", errors.New("permission denied")
	})

	content := readINIContent("/path/to/UserGame.ini", globalControl, globalExecutor)

	if !strings.Contains(content, "sudovalue") {
		t.Fatalf("expected sudo cat fallback content, got: %q", content)
	}
}

// TestReadINIContent_ReturnsEmptyWhenBothFail verifies that when both cat and
// sudo cat fail, readINIContent returns "" without panicking.
func TestReadINIContent_ReturnsEmptyWhenBothFail(t *testing.T) {
	setGlobalExecutor(t, func(cmd string) (string, error) {
		return "", errors.New("no such file")
	})

	content := readINIContent("/path/to/nonexistent.ini", globalControl, globalExecutor)

	if content != "" {
		t.Fatalf("expected empty string, got: %q", content)
	}
}
