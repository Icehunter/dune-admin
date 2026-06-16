package main

import (
	"context"
	"testing"
)

// TestServerForChannelRouting verifies the package-level channel→server resolver
// that handleDiscordInteraction uses: a command in a server's announce or status
// channel resolves to that server + guild; a channel owned by no server resolves
// to not-ok (→ the handler replies with the polite ephemeral error).
func TestServerForChannelRouting(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)
	mustUpsertServerLink(t, store, discordServerLink{
		ServerID: a, GuildID: "g1", AnnounceChannelID: "a-announce", StatusChannelID: "a-status",
	})
	mustUpsertServerLink(t, store, discordServerLink{
		ServerID: b, GuildID: "g2", AnnounceChannelID: "b-announce",
	})

	prevStore := globalDiscordGuildsStore
	t.Cleanup(func() { globalDiscordGuildsStore = prevStore })
	globalDiscordGuildsStore = store

	t.Run("announce channel routes to its server", func(t *testing.T) {
		sid, gid, ok := serverForChannel("a-announce")
		if !ok || sid != a || gid != "g1" {
			t.Fatalf("got (%d,%q,%v), want (%d,g1,true)", sid, gid, ok, a)
		}
	})
	t.Run("status channel routes to its server", func(t *testing.T) {
		sid, gid, ok := serverForChannel("a-status")
		if !ok || sid != a || gid != "g1" {
			t.Fatalf("got (%d,%q,%v), want (%d,g1,true)", sid, gid, ok, a)
		}
	})
	t.Run("second server's channel routes to it", func(t *testing.T) {
		sid, gid, ok := serverForChannel("b-announce")
		if !ok || sid != b || gid != "g2" {
			t.Fatalf("got (%d,%q,%v), want (%d,g2,true)", sid, gid, ok, b)
		}
	})
	t.Run("unmapped channel → not ok", func(t *testing.T) {
		if _, _, ok := serverForChannel("random-channel"); ok {
			t.Error("a channel owned by no server must not resolve")
		}
	})
	t.Run("nil store → not ok", func(t *testing.T) {
		globalDiscordGuildsStore = nil
		if _, _, ok := serverForChannel("a-announce"); ok {
			t.Error("must not resolve when the store is unavailable")
		}
		globalDiscordGuildsStore = store
	})
}

// TestBuildDiscordDepsBindsServer verifies the per-channel deps bind to the
// resolved server: getLink reads the invoking user's character ON THAT SERVER,
// and registerLink writes it there. Two servers, same user, two characters.
func TestBuildDiscordDepsBindsServer(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)

	prevStore := globalDiscordGuildsStore
	t.Cleanup(func() { globalDiscordGuildsStore = prevStore })
	globalDiscordGuildsStore = store

	ctx := context.Background()

	// Register the same Discord user on each server via that server's deps.
	if err := buildDiscordDeps(a).registerLink(ctx, "u1", 100, "AlphaChar", "av-a"); err != nil {
		t.Fatalf("register on a: %v", err)
	}
	if err := buildDiscordDeps(b).registerLink(ctx, "u1", 200, "BetaChar", "av-b"); err != nil {
		t.Fatalf("register on b: %v", err)
	}

	// getLink via each server's deps returns that server's character.
	if name, ok, err := buildDiscordDeps(a).getLink(ctx, "u1"); err != nil || !ok || name != "AlphaChar" {
		t.Errorf("server a getLink = %q ok=%v err=%v, want AlphaChar", name, ok, err)
	}
	if name, ok, err := buildDiscordDeps(b).getLink(ctx, "u1"); err != nil || !ok || name != "BetaChar" {
		t.Errorf("server b getLink = %q ok=%v err=%v, want BetaChar", name, ok, err)
	}

	// deleteLink via server a's deps removes only the server-a character.
	if deleted, err := buildDiscordDeps(a).deleteLink(ctx, "u1"); err != nil || !deleted {
		t.Fatalf("delete on a: deleted=%v err=%v", deleted, err)
	}
	if _, ok, _ := buildDiscordDeps(a).getLink(ctx, "u1"); ok {
		t.Error("server a character should be gone")
	}
	if name, ok, _ := buildDiscordDeps(b).getLink(ctx, "u1"); !ok || name != "BetaChar" {
		t.Errorf("server b character should survive, got %q ok=%v", name, ok)
	}
}
