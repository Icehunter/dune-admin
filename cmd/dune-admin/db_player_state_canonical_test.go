package main

import (
	"strings"
	"testing"
)

// These tests pin the #290 follow-up: every query that resolves a
// dune.player_state row per account must use ONE canonical-row definition —
// "most recently active" (last_login_time DESC NULLS LAST, id DESC) — via a
// LATERAL join (or DISTINCT ON), never a bare account_id join that fans out
// when an account has duplicate player_state rows.

const canonicalOrderFragment = "last_login_time DESC NULLS LAST"

// barePlayerStateJoin is the fan-out pattern that must not reappear.
const barePlayerStateJoin = "LEFT JOIN dune.player_state ps ON ps.account_id"

func TestPlayerStateCanonicalJoinOn_GeneratesAliasedLateral(t *testing.T) {
	t.Parallel()
	got := playerStateCanonicalJoinOn("sa", "sps")
	for _, want := range []string{
		"LEFT JOIN LATERAL",
		"sa.owner_account_id",
		") sps ON true",
		canonicalOrderFragment,
		"LIMIT 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("playerStateCanonicalJoinOn missing %q:\n%s", want, got)
		}
	}
}

func TestPlayerStateFanOutQueriesUseCanonicalJoin(t *testing.T) {
	t.Parallel()
	queries := map[string]string{
		"playerStateCanonicalJoin": playerStateCanonicalJoin,
		"serverCountsSQL":          serverCountsSQL,
		"serverByFactionSQL":       serverByFactionSQL,
		"findPlayersByNameSQL":     findPlayersByNameSQL,
		"guildMembersSQL":          guildMembersSQL,
		"guildInvitesSQL":          guildInvitesSQL,
	}
	for name, sql := range queries {
		if !strings.Contains(sql, "LEFT JOIN LATERAL") {
			t.Errorf("%s must use the canonical LATERAL join", name)
		}
		if !strings.Contains(sql, canonicalOrderFragment) {
			t.Errorf("%s must order by the canonical most-recently-active definition", name)
		}
		if strings.Contains(sql, barePlayerStateJoin) {
			t.Errorf("%s still contains the bare fan-out join", name)
		}
	}
}

// TestGuildInvitesSQL_CanonicalisesSenderJoinToo — the invites query joins
// player_state twice (invitee + sender); both must be canonical.
func TestGuildInvitesSQL_CanonicalisesSenderJoinToo(t *testing.T) {
	t.Parallel()
	if got := strings.Count(guildInvitesSQL, "LEFT JOIN LATERAL"); got != 2 {
		t.Fatalf("guildInvitesSQL LATERAL joins = %d, want 2 (invitee + sender)", got)
	}
	if strings.Contains(guildInvitesSQL, "LEFT JOIN dune.player_state sps ON sps.account_id") {
		t.Fatal("guildInvitesSQL sender join still bare")
	}
}

// TestResolvePlayerCharacterIDSQL_UsesCanonicalDefinition — the character-id
// resolver must pick the SAME row the players list shows. It previously
// ordered by oldest id, so tag/journey edits could silently target a stale
// duplicate while the list showed the live character.
func TestResolvePlayerCharacterIDSQL_UsesCanonicalDefinition(t *testing.T) {
	t.Parallel()
	if !strings.Contains(resolvePlayerCharacterIDSQL, canonicalOrderFragment) {
		t.Fatalf("resolvePlayerCharacterIDSQL must use the canonical ordering:\n%s", resolvePlayerCharacterIDSQL)
	}
	if strings.Contains(resolvePlayerCharacterIDSQL, "ORDER BY id LIMIT 1") {
		t.Fatal("resolvePlayerCharacterIDSQL still picks the oldest row")
	}
}

// TestOnlineStateSQL_OneRowPerAccountAndNoOrphans — the activity view reads
// player_state directly, so it needs DISTINCT ON for duplicates and an
// accounts EXISTS filter so orphaned rows don't show as phantom players.
func TestOnlineStateSQL_OneRowPerAccountAndNoOrphans(t *testing.T) {
	t.Parallel()
	for _, want := range []string{
		"DISTINCT ON (ps.account_id)",
		canonicalOrderFragment,
		"EXISTS (SELECT 1 FROM dune.accounts",
	} {
		if !strings.Contains(onlineStateSQL, want) {
			t.Fatalf("onlineStateSQL missing %q:\n%s", want, onlineStateSQL)
		}
	}
}

// TestCheckPlayerOfflineSQL_ConservativeOnDuplicates — the offline guard
// protects live-state writes; with duplicate rows for one pawn it must
// prefer any non-Offline row (fail closed) rather than reading an arbitrary
// first row.
func TestCheckPlayerOfflineSQL_ConservativeOnDuplicates(t *testing.T) {
	t.Parallel()
	if !strings.Contains(checkPlayerOfflineSQL, `ORDER BY (online_status::text = 'Offline')`) {
		t.Fatalf("checkPlayerOfflineSQL must sort non-Offline rows first:\n%s", checkPlayerOfflineSQL)
	}
	if !strings.Contains(checkPlayerOfflineSQL, "LIMIT 1") {
		t.Fatal("checkPlayerOfflineSQL must bound to one row")
	}
}
