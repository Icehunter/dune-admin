package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ── Server state ──────────────────────────────────────────────────────────────

// serverState is the coarse lifecycle phase shown in the status embed.
type serverState string

const (
	serverStateOffline    serverState = "Offline"
	serverStateBooting    serverState = "Booting"
	serverStateOnline     serverState = "Online"
	serverStateRestarting serverState = "Restarting"
)

// Embed accent colours (RGB ints) per state.
const (
	statusColorOffline    = 0x95A5A6 // grey
	statusColorBooting    = 0xF1C40F // yellow
	statusColorOnline     = 0x2ECC71 // green
	statusColorRestarting = 0xE67E22 // orange
)

// statusMinInterval is the floor for the update interval to avoid Discord rate
// limits and pointless churn. The configured value is clamped up to this.
const statusMinInterval = 30 * time.Second

// statusMessageMetaKey is the meta-table key under which the posted embed's
// "channelID:messageID" is persisted so restarts edit, not duplicate.
const statusMessageMetaKey = "discord_status_message"

// ── Embed data ────────────────────────────────────────────────────────────────

// mapPlayerCount is the player population on a single active map.
type mapPlayerCount struct {
	Map     string
	Players int
}

// statusEmbedData is the fully-resolved data the embed builder renders. It is
// produced by collectStatusData and consumed by buildStatusEmbed; keeping them
// separate makes the builder a pure, unit-testable function.
type statusEmbedData struct {
	ServerTitle   string
	State         serverState
	Maps          []mapPlayerCount
	CurrentOnline int   // players currently online (best-effort)
	TotalPlayers  int64 // total characters known to the server (best-effort)
	UniquePlayers int64 // unique players seen in the last 24h
}

// ── Embed builder (pure) ──────────────────────────────────────────────────────

// buildStatusEmbed renders statusEmbedData into a discordgo.MessageEmbed.
// The body text is English/templated — only UI strings in the dashboard are
// localized; the embed is a Discord-facing artifact shared by all users.
func buildStatusEmbed(data statusEmbedData, now time.Time) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:     "🏜️ Dune Awakening — Server Status",
		Color:     statusColor(data.State),
		Timestamp: now.UTC().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Auto-updated by Dune-Admin",
		},
		Fields:    []*discordgo.MessageEmbedField{},
	}

	// Assemble everything into the Description for precise top-to-bottom layout
	var desc strings.Builder

	// 1. Server Status
	fmt.Fprintf(&desc, "%s **Server Status:** %s\n\u200B\n", statusEmoji(data.State), data.State)

	// 2. Active Maps
	desc.WriteString("🌍 **Active Maps**\n")
	if len(data.Maps) > 0 {
		desc.WriteString(formatMapLines(data.Maps))
	} else {
		desc.WriteString("*Nobody is currently online.*")
	}
	desc.WriteString("\n\u200B\n")

	// 3. Population Footer
	fmt.Fprintf(&desc, "👥 **%d Online** | **%d Total** | **%d Unique (24h)**",
		data.CurrentOnline, data.TotalPlayers, data.UniquePlayers)

	embed.Description = desc.String()

	return embed
}

// formatMapLines renders one "• **Map** — N players" line per map.
func formatMapLines(maps []mapPlayerCount) string {
	var sb strings.Builder
	for _, m := range maps {
		name := m.Map
		if name == "" {
			name = "Unknown"
		}
		// Fika style: "• **MapName** — N player(s)"
		fmt.Fprintf(&sb, "• **%s** — %d player(s)\n", name, m.Players)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func statusColor(s serverState) int {
	switch s {
	case serverStateOnline:
		return statusColorOnline
	case serverStateBooting:
		return statusColorBooting
	case serverStateRestarting:
		return statusColorRestarting
	default:
		return statusColorOffline
	}
}

func statusEmoji(s serverState) string {
	switch s {
	case serverStateOnline:
		return "🟢"
	case serverStateBooting:
		return "🟡"
	case serverStateRestarting:
		return "🟠"
	default:
		return "⚫"
	}
}

// ── Server-state + per-map derivation ─────────────────────────────────────────

// deriveServerState maps a control-plane status into a coarse serverState.
// A nil status or error means the server is treated as Offline. When servers
// exist but none report Ready, the server is Booting.
func deriveServerState(status *BattlegroupStatus, err error) serverState {
	if err != nil || status == nil || len(status.Servers) == 0 {
		return serverStateOffline
	}
	for _, s := range status.Servers {
		if s.Ready || strings.EqualFold(s.Phase, "Running") {
			return serverStateOnline
		}
	}
	return serverStateBooting
}

// aggregateMapCounts sums per-partition player counts by map name, returning a
// stable, descending-by-population slice. Blank map names bucket as "Unknown".
func aggregateMapCounts(servers []ServerRow) []mapPlayerCount {
	totals := map[string]int{}
	for _, s := range servers {
		name := prettyRegionName(s.Map)
		if name == "" {
			name = prettyRegionName(s.Sietch)
		}
		if name == "" {
			name = "Unknown"
		}
		totals[name] += s.Players
	}
	out := make([]mapPlayerCount, 0, len(totals))
	for name, players := range totals {
		out = append(out, mapPlayerCount{Map: name, Players: players})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Players != out[j].Players {
			return out[i].Players > out[j].Players
		}
		return out[i].Map < out[j].Map
	})
	return out
}

// ── 24h unique players (SQLite) ───────────────────────────────────────────────

// countUniquePlayers24h returns the number of distinct accounts that started a
// play session in the 24 hours preceding now. A nil db yields 0 with no error
// (session tracking disabled — the embed degrades gracefully).
func countUniquePlayers24h(ctx context.Context, db *sql.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	since := now.UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var count int64
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT account_id) FROM play_sessions WHERE started_at >= ?`,
		since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unique 24h: %w", err)
	}
	return count, nil
}

// ── Persistence (meta table in the unified SQLite store) ──────────────────────

// statusMessageStore persists the posted embed's channel + message ID so the
// loop edits the same message across restarts rather than re-posting.
type statusMessageStore interface {
	loadStatusMessage() (channelID, messageID string, err error)
	saveStatusMessage(channelID, messageID string) error
}

// sqliteStatusStore implements statusMessageStore on the unified store's meta
// table, storing "channelID:messageID" under statusMessageMetaKey.
type sqliteStatusStore struct {
	db *sql.DB
}

func newSqliteStatusStore(db *sql.DB) *sqliteStatusStore {
	return &sqliteStatusStore{db: db}
}

func (s *sqliteStatusStore) loadStatusMessage() (string, string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, statusMessageMetaKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("load status message: %w", err)
	}
	channelID, messageID, _ := strings.Cut(raw, ":")
	return channelID, messageID, nil
}

func (s *sqliteStatusStore) saveStatusMessage(channelID, messageID string) error {
	value := channelID + ":" + messageID
	_, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		statusMessageMetaKey, value)
	if err != nil {
		return fmt.Errorf("save status message: %w", err)
	}
	return nil
}

// ── Post-or-edit ──────────────────────────────────────────────────────────────

// statusEmbedSender is the thin slice of *discordgo.Session needed to publish
// the status embed. Defined as an interface so the loop is unit-testable with a
// fake session.
type statusEmbedSender interface {
	ChannelMessageSendEmbed(channelID string, embed *discordgo.MessageEmbed) (messageID string, error error)
	ChannelMessageEditEmbed(channelID, messageID string, embed *discordgo.MessageEmbed) error
}

// discordSessionAdapter adapts a *discordgo.Session to statusEmbedSender,
// discarding the *discordgo.Message returned by send and keeping just its ID.
type discordSessionAdapter struct {
	sess *discordgo.Session
}

func (a discordSessionAdapter) ChannelMessageSendEmbed(channelID string, embed *discordgo.MessageEmbed) (string, error) {
	msg, err := a.sess.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

func (a discordSessionAdapter) ChannelMessageEditEmbed(channelID, messageID string, embed *discordgo.MessageEmbed) error {
	_, err := a.sess.ChannelMessageEditEmbed(channelID, messageID, embed)
	return err
}

// postOrEditStatusEmbed edits the stored message when one exists for the target
// channel; otherwise (no stored ID, channel changed, or the stored message was
// deleted) it sends a fresh embed and persists the new ID.
func postOrEditStatusEmbed(sess statusEmbedSender, store statusMessageStore, channelID string, embed *discordgo.MessageEmbed) error {
	storedChannel, storedMsg, err := store.loadStatusMessage()
	if err != nil {
		log.Printf("discord status: load stored message: %v", err)
		// Fall through and treat as no stored message.
		storedChannel, storedMsg = "", ""
	}

	if storedMsg != "" && storedChannel == channelID {
		editErr := sess.ChannelMessageEditEmbed(channelID, storedMsg, embed)
		if editErr == nil {
			return nil
		}
		if !isUnknownMessageErr(editErr) {
			return fmt.Errorf("edit status embed: %w", editErr)
		}
		// Stored message was deleted in Discord — fall through to re-send.
		log.Printf("discord status: stored message gone, re-posting")
	}

	return sendAndPersist(sess, store, channelID, embed)
}

// sendAndPersist sends a new embed and persists its ID + channel.
func sendAndPersist(sess statusEmbedSender, store statusMessageStore, channelID string, embed *discordgo.MessageEmbed) error {
	newID, err := sess.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return fmt.Errorf("send status embed: %w", err)
	}
	if err := store.saveStatusMessage(channelID, newID); err != nil {
		return fmt.Errorf("persist status message id: %w", err)
	}
	return nil
}

// isUnknownMessageErr reports whether err is Discord's "Unknown Message" REST
// error (the message was deleted), in which case we should re-post.
func isUnknownMessageErr(err error) bool {
	var rest *discordgo.RESTError
	if errors.As(err, &rest) && rest.Message != nil {
		return rest.Message.Code == discordgo.ErrCodeUnknownMessage
	}
	return false
}

// ── Loop lifecycle ────────────────────────────────────────────────────────────

// statusLoopDeps are the injectable inputs to runStatusLoop.
type statusLoopDeps struct {
	interval time.Duration
	tick     func(ctx context.Context)
}

// globalStatusCancel stops the running status loop; guarded by statusLoopMu.
var (
	statusLoopMu        sync.Mutex
	globalStatusCancel  context.CancelFunc
	statusLoopIsRunning bool
)

// runStatusLoop ticks at deps.interval and invokes deps.tick until ctx is done.
// It fires once immediately so the embed appears without waiting a full interval.
func runStatusLoop(ctx context.Context, deps statusLoopDeps) {
	if deps.tick == nil {
		return
	}
	interval := deps.interval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	deps.tick(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deps.tick(ctx)
		}
	}
}

// discordStatusEnabled returns the effective status-embed enabled flag
// (default off — opt-in).
func discordStatusEnabled(cfg appConfig) bool {
	if cfg.DiscordStatusEnabled == nil {
		return false
	}
	return *cfg.DiscordStatusEnabled
}

// discordStatusInterval returns the configured update interval, defaulting to
// 60s and clamping up to statusMinInterval.
func discordStatusInterval(cfg appConfig) time.Duration {
	if cfg.DiscordStatusIntervalSeconds <= 0 {
		return 60 * time.Second
	}
	d := time.Duration(cfg.DiscordStatusIntervalSeconds) * time.Second
	if d < statusMinInterval {
		return statusMinInterval
	}
	return d
}

// statusLoopRunning reports whether the status loop goroutine is active.
func statusLoopRunning() bool {
	statusLoopMu.Lock()
	defer statusLoopMu.Unlock()
	return statusLoopIsRunning
}

// stopDiscordStatusLoop cancels the running status loop (if any).
func stopDiscordStatusLoop() {
	statusLoopMu.Lock()
	defer statusLoopMu.Unlock()
	if globalStatusCancel != nil {
		globalStatusCancel()
		globalStatusCancel = nil
	}
	statusLoopIsRunning = false
}

// applyDiscordStatusLoop stops any running status loop and starts a new one if
// the config enables it and a channel is set. Mirrors applyEventEngine /
// applyDiscordConfig so toggling takes effect without a process restart.
func applyDiscordStatusLoop(cfg appConfig) {
	stopDiscordStatusLoop()

	if !discordStatusEnabled(cfg) || cfg.DiscordStatusChannelID == "" {
		return
	}

	channelID := cfg.DiscordStatusChannelID
	deps := statusLoopDeps{
		interval: discordStatusInterval(cfg),
		tick: func(ctx context.Context) {
			runStatusTick(ctx, channelID)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	statusLoopMu.Lock()
	globalStatusCancel = cancel
	statusLoopIsRunning = true
	statusLoopMu.Unlock()

	go runStatusLoop(ctx, deps)
	log.Printf("discord status: embed loop started (channel %s, every %s)", channelID, deps.interval)
}

// runStatusTick collects status data, builds the embed, and posts-or-edits it.
// Guards: a live bot session, the unified store, and a channel must all be
// present. Errors are logged, never fatal — the loop keeps ticking.
func runStatusTick(ctx context.Context, channelID string) {
	sess, _ := getDiscordState()
	if sess == nil {
		return // bot not connected yet; try again next tick
	}
	if globalStore == nil {
		log.Printf("discord status: unified store unavailable — skipping tick")
		return
	}

	data := collectStatusData(ctx)
	embed := buildStatusEmbed(data, time.Now())

	store := newSqliteStatusStore(globalStore)
	if err := postOrEditStatusEmbed(discordSessionAdapter{sess: sess}, store, channelID, embed); err != nil {
		log.Printf("discord status: post/edit: %v", err)
	}
}

// collectStatusData gathers the live status from the control plane, DB, and the
// session store. Every source is best-effort: a failure degrades that field
// rather than aborting the tick.
func collectStatusData(ctx context.Context) statusEmbedData {
	data := statusEmbedData{State: serverStateOffline}
	applyControlStatus(ctx, &data)
	applyDBStats(ctx, &data)
	applyUnique24h(ctx, &data)
	return data
}

// applyControlStatus fills State, Maps, and CurrentOnline from the control plane.
func applyControlStatus(ctx context.Context, data *statusEmbedData) {
	if globalControl == nil {
		return
	}
	status, err := globalControl.GetStatus(ctx, globalExecutor)
	data.State = deriveServerState(status, err)
	if err != nil || status == nil {
		return
	}
	data.ServerTitle = status.Title
	if data.ServerTitle == "" {
		data.ServerTitle = status.Name
	}
	data.Maps = aggregateMapCounts(status.Servers)
	for _, m := range data.Maps {
		data.CurrentOnline += m.Players
	}
}

// applyDBStats fills TotalPlayers and supplies fallbacks for CurrentOnline and
// Maps when the control plane provided none.
func applyDBStats(ctx context.Context, data *statusEmbedData) {
	if globalDB == nil {
		return
	}
	stats, err := cmdFetchServerStats(ctx, globalDB)
	if err != nil {
		log.Printf("discord status: server stats: %v", err)
		return
	}
	data.TotalPlayers = stats.TotalPlayers
	if data.CurrentOnline == 0 {
		data.CurrentOnline = int(stats.OnlinePlayers)
	}
	if len(data.Maps) == 0 {
		data.Maps = mapCountsFromLabeled(stats.ByMap)
	}
}

// applyUnique24h fills UniquePlayers from the session store.
func applyUnique24h(ctx context.Context, data *statusEmbedData) {
	uniq, err := countUniquePlayers24h(ctx, globalStore, time.Now())
	if err != nil {
		log.Printf("discord status: unique 24h: %v", err)
		return
	}
	data.UniquePlayers = uniq
}

// mapCountsFromLabeled converts the DB by-map distribution into mapPlayerCount.
// This counts characters per map (not live online) — used only as a fallback
// when the control plane provides no live per-partition counts.
func mapCountsFromLabeled(labeled []labeledCount) []mapPlayerCount {
	out := make([]mapPlayerCount, 0, len(labeled))
	for _, l := range labeled {
		out = append(out, mapPlayerCount{Map: l.Label, Players: int(l.Count)})
	}
	return out
}
