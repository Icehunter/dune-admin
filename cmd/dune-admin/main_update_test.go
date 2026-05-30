package main

import (
	"strings"
	"testing"
)

func TestCheckForUpdate_AlreadyLatest(t *testing.T) {
	AppVersion = "0.16.0"
	fetcher := func(url string) ([]byte, error) {
		return []byte(`{"tag_name":"v0.16.0","html_url":"https://x"}`), nil
	}
	msg, err := checkForUpdate(fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "up to date") {
		t.Errorf("want 'up to date' in message, got: %q", msg)
	}
}

func TestCheckForUpdate_Available(t *testing.T) {
	AppVersion = "0.15.0"
	fetcher := func(url string) ([]byte, error) {
		return []byte(`{"tag_name":"v0.16.0","html_url":"https://github.com/Icehunter/dune-admin/releases/tag/v0.16.0"}`), nil
	}
	msg, err := checkForUpdate(fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "0.16.0") {
		t.Errorf("want version in message, got: %q", msg)
	}
}
