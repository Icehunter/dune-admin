package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var discordMu sync.RWMutex

// globalDiscordSession and globalDiscordGuildID are set by the bot goroutine
// and read by HTTP handlers; access must go through getDiscordState / setDiscordState.
var globalDiscordSession *discordgo.Session
var globalDiscordGuildID string

// discordCancelMu guards globalDiscordCancel; kept separate from discordMu to
// avoid holding the session RWLock during lifecycle transitions.
var discordCancelMu sync.Mutex
var globalDiscordCancel context.CancelFunc

func getDiscordState() (*discordgo.Session, string) {
	discordMu.RLock()
	defer discordMu.RUnlock()
	return globalDiscordSession, globalDiscordGuildID
}

func setDiscordState(s *discordgo.Session, guildID string) {
	discordMu.Lock()
	defer discordMu.Unlock()
	globalDiscordSession = s
	globalDiscordGuildID = guildID
}

// hasConfiguredGuild reports whether any Discord guild is configured — either the
// legacy single guild_id (app_config_discord table, seeded from config.yaml on
// first boot) or at least one guild saved via the "Manage server" UI (the
// discord_guilds / discord_servers tables). The gateway connection only needs the
// token, so starting without a legacy guild_id is safe as long as one exists
// somewhere in the DB.
func hasConfiguredGuild(cfg appConfig) bool {
	if cfg.DiscordGuildID != "" {
		return true
	}
	if len(listConfiguredGuilds()) > 0 {
		return true
	}
	if globalDiscordGuildsStore != nil {
		links, err := globalDiscordGuildsStore.listServerLinks()
		if err != nil {
			componentLog("discord").Warn().Err(err).Msg("list server links failed")
		} else if len(links) > 0 {
			return true
		}
	}
	return false
}

// discordBotEnabled returns the effective bot-enabled flag.
// Missing yaml key → default off (Discord is opt-in unlike the market bot).
func discordBotEnabled(cfg appConfig) bool {
	if cfg.DiscordBotEnabled == nil {
		return false
	}
	return *cfg.DiscordBotEnabled
}

// startEmbeddedDiscordBotIfEnabled starts the Discord bot in the background
// and returns a cancel func that cleanly closes the session on shutdown.
// Returns nil when the bot is disabled or token/guild are not configured.
// The gateway connection is established asynchronously so startup is not blocked.
//
// Mirrors startEmbeddedMarketBotIfEnabled (main.go) — outbound gateway WS,
// so no public ingress is needed; works behind NAT.
func startEmbeddedDiscordBotIfEnabled(cfg appConfig) context.CancelFunc {
	if !discordBotEnabled(cfg) {
		return nil
	}
	if cfg.DiscordBotToken == "" {
		componentLog("discord").Info().Msg("bot enabled but discord_bot_token is not set — skipping")
		return nil
	}
	if !hasConfiguredGuild(cfg) {
		componentLog("discord").Info().Msg("bot enabled but no Discord guild is configured (set one in Manage Server or via config) — skipping")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go discordConnect(ctx, cfg)
	return cancel
}

// stopDiscordBot cancels the running Discord bot goroutine (if any) and clears
// globalDiscordCancel. Safe to call when no bot is running.
func stopDiscordBot() {
	discordCancelMu.Lock()
	defer discordCancelMu.Unlock()
	if globalDiscordCancel != nil {
		globalDiscordCancel()
		globalDiscordCancel = nil
	}
}

// applyDiscordConfig stops any running Discord bot and starts a new one if the
// new config enables it. Called after handleSaveConfig writes the config to disk
// so that enable/disable takes effect without a process restart.
func applyDiscordConfig(cfg appConfig) {
	stopDiscordBot()
	// The status embed loops depend on a live bot session; (re)apply them whenever
	// the bot does. Each guild loop guards a nil session per tick, so starting
	// before the gateway is fully open is safe — it simply skips until the
	// session arrives. Per-guild config is read from discord_guilds.
	applyDiscordStatusLoops()
	if !discordBotEnabled(cfg) {
		return
	}
	cancel := startEmbeddedDiscordBotIfEnabled(cfg)
	if cancel == nil {
		return
	}
	discordCancelMu.Lock()
	globalDiscordCancel = cancel
	discordCancelMu.Unlock()
}

// discordConnect opens the Discord gateway, runs post-open setup, then blocks
// until ctx is cancelled. Runs in its own goroutine so startup is non-blocking.
func discordConnect(ctx context.Context, cfg appConfig) {
	dg, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		fmt.Printf("discord: failed to create session: %v\n", err)
		return
	}

	// Interactions (slash commands) are delivered over the gateway socket, so
	// we only need the minimum intent set — no message-content or other
	// privileged intents are required.
	dg.Identify.Intents = discordgo.IntentsGuilds

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type == discordgo.InteractionApplicationCommand {
			handleDiscordInteraction(s, i)
		} else if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
			handleDiscordAutocomplete(s, i)
		}
	})

	if err := dg.Open(); err != nil {
		fmt.Printf("discord: failed to open gateway connection: %v\n", err)
		return
	}

	if ctx.Err() != nil {
		_ = dg.Close()
		return
	}

	discordPostOpen(dg, cfg)
	discordShutdownWatcher(ctx, dg)
}

// discordPostOpen runs non-fatal setup after the Discord gateway is open:
// per-guild slash-command registration for every configured guild. Character
// links now live in the unified SQLite store, so there is no per-pool table to
// ensure.
func discordPostOpen(dg *discordgo.Session, cfg appConfig) {
	registerAllGuildCommands(dg)

	// globalDiscordGuildID keeps the seed/legacy guild for role-picker fallback.
	setDiscordState(dg, cfg.DiscordGuildID)
	componentLog("discord").Info().Str("guild_id", cfg.DiscordGuildID).Msg("bot connected")
	// No "bot connected" announce — it spams the channel on every (re)connect.
	// Live connection status is surfaced by the persistent status embed (#188).
}

// registerAllGuildCommands registers slash commands for every configured guild
// in discord_guilds. Guild-scoped registration is instant, so multi-guild
// fan-out is cheap.
func registerAllGuildCommands(dg *discordgo.Session) {
	for _, g := range listConfiguredGuilds() {
		if err := registerDiscordCommands(dg, g.GuildID); err != nil {
			componentLog("discord").Warn().Str("guild_id", g.GuildID).Err(err).Msg("command registration failed")
		}
	}
}

// listConfiguredGuilds returns all configured guilds (roles config), or nil if
// the store is unavailable.
func listConfiguredGuilds() []discordGuild {
	if globalDiscordGuildsStore == nil {
		return nil
	}
	guilds, err := globalDiscordGuildsStore.listGuilds()
	if err != nil {
		componentLog("discord").Warn().Err(err).Msg("list guilds failed")
		return nil
	}
	return guilds
}

// discordApplyMu serializes applyDiscordGuilds so overlapping CRUD writes don't
// race on the status-loop registry or issue interleaved command registrations.
var discordApplyMu sync.Mutex

// applyDiscordGuildsFn is the worker applyDiscordGuildsAsync runs. It's a package
// var so tests can substitute it; production always uses applyDiscordGuilds.
var applyDiscordGuildsFn = applyDiscordGuilds

// applyDiscordGuildsAsync runs applyDiscordGuilds off the request goroutine so an
// HTTP save returns immediately. Command registration is best-effort — failures
// are logged, never surfaced to the caller — so there is nothing to wait for.
// Applies are serialized by discordApplyMu to avoid racing the status-loop
// registry when writes overlap.
func applyDiscordGuildsAsync(removedGuildIDs ...string) {
	go func() {
		discordApplyMu.Lock()
		defer discordApplyMu.Unlock()
		applyDiscordGuildsFn(removedGuildIDs...)
	}()
}

// applyDiscordGuilds re-registers (or bulk-clears) slash commands on the live
// session to match the current discord_guilds rows, without a bot restart.
// Called after any discord_guilds CRUD write. Configured guilds get their
// commands registered; the removedGuildIDs (if any) are bulk-cleared.
func applyDiscordGuilds(removedGuildIDs ...string) {
	sess, _ := getDiscordState()
	if sess == nil {
		return // bot not connected — registration happens at next discordPostOpen
	}
	registerAllGuildCommands(sess)
	for _, gid := range removedGuildIDs {
		if gid == "" {
			continue
		}
		if err := clearGuildCommands(sess, gid); err != nil {
			componentLog("discord").Warn().Str("guild_id", gid).Err(err).Msg("clear guild commands failed")
		}
	}
	// Status loops follow the discord_servers links too.
	applyDiscordStatusLoops()
}

// clearGuildCommands removes all of the bot's slash commands from a guild
// (used when a guild mapping is deleted).
func clearGuildCommands(dg *discordgo.Session, guildID string) error {
	if _, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, guildID, nil); err != nil {
		return fmt.Errorf("clear commands for guild %q: %w", guildID, err)
	}
	return nil
}

// discordShutdownWatcher blocks until ctx is cancelled, then closes the session.
func discordShutdownWatcher(ctx context.Context, dg *discordgo.Session) {
	<-ctx.Done()
	if err := dg.Close(); err != nil {
		componentLog("discord").Warn().Err(err).Msg("session close error")
	}
	setDiscordState(nil, "")
	componentLog("discord").Info().Msg("bot disconnected")
}

// handleDiscordInteraction extracts the interaction into our internal types,
// resolves the target SERVER from the channel it was invoked in, loads that
// server's guild auth config, and dispatches through the testable router in
// handlers_discord.go. A channel that belongs to no server's announce/status
// channel gets a polite ephemeral error; a registered-but-unavailable server
// (no live pool) gets "server unavailable".
func handleDiscordInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Member == nil || i.Member.User == nil {
		return // DMs are not supported
	}

	serverID, guildID, ok := serverForChannel(i.ChannelID)
	if !ok {
		sendDiscordReply(s, i, discordReply{
			Content:   "Run this in one of a server's Discord channels (its announce or status channel).",
			Ephemeral: true,
		})
		return
	}

	guild, ok := resolveGuildContext(guildID)
	if !ok {
		sendDiscordReply(s, i, discordReply{
			Content:   "This server's Discord guild isn't configured. Ask an admin to set it up.",
			Ephemeral: true,
		})
		return
	}

	pool := poolForServer(serverID)
	if pool == nil {
		sendDiscordReply(s, i, discordReply{
			Content:   "That server is currently unavailable. Try again later.",
			Ephemeral: true,
		})
		return
	}

	interaction := buildDiscordInteraction(i)
	// The channel fixes the guild for auth; override GuildID so authz matches the
	// server's linked guild even if the raw interaction guild differs.
	interaction.GuildID = guildID
	cfg := discordConfigFromGuild(guild)
	deps := buildDiscordDeps(serverID)

	reply := dispatchDiscordCommand(context.Background(), interaction, cfg, deps)
	sendDiscordReply(s, i, reply)
}

func handleDiscordAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if data.Name != "market" {
		return
	}

	var focused string
	for _, opt := range data.Options {
		if opt.Focused {
			focused = strings.ToLower(opt.StringValue())
			break
		}
	}

	var choices []*discordgo.ApplicationCommandOptionChoice
	count := 0
	for tmpl, name := range itemData.Names {
		if focused == "" || strings.Contains(strings.ToLower(name), focused) || strings.Contains(strings.ToLower(tmpl), focused) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  name,
				Value: tmpl,
			})
			count++
			if count >= 25 { // Discord API limits choices to 25
				break
			}
		}
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
}

// serverForChannel resolves the server + guild that owns channelID (its announce
// or status channel), via the unified store. ok=false on no store, no match, or
// a lookup error (logged).
func serverForChannel(channelID string) (serverID int, guildID string, ok bool) {
	if globalDiscordGuildsStore == nil {
		return 0, "", false
	}
	sid, gid, found, err := globalDiscordGuildsStore.serverForChannel(channelID)
	if err != nil {
		componentLog("discord").Warn().Str("channel_id", channelID).Err(err).Msg("server for channel failed")
		return 0, "", false
	}
	return sid, gid, found
}

// poolForServer resolves a server id to its live pgx pool via the registry.
// Returns nil when the server isn't registered or its pool is nil.
func poolForServer(serverID int) *pgxpool.Pool {
	sc := globalRegistry.Get(serverScope(serverID))
	if sc == nil {
		return nil
	}
	return sc.DB
}

// buildDiscordDeps binds every command dependency to the ONE server resolved
// from the invoking channel. Each dep looks the server's pool up at call time
// via the registry, so a server going offline degrades gracefully.
func buildDiscordDeps(serverID int) discordDeps {
	return discordDeps{
		status: func(ctx context.Context) (string, error) {
			return statusSummaryForServer(ctx, serverID)
		},
		lookup: func(ctx context.Context, name string) ([]playerInfo, error) {
			pool := poolForServer(serverID)
			if pool == nil {
				return nil, fmt.Errorf("server %d not connected", serverID)
			}
			return cmdFindPlayersByName(ctx, pool, name)
		},
		giveCurrency: func(ctx context.Context, controllerID, amount int64) (int64, error) {
			pool := poolForServer(serverID)
			if pool == nil {
				return 0, fmt.Errorf("server %d not connected", serverID)
			}
			return cmdGiveCurrencyCtx(ctx, pool, controllerID, amount)
		},
		registerLink: func(_ context.Context, discordUserID string, accountID int64, charName, avatarURL string) error {
			if globalDiscordGuildsStore == nil {
				return fmt.Errorf("store not available")
			}
			return globalDiscordGuildsStore.upsertUserLink(discordUserID, serverID, accountID, charName, avatarURL)
		},
		deleteLink: func(_ context.Context, discordUserID string) (bool, error) {
			if globalDiscordGuildsStore == nil {
				return false, fmt.Errorf("store not available")
			}
			return globalDiscordGuildsStore.deleteUserLink(discordUserID, serverID)
		},
		getLink: func(_ context.Context, discordUserID string) (string, bool, error) {
			if globalDiscordGuildsStore == nil {
				return "", false, fmt.Errorf("store not available")
			}
			_, charName, ok, err := globalDiscordGuildsStore.getUserLink(discordUserID, serverID)
			return charName, ok, err
		},
		fetchCurrency: func(ctx context.Context, controllerID int64) ([]currencyRow, error) {
			pool := poolForServer(serverID)
			if pool == nil {
				return nil, fmt.Errorf("server %d not connected", serverID)
			}
			return cmdFetchPlayerCurrencyCtx(ctx, pool, controllerID)
		},
		fetchInventory: func(ctx context.Context, actorID int64) ([]itemInfo, error) {
			pool := poolForServer(serverID)
			if pool == nil {
				return nil, fmt.Errorf("server %d not connected", serverID)
			}
			return cmdFetchPlayerInventoryCtx(ctx, pool, actorID)
		},
		marketStats: func(ctx context.Context, templateID string) (int64, int64, error) {
			pool := poolForServer(serverID)
			if pool == nil {
				return 0, 0, fmt.Errorf("server %d not connected", serverID)
			}
			var t string
			var minPrice, avgPrice int64
			err := pool.QueryRow(ctx, "SELECT * FROM dune.dune_exchange_get_item_price_stats(ARRAY[$1::text])", templateID).Scan(&t, &minPrice, &avgPrice)
			if err == pgx.ErrNoRows {
				return 0, 0, nil
			}
			return minPrice, avgPrice, err
		},
	}
}

// buildDiscordInteraction parses a discordgo interaction into our internal type.
func buildDiscordInteraction(i *discordgo.InteractionCreate) discordInteraction {
	member := discordMember{
		UserID:          i.Member.User.ID,
		AvatarHash:      i.Member.User.Avatar,
		Roles:           i.Member.Roles,
		IsAdministrator: (i.Member.Permissions & discordgo.PermissionAdministrator) != 0,
	}
	data := i.ApplicationCommandData()
	opts := make(map[string]any, len(data.Options))
	for _, opt := range data.Options {
		switch opt.Type {
		case discordgo.ApplicationCommandOptionString:
			opts[opt.Name] = opt.StringValue()
		case discordgo.ApplicationCommandOptionInteger:
			opts[opt.Name] = opt.IntValue()
		}
	}
	return discordInteraction{
		GuildID:   i.GuildID,
		ChannelID: i.ChannelID,
		Member:    member,
		Command:   data.Name,
		Options:   opts,
	}
}

// sendDiscordReply responds to a Discord slash command interaction.
// Write commands should defer the interaction before calling this for slow ops
// (currently all commands are fast enough that we respond directly).
func sendDiscordReply(s *discordgo.Session, i *discordgo.InteractionCreate, reply discordReply) {
	flags := discordgo.MessageFlags(0)
	if reply.Ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: reply.Content,
			Flags:   flags,
		},
	})
	if err != nil {
		componentLog("discord").Warn().Err(err).Msg("failed to respond to interaction")
	}
}

// registerDiscordCommands registers the guild-scoped slash commands.
// Guild-scoped registration is instant (vs global which takes up to 1 hour).
func registerDiscordCommands(dg *discordgo.Session, guildID string) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "status",
			Description: "Show server status and online player count",
		},
		{
			Name:        "lookup",
			Description: "Look up a player by character name",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Character name to search for",
					Required:    true,
				},
			},
		},
		{
			// default_member_permissions: require Manage Server to hide from
			// non-privileged members in the Discord UI. The bot enforces its own
			// tier check server-side regardless.
			Name:                     "give-currency",
			Description:              "Grant Solaris to a player",
			DefaultMemberPermissions: int64Ptr(discordgo.PermissionManageGuild),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Character name of the recipient",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Amount of Solaris to grant",
					Required:    true,
					MinValue:    float64Ptr(1),
				},
			},
		},
		{
			Name:        "register",
			Description: "Link your Discord account to your in-game character",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Your in-game character name",
					Required:    true,
				},
			},
		},
		{
			Name:        "unregister",
			Description: "Unlink your Discord account from your in-game character",
		},
		{
			Name:        "mystats",
			Description: "Show your character's current stats",
		},
		{
			Name:        "mybalance",
			Description: "Show your current Solaris balance",
		},
		{
			Name:        "myinventory",
			Description: "Show your current inventory",
		},
		{
			Name:        "market",
			Description: "Look up market prices for an item",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "item",
					Description:  "The item name to search for",
					Required:     true,
					Autocomplete: true,
				},
			},
		},
	}

	// One bulk-overwrite call replaces the whole command set atomically, instead
	// of a separate (rate-limited) REST round-trip per command. This is both far
	// faster and idempotent — re-running it converges to the same set.
	if _, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, guildID, commands); err != nil {
		return fmt.Errorf("register commands for guild %q: %w", guildID, err)
	}
	return nil
}

// postDiscordAnnouncement sends a message to a channel using the active
// Discord session. Guards the nil session so callers don't need to.
// Used by the milestone/bridge spec to announce in-game rewards to Discord.
func postDiscordAnnouncement(channelID, message string) error {
	if channelID == "" {
		return nil
	}
	sess, _ := getDiscordState()
	if sess == nil {
		return fmt.Errorf("discord session not active")
	}
	_, err := sess.ChannelMessageSend(channelID, message)
	return err
}

// announceSend is the channel-message sender used by announceToServer; a var so
// tests can substitute a capturing fake without a live Discord session.
var announceSend = postDiscordAnnouncement

// announceToServer posts message to serverID's own announce channel (each server
// links to exactly one guild and has one announce channel). A server with no
// link or no announce channel is a no-op.
func announceToServer(serverID int, message string) error {
	// Prefer the server's own announce channel; fall back to the legacy global
	// announce channel so a deployment that set an announce channel without a
	// per-server guild link keeps announcing (instead of silently dropping it).
	channelID := loadedConfig.DiscordAnnounceChannelID
	if globalDiscordGuildsStore != nil {
		link, ok, err := globalDiscordGuildsStore.getServerLink(serverID)
		if err != nil {
			return fmt.Errorf("announce: get server link %d: %w", serverID, err)
		}
		if ok && link.AnnounceChannelID != "" {
			channelID = link.AnnounceChannelID
		}
	}
	if channelID == "" {
		return nil
	}
	if perr := announceSend(channelID, message); perr != nil {
		componentLog("discord").Warn().Int("server_id", serverID).Err(perr).Msg("announce to server failed")
	}
	return nil
}

// ── Dep wrappers ──────────────────────────────────────────────────────────────

// discordStatusQuery counts online/total player characters for one server pool.
const discordStatusQuery = `
	SELECT
		COUNT(*) FILTER (WHERE COALESCE(ps.online_status::text, 'Offline') <> 'Offline'),
		COUNT(*)
	FROM dune.actors a
	LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
	WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1`

// statusSummaryForServer builds the /status reply for one server (the channel's
// server). A down pool reports that the server is not connected.
func statusSummaryForServer(ctx context.Context, serverID int) (string, error) {
	pool := poolForServer(serverID)
	if pool == nil {
		return "This server is not currently connected.", nil
	}
	var online, total int64
	if err := pool.QueryRow(ctx, discordStatusQuery, gmIdentityAccountID).Scan(&online, &total); err != nil {
		return "", fmt.Errorf("status query for server %d: %w", serverID, err)
	}
	return fmt.Sprintf("🌐 Server #%d · **%d / %d** players active", serverID, online, total), nil
}

// ── Pointer helpers ───────────────────────────────────────────────────────────

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
