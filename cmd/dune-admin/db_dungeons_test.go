package main

import (
	"strings"
	"testing"
)

// Dungeon history (#318: "Player -> Actions -> History -> Dungeon records not
// tracking").
//
// dune.dungeon_completion_players.player_id is a foreign key to actors(id), and
// a player owns three actor rows: controller, state and pawn. The reporter
// suspected the wrong one was being used, and a query against their server
// settled it — every row's class was
//
//	/Game/Dune/Characters/Player/BP_DunePlayerController.BP_DunePlayerController_C
//
// so the game stores the PlayerController actor. The UI passed playerInfo.ID,
// which is the character (pawn) actor, so the lookup never matched and the table
// was always empty.
//
// The query resolves the controller itself rather than relying on every caller
// passing the right one, because both ids are in play across this API: the
// vehicles endpoint is keyed by controller id while the stats endpoints are keyed
// by account id. Resolving here means a pawn id and a controller id both work.
func TestPlayerDungeonsSQLResolvesController(t *testing.T) {
	t.Parallel()

	if !strings.Contains(playerDungeonsSQL, "dune.player_state") {
		t.Errorf("dungeon query does not resolve through dune.player_state, so a pawn id "+
			"will never match the controller id the game stores:\n%s", playerDungeonsSQL)
	}
	if !strings.Contains(playerDungeonsSQL, "player_controller_id") {
		t.Errorf("dungeon query does not select player_controller_id:\n%s", playerDungeonsSQL)
	}
	if !strings.Contains(playerDungeonsSQL, "player_pawn_id") {
		t.Errorf("dungeon query does not map the incoming pawn id via player_pawn_id:\n%s", playerDungeonsSQL)
	}
	// A controller id passed straight in must still work, so the resolution has
	// to fall back to the argument rather than yielding NULL.
	if !strings.Contains(playerDungeonsSQL, "COALESCE") {
		t.Errorf("dungeon query does not fall back to the supplied id, so passing a "+
			"controller id directly would match nothing:\n%s", playerDungeonsSQL)
	}
	// The join to the completion rows must survive.
	for _, want := range []string{
		"dune.dungeon_completion_players",
		"dune.dungeon_completion",
		"dc.completion_id = dcp.completion_id",
	} {
		if !strings.Contains(playerDungeonsSQL, want) {
			t.Errorf("dungeon query lost %q:\n%s", want, playerDungeonsSQL)
		}
	}
}

// TestHistorySectionPassesControllerID pins the caller side. playerInfo carries
// both ids; the dungeon lookup must not silently go back to the pawn.
func TestPlayerDungeonsQueryBindsOneArgument(t *testing.T) {
	t.Parallel()
	// $1 is used in both the resolution subquery and the fallback, and there must
	// be no $2 — the caller passes a single id.
	if strings.Contains(playerDungeonsSQL, "$2") {
		t.Errorf("dungeon query expects more than one bound argument:\n%s", playerDungeonsSQL)
	}
	if !strings.Contains(playerDungeonsSQL, "$1") {
		t.Errorf("dungeon query binds no argument:\n%s", playerDungeonsSQL)
	}
}
