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
	store        *welcomeStore
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
	for _, acc := range accounts {
		if strings.TrimSpace(acc.FlsID) == "" {
			continue
		}
		exists, existsErr := deps.store.grantExists(acc.FlsID, version, acc.AccountID)
		if existsErr != nil {
			return granted, failed, skipped, existsErr
		}
		if exists {
			skipped++
			continue
		}
		skippedItems, grantErr := deps.grant(ctx, acc.PawnID, acc.FlsID, items)
		if grantErr != nil {
			_ = deps.store.insertFailed(acc.FlsID, version, acc.AccountID, acc.CharacterName, grantErr.Error())
			failed++
			continue
		}
		if len(skippedItems) > 0 {
			_ = deps.store.insertFailed(acc.FlsID, version, acc.AccountID, acc.CharacterName,
				"items skipped: "+strings.Join(skippedItems, "; "))
			failed++
			continue
		}
		_ = deps.store.insertGranted(acc.FlsID, version, acc.AccountID, acc.CharacterName)
		granted++
	}
	return granted, failed, skipped, nil
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
	})
	reasons := make([]string, 0, len(skipped))
	for _, s := range skipped {
		reasons = append(reasons, s.Template+": "+s.Reason)
	}
	return reasons, nil
}

// ── live runtime config (updatable via the API without a restart) ───────────

type welcomePackageRuntime struct {
	enabled  bool
	version  string
	items    []welcomePackageItem
	interval time.Duration
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
func buildWelcomeRuntime(enabled bool, version string, scanSecs int, items []welcomePackageItem) welcomePackageRuntime {
	if strings.TrimSpace(version) == "" {
		version = "v1"
	}
	interval := time.Duration(scanSecs) * time.Second
	if interval < welcomeMinScanInterval {
		interval = welcomeDefaultScanInterval
	}
	return welcomePackageRuntime{enabled: enabled, version: version, items: items, interval: interval}
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
	if !rt.enabled || welcomeStoreDB == nil {
		return
	}
	if err := validateWelcomeItems(rt.items); err != nil {
		return // nothing valid to grant yet; stay quiet
	}
	g, f, _, err := welcomePackageScanOnce(ctx, rt.version, rt.items, welcomeScanDeps{
		listAccounts: listWelcomeOnlineAccounts,
		grant:        welcomeGrantViaGiveItems,
		store:        welcomeStoreDB,
	})
	if err != nil {
		log.Printf("welcome-package: scan error: %v", err)
		return
	}
	if g > 0 || f > 0 {
		log.Printf("welcome-package: granted=%d failed=%d version=%q", g, f, rt.version)
	}
}
