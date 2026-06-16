package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
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

	t.Run("missing guild ID returns nil when no guild configured anywhere", func(t *testing.T) {
		// Ensure no guild store is wired so the check has nothing to fall back to.
		orig := globalDiscordGuildsStore
		globalDiscordGuildsStore = nil
		t.Cleanup(func() { globalDiscordGuildsStore = orig })

		cfg := appConfig{
			DiscordBotEnabled: boolPtr(true),
			DiscordBotToken:   "fake-token",
			DiscordGuildID:    "",
		}
		cancel := startEmbeddedDiscordBotIfEnabled(cfg)
		if cancel != nil {
			cancel()
			t.Fatal("expected nil cancel when no guild is configured anywhere")
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

func TestStopDiscordBot_ClearsCancel(t *testing.T) {
	stopped := false
	discordCancelMu.Lock()
	globalDiscordCancel = func() { stopped = true }
	discordCancelMu.Unlock()
	t.Cleanup(func() {
		discordCancelMu.Lock()
		globalDiscordCancel = nil
		discordCancelMu.Unlock()
	})

	stopDiscordBot()

	discordCancelMu.Lock()
	remaining := globalDiscordCancel
	discordCancelMu.Unlock()

	if remaining != nil {
		t.Fatal("expected globalDiscordCancel to be nil after stop")
	}
	if !stopped {
		t.Fatal("expected previous cancel func to be called by stopDiscordBot")
	}
}

func TestApplyDiscordConfig_StopsRunningBot(t *testing.T) {
	stopped := false
	discordCancelMu.Lock()
	globalDiscordCancel = func() { stopped = true }
	discordCancelMu.Unlock()
	t.Cleanup(func() {
		discordCancelMu.Lock()
		globalDiscordCancel = nil
		discordCancelMu.Unlock()
	})

	applyDiscordConfig(appConfig{DiscordBotEnabled: boolPtr(false)})

	discordCancelMu.Lock()
	remaining := globalDiscordCancel
	discordCancelMu.Unlock()

	if remaining != nil {
		t.Fatal("expected globalDiscordCancel nil after disabling")
	}
	if !stopped {
		t.Fatal("expected running cancel func to be called on disable")
	}
}

func TestApplyDiscordConfig_NoopWhenDisabledAndNotRunning(t *testing.T) {
	discordCancelMu.Lock()
	globalDiscordCancel = nil
	discordCancelMu.Unlock()

	applyDiscordConfig(appConfig{DiscordBotEnabled: boolPtr(false)})

	discordCancelMu.Lock()
	remaining := globalDiscordCancel
	discordCancelMu.Unlock()

	if remaining != nil {
		t.Fatal("expected globalDiscordCancel to remain nil")
	}
}

func TestApplyDiscordConfig_SetsCancel_WhenEnabled(t *testing.T) {
	discordCancelMu.Lock()
	globalDiscordCancel = nil
	discordCancelMu.Unlock()
	t.Cleanup(func() {
		stopDiscordBot()
	})

	cfg := appConfig{
		DiscordBotEnabled: boolPtr(true),
		DiscordBotToken:   "fake-token-apply-test",
		DiscordGuildID:    "123456789012345678",
	}

	done := make(chan struct{})
	go func() {
		applyDiscordConfig(cfg)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("applyDiscordConfig blocked for >2s — should return immediately")
	}

	discordCancelMu.Lock()
	remaining := globalDiscordCancel
	discordCancelMu.Unlock()

	if remaining == nil {
		t.Fatal("expected globalDiscordCancel to be set after enabling")
	}
}

// openDiscordTestStore opens a temp SQLite DB with the servers + discord_guilds
// schemas wired, inserts one server row, and returns the store plus a cleanup func.
func openDiscordTestStore(t *testing.T) *discordGuildsStore {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "discord_test.sqlite"))
	if err != nil {
		t.Fatalf("open discord test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := initServersSchema(db); err != nil {
		t.Fatalf("init servers schema: %v", err)
	}
	if err := initDiscordGuildsSchema(db); err != nil {
		t.Fatalf("init discord_guilds schema: %v", err)
	}
	// Insert a server so the FK on discord_servers is satisfied.
	if _, err := db.Exec(`INSERT INTO servers (id, name, position) VALUES (1, 'test', 0)`); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	return newDiscordGuildsStore(db)
}

// TestHasConfiguredGuild covers the three branches of hasConfiguredGuild.
func TestHasConfiguredGuild(t *testing.T) {
	// Save/restore the global guild store.
	orig := globalDiscordGuildsStore
	t.Cleanup(func() { globalDiscordGuildsStore = orig })

	t.Run("legacy DiscordGuildID set returns true", func(t *testing.T) {
		globalDiscordGuildsStore = nil
		cfg := appConfig{DiscordGuildID: "123456789012345678"}
		if !hasConfiguredGuild(cfg) {
			t.Fatal("expected true when legacy DiscordGuildID is set")
		}
	})

	t.Run("empty legacy field but server link in DB returns true", func(t *testing.T) {
		store := openDiscordTestStore(t)
		err := store.upsertServerLink(discordServerLink{
			ServerID: 1,
			GuildID:  "987654321098765432",
		})
		if err != nil {
			t.Fatalf("upsertServerLink: %v", err)
		}
		globalDiscordGuildsStore = store
		cfg := appConfig{DiscordGuildID: ""}
		if !hasConfiguredGuild(cfg) {
			t.Fatal("expected true when a server link with a guild is in the DB")
		}
	})

	t.Run("nothing configured returns false", func(t *testing.T) {
		globalDiscordGuildsStore = nil
		cfg := appConfig{DiscordGuildID: ""}
		if hasConfiguredGuild(cfg) {
			t.Fatal("expected false when no guild is configured anywhere")
		}
	})
}

// TestStartEmbeddedDiscordBotWithServerLink verifies that the bot starts when
// DiscordGuildID is empty but a guild is configured via the Manage Server UI
// (a server link in the discord_servers table).
func TestStartEmbeddedDiscordBotWithServerLink(t *testing.T) {
	orig := globalDiscordGuildsStore
	t.Cleanup(func() { globalDiscordGuildsStore = orig })

	store := openDiscordTestStore(t)
	if err := store.upsertServerLink(discordServerLink{
		ServerID: 1,
		GuildID:  "111222333444555666",
	}); err != nil {
		t.Fatalf("upsertServerLink: %v", err)
	}
	globalDiscordGuildsStore = store

	cfg := appConfig{
		DiscordBotEnabled: boolPtr(true),
		DiscordBotToken:   "fake-token-server-link-test",
		DiscordGuildID:    "", // legacy field intentionally empty — guild from DB only
	}

	done := make(chan context.CancelFunc, 1)
	go func() {
		done <- startEmbeddedDiscordBotIfEnabled(cfg)
	}()

	select {
	case cancel := <-done:
		if cancel == nil {
			t.Fatal("expected non-nil cancel when guild is configured via server link")
		}
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("startEmbeddedDiscordBotIfEnabled blocked for >2s")
	}
}
