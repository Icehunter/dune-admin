package main

import (
	"context"
	"database/sql"
	"testing"
)

// ── discord_guilds (roles) round-trip ──────────────────────────────────────────

func TestDiscordGuildsStore_RolesRoundTrip(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	store := newDiscordGuildsStore(db)

	in := discordGuild{GuildID: "g1", RolesViewer: "1,2", RolesEconomy: "3", RolesAdmin: "4"}
	if err := store.upsertGuild(in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, ok, err := store.getGuild("g1")
	if err != nil || !ok {
		t.Fatalf("getGuild: ok=%v err=%v", ok, err)
	}
	if got != in {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, in)
	}

	in.RolesAdmin = "4,5"
	if err := store.upsertGuild(in); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got, _, _ = store.getGuild("g1")
	if got.RolesAdmin != "4,5" {
		t.Errorf("update not applied: %+v", got)
	}

	all, err := store.listGuilds()
	if err != nil || len(all) != 1 {
		t.Fatalf("listGuilds: %d %v", len(all), err)
	}
}

// ── discord_servers round-trip + one-guild-per-server + cascade ─────────────────

func mustUpsertGuild(t *testing.T, s *discordGuildsStore, guildID string) {
	t.Helper()
	if err := s.upsertGuild(discordGuild{GuildID: guildID}); err != nil {
		t.Fatalf("upsert guild %s: %v", guildID, err)
	}
}

func mustUpsertServerLink(t *testing.T, s *discordGuildsStore, link discordServerLink) {
	t.Helper()
	if err := s.upsertServerLink(link); err != nil {
		t.Fatalf("upsert server link %d→%s: %v", link.ServerID, link.GuildID, err)
	}
}

func TestDiscordServers_RoundTrip(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)

	// One guild holding two servers (each its own row + channels).
	mustUpsertServerLink(t, store, discordServerLink{
		ServerID: a, GuildID: "g1", StatusChannelID: "sa", AnnounceChannelID: "aa",
		StatusEnabled: true, StatusIntervalSeconds: 90,
	})
	mustUpsertServerLink(t, store, discordServerLink{ServerID: b, GuildID: "g1", AnnounceChannelID: "ab"})

	links, err := store.listServerLinks()
	if err != nil || len(links) != 2 {
		t.Fatalf("listServerLinks: %d %v", len(links), err)
	}

	gotA, ok, err := store.getServerLink(a)
	if err != nil || !ok {
		t.Fatalf("getServerLink a: ok=%v err=%v", ok, err)
	}
	if gotA.GuildID != "g1" || gotA.StatusChannelID != "sa" || gotA.AnnounceChannelID != "aa" ||
		!gotA.StatusEnabled || gotA.StatusIntervalSeconds != 90 {
		t.Errorf("server a link not persisted: %+v", gotA)
	}

	ids, err := store.serversForGuild("g1")
	if err != nil || len(ids) != 2 {
		t.Fatalf("serversForGuild: %v %v", ids, err)
	}

	// Update path: change a channel.
	mustUpsertServerLink(t, store, discordServerLink{ServerID: a, GuildID: "g1", StatusChannelID: "sa2"})
	gotA, _, _ = store.getServerLink(a)
	if gotA.StatusChannelID != "sa2" {
		t.Errorf("update not applied: %+v", gotA)
	}

	// Delete one link.
	if err := store.deleteServerLink(b); err != nil {
		t.Fatalf("delete server link: %v", err)
	}
	if ids, _ := store.serversForGuild("g1"); len(ids) != 1 {
		t.Errorf("expected 1 link after delete, got %d", len(ids))
	}
}

// TestDiscordServers_OneGuildPerServer proves server_id is the PK: re-upserting
// the same server with a different guild REASSIGNS it (still exactly one row).
func TestDiscordServers_OneGuildPerServer(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	store := newDiscordGuildsStore(db)

	mustUpsertServerLink(t, store, discordServerLink{ServerID: a, GuildID: "g1"})
	mustUpsertServerLink(t, store, discordServerLink{ServerID: a, GuildID: "g2"})

	link, ok, err := store.getServerLink(a)
	if err != nil || !ok {
		t.Fatalf("getServerLink: ok=%v err=%v", ok, err)
	}
	if link.GuildID != "g2" {
		t.Errorf("re-link did not reassign guild: %+v", link)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM discord_servers WHERE server_id = ?`, a).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly one link per server, got %d", count)
	}
}

// TestServerForChannel resolves a server by either its announce OR status channel.
func TestServerForChannel(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)
	mustUpsertServerLink(t, store, discordServerLink{
		ServerID: a, GuildID: "g1", AnnounceChannelID: "announce-a", StatusChannelID: "status-a",
	})
	mustUpsertServerLink(t, store, discordServerLink{
		ServerID: b, GuildID: "g2", AnnounceChannelID: "announce-b",
	})

	cases := []struct {
		channel    string
		wantServer int
		wantGuild  string
		wantOK     bool
	}{
		{"announce-a", a, "g1", true},
		{"status-a", a, "g1", true},
		{"announce-b", b, "g2", true},
		{"unknown-channel", 0, "", false},
		{"", 0, "", false},
	}
	for _, tc := range cases {
		sid, gid, ok, err := store.serverForChannel(tc.channel)
		if err != nil {
			t.Fatalf("serverForChannel(%q): %v", tc.channel, err)
		}
		if ok != tc.wantOK || sid != tc.wantServer || gid != tc.wantGuild {
			t.Errorf("serverForChannel(%q) = (%d,%q,%v), want (%d,%q,%v)",
				tc.channel, sid, gid, ok, tc.wantServer, tc.wantGuild, tc.wantOK)
		}
	}
}

func TestDiscordServers_CascadeOnServerDelete(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)
	mustUpsertServerLink(t, store, discordServerLink{ServerID: a, GuildID: "g1"})
	mustUpsertServerLink(t, store, discordServerLink{ServerID: b, GuildID: "g1"})
	// A user link on server a must also cascade away.
	if err := store.upsertUserLink("u1", a, 100, "Char", ""); err != nil {
		t.Fatalf("upsert user link: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM servers WHERE id = ?`, a); err != nil {
		t.Fatalf("delete server a: %v", err)
	}

	// Link for server a gone; server b link untouched.
	if _, ok, _ := store.getServerLink(a); ok {
		t.Error("server a link should have cascaded away")
	}
	if _, ok, _ := store.getServerLink(b); !ok {
		t.Error("server b link must survive deletion of server a")
	}
	// User link on server a cascaded.
	if _, _, ok, _ := store.getUserLink("u1", a); ok {
		t.Error("user link on server a should have cascaded away")
	}
}

func TestDiscordServerLink_RejectsOrphanServer(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	store := newDiscordGuildsStore(db)
	if err := store.upsertServerLink(discordServerLink{ServerID: 999, GuildID: "g1"}); err == nil {
		t.Fatal("upsert server link with dangling server_id succeeded; FK not enforced")
	}
}

// ── user links: one character per (user, server) ───────────────────────────────

func TestDiscordUserLink_PerUserPerServer(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)

	// Same user registers a DIFFERENT character on each server.
	if err := store.upsertUserLink("u1", a, 100, "Alpha", "av1"); err != nil {
		t.Fatalf("link u1/a: %v", err)
	}
	if err := store.upsertUserLink("u1", b, 200, "Beta", "av2"); err != nil {
		t.Fatalf("link u1/b: %v", err)
	}

	acct, name, ok, err := store.getUserLink("u1", a)
	if err != nil || !ok || acct != 100 || name != "Alpha" {
		t.Fatalf("u1/a = acct=%d name=%q ok=%v err=%v", acct, name, ok, err)
	}
	acct, name, ok, _ = store.getUserLink("u1", b)
	if !ok || acct != 200 || name != "Beta" {
		t.Fatalf("u1/b = acct=%d name=%q ok=%v", acct, name, ok)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM discord_user_links WHERE discord_user_id = 'u1'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected two links (one per server), got %d", count)
	}

	// Re-register on server a replaces only the server-a character.
	if err := store.upsertUserLink("u1", a, 300, "Gamma", "av3"); err != nil {
		t.Fatalf("re-register u1/a: %v", err)
	}
	acct, name, _, _ = store.getUserLink("u1", a)
	if acct != 300 || name != "Gamma" {
		t.Errorf("re-register did not replace server a char: acct=%d name=%q", acct, name)
	}
	if _, name, _, _ := store.getUserLink("u1", b); name != "Beta" {
		t.Errorf("server b char should be untouched, got %q", name)
	}

	// Delete only the server-a link.
	deleted, err := store.deleteUserLink("u1", a)
	if err != nil || !deleted {
		t.Fatalf("deleteUserLink u1/a: deleted=%v err=%v", deleted, err)
	}
	if _, _, ok, _ := store.getUserLink("u1", a); ok {
		t.Error("server a link should be gone after delete")
	}
	if _, _, ok, _ := store.getUserLink("u1", b); !ok {
		t.Error("server b link should survive deletion of the server a link")
	}
}

func TestDiscordUserLinksForServer(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	store := newDiscordGuildsStore(db)
	if err := store.upsertUserLink("u1", a, 100, "Alpha", "av1"); err != nil {
		t.Fatalf("link u1: %v", err)
	}
	if err := store.upsertUserLink("u2", b, 200, "Beta", "av2"); err != nil {
		t.Fatalf("link u2: %v", err)
	}

	links, err := store.userLinksForServer(a)
	if err != nil {
		t.Fatalf("userLinksForServer: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link for server a, got %d", len(links))
	}
	info, ok := links[100]
	if !ok || info.discordUserID != "u1" || info.avatar != "av1" {
		t.Errorf("link for account 100 = %+v, want u1/av1", info)
	}
}

// ── resolveGuildContext ────────────────────────────────────────────────────────

func TestResolveGuildContext(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	store := newDiscordGuildsStore(db)
	mustUpsertGuild(t, store, "ga")

	prevStore := globalDiscordGuildsStore
	t.Cleanup(func() { globalDiscordGuildsStore = prevStore })
	globalDiscordGuildsStore = store

	t.Run("known guild → configured", func(t *testing.T) {
		g, ok := resolveGuildContext("ga")
		if !ok || g.GuildID != "ga" {
			t.Fatalf("resolve ga: ok=%v g=%+v", ok, g)
		}
	})
	t.Run("unknown guild → not configured", func(t *testing.T) {
		if _, ok := resolveGuildContext("nope"); ok {
			t.Error("unknown guild resolved ok")
		}
	})
	t.Run("empty guild id → not configured", func(t *testing.T) {
		if _, ok := resolveGuildContext(""); ok {
			t.Error("empty guild resolved ok")
		}
	})
}

// ── per-guild config + auth ────────────────────────────────────────────────────

func TestDiscordConfigFromGuild_PerGuildAuth(t *testing.T) {
	// Two guilds with different economy roles → a member with role "econ-A" is
	// authorized for give-currency in guild A but not guild B.
	cfgA := discordConfigFromGuild(discordGuild{GuildID: "A", RolesEconomy: "econ-A"})
	cfgB := discordConfigFromGuild(discordGuild{GuildID: "B", RolesEconomy: "econ-B"})

	member := discordMember{UserID: "u", Roles: []string{"econ-A"}}

	if !authorizeDiscord("A", member, tierEconomy, cfgA) {
		t.Error("member with econ-A should be authorized in guild A")
	}
	if authorizeDiscord("B", member, tierEconomy, cfgB) {
		t.Error("member with econ-A must NOT be authorized in guild B")
	}
	if authorizeDiscord("A", member, tierEconomy, cfgB) {
		t.Error("guild id mismatch must deny")
	}
}

// ── seed migration (legacy single-guild → guild + one server link) ─────────────

func seedLegacyDiscordConfig(t *testing.T, db *sql.DB, guildID, announce string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO app_config_discord
			(id, bot_enabled, bot_token, guild_id, roles_viewer, roles_economy, roles_admin,
			 announce_channel_id, status_enabled, status_channel_id, status_interval_seconds)
		VALUES (1, 1, 'tok', ?, 'rv', 're', 'ra', ?, 1, 'sc', 120)`,
		guildID, announce); err != nil {
		t.Fatalf("seed app_config_discord: %v", err)
	}
}

func TestSeedDiscordGuilds_SingleGuildLegacy(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	sid := int(insertTestServer(t, db, "S"))
	seedLegacyDiscordConfig(t, db, "legacy-guild", "legacy-announce")

	if err := seedDiscordGuilds(db, sid); err != nil {
		t.Fatalf("seed: %v", err)
	}

	store := newDiscordGuildsStore(db)
	g, ok, err := store.getGuild("legacy-guild")
	if err != nil || !ok {
		t.Fatalf("seeded guild missing: ok=%v err=%v", ok, err)
	}
	if g.RolesViewer != "rv" || g.RolesEconomy != "re" || g.RolesAdmin != "ra" {
		t.Errorf("seeded guild roles not copied: %+v", g)
	}

	link, ok, err := store.getServerLink(sid)
	if err != nil || !ok {
		t.Fatalf("seeded server link missing: ok=%v err=%v", ok, err)
	}
	if link.GuildID != "legacy-guild" || link.AnnounceChannelID != "legacy-announce" ||
		link.StatusChannelID != "sc" || !link.StatusEnabled || link.StatusIntervalSeconds != 120 {
		t.Errorf("seeded link did not copy legacy fields: %+v", link)
	}

	// Idempotent: re-run is a no-op, still one guild + one link.
	if err := seedDiscordGuilds(db, sid); err != nil {
		t.Fatalf("seed re-run: %v", err)
	}
	all, _ := store.listGuilds()
	if len(all) != 1 {
		t.Errorf("re-run produced %d guilds, want 1", len(all))
	}
}

func TestSeedDiscordGuilds_NoLegacyGuild(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	sid := int(insertTestServer(t, db, "S"))
	seedLegacyDiscordConfig(t, db, "", "")
	if err := seedDiscordGuilds(db, sid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, _ := newDiscordGuildsStore(db).listGuilds()
	if len(all) != 0 {
		t.Errorf("seeded %d guilds with no legacy guild_id, want 0", len(all))
	}
}

func TestSeedDiscordGuilds_SkipsWhenAlreadyPopulated(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	sid := int(insertTestServer(t, db, "S"))
	store := newDiscordGuildsStore(db)
	mustUpsertGuild(t, store, "existing")
	seedLegacyDiscordConfig(t, db, "legacy-guild", "legacy-announce")

	if err := seedDiscordGuilds(db, sid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, ok, _ := store.getGuild("legacy-guild"); ok {
		t.Error("seed must not overwrite an already-populated discord_guilds table")
	}
}

// ── legacy user-link migration ─────────────────────────────────────────────────

func TestMigrateLegacyDiscordUserLinks(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	sid := int(insertTestServer(t, db, "S"))
	store := newDiscordGuildsStore(db)

	read := func(_ context.Context) ([]legacyUserLink, error) {
		return []legacyUserLink{
			{discordUserID: "u1", accountID: 100, characterName: "Alpha", avatarURL: "av1"},
			{discordUserID: "u2", accountID: 200, characterName: "Beta", avatarURL: ""},
		}, nil
	}
	migrateLegacyDiscordUserLinks(db, sid, read)

	if _, name, ok, _ := store.getUserLink("u1", sid); !ok || name != "Alpha" {
		t.Errorf("u1 link = name:%q ok:%v, want Alpha", name, ok)
	}
	if _, _, ok, _ := store.getUserLink("u2", sid); !ok {
		t.Error("u2 link missing")
	}

	// Marker set → a second call with a panicking reader must be a no-op.
	migrateLegacyDiscordUserLinks(db, sid, func(_ context.Context) ([]legacyUserLink, error) {
		t.Fatal("reader must not be called once the marker is set")
		return nil, nil
	})
}

func TestMigrateLegacyDiscordUserLinks_NilReaderLeavesMarkerUnset(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	sid := int(insertTestServer(t, db, "S"))
	// No Postgres pool this boot → nil reader, marker stays unset so a later boot
	// retries.
	migrateLegacyDiscordUserLinks(db, sid, nil)
	if v, _ := metaGet(db, "migrated:discord_user_links"); v != "" {
		t.Errorf("marker should stay unset with no reader, got %q", v)
	}
}

// ── announce → server's own channel ────────────────────────────────────────────

func TestAnnounceToServer_PostsToServerChannel(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	a := int(insertTestServer(t, db, "A"))
	b := int(insertTestServer(t, db, "B"))
	c := int(insertTestServer(t, db, "C"))
	store := newDiscordGuildsStore(db)
	mustUpsertServerLink(t, store, discordServerLink{ServerID: a, GuildID: "g1", AnnounceChannelID: "chan-a"})
	mustUpsertServerLink(t, store, discordServerLink{ServerID: b, GuildID: "g2", AnnounceChannelID: ""}) // no channel
	// server c has no link at all.
	_ = c

	prevStore := globalDiscordGuildsStore
	prevPost := announceSend
	prevAnnounce := loadedConfig.DiscordAnnounceChannelID
	t.Cleanup(func() {
		globalDiscordGuildsStore = prevStore
		announceSend = prevPost
		loadedConfig.DiscordAnnounceChannelID = prevAnnounce
	})
	globalDiscordGuildsStore = store
	loadedConfig.DiscordAnnounceChannelID = "" // no legacy fallback for the base cases

	var got []string
	announceSend = func(channelID, _ string) error {
		got = append(got, channelID)
		return nil
	}

	if err := announceToServer(a, "hi"); err != nil {
		t.Fatalf("announceToServer a: %v", err)
	}
	if len(got) != 1 || got[0] != "chan-a" {
		t.Errorf("server a announce = %v, want [chan-a]", got)
	}

	// Server with empty channel → no post.
	got = nil
	if err := announceToServer(b, "hi"); err != nil {
		t.Fatalf("announceToServer b: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("server b (no channel) should not post, got %v", got)
	}

	// Server with no link → no post, no error.
	if err := announceToServer(c, "hi"); err != nil {
		t.Fatalf("announceToServer c: %v", err)
	}

	// With a legacy global announce channel configured, servers without their own
	// channel (b: empty, c: no link) fall back to it instead of dropping the post.
	loadedConfig.DiscordAnnounceChannelID = "global-chan"
	got = nil
	if err := announceToServer(b, "hi"); err != nil {
		t.Fatalf("announceToServer b (fallback): %v", err)
	}
	if err := announceToServer(c, "hi"); err != nil {
		t.Fatalf("announceToServer c (fallback): %v", err)
	}
	if len(got) != 2 || got[0] != "global-chan" || got[1] != "global-chan" {
		t.Errorf("fallback announce = %v, want [global-chan global-chan]", got)
	}

	// A server WITH its own channel still prefers it over the global fallback.
	got = nil
	if err := announceToServer(a, "hi"); err != nil {
		t.Fatalf("announceToServer a (fallback set): %v", err)
	}
	if len(got) != 1 || got[0] != "chan-a" {
		t.Errorf("server a should prefer its own channel, got %v", got)
	}
}
