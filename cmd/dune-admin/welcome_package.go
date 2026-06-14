package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// welcomePackageItem is one configured grant in the welcome package. It maps
// directly onto the existing give-items path: quality 0 → live RMQ grant for
// online players, quality > 0 → DB-write fallback.
type welcomePackageItem struct {
	Template string `yaml:"template" json:"template"`
	Qty      int64  `yaml:"qty"      json:"qty"`
	Quality  int64  `yaml:"quality"  json:"quality"`
}

func validateWelcomeItems(items []welcomePackageItem) error {
	if len(items) == 0 {
		return fmt.Errorf("welcome package has no items")
	}
	for _, it := range items {
		if strings.TrimSpace(it.Template) == "" {
			return fmt.Errorf("welcome item template must not be empty")
		}
		if it.Qty <= 0 {
			return fmt.Errorf("welcome item %q quantity must be greater than 0", it.Template)
		}
		if it.Quality < 0 {
			return fmt.Errorf("welcome item %q quality must be >= 0", it.Template)
		}
	}
	return nil
}

// welcomeAccount is one eligible player the scanner may grant to.
type welcomeAccount struct {
	AccountID     int64
	PawnID        int64 // actor id consumed by the give-items path
	FlsID         string
	CharacterName string
	// Region is the player's current map/zone (dune.actors.map). Used by the
	// region join/leave broadcast (#167) to target everyone in the same region.
	// Empty when unknown.
	Region string
}

// welcomeScanDeps are injected so the scan loop is unit-testable without a DB.
type welcomeScanDeps struct {
	listAccounts func(context.Context) ([]welcomeAccount, error)
	grant        func(ctx context.Context, pawnID int64, flsID string, items []welcomePackageItem) ([]string, error)
	// whisper is called at most once per (flsID, version) to send a welcome
	// message. nil disables the whisper feature.
	whisper func(ctx context.Context, accountID int64, flsID string, message string) error
	store   *welcomeStore
}

// welcomePackageScanOnce grants the package to each eligible account exactly
// once and returns (granted, failed, skipped) counts. An account is skipped if
// it already has a ledger row for this version, granted on a clean grant, and
// failed if the grant errors or any item is skipped (recorded so the operator
// can retry). Accounts without an FLS id are ignored entirely (no ledger row),
// so a later scan retries once the identity resolves.
func welcomePackageScanOnce(ctx context.Context, version string, items []welcomePackageItem, deps welcomeScanDeps) (granted, failed, skipped int, err error) {
	accounts, err := deps.listAccounts(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("list welcome accounts: %w", err)
	}
	msgVersion := version + ":msg"
	for _, acc := range accounts {
		if strings.TrimSpace(acc.FlsID) == "" {
			continue
		}
		g, f, s, gErr := grantItemsToAccount(ctx, acc, version, items, deps)
		if gErr != nil {
			return granted, failed, skipped, gErr
		}
		granted += g
		failed += f
		skipped += s
		if wErr := whisperAccount(ctx, acc, msgVersion, deps); wErr != nil {
			return granted, failed, skipped, wErr
		}
	}
	return granted, failed, skipped, nil
}

func grantItemsToAccount(ctx context.Context, acc welcomeAccount, version string, items []welcomePackageItem, deps welcomeScanDeps) (granted, failed, skipped int, err error) {
	if len(items) == 0 {
		return 0, 0, 0, nil
	}
	exists, existsErr := deps.store.grantExists(acc.FlsID, version, acc.AccountID)
	if existsErr != nil {
		return 0, 0, 0, existsErr
	}
	if exists {
		return 0, 0, 1, nil
	}
	skippedItems, grantErr := deps.grant(ctx, acc.PawnID, acc.FlsID, items)
	if grantErr != nil {
		_ = deps.store.insertFailed(acc.FlsID, version, acc.AccountID, acc.CharacterName, grantErr.Error())
		return 0, 1, 0, nil
	}
	if len(skippedItems) > 0 {
		_ = deps.store.insertFailed(acc.FlsID, version, acc.AccountID, acc.CharacterName,
			"items skipped: "+strings.Join(skippedItems, "; "))
		return 0, 1, 0, nil
	}
	_ = deps.store.insertGranted(acc.FlsID, version, acc.AccountID, acc.CharacterName)
	return 1, 0, 0, nil
}

// overrideGrantToAccount grants a package to one chosen account, bypassing the
// already-granted guard used by grantItemsToAccount. It is the manual "force a
// grant" path: it calls deps.grant directly and records the resulting ledger
// row (granted on success, failed on error or skipped items). Unlike the
// scanner, failures surface to the caller as an error so the operator sees them.
func overrideGrantToAccount(ctx context.Context, acc welcomeAccount, version string, items []welcomePackageItem, deps welcomeScanDeps) error {
	if strings.TrimSpace(acc.FlsID) == "" {
		return fmt.Errorf("player has no FLS id (cannot record grant)")
	}
	if len(items) == 0 {
		return fmt.Errorf("package %q has no items", version)
	}
	skippedItems, grantErr := deps.grant(ctx, acc.PawnID, acc.FlsID, items)
	if grantErr != nil {
		_ = deps.store.insertFailed(acc.FlsID, version, acc.AccountID, acc.CharacterName, grantErr.Error())
		return fmt.Errorf("grant items: %w", grantErr)
	}
	if len(skippedItems) > 0 {
		reason := "items skipped: " + strings.Join(skippedItems, "; ")
		_ = deps.store.insertFailed(acc.FlsID, version, acc.AccountID, acc.CharacterName, reason)
		return fmt.Errorf("%s", reason)
	}
	_ = deps.store.insertGranted(acc.FlsID, version, acc.AccountID, acc.CharacterName)
	return nil
}

func whisperAccount(ctx context.Context, acc welcomeAccount, msgVersion string, deps welcomeScanDeps) error {
	if deps.whisper == nil {
		return nil
	}
	msgExists, msgErr := deps.store.grantExists(acc.FlsID, msgVersion, acc.AccountID)
	if msgErr != nil {
		return msgErr
	}
	if msgExists {
		return nil
	}
	if wErr := deps.whisper(ctx, acc.AccountID, acc.FlsID, msgVersion); wErr != nil {
		_ = deps.store.insertFailed(acc.FlsID, msgVersion, acc.AccountID, acc.CharacterName, wErr.Error())
	} else {
		_ = deps.store.insertGranted(acc.FlsID, msgVersion, acc.AccountID, acc.CharacterName)
	}
	return nil
}

// welcomeGrantViaGiveItems is the production grant function: it reuses the exact
// shipped give-items path (live RMQ for online players, DB-write fallback
// otherwise) and returns "template: reason" strings for any skipped items.
func welcomeGrantViaGiveItems(ctx context.Context, pawnID int64, flsID string, items []welcomePackageItem) ([]string, error) {
	online, resolvedFls := resolveGiveItemsOnlinePath(ctx, pawnID, func(ctx context.Context, id int64) error { return checkPlayerOfflinePool(ctx, globalDB, id) }, flsIDFromActorID)
	if resolvedFls == "" {
		resolvedFls = flsID
	}
	req := giveItemsRequest{PlayerID: pawnID, Items: make([]giveItemInput, 0, len(items))}
	for _, it := range items {
		req.Items = append(req.Items, giveItemInput(it))
	}
	_, skipped := processGiveItems(ctx, req, online, resolvedFls, giveItemsDeps{
		checkCapacity: checkInventoryCapacity,
		rmqAdd:        rmqAddItemToInventory,
		dbGive: func(playerID int64, template string, qty, quality int64) (msgMutate, bool) {
			msg, ok := cmdGiveItem(globalDB, playerID, template, qty, quality)().(msgMutate)
			return msg, ok
		},
		needsDBPath: itemNeedsDBPath,
	})
	reasons := make([]string, 0, len(skipped))
	for _, s := range skipped {
		reasons = append(reasons, s.Template+": "+s.Reason)
	}
	return reasons, nil
}

// ── live runtime config (updatable via the API without a restart) ───────────

// welcomePackage is one named, versioned item set in the library. The operator
// can keep several and pick which one is active (granted).
type welcomePackage struct {
	Version string               `yaml:"version" json:"version"`
	Items   []welcomePackageItem `yaml:"items"   json:"items"`
}

type welcomePackageRuntime struct {
	enabled                    bool
	interval                   time.Duration
	activeVersions             []string
	packages                   []welcomePackage
	welcomeMessageEnabled      bool
	welcomeMessage             string
	welcomeWhisperSourcePlayer string
	// MOTD (#163/#167/#135): a per-join message, independent of the package
	// system — fires every time a player joins, even when no package is active.
	motdEnabled      bool
	motdMessage      string
	motdSourcePlayer string
	// Region join/leave broadcast (#167): announces when a player joins/leaves a
	// region. channelType "whisper" sends per-player whispers (original path);
	// "map" publishes once to chat.map/{region}.0 (live-confirmed 2026-05-31).
	regionJoinEnabled   bool
	regionLeaveEnabled  bool
	regionJoinTemplate  string
	regionLeaveTemplate string
	regionChatChannel   string // "whisper" (default) | "map"
}

// welcomeMessageOptions carries the optional whisper config passed to
// buildWelcomeRuntime. Keeping it in a struct avoids a long parameter list.
type welcomeMessageOptions struct {
	enabled      bool
	message      string
	sourcePlayer string
}

// motdOptions carries the optional Message-of-the-Day config passed to
// buildWelcomeRuntime as a trailing variadic so existing callers are unaffected.
type motdOptions struct {
	enabled      bool
	message      string
	sourcePlayer string
}

// regionBroadcastOptions carries the region join/leave broadcast config.
type regionBroadcastOptions struct {
	joinEnabled   bool
	leaveEnabled  bool
	joinTemplate  string
	leaveTemplate string
	chatChannel   string // "whisper" (default) | "map"
}

// welcomeExtraOptions bundles the optional MOTD and region-broadcast config
// passed to buildWelcomeRuntime as a trailing variadic so existing callers
// (which pass nothing) are unaffected.
type welcomeExtraOptions struct {
	motd   motdOptions
	region regionBroadcastOptions
}

// activePackages returns all packages whose version is in activeVersions.
func (rt welcomePackageRuntime) activePackages() []welcomePackage {
	out := make([]welcomePackage, 0, len(rt.activeVersions))
	for _, v := range rt.activeVersions {
		if i := findPackage(rt.packages, v); i >= 0 {
			out = append(out, rt.packages[i])
		}
	}
	return out
}

// active returns the first active package (backwards-compat helper).
func (rt welcomePackageRuntime) active() (welcomePackage, bool) {
	pkgs := rt.activePackages()
	if len(pkgs) == 0 {
		return welcomePackage{}, false
	}
	return pkgs[0], true
}

func findPackage(packages []welcomePackage, version string) int {
	for i, p := range packages {
		if p.Version == version {
			return i
		}
	}
	return -1
}

var (
	welcomeMu      sync.RWMutex
	welcomeRuntime welcomePackageRuntime
	welcomeStoreDB *welcomeStore
)

func setWelcomeRuntime(rt welcomePackageRuntime) {
	welcomeMu.Lock()
	defer welcomeMu.Unlock()
	welcomeRuntime = rt
}

func getWelcomeRuntime() welcomePackageRuntime {
	welcomeMu.RLock()
	defer welcomeMu.RUnlock()
	return welcomeRuntime
}

// buildWelcomeRuntime normalizes raw config (version default, interval clamp)
// into a runtime value. Shared by startup and the config API so both apply the
// same defaults.
func buildWelcomeRuntime(enabled bool, activeVersions []string, scanSecs int, packages []welcomePackage, msg welcomeMessageOptions, opts ...welcomeExtraOptions) welcomePackageRuntime {
	if packages == nil {
		packages = []welcomePackage{}
	}
	// Filter activeVersions to only those that exist in the package library.
	valid := activeVersions[:0:0]
	for _, v := range activeVersions {
		if findPackage(packages, v) >= 0 {
			valid = append(valid, v)
		}
	}
	// Default to first package when nothing valid is selected.
	if len(valid) == 0 && len(packages) > 0 {
		valid = []string{packages[0].Version}
	}
	interval := time.Duration(scanSecs) * time.Second
	if interval < welcomeMinScanInterval {
		interval = welcomeDefaultScanInterval
	}
	rt := welcomePackageRuntime{
		enabled:                    enabled,
		interval:                   interval,
		activeVersions:             valid,
		packages:                   packages,
		welcomeMessageEnabled:      msg.enabled,
		welcomeMessage:             msg.message,
		welcomeWhisperSourcePlayer: msg.sourcePlayer,
	}
	if len(opts) > 0 {
		rt.motdEnabled = opts[0].motd.enabled
		rt.motdMessage = opts[0].motd.message
		rt.motdSourcePlayer = opts[0].motd.sourcePlayer
		rt.regionJoinEnabled = opts[0].region.joinEnabled
		rt.regionLeaveEnabled = opts[0].region.leaveEnabled
		rt.regionJoinTemplate = opts[0].region.joinTemplate
		rt.regionLeaveTemplate = opts[0].region.leaveTemplate
		rt.regionChatChannel = opts[0].region.chatChannel
	}
	return rt
}

const welcomeMinScanInterval = 5 * time.Second
const welcomeDefaultScanInterval = 30 * time.Second

// runWelcomePackageScanner loops until ctx is cancelled, scanning on each tick.
// enabled/version/items are read live so API changes apply without a restart
// (the scan interval is fixed at start). The scanner is always running; when the
// feature is disabled each tick is a cheap no-op.
func runWelcomePackageScanner(ctx context.Context) {
	interval := getWelcomeRuntime().interval
	if interval < welcomeMinScanInterval {
		interval = welcomeDefaultScanInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		sc := globalRegistry.Active()
		welcomePackageScanTick(ctx, sc, resolveWelcomeStore(sc))
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// resolveWelcomeStore returns the welcome store scoped to sc's server ID. Falls
// back to the legacy welcomeStoreDB when globalStore is unavailable.
func resolveWelcomeStore(sc *ServerContext) *welcomeStore {
	if sc != nil && globalStore != nil {
		return newWelcomeStore(globalStore, sc.StoreScope)
	}
	return welcomeStoreDB
}

func welcomePackageScanTick(ctx context.Context, sc *ServerContext, store *welcomeStore) {
	rt := getWelcomeRuntime()
	motdActive := rt.motdEnabled && strings.TrimSpace(rt.motdMessage) != ""
	regionActive := regionBroadcastActive(rt)
	presenceActive := motdActive || regionActive
	pkgActive := rt.enabled && store != nil && len(rt.activePackages()) > 0

	// Keep the join/leave-detection baseline fresh only while a presence-driven
	// feature (MOTD or region broadcast) is active. When both are off, reset so
	// re-enabling starts from a clean baseline (no message to players who were
	// already online when the operator flipped it on).
	if !presenceActive {
		welcomePresence.reset()
	}
	if !presenceActive && !pkgActive {
		return
	}

	// MOTD, region broadcast, and package grants all consume the current online
	// set — fetch once using the server's Postgres pool.
	var pool *pgxpool.Pool
	if sc != nil {
		pool = sc.DB
	}
	online, err := cmdListWelcomeOnlineAccounts(ctx, pool)
	if err != nil {
		log.Printf("welcome: list online accounts: %v", err)
		return
	}
	if pkgActive {
		runWelcomePackageGrants(ctx, rt, online, store, welcomeGrantViaGiveItems)
	}
	if presenceActive {
		runPresenceWhispers(ctx, rt, online, motdActive, regionActive)
	}
}

// regionBroadcastActive reports whether at least one half (join or leave) of the
// region broadcast is enabled with a non-blank template.
func regionBroadcastActive(rt welcomePackageRuntime) bool {
	joinOn := rt.regionJoinEnabled && strings.TrimSpace(rt.regionJoinTemplate) != ""
	leaveOn := rt.regionLeaveEnabled && strings.TrimSpace(rt.regionLeaveTemplate) != ""
	return joinOn || leaveOn
}

// runPresenceWhispers diffs the online set once and dispatches both the MOTD
// (private whisper to the joining player) and the region broadcast (whisper to
// everyone in the joined/left region). Both consume the same join/leave events so
// the tracker is observed exactly once per tick.
func runPresenceWhispers(ctx context.Context, rt welcomePackageRuntime, online []welcomeAccount, motdActive, regionActive bool) {
	joins, leaves := welcomePresence.observeJoinsLeaves(online)
	if motdActive {
		for _, m := range motdWhispersForJoins(joins, rt.motdEnabled, rt.motdMessage, rt.motdSourcePlayer) {
			if err := sendWelcomeWhisper(ctx, m.accountID, m.sourcePlayer, m.message); err != nil {
				log.Printf("motd: whisper to account %d failed: %v", m.accountID, err)
			}
		}
	}
	if regionActive {
		cfg := regionBroadcastConfig{
			joinEnabled:   rt.regionJoinEnabled,
			leaveEnabled:  rt.regionLeaveEnabled,
			joinTemplate:  rt.regionJoinTemplate,
			leaveTemplate: rt.regionLeaveTemplate,
		}
		if rt.regionChatChannel == "map" {
			runMapChatBroadcastOnJoinLeave(ctx, joins, leaves, cfg, sendWelcomeMapChat)
		} else {
			runRegionBroadcastOnJoinLeave(ctx, joins, leaves, online, cfg, sendWelcomeWhisper)
		}
	}
}

// runWelcomePackageGrants grants the active package(s) to eligible accounts in
// the given online snapshot, sending the package's companion welcome message
// (once per version) when configured. store and grantFn are explicit so the
// caller can scope grants to the right server and tests can stub the grant path.
func runWelcomePackageGrants(ctx context.Context, rt welcomePackageRuntime, online []welcomeAccount, store *welcomeStore, grantFn func(context.Context, int64, string, []welcomePackageItem) ([]string, error)) {
	var whisperFn func(context.Context, int64, string, string) error
	if rt.welcomeMessageEnabled && strings.TrimSpace(rt.welcomeMessage) != "" {
		msg := rt.welcomeMessage
		srcPlayer := rt.welcomeWhisperSourcePlayer
		whisperFn = func(wctx context.Context, accountID int64, _ string, _ string) error {
			return sendWelcomeWhisper(wctx, accountID, srcPlayer, msg)
		}
	}
	listOnline := func(context.Context) ([]welcomeAccount, error) { return online, nil }
	for _, pkg := range rt.activePackages() {
		if err := validateWelcomeItems(pkg.Items); err != nil {
			continue
		}
		g, f, _, err := welcomePackageScanOnce(ctx, pkg.Version, pkg.Items, welcomeScanDeps{
			listAccounts: listOnline,
			grant:        grantFn,
			whisper:      whisperFn,
			store:        store,
		})
		if err != nil {
			log.Printf("welcome-package: scan error (version=%q): %v", pkg.Version, err)
			continue
		}
		if g > 0 || f > 0 {
			log.Printf("welcome-package: granted=%d failed=%d version=%q", g, f, pkg.Version)
		}
	}
}

// sendWelcomeMapChat publishes one map-chat message to the given region's channel.
// Uses the seeded GM persona as the sender (the same identity used for whispers).
func sendWelcomeMapChat(ctx context.Context, region, _ string, message string) error {
	gm, err := cmdGetGMIdentity(ctx)
	if err != nil {
		return fmt.Errorf("region map chat: gm identity: %w", err)
	}
	return rmqSendMapChat(region, 0, gm.FuncomID, gm.HexID, message)
}

// sendWelcomeWhisper sends a welcome whisper to a player via the existing GM
// persona whisper path. sourcePlayerFlsID is the sender identity; leave blank
// to use the seeded GM persona. Called from the scanner on each new account.
func sendWelcomeWhisper(ctx context.Context, accountID int64, sourcePlayerFlsID, message string) error {
	return processWhisper(ctx, accountID, message, whisperDeps{
		getGM: func(c context.Context) (gmIdentity, error) {
			if sourcePlayerFlsID != "" {
				// Resolve a specific source player's identity for the sender.
				funcomID, charName, err := cmdResolveRecipientChatIdentity(c, 0)
				_ = charName
				if err == nil {
					return gmIdentity{FuncomID: funcomID, HexID: sourcePlayerFlsID}, nil
				}
			}
			return cmdGetGMIdentity(c)
		},
		resolveRecip: cmdResolveRecipientChatIdentity,
		send:         rmqSendWhisper,
	})
}
