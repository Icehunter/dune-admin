package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
		Fields: []*discordgo.MessageEmbedField{},
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

// partitionLabel returns the display label for a single ServerRow.
//
// The Survival / Hagga Basin map is the operator's home instance, so it is
// labeled with the configured server name from the director (DisplayName, e.g.
// "Sietch Umbu"). Every other map uses its pretty region name (Overland, Deep
// Desert, …): the director's per-partition label is an internal codename for
// some maps (e.g. "Abbir") and inconsistent across maps, so it is used only as
// a fallback for unrecognised maps. Falls back to "Unknown".
func partitionLabel(s ServerRow) string {
	if s.DisplayName != "" && isSurvivalMap(s.Map) {
		return s.DisplayName
	}
	if label := prettyRegionName(s.Map); label != "" {
		return label
	}
	if s.Sietch != "" {
		return s.Sietch
	}
	return "Unknown"
}

// isSurvivalMap reports whether a director map name is the Survival / Hagga
// Basin map (e.g. "Survival_1") — the operator's home instance.
func isSurvivalMap(mapName string) bool {
	m := strings.TrimSpace(mapName)
	m = strings.TrimPrefix(m, "Map_")
	m = strings.TrimPrefix(m, "SH_")
	m = stripTrailingNumericSuffix(m)
	return strings.EqualFold(m, "Survival")
}

type partitionRow struct {
	baseLabel string
	players   int
	partition int
}

// groupByLabel buckets rows by base label, preserving first-occurrence order,
// and sorts each group by partition index for deterministic #N suffix assignment.
func groupByLabel(rows []partitionRow) ([]string, map[string][]partitionRow) {
	order := make([]string, 0, len(rows))
	groups := make(map[string][]partitionRow, len(rows))
	for _, r := range rows {
		if _, seen := groups[r.baseLabel]; !seen {
			order = append(order, r.baseLabel)
		}
		groups[r.baseLabel] = append(groups[r.baseLabel], r)
	}
	for k := range groups {
		sort.Slice(groups[k], func(i, j int) bool {
			return groups[k][i].partition < groups[k][j].partition
		})
	}
	return order, groups
}

// aggregateMapCounts returns one entry per partition, labeled by partitionLabel
// (server name for the Survival home instance, pretty region name otherwise). When
// multiple partitions share the same base label (e.g. several "Hagga Basin"
// shards), a " #N" suffix is appended using 1-based position ordered by
// Partition index. Single-partition groups keep the bare label. Output is
// sorted players-desc, label-asc.
func aggregateMapCounts(servers []ServerRow) []mapPlayerCount {
	rows := make([]partitionRow, 0, len(servers))
	for _, s := range servers {
		rows = append(rows, partitionRow{baseLabel: partitionLabel(s), players: s.Players, partition: s.Partition})
	}

	order, groups := groupByLabel(rows)

	out := make([]mapPlayerCount, 0, len(rows))
	for _, base := range order {
		group := groups[base]
		multi := len(group) > 1
		for i, r := range group {
			label := base
			if multi {
				label = fmt.Sprintf("%s #%d", base, i+1)
			}
			out = append(out, mapPlayerCount{Map: label, Players: r.players})
		}
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
func countUniquePlayers24h(ctx context.Context, db *sql.DB, serverID int, now time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	since := now.UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var count int64
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT account_id) FROM play_sessions WHERE server_id = ? AND started_at >= ?`,
		serverID, since).Scan(&count)
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

// sqliteStatusStore implements statusMessageStore on the server_discord_status
// table (keyed by (servers.id, guild_id) with an FK on server_id so server
// deletion cascades all its rows). guildID scopes the posted-message pointer so
// multiple guilds mapped to the same server each track their own embed.
type sqliteStatusStore struct {
	db       *sql.DB
	serverID int
	guildID  string
}

func newSqliteStatusStore(db *sql.DB, serverID int) *sqliteStatusStore {
	return &sqliteStatusStore{db: db, serverID: serverID}
}

// newSqliteStatusStoreForGuild scopes the status-message pointer to a single
// (server, guild) pair so per-guild loops never clobber each other.
func newSqliteStatusStoreForGuild(db *sql.DB, serverID int, guildID string) *sqliteStatusStore {
	return &sqliteStatusStore{db: db, serverID: serverID, guildID: guildID}
}

func (s *sqliteStatusStore) loadStatusMessage() (string, string, error) {
	var channelID, messageID string
	err := s.db.QueryRow(
		`SELECT channel_id, message_id FROM server_discord_status WHERE server_id = ? AND guild_id = ?`,
		s.serverID, s.guildID).Scan(&channelID, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("load status message: %w", err)
	}
	return channelID, messageID, nil
}

func (s *sqliteStatusStore) saveStatusMessage(channelID, messageID string) error {
	_, err := s.db.Exec(
		`INSERT INTO server_discord_status (server_id, guild_id, channel_id, message_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(server_id, guild_id) DO UPDATE SET channel_id = excluded.channel_id, message_id = excluded.message_id`,
		s.serverID, s.guildID, channelID, messageID)
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
		componentLog("discord").Warn().Err(err).Msg("status: load stored message failed")
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
		componentLog("discord").Info().Msg("status: stored message gone, re-posting")
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

// isMissingPermissionsErr reports whether err is a Discord permission/access
// failure (Missing Permissions / Missing Access). These are configuration
// problems — the bot can't view or post in the channel — not transient, so
// retrying won't help until an operator fixes the channel permissions.
func isMissingPermissionsErr(err error) bool {
	var rest *discordgo.RESTError
	if errors.As(err, &rest) && rest.Message != nil {
		return rest.Message.Code == discordgo.ErrCodeMissingPermissions ||
			rest.Message.Code == discordgo.ErrCodeMissingAccess
	}
	return false
}

// statusLogAction is what to log after a status post attempt, given whether the
// previous attempt for the same server was already failing.
type statusLogAction int

const (
	statusLogSuppress  statusLogAction = iota // same state as last tick — stay quiet
	statusLogWarn                             // newly failing — warn once
	statusLogRecovered                        // failing → succeeded — note the recovery
)

// statusPostFailed remembers, per server, whether the last status post failed so
// a persistent failure (e.g. missing permissions in an announcement channel)
// warns once instead of every tick, and a recovery is logged once.
var (
	statusPostFailMu sync.Mutex
	statusPostFailed = map[int]bool{}
)

// nextStatusLogAction records the latest outcome for serverID and reports what to
// log: warn on the first failure, suppress while the state is unchanged, and note
// a recovery when a previously-failing server posts successfully again.
func nextStatusLogAction(serverID int, failed bool) statusLogAction {
	statusPostFailMu.Lock()
	defer statusPostFailMu.Unlock()
	was := statusPostFailed[serverID]
	statusPostFailed[serverID] = failed
	switch {
	case failed && !was:
		return statusLogWarn
	case !failed && was:
		return statusLogRecovered
	default:
		return statusLogSuppress
	}
}

// resetStatusFailState forgets a server's failure memory so the next failure
// warns again. Called when a status loop (re)starts so reconfiguring a broken
// channel surfaces fresh feedback.
func resetStatusFailState(serverID int) {
	statusPostFailMu.Lock()
	delete(statusPostFailed, serverID)
	statusPostFailMu.Unlock()
}

// logStatusPostResult emits a throttled, actionable log line for a status post
// outcome so a misconfigured channel doesn't spam a warning every interval.
func logStatusPostResult(link discordServerLink, err error) {
	switch nextStatusLogAction(link.ServerID, err != nil) {
	case statusLogWarn:
		evt := componentLog("discord").Warn().
			Str("guild_id", link.GuildID).Int("server_id", link.ServerID).
			Str("channel_id", link.StatusChannelID).Err(err)
		if isMissingPermissionsErr(err) {
			evt = evt.Str("fix", "give the bot View Channel + Send Messages + Embed Links in the status channel; "+
				"announcement channels restrict who can post by default, so add the bot's role there or use a normal text channel")
		}
		evt.Msg("status: post/edit failed (further identical failures suppressed until it recovers)")
	case statusLogRecovered:
		componentLog("discord").Info().
			Str("guild_id", link.GuildID).Int("server_id", link.ServerID).
			Msg("status: posting recovered")
	case statusLogSuppress:
		// Same state as the previous tick — already logged; stay quiet.
	}
}

// ── Loop lifecycle ────────────────────────────────────────────────────────────

// statusLoopDeps are the injectable inputs to runStatusLoop.
type statusLoopDeps struct {
	interval time.Duration
	tick     func(ctx context.Context)
}

// statusLoopCancels holds one cancel func per running per-server status loop,
// keyed by server_id; guarded by statusLoopMu. Each server links to exactly one
// guild, so server_id alone keys the loop.
var (
	statusLoopMu      sync.Mutex
	statusLoopCancels = map[int]context.CancelFunc{}
)

// statusApplyMu serializes a full applyDiscordStatusLoops reconfigure. It's
// invoked from several concurrent request paths (config save, guild/server CRUD,
// server delete) plus startup; without this the stop-then-start sequence can
// interleave and orphan a status-loop goroutine.
var statusApplyMu sync.Mutex

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

// statusLoopRunning reports whether any per-guild status loop is active.
func statusLoopRunning() bool {
	statusLoopMu.Lock()
	defer statusLoopMu.Unlock()
	return len(statusLoopCancels) > 0
}

// stopDiscordStatusLoop cancels every running per-server status loop.
func stopDiscordStatusLoop() {
	statusLoopMu.Lock()
	defer statusLoopMu.Unlock()
	for sid, cancel := range statusLoopCancels {
		cancel()
		delete(statusLoopCancels, sid)
	}
}

// statusIntervalFromSeconds clamps a per-guild configured interval (seconds) to
// the same defaults/floor as the legacy global path.
func statusIntervalFromSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Second
	}
	d := time.Duration(seconds) * time.Second
	if d < statusMinInterval {
		return statusMinInterval
	}
	return d
}

// listStatusServerLinks returns every discord_servers link, or nil when the
// store is unavailable.
func listStatusServerLinks() []discordServerLink {
	if globalDiscordGuildsStore == nil {
		return nil
	}
	rows, err := globalDiscordGuildsStore.listServerLinks()
	if err != nil {
		componentLog("discord").Warn().Err(err).Msg("list server links for status loops failed")
		return nil
	}
	return rows
}

// applyDiscordStatusLoops stops all running status loops and starts one per
// status-enabled discord_servers link (keyed by server_id). Each server posts
// its own embed to its status channel in its guild. Toggling/CRUD takes effect
// without a process restart.
func applyDiscordStatusLoops() {
	statusApplyMu.Lock()
	defer statusApplyMu.Unlock()
	stopDiscordStatusLoop()
	for _, link := range listStatusServerLinks() {
		if !link.StatusEnabled || link.StatusChannelID == "" {
			continue
		}
		startServerStatusLoop(link)
	}
}

// startServerStatusLoop launches a single server's status loop and records its
// cancel func keyed by server_id.
func startServerStatusLoop(link discordServerLink) {
	row := link                        // capture
	resetStatusFailState(row.ServerID) // re-arm warnings for this (re)started loop
	deps := statusLoopDeps{
		interval: statusIntervalFromSeconds(row.StatusIntervalSeconds),
		tick: func(ctx context.Context) {
			runStatusTickForServer(ctx, row)
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	statusLoopMu.Lock()
	if old, ok := statusLoopCancels[row.ServerID]; ok {
		old() // never leak a prior loop for this server
	}
	statusLoopCancels[row.ServerID] = cancel
	statusLoopMu.Unlock()
	go runStatusLoop(ctx, deps)
	componentLog("discord").Info().Str("guild_id", row.GuildID).Int("server_id", row.ServerID).
		Str("channel_id", row.StatusChannelID).Dur("interval", deps.interval).
		Msg("status: per-server embed loop started")
}

// runStatusTickForServer collects status for the server, builds the embed, and
// posts-or-edits it in the server's status channel. Guards: a live bot session,
// the unified store, and a resolvable server. Errors are logged, never fatal —
// the loop keeps ticking.
func runStatusTickForServer(ctx context.Context, link discordServerLink) {
	sess, _ := getDiscordState()
	if sess == nil {
		return // bot not connected yet; try again next tick
	}
	if globalStore == nil {
		componentLog("discord").Info().Msg("status: unified store unavailable — skipping tick")
		return
	}
	sc := globalRegistry.Get(serverScope(link.ServerID))
	data := collectStatusData(ctx, sc, globalStore)
	embed := buildStatusEmbed(data, time.Now())

	store := newSqliteStatusStoreForGuild(globalStore, storeScopeForID(link.ServerID), link.GuildID)
	err := postOrEditStatusEmbed(discordSessionAdapter{sess: sess}, store, link.StatusChannelID, embed)
	logStatusPostResult(link, err)
}

// collectStatusData gathers the live status from the control plane, DB, and the
// session store. Every source is best-effort: a failure degrades that field
// rather than aborting the tick.
func collectStatusData(ctx context.Context, sc *ServerContext, sdb *sql.DB) statusEmbedData {
	data := statusEmbedData{State: serverStateOffline}
	applyControlStatus(ctx, sc, &data)
	applyDBStats(ctx, sc, &data)
	serverID := defaultServerID
	if sc != nil {
		serverID = sc.StoreScope
	}
	applyUnique24h(ctx, sdb, serverID, &data)
	return data
}

// applyControlStatus fills State, Maps, and CurrentOnline from the control plane.
func applyControlStatus(ctx context.Context, sc *ServerContext, data *statusEmbedData) {
	if sc == nil || sc.Control == nil {
		return
	}
	status, err := sc.Control.GetStatus(ctx, sc.Executor)
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
func applyDBStats(ctx context.Context, sc *ServerContext, data *statusEmbedData) {
	if sc == nil || sc.DB == nil {
		return
	}
	stats, err := cmdFetchServerStats(ctx, sc.DB)
	if err != nil {
		serverLog("discord", sc).Warn().Err(err).Msg("status: server stats query failed")
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
func applyUnique24h(ctx context.Context, sdb *sql.DB, serverID int, data *statusEmbedData) {
	uniq, err := countUniquePlayers24h(ctx, sdb, serverID, time.Now())
	if err != nil {
		componentLog("discord").Warn().Int("server_id", serverID).Err(err).Msg("status: unique 24h query failed")
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
