package main

import (
	"context"
	"log"
	"strings"
	"unicode"
)

// ── Region join/leave broadcast (#167) ──────────────────────────────────────
// When a player joins or leaves a region, whisper a configurable notice to every
// player currently online in that same region. There is NO region/zone chat
// channel in the wire plumbing — only direct whispers (chat.whispers) — so this
// is the closest feasible "region chat" behaviour: it reuses the proven GM-
// whisper path (sendWelcomeWhisper / rmqSendWhisper). Distinct from the MOTD,
// which is a private whisper to the joining player only.

// regionDefaultPlayerName is substituted for {player} when an account has no
// resolvable character name, so the message is never malformed.
const regionDefaultPlayerName = "A traveler"

// regionBroadcastConfig is the live config for the join/leave broadcast. The
// join and leave halves are toggled independently.
type regionBroadcastConfig struct {
	joinEnabled   bool
	leaveEnabled  bool
	joinTemplate  string
	leaveTemplate string
	// sourcePlayer is the sender identity (blank → seeded GM persona).
	sourcePlayer string
}

// regionAnnouncement is one rendered notice bound to a region: every online
// player in that region should receive `text`.
type regionAnnouncement struct {
	region       string
	text         string
	sourcePlayer string
}

// prettyRegionName turns an internal map key (e.g. "HaggaBasin", "Map_TheShield", "Survival_1")
// into a human-readable label ("Hagga Basin", "The Shield"). It strips a leading
// "Map_" and trailing "_0", handles specific hardcoded aliases, then inserts spaces.
func prettyRegionName(region string) string {
	region = strings.TrimSpace(region)
	region = strings.TrimPrefix(region, "Map_")
	region = strings.TrimPrefix(region, "SH_")

	// Strip trailing _0, _1, etc.
	if idx := strings.LastIndexByte(region, '_'); idx != -1 {
		hasDigit := false
		onlyDigits := true
		for i := idx + 1; i < len(region); i++ {
			if region[i] >= '0' && region[i] <= '9' {
				hasDigit = true
			} else {
				onlyDigits = false
				break
			}
		}
		if hasDigit && onlyDigits {
			region = region[:idx]
		}
	}

	// Hardcoded region aliases
	switch strings.ToLower(region) {
	case "survival":
		return "Hagga Basin"
	case "overmap":
		return "Overland"
	case "deepdesert":
		return "Deep Desert"
	}

	if region == "" {
		return ""
	}

	// Auto-format CamelCase to Camel Case
	runes := []rune(region)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]) && runes[i-1] != ' ' {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// renderRegionAnnouncement substitutes {player} and {region} in template. The
// region is humanised via prettyRegionName; a blank player name falls back to
// regionDefaultPlayerName. Pure (no side effects), so it is trivially testable.
func renderRegionAnnouncement(template, player, region string) string {
	name := strings.TrimSpace(player)
	if name == "" {
		name = regionDefaultPlayerName
	}
	out := strings.ReplaceAll(template, "{player}", name)
	out = strings.ReplaceAll(out, "{region}", prettyRegionName(region))
	return out
}

// regionAnnouncementsFor builds the announcements to send for the given join and
// leave events under cfg. Disabled halves and blank templates produce nothing
// (we never whisper an empty body); events whose region is unknown are skipped
// (no one to target). Pure (no side effects).
func regionAnnouncementsFor(joins, leaves []welcomeAccount, cfg regionBroadcastConfig) []regionAnnouncement {
	out := make([]regionAnnouncement, 0, len(joins)+len(leaves))
	out = appendAnnouncements(out, joins, cfg.joinEnabled, cfg.joinTemplate, cfg.sourcePlayer)
	out = appendAnnouncements(out, leaves, cfg.leaveEnabled, cfg.leaveTemplate, cfg.sourcePlayer)
	return out
}

func appendAnnouncements(out []regionAnnouncement, accts []welcomeAccount, enabled bool, template, sourcePlayer string) []regionAnnouncement {
	if !enabled || strings.TrimSpace(template) == "" {
		return out
	}
	for _, acc := range accts {
		if strings.TrimSpace(acc.Region) == "" {
			continue
		}
		out = append(out, regionAnnouncement{
			region:       acc.Region,
			text:         renderRegionAnnouncement(template, acc.CharacterName, acc.Region),
			sourcePlayer: sourcePlayer,
		})
	}
	return out
}

// regionChatSender sends one whisper to a recipient account. It mirrors the
// sendWelcomeWhisper signature so the production path can pass it directly while
// tests inject a fake.
type regionChatSender func(ctx context.Context, accountID int64, sourcePlayer, message string) error

// runRegionBroadcastOnJoinLeave whispers each announcement (built from the
// join/leave events under cfg) to every player in `online` who is currently in
// the announcement's region. Send failures are logged, never fatal, so one bad
// recipient can't suppress the rest.
func runRegionBroadcastOnJoinLeave(ctx context.Context, joins, leaves, online []welcomeAccount, cfg regionBroadcastConfig, send regionChatSender) {
	anns := regionAnnouncementsFor(joins, leaves, cfg)
	if len(anns) == 0 {
		return
	}
	for _, ann := range anns {
		for _, player := range online {
			if player.Region != ann.region {
				continue
			}
			if err := send(ctx, player.AccountID, ann.sourcePlayer, ann.text); err != nil {
				log.Printf("region-broadcast: whisper to account %d failed: %v", player.AccountID, err)
			}
		}
	}
}
