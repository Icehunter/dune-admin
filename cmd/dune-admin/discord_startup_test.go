package main

import (
	"testing"
	"time"
)

func TestDiscordBotEnabled(t *testing.T) {
	t.Run("nil pointer returns false", func(t *testing.T) {
		cfg := appConfig{DiscordBotEnabled: nil}
		if discordBotEnabled(cfg) {
			t.Fatal("expected false when DiscordBotEnabled is nil")
		}
	})

	t.Run("false pointer returns false", func(t *testing.T) {
		cfg := appConfig{DiscordBotEnabled: boolPtr(false)}
		if discordBotEnabled(cfg) {
			t.Fatal("expected false when DiscordBotEnabled is *false")
		}
	})

	t.Run("true pointer returns true", func(t *testing.T) {
		cfg := appConfig{DiscordBotEnabled: boolPtr(true)}
		if !discordBotEnabled(cfg) {
			t.Fatal("expected true when DiscordBotEnabled is *true")
		}
	})
}

func TestStartEmbeddedDiscordBotValidation(t *testing.T) {
	t.Run("disabled bot returns nil", func(t *testing.T) {
		cfg := appConfig{DiscordBotEnabled: boolPtr(false)}
		cancel := startEmbeddedDiscordBotIfEnabled(cfg)
		if cancel != nil {
			cancel()
			t.Fatal("expected nil cancel when bot is disabled")
		}
	})

	t.Run("missing token returns nil", func(t *testing.T) {
		cfg := appConfig{
			DiscordBotEnabled: boolPtr(true),
			DiscordBotToken:   "",
			DiscordGuildID:    "123456789",
		}
		cancel := startEmbeddedDiscordBotIfEnabled(cfg)
		if cancel != nil {
			cancel()
			t.Fatal("expected nil cancel when token is empty")
		}
	})

	t.Run("missing guild ID returns nil", func(t *testing.T) {
		cfg := appConfig{
			DiscordBotEnabled: boolPtr(true),
			DiscordBotToken:   "fake-token",
			DiscordGuildID:    "",
		}
		cancel := startEmbeddedDiscordBotIfEnabled(cfg)
		if cancel != nil {
			cancel()
			t.Fatal("expected nil cancel when guild ID is empty")
		}
	})
}

// TestStartEmbeddedDiscordBotAsync verifies that the function returns
// immediately (non-blocking) when given a valid-looking config, even though
// the actual Discord gateway connection will fail in the background.
func TestStartEmbeddedDiscordBotAsync(t *testing.T) {
	cfg := appConfig{
		DiscordBotEnabled: boolPtr(true),
		DiscordBotToken:   "fake-token-for-async-test",
		DiscordGuildID:    "123456789012345678",
	}

	done := make(chan struct{})
	var cancel func()

	go func() {
		cancel = startEmbeddedDiscordBotIfEnabled(cfg)
		close(done)
	}()

	select {
	case <-done:
		// Good: returned before the timeout — function is non-blocking.
		if cancel != nil {
			cancel()
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startEmbeddedDiscordBotIfEnabled blocked for >2s — should return immediately")
	}
}
