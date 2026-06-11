package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var discordMu sync.RWMutex

// globalDiscordSession and globalDiscordGuildID are set by the bot goroutine
// and read by HTTP handlers; access must go through getDiscordState / setDiscordState.
var globalDiscordSession *discordgo.Session
var globalDiscordGuildID string

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
		log.Println("discord: bot enabled but discord_bot_token is not set — skipping")
		return nil
	}
	if cfg.DiscordGuildID == "" {
		log.Println("discord: bot enabled but discord_guild_id is not set — skipping")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go discordConnect(ctx, cfg)
	return cancel
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

	dcfg := discordConfigFromApp(cfg)

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		handleDiscordInteraction(s, i, dcfg)
	})

	if err := dg.Open(); err != nil {
		fmt.Printf("discord: failed to open gateway connection: %v\n", err)
		return
	}

	if ctx.Err() != nil {
		_ = dg.Close()
		return
	}

	discordPostOpen(dg, cfg, dcfg)
	discordShutdownWatcher(ctx, dg)
}

// discordPostOpen runs non-fatal setup after the Discord gateway is open:
// command registration, discord_links table creation, and startup announcement.
func discordPostOpen(dg *discordgo.Session, cfg appConfig, dcfg discordConfig) {
	if err := registerDiscordCommands(dg, cfg.DiscordGuildID); err != nil {
		log.Printf("discord: command registration failed: %v", err)
	}

	if globalDB != nil {
		if err := cmdEnsureDiscordLinksTable(context.Background(), globalDB); err != nil {
			log.Printf("discord: failed to ensure discord_links table: %v", err)
		}
	}

	setDiscordState(dg, cfg.DiscordGuildID)
	log.Printf("discord: bot connected (guild %s)", cfg.DiscordGuildID)

	if dcfg.AnnounceChannelID != "" {
		if err := postDiscordAnnouncement(dcfg.AnnounceChannelID, "✅ dune-admin bot connected."); err != nil {
			log.Printf("discord: announce seam test failed: %v", err)
		}
	}
}

// discordShutdownWatcher blocks until ctx is cancelled, then closes the session.
func discordShutdownWatcher(ctx context.Context, dg *discordgo.Session) {
	<-ctx.Done()
	if err := dg.Close(); err != nil {
		log.Printf("discord: session close error: %v", err)
	}
	setDiscordState(nil, "")
	log.Println("discord: bot disconnected")
}

// handleDiscordInteraction extracts the interaction into our internal types
// and dispatches through the testable router in handlers_discord.go.
func handleDiscordInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, cfg discordConfig) {
	if i.Member == nil || i.Member.User == nil {
		return // DMs are not supported
	}

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

	interaction := discordInteraction{
		GuildID: i.GuildID,
		Member:  member,
		Command: data.Name,
		Options: opts,
	}

	deps := discordDeps{
		status: discordStatusDep,
		lookupPlayer: func(ctx context.Context, name string) ([]playerInfo, error) {
			return cmdFindPlayersByName(ctx, globalDB, name)
		},
		giveCurrency: func(ctx context.Context, controllerID, amount int64) (int64, error) {
			return cmdGiveCurrencyCtx(ctx, globalDB, controllerID, amount)
		},
		registerLink: func(ctx context.Context, discordUserID string, accountID int64, charName, avatarURL string) error {
			return cmdRegisterDiscordLink(ctx, globalDB, discordUserID, accountID, charName, avatarURL)
		},
		deleteLink: func(ctx context.Context, discordUserID string) (bool, error) {
			return cmdDeleteDiscordLink(ctx, globalDB, discordUserID)
		},
		getLink: func(ctx context.Context, discordUserID string) (int64, string, error) {
			return cmdGetDiscordLink(ctx, globalDB, discordUserID)
		},
		fetchCurrency: func(ctx context.Context, controllerID int64) ([]currencyRow, error) {
			return cmdFetchPlayerCurrencyCtx(ctx, globalDB, controllerID)
		},
		fetchInventory: func(ctx context.Context, actorID int64) ([]itemInfo, error) {
			return cmdFetchPlayerInventoryCtx(ctx, globalDB, actorID)
		},
	}

	reply := dispatchDiscordCommand(context.Background(), interaction, cfg, deps)
	sendDiscordReply(s, i, reply)
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
		log.Printf("discord: failed to respond to interaction: %v", err)
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
	}

	for _, cmd := range commands {
		if _, err := dg.ApplicationCommandCreate(dg.State.User.ID, guildID, cmd); err != nil {
			return fmt.Errorf("register command %q: %w", cmd.Name, err)
		}
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

// ── Dep wrappers ──────────────────────────────────────────────────────────────

// discordStatusDep is the live status dep: fetches online/total counts.
func discordStatusDep(ctx context.Context) (string, error) {
	if globalDB == nil {
		return "", fmt.Errorf("database not connected")
	}
	var online, total int64
	err := globalDB.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE COALESCE(ps.online_status::text, 'Offline') <> 'Offline'),
			COUNT(*)
		FROM dune.actors a
		LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1`,
		gmIdentityAccountID).Scan(&online, &total)
	if err != nil {
		return "", fmt.Errorf("status query: %w", err)
	}
	return fmt.Sprintf("🌐 Server online · **%d / %d** players active", online, total), nil
}

// ── Pointer helpers ───────────────────────────────────────────────────────────

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
