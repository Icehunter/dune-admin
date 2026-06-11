package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// ── Tiers ─────────────────────────────────────────────────────────────────────

// discordTier is an ordered capability level for Discord bot commands.
// Higher values are more privileged. Unknown/unmapped members have tier 0.
type discordTier int

const (
	tierViewer  discordTier = 1 // read-only commands: /status, /lookup
	tierEconomy discordTier = 2 // economy mutations: /give-currency
	tierAdmin   discordTier = 3 // server admin operations (future)
)

// commandTier returns the minimum tier required to invoke a slash command.
// Unknown commands default to tierAdmin as a safe fallback.
func commandTier(cmd string) discordTier {
	switch cmd {
	case "status", "lookup", "register", "unregister", "mystats", "mybalance", "myinventory":
		return tierViewer
	case "give-currency":
		return tierEconomy
	default:
		return tierAdmin
	}
}

// ── Types ─────────────────────────────────────────────────────────────────────

// discordConfig holds the Discord-relevant slice of appConfig, pre-parsed for
// use by the router and authz checks.
type discordConfig struct {
	GuildID           string
	RolesViewer       []string
	RolesEconomy      []string
	RolesAdmin        []string
	AnnounceChannelID string
}

// discordConfigFromApp builds a discordConfig from the global appConfig.
func discordConfigFromApp(cfg appConfig) discordConfig {
	return discordConfig{
		GuildID:           cfg.DiscordGuildID,
		RolesViewer:       splitRoleIDs(cfg.DiscordRolesViewer),
		RolesEconomy:      splitRoleIDs(cfg.DiscordRolesEconomy),
		RolesAdmin:        splitRoleIDs(cfg.DiscordRolesAdmin),
		AnnounceChannelID: cfg.DiscordAnnounceChannelID,
	}
}

// splitRoleIDs parses a comma-separated string of Discord role IDs into a
// slice, trimming whitespace and dropping empty entries.
func splitRoleIDs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// discordMember represents the invoker of a Discord slash command, extracted
// from discordgo types by the thin session adapter in discord.go.
type discordMember struct {
	UserID          string
	AvatarHash      string   // Discord avatar hash; empty when user has no custom avatar
	Roles           []string // list of role IDs the member holds
	IsAdministrator bool     // true when the member has the ADMINISTRATOR permission bit
}

// discordInteraction is the parsed representation of a Discord slash command
// interaction, extracted from discordgo types in the session adapter.
type discordInteraction struct {
	GuildID string
	Member  discordMember
	Command string
	// Options maps option name → value (string or int64 depending on the command).
	Options map[string]any
}

// discordReply is the response to send back to Discord.
type discordReply struct {
	Content   string
	Ephemeral bool // if true, only the invoker sees the reply
}

// discordDeps are the injected dependencies for discordDispatchCommand,
// enabling unit testing without a live Discord connection or DB.
type discordDeps struct {
	// admin commands
	status       func(ctx context.Context) (string, error)
	lookupPlayer func(ctx context.Context, name string) ([]playerInfo, error)
	giveCurrency func(ctx context.Context, controllerID, amount int64) (newBalance int64, err error)

	// registration
	registerLink func(ctx context.Context, discordUserID string, accountID int64, charName, avatarURL string) error
	deleteLink   func(ctx context.Context, discordUserID string) (bool, error)

	// self-service (require registration)
	getLink        func(ctx context.Context, discordUserID string) (accountID int64, charName string, err error)
	fetchCurrency  func(ctx context.Context, controllerID int64) ([]currencyRow, error)
	fetchInventory func(ctx context.Context, controllerID int64) ([]itemInfo, error)
}

// ── Authorization ─────────────────────────────────────────────────────────────

// memberTier resolves the highest discordTier granted by the member's roles.
// Returns 0 when no configured role matches.
func memberTier(member discordMember, cfg discordConfig) discordTier {
	roleSet := make(map[string]bool, len(member.Roles))
	for _, r := range member.Roles {
		roleSet[r] = true
	}

	best := discordTier(0)
	for _, r := range cfg.RolesAdmin {
		if roleSet[r] && tierAdmin > best {
			best = tierAdmin
		}
	}
	for _, r := range cfg.RolesEconomy {
		if roleSet[r] && tierEconomy > best {
			best = tierEconomy
		}
	}
	for _, r := range cfg.RolesViewer {
		if roleSet[r] && tierViewer > best {
			best = tierViewer
		}
	}
	return best
}

// authorizeDiscord returns true when the invoking member is permitted to run
// a command with the given required tier. The guild ID must match, and the
// member must hold a role at or above the required tier, or be the guild owner.
// This check is the real security boundary and runs on every invocation,
// independent of Discord's default_member_permissions (which only hides UI).
func authorizeDiscord(guildID string, member discordMember, required discordTier, cfg discordConfig) bool {
	if guildID != cfg.GuildID {
		return false
	}
	if member.IsAdministrator {
		return true
	}
	return memberTier(member, cfg) >= required
}

// ── Router ────────────────────────────────────────────────────────────────────

// dispatchDiscordCommand authorizes the invoker, then routes to the appropriate
// command handler. It always returns a sendable discordReply. Errors from
// command handlers are logged and surfaced as generic error messages so the
// bot always responds within Discord's 3-second interaction window.
func dispatchDiscordCommand(ctx context.Context, i discordInteraction, cfg discordConfig, deps discordDeps) discordReply {
	required := commandTier(i.Command)
	if !authorizeDiscord(i.GuildID, i.Member, required, cfg) {
		return discordReply{Content: "Not authorized.", Ephemeral: true}
	}

	switch i.Command {
	case "status":
		return handleDiscordStatus(ctx, deps)
	case "lookup":
		return handleDiscordLookup(ctx, i.Options, deps)
	case "give-currency":
		return handleDiscordGiveCurrency(ctx, i.Options, deps)
	case "register":
		return handleDiscordRegister(ctx, i.Member, i.Options, deps)
	case "unregister":
		return handleDiscordUnregister(ctx, i.Member.UserID, deps)
	case "mystats":
		return handleDiscordMyStats(ctx, i.Member.UserID, deps)
	case "mybalance":
		return handleDiscordMyBalance(ctx, i.Member.UserID, deps)
	case "myinventory":
		return handleDiscordMyInventory(ctx, i.Member.UserID, deps)
	default:
		return discordReply{
			Content:   fmt.Sprintf("Unknown command: %q", i.Command),
			Ephemeral: true,
		}
	}
}

// ── Command handlers ──────────────────────────────────────────────────────────

// handleDiscordStatus returns the current server population summary.
func handleDiscordStatus(ctx context.Context, deps discordDeps) discordReply {
	summary, err := deps.status(ctx)
	if err != nil {
		log.Printf("discord /status: %v", err)
		return discordReply{Content: "Error fetching server status.", Ephemeral: true}
	}
	return discordReply{Content: summary}
}

// handleDiscordLookup finds players by character name and returns their info.
func handleDiscordLookup(ctx context.Context, opts map[string]any, deps discordDeps) discordReply {
	name, ok := optString(opts, "name")
	if !ok || name == "" {
		return discordReply{Content: "Missing required option: name.", Ephemeral: true}
	}

	players, err := deps.lookupPlayer(ctx, name)
	if err != nil {
		log.Printf("discord /lookup %q: %v", name, err)
		return discordReply{Content: "Error looking up player.", Ephemeral: true}
	}
	if len(players) == 0 {
		return discordReply{Content: fmt.Sprintf("No player found matching %q.", name), Ephemeral: true}
	}

	if len(players) > 1 {
		return discordReply{
			Content: fmt.Sprintf("%d matches for %q — be more specific.", len(players), name),
		}
	}

	p := players[0]
	return discordReply{
		Content: formatPlayerLookup(p),
	}
}

// handleDiscordGiveCurrency grants Solaris to a player by character name.
// The name must resolve to exactly one player; ambiguous names are rejected.
func handleDiscordGiveCurrency(ctx context.Context, opts map[string]any, deps discordDeps) discordReply {
	name, ok := optString(opts, "name")
	if !ok || name == "" {
		return discordReply{Content: "Missing required option: name.", Ephemeral: true}
	}
	amount, ok := optInt64(opts, "amount")
	if !ok {
		return discordReply{Content: "Missing required option: amount.", Ephemeral: true}
	}

	players, err := deps.lookupPlayer(ctx, name)
	if err != nil {
		log.Printf("discord /give-currency lookup %q: %v", name, err)
		return discordReply{Content: "Error looking up player.", Ephemeral: true}
	}
	if len(players) == 0 {
		return discordReply{Content: fmt.Sprintf("No player found matching %q.", name), Ephemeral: true}
	}
	if len(players) > 1 {
		return discordReply{
			Content:   fmt.Sprintf("Ambiguous name %q — %d players match. Be more specific.", name, len(players)),
			Ephemeral: true,
		}
	}

	p := players[0]
	newBalance, err := deps.giveCurrency(ctx, p.ControllerID, amount)
	if err != nil {
		log.Printf("discord /give-currency player=%d amount=%d: %v", p.ControllerID, amount, err)
		return discordReply{Content: "Error granting currency.", Ephemeral: true}
	}
	return discordReply{
		Content: fmt.Sprintf("Granted %d Solaris to **%s** — new balance: %d", amount, p.Name, newBalance),
	}
}

// ── Option helpers ────────────────────────────────────────────────────────────

// optString extracts a string option value from the slash command options map.
func optString(opts map[string]any, key string) (string, bool) {
	if opts == nil {
		return "", false
	}
	v, ok := opts[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// optInt64 extracts an int64 option value from the slash command options map.
func optInt64(opts map[string]any, key string) (int64, bool) {
	if opts == nil {
		return 0, false
	}
	v, ok := opts[key]
	if !ok {
		return 0, false
	}
	n, ok := v.(int64)
	return n, ok
}

// ── Formatters ────────────────────────────────────────────────────────────────

// ── Registration ──────────────────────────────────────────────────────────────

func handleDiscordRegister(ctx context.Context, member discordMember, opts map[string]any, deps discordDeps) discordReply {
	name, ok := optString(opts, "name")
	if !ok || strings.TrimSpace(name) == "" {
		return discordReply{Content: "❌ Please provide a character name.", Ephemeral: true}
	}
	players, err := deps.lookupPlayer(ctx, name)
	if err != nil {
		return discordReply{Content: "❌ Lookup failed — try again.", Ephemeral: true}
	}
	if len(players) == 0 {
		return discordReply{Content: fmt.Sprintf("❌ No character found named **%s**.", name), Ephemeral: true}
	}
	if len(players) > 1 {
		return discordReply{Content: fmt.Sprintf("❌ Multiple characters match **%s** — be more specific.", name), Ephemeral: true}
	}
	p := players[0]
	avatarURL := discordAvatarURL(member.UserID, member.AvatarHash)
	if err := deps.registerLink(ctx, member.UserID, p.AccountID, p.Name, avatarURL); err != nil {
		return discordReply{Content: "❌ Registration failed — try again.", Ephemeral: true}
	}
	return discordReply{Content: fmt.Sprintf("✅ Registered as **%s**.", p.Name), Ephemeral: true}
}

// discordAvatarURL constructs the Discord CDN URL for a user's avatar.
// Falls back to a deterministic default avatar when the user has no custom avatar.
func discordAvatarURL(userID, avatarHash string) string {
	if avatarHash != "" {
		return fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.webp?size=128", userID, avatarHash)
	}
	return ""
}

func handleDiscordUnregister(ctx context.Context, userID string, deps discordDeps) discordReply {
	deleted, err := deps.deleteLink(ctx, userID)
	if err != nil {
		return discordReply{Content: "❌ Unregister failed — try again.", Ephemeral: true}
	}
	if !deleted {
		return discordReply{Content: "ℹ️ You're not registered.", Ephemeral: true}
	}
	return discordReply{Content: "✅ Unregistered.", Ephemeral: true}
}

// ── Self-service helpers ──────────────────────────────────────────────────────

// discordResolvePlayer looks up the registered character for the given Discord
// user ID and returns the playerInfo. Returns a ready error reply when the user
// is not registered or the character can't be found.
func discordResolvePlayer(ctx context.Context, userID string, deps discordDeps) (playerInfo, discordReply, bool) {
	_, charName, err := deps.getLink(ctx, userID)
	if err != nil {
		return playerInfo{}, discordReply{Content: "❌ Registration lookup failed.", Ephemeral: true}, false
	}
	if charName == "" {
		return playerInfo{}, discordReply{Content: "You're not registered. Use **/register** first.", Ephemeral: true}, false
	}
	players, err := deps.lookupPlayer(ctx, charName)
	if err != nil || len(players) == 0 {
		return playerInfo{}, discordReply{Content: fmt.Sprintf("❌ Could not find character **%s**.", charName), Ephemeral: true}, false
	}
	return players[0], discordReply{}, true
}

// ── Self-service commands ─────────────────────────────────────────────────────

func handleDiscordMyStats(ctx context.Context, userID string, deps discordDeps) discordReply {
	p, errReply, ok := discordResolvePlayer(ctx, userID, deps)
	if !ok {
		return errReply
	}
	online := p.OnlineStatus
	if online == "" {
		online = "Offline"
	}
	return discordReply{
		Content: fmt.Sprintf("🗡️ **%s**\nStatus: %s · Map: %s · Faction: %d",
			p.Name, online, p.Map, p.FactionID),
		Ephemeral: true,
	}
}

func handleDiscordMyBalance(ctx context.Context, userID string, deps discordDeps) discordReply {
	p, errReply, ok := discordResolvePlayer(ctx, userID, deps)
	if !ok {
		return errReply
	}
	rows, err := deps.fetchCurrency(ctx, p.ControllerID)
	if err != nil {
		return discordReply{Content: "❌ Could not fetch currency.", Ephemeral: true}
	}
	if len(rows) == 0 {
		return discordReply{Content: fmt.Sprintf("💰 **%s** — no currency data found.", p.Name), Ephemeral: true}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "💰 **%s** — Balances\n", p.Name)
	for _, r := range rows {
		fmt.Fprintf(&sb, "• %s: **%d**\n", currencyLabel(int64(r.CurrencyID)), r.Balance)
	}
	return discordReply{Content: strings.TrimRight(sb.String(), "\n"), Ephemeral: true}
}

func handleDiscordMyInventory(ctx context.Context, userID string, deps discordDeps) discordReply {
	p, errReply, ok := discordResolvePlayer(ctx, userID, deps)
	if !ok {
		return errReply
	}
	items, err := deps.fetchInventory(ctx, p.ID)
	if err != nil {
		return discordReply{Content: "❌ Could not fetch inventory.", Ephemeral: true}
	}
	if len(items) == 0 {
		return discordReply{Content: fmt.Sprintf("🎒 **%s** — inventory is empty.", p.Name), Ephemeral: true}
	}
	return discordReply{Content: formatInventoryMessage(p, items), Ephemeral: true}
}

func formatInventoryMessage(p playerInfo, items []itemInfo) string {
	type stack struct {
		name string
		qty  int64
	}
	seen := make(map[string]*stack)
	order := make([]string, 0, len(items))
	for _, it := range items {
		name := it.Name
		if name == "" {
			name = it.TemplateID
		}
		if s, exists := seen[it.TemplateID]; exists {
			s.qty += int64(it.StackSize)
		} else {
			seen[it.TemplateID] = &stack{name: name, qty: int64(it.StackSize)}
			order = append(order, it.TemplateID)
		}
	}

	const maxLines = 15
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎒 **%s** — Inventory (%d slots)\n", p.Name, len(items))
	for i, key := range order {
		if i >= maxLines {
			fmt.Fprintf(&sb, "_…and %d more unique items_", len(order)-i)
			break
		}
		s := seen[key]
		fmt.Fprintf(&sb, "• %s ×%d\n", s.name, s.qty)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// currencyLabel returns a human-readable name for a currency ID.
// Solaris is always ID 0 in Dune Awakening.
func currencyLabel(id int64) string {
	if id == 0 {
		return "Solaris"
	}
	return fmt.Sprintf("Currency #%d", id)
}

// formatPlayerLookup returns a human-readable summary of a player for Discord.
func formatPlayerLookup(p playerInfo) string {
	online := p.OnlineStatus
	if online == "" {
		online = "Unknown"
	}
	return fmt.Sprintf("**%s** — ID: %d · Account: %d · Status: %s · Map: %s",
		p.Name, p.ID, p.AccountID, online, p.Map)
}

// ── Role discovery ────────────────────────────────────────────────────────────

// errDiscordNotConnected is returned when the Discord bot session is not active.
var errDiscordNotConnected = errors.New("discord bot not connected")

// discordRoleRow is the JSON shape returned by the roles endpoint.
type discordRoleRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// guildRolesFetchFn matches discordgo.Session.GuildRoles for injection in tests.
type guildRolesFetchFn func(guildID string) ([]*discordgo.Role, error)

// roleFetcherFn is the dep injected into handleGetDiscordRolesInner.
type roleFetcherFn func(guildID string) ([]discordRoleRow, error)

// cmdListDiscordRoles fetches all guild roles and filters out @everyone.
func cmdListDiscordRoles(guildID string, fetchRoles guildRolesFetchFn) ([]discordRoleRow, error) {
	raw, err := fetchRoles(guildID)
	if err != nil {
		return nil, fmt.Errorf("fetch guild roles: %w", err)
	}
	out := make([]discordRoleRow, 0, len(raw))
	for _, r := range raw {
		if r.Name == "@everyone" {
			continue
		}
		out = append(out, discordRoleRow{ID: r.ID, Name: r.Name})
	}
	return out, nil
}

// handleGetDiscordRoles is the HTTP handler registered in server.go.
// Returns the guild's role list so the settings UI can show a role picker.
func handleGetDiscordRoles(w http.ResponseWriter, _ *http.Request) {
	handleGetDiscordRolesInner(w, globalDiscordGuildID, func(guildID string) ([]discordRoleRow, error) {
		if globalDiscordSession == nil {
			return nil, errDiscordNotConnected
		}
		return cmdListDiscordRoles(guildID, func(id string) ([]*discordgo.Role, error) {
			return globalDiscordSession.GuildRoles(id)
		})
	})
}

func handleGetDiscordRolesInner(w http.ResponseWriter, guildID string, fetch roleFetcherFn) {
	roles, err := fetch(guildID)
	if err != nil {
		if errors.Is(err, errDiscordNotConnected) {
			jsonErr(w, err, http.StatusServiceUnavailable)
			return
		}
		log.Printf("handleGetDiscordRoles: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, roles)
}
