package main

import (
	"context"
	"log"
	"strings"
)

// ── Message of the Day (#163/#167/#135) ─────────────────────────────────────
// A configurable in-game message whispered to a player every time they join the
// server (a per-session trigger, distinct from the welcome package's once-per-
// version grant message). Join detection is the presenceTracker diffing the
// online set across scanner ticks; the send reuses the proven GM-whisper path.

// motdDefaultPlayerName is substituted for {player} when a joining account has
// no resolvable character name, so the message is never malformed.
const motdDefaultPlayerName = "traveler"

// renderMOTD substitutes message placeholders with the joining player's details.
// v1 supports {player} (the character name). Kept pure so it is trivially
// testable and free of side effects.
func renderMOTD(template string, acc welcomeAccount) string {
	name := strings.TrimSpace(acc.CharacterName)
	if name == "" {
		name = motdDefaultPlayerName
	}
	return strings.ReplaceAll(template, "{player}", name)
}

// motdWhisper is one resolved message to send: recipient account, sender
// identity (blank → seeded GM persona), and the rendered text.
type motdWhisper struct {
	accountID    int64
	sourcePlayer string
	message      string
}

// motdWhispersForJoins builds the whispers to send for a set of join events
// under the given MOTD config. Returns nil when MOTD is disabled or the message
// is blank, so the caller never sends an empty message. Pure (no side effects).
func motdWhispersForJoins(joins []welcomeAccount, enabled bool, message, sourcePlayer string) []motdWhisper {
	if !enabled || strings.TrimSpace(message) == "" {
		return nil
	}
	out := make([]motdWhisper, 0, len(joins))
	for _, acc := range joins {
		out = append(out, motdWhisper{
			accountID:    acc.AccountID,
			sourcePlayer: sourcePlayer,
			message:      renderMOTD(message, acc),
		})
	}
	return out
}

// welcomePresence is the join-detection state for the MOTD feature. Touched only
// by the single welcome-scanner goroutine, so it needs no synchronisation.
var welcomePresence = newPresenceTracker()

// runMOTDOnJoin observes the current online set, then whispers the configured
// MOTD to each newly-joined player. Called from the scanner tick with the
// online snapshot already fetched (shared with the package scan).
func runMOTDOnJoin(ctx context.Context, rt welcomePackageRuntime, online []welcomeAccount) {
	joins := welcomePresence.observe(online)
	for _, m := range motdWhispersForJoins(joins, rt.motdEnabled, rt.motdMessage, rt.motdSourcePlayer) {
		if err := sendWelcomeWhisper(ctx, m.accountID, m.sourcePlayer, m.message); err != nil {
			log.Printf("motd: whisper to account %d failed: %v", m.accountID, err)
		}
	}
}
