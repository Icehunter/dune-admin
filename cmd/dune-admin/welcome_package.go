package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
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
	online, resolvedFls := resolveGiveItemsOnlinePath(ctx, pawnID, checkPlayerOffline, flsIDFromActorID)
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
			msg, ok := cmdGiveItem(playerID, template, qty, quality)().(msgMutate)
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
func buildWelcomeRuntime(enabled bool, activeVersions []string, scanSecs int, packages []welcomePackage, msg welcomeMessageOptions, motd ...motdOptions) welcomePackageRuntime {
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
	if len(motd) > 0 {
		rt.motdEnabled = motd[0].enabled
		rt.motdMessage = motd[0].message
		rt.motdSourcePlayer = motd[0].sourcePlayer
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
		welcomePackageScanTick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func welcomePackageScanTick(ctx context.Context) {
	rt := getWelcomeRuntime()
	motdActive := rt.motdEnabled && strings.TrimSpace(rt.motdMessage) != ""
	pkgActive := rt.enabled && welcomeStoreDB != nil && len(rt.activePackages()) > 0

	// Keep the join-detection baseline fresh only while MOTD is active. When it
	// is off, reset so re-enabling starts from a clean baseline (no MOTD to
	// players who were already online when the operator flipped it on).
	if !motdActive {
		welcomePresence.reset()
	}
	if !motdActive && !pkgActive {
		return
	}

	// MOTD and package grants both consume the current online set — fetch once.
	online, err := listWelcomeOnlineAccounts(ctx)
	if err != nil {
		log.Printf("welcome: list online accounts: %v", err)
		return
	}
	if pkgActive {
		runWelcomePackageGrants(ctx, rt, online)
	}
	if motdActive {
		runMOTDOnJoin(ctx, rt, online)
	}
}

// runWelcomePackageGrants grants the active package(s) to eligible accounts in
// the given online snapshot, sending the package's companion welcome message
// (once per version) when configured.
func runWelcomePackageGrants(ctx context.Context, rt welcomePackageRuntime, online []welcomeAccount) {
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
			grant:        welcomeGrantViaGiveItems,
			whisper:      whisperFn,
			store:        welcomeStoreDB,
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
