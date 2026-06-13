package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// welcomeConfigResponse is the shape returned by the config endpoints and
// consumed by the WelcomePackage tab. It carries the whole package library plus
// the active-version pointer(s).
// ActiveVersion is kept for backwards compatibility (first element of ActiveVersions).
// New clients should use ActiveVersions.
type welcomeConfigResponse struct {
	Enabled                    bool             `json:"enabled"`
	ScanIntervalSecs           int              `json:"scan_interval_secs"`
	ActiveVersion              string           `json:"active_version"`
	ActiveVersions             []string         `json:"active_versions"`
	Packages                   []welcomePackage `json:"packages"`
	WelcomeMessageEnabled      bool             `json:"welcome_message_enabled"`
	WelcomeMessage             string           `json:"welcome_message"`
	WelcomeWhisperSourcePlayer string           `json:"welcome_whisper_source_player"`
	MotdEnabled                bool             `json:"motd_enabled"`
	MotdMessage                string           `json:"motd_message"`
	MotdSourcePlayer           string           `json:"motd_source_player"`
}

func currentWelcomeConfig() welcomeConfigResponse {
	rt := getWelcomeRuntime()
	pkgs := rt.packages
	if pkgs == nil {
		pkgs = []welcomePackage{}
	}
	avs := rt.activeVersions
	if avs == nil {
		avs = []string{}
	}
	firstActive := ""
	if len(avs) > 0 {
		firstActive = avs[0]
	}
	return welcomeConfigResponse{
		Enabled:                    rt.enabled,
		ScanIntervalSecs:           int(rt.interval / time.Second),
		ActiveVersion:              firstActive,
		ActiveVersions:             avs,
		Packages:                   pkgs,
		WelcomeMessageEnabled:      rt.welcomeMessageEnabled,
		WelcomeMessage:             rt.welcomeMessage,
		WelcomeWhisperSourcePlayer: rt.welcomeWhisperSourcePlayer,
		MotdEnabled:                rt.motdEnabled,
		MotdMessage:                rt.motdMessage,
		MotdSourcePlayer:           rt.motdSourcePlayer,
	}
}

// @Summary Get welcome-package config
// @Tags welcome-package
// @Produce json
// @Success 200 {object} welcomeConfigResponse
// @Router /api/v1/welcome-package/config [get]
func handleGetWelcomeConfig(w http.ResponseWriter, _ *http.Request) {
	// Ensure the runtime reflects the latest DB-persisted config (or the
	// YAML seed if this is the first boot after migration).
	if welcomeStoreDB != nil {
		if err := applyWelcomeConfigFromStore(); err != nil {
			log.Printf("handleGetWelcomeConfig: %v", err)
		}
	}
	jsonOK(w, currentWelcomeConfig())
}

// applyWelcomeConfigFromStore reads the welcome_config table and updates the
// in-memory runtime. On first boot (table empty) it seeds from loadedConfig
// (the YAML values) so existing deployments migrate automatically.
func applyWelcomeConfigFromStore() error {
	row, ok, err := welcomeStoreDB.loadConfig()
	if err != nil {
		return fmt.Errorf("load welcome config: %w", err)
	}
	if !ok {
		// First boot: seed from YAML fields.
		return seedWelcomeConfigFromYAML()
	}
	var pkgs []welcomePackage
	if err := json.Unmarshal([]byte(row.PackagesJSON), &pkgs); err != nil {
		return fmt.Errorf("parse welcome packages JSON: %w", err)
	}
	setWelcomeRuntime(buildWelcomeRuntime(row.Enabled, row.ActiveVersions, row.ScanSecs, pkgs, welcomeMessageOptions{
		enabled:      row.WelcomeMessageEnabled,
		message:      row.WelcomeMessage,
		sourcePlayer: row.WelcomeWhisperSourcePlayer,
	}, motdOptions{
		enabled:      row.MotdEnabled,
		message:      row.MotdMessage,
		sourcePlayer: row.MotdSourcePlayer,
	}))
	return nil
}

// seedWelcomeConfigFromYAML reads the legacy YAML fields from loadedConfig,
// saves them into the DB store (one-time migration), and applies them live.
func seedWelcomeConfigFromYAML() error {
	pkgs := loadedConfig.WelcomePackages
	active := loadedConfig.WelcomePackageActiveVersion
	if len(pkgs) == 0 && len(loadedConfig.WelcomePackageItems) > 0 {
		v := loadedConfig.WelcomePackageVersion
		if v == "" {
			v = "v1"
		}
		pkgs = []welcomePackage{{Version: v, Items: loadedConfig.WelcomePackageItems}}
		if active == "" {
			active = v
		}
	}
	pkgsJSON, err := json.Marshal(pkgs)
	if err != nil {
		return fmt.Errorf("marshal welcome packages: %w", err)
	}
	scanSecs := loadedConfig.WelcomePackageScanSecs
	enabled := loadedConfig.WelcomePackageEnabled != nil && *loadedConfig.WelcomePackageEnabled
	row := welcomeConfigRow{
		Enabled:  enabled,
		ScanSecs: scanSecs,
		ActiveVersions: func() []string {
			if active != "" {
				return []string{active}
			}
			return nil
		}(),
		PackagesJSON: string(pkgsJSON),
	}
	if err := welcomeStoreDB.saveConfig(row); err != nil {
		return fmt.Errorf("seed welcome config: %w", err)
	}
	setWelcomeRuntime(buildWelcomeRuntime(enabled, row.ActiveVersions, scanSecs, pkgs, welcomeMessageOptions{}))
	return nil
}

// validateActivePackages validates items inside any active packages.
// Empty active-package list is allowed (scanner just grants nothing).
func validateActivePackages(rt welcomePackageRuntime) error {
	for _, pkg := range rt.activePackages() {
		if err := validateWelcomeItems(pkg.Items); err != nil {
			return fmt.Errorf("package %q: %w", pkg.Version, err)
		}
	}
	return nil
}

// @Summary Update welcome-package config (applies live + persists)
// @Tags welcome-package
// @Accept json
// @Produce json
// @Param body body welcomeConfigResponse true "config"
// @Success 200 {object} welcomeConfigResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/welcome-package/config [put]
func handlePutWelcomeConfig(w http.ResponseWriter, r *http.Request) {
	var req welcomeConfigResponse
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}

	// Resolve active versions: prefer ActiveVersions; fall back to legacy ActiveVersion.
	activeVersions := req.ActiveVersions
	if len(activeVersions) == 0 && req.ActiveVersion != "" {
		activeVersions = []string{req.ActiveVersion}
	}

	rt := buildWelcomeRuntime(req.Enabled, activeVersions, req.ScanIntervalSecs, req.Packages, welcomeMessageOptions{
		enabled:      req.WelcomeMessageEnabled,
		message:      req.WelcomeMessage,
		sourcePlayer: req.WelcomeWhisperSourcePlayer,
	}, motdOptions{
		enabled:      req.MotdEnabled,
		message:      req.MotdMessage,
		sourcePlayer: req.MotdSourcePlayer,
	})

	if req.Enabled {
		if err := validateActivePackages(rt); err != nil {
			jsonErr(w, err, http.StatusBadRequest)
			return
		}
	}

	// Persist to the SQLite store (replaces the old config.yaml write path).
	if welcomeStoreDB != nil {
		pkgsJSON, err := json.Marshal(req.Packages)
		if err != nil {
			jsonErr(w, fmt.Errorf("marshal packages: %w", err), http.StatusInternalServerError)
			return
		}
		row := welcomeConfigRow{
			Enabled:                    rt.enabled,
			ScanSecs:                   int(rt.interval / time.Second),
			ActiveVersions:             rt.activeVersions,
			PackagesJSON:               string(pkgsJSON),
			WelcomeMessageEnabled:      rt.welcomeMessageEnabled,
			WelcomeMessage:             rt.welcomeMessage,
			WelcomeWhisperSourcePlayer: rt.welcomeWhisperSourcePlayer,
			MotdEnabled:                rt.motdEnabled,
			MotdMessage:                rt.motdMessage,
			MotdSourcePlayer:           rt.motdSourcePlayer,
		}
		if err := welcomeStoreDB.saveConfig(row); err != nil {
			log.Printf("handlePutWelcomeConfig: save to store: %v", err)
			jsonErr(w, fmt.Errorf("failed to save config"), http.StatusInternalServerError)
			return
		}
	}

	// Apply live — the scanner reads this on its next tick, no restart needed.
	setWelcomeRuntime(rt)

	jsonOK(w, currentWelcomeConfig())
}

// @Summary List welcome-package grant ledger rows
// @Tags welcome-package
// @Produce json
// @Param limit query int false "max rows (default 100)"
// @Success 200 {array} welcomeGrantRecord
// @Failure 503 {object} map[string]string
// @Router /api/v1/welcome-package/grants [get]
func handleGetWelcomeGrants(w http.ResponseWriter, r *http.Request) {
	if welcomeStoreDB == nil {
		jsonErr(w, fmt.Errorf("welcome package store not available"), http.StatusServiceUnavailable)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := welcomeStoreDB.listGrants(limit)
	if err != nil {
		log.Printf("handleGetWelcomeGrants: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

// @Summary Retry a failed welcome-package grant (clears the ledger row)
// @Tags welcome-package
// @Accept json
// @Produce json
// @Param body body object true "fls_id, package_version, account_id"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/welcome-package/retry [post]
func handleRetryWelcomeGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID          string `json:"fls_id"`
		PackageVersion string `json:"package_version"`
		AccountID      int64  `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if welcomeStoreDB == nil {
		jsonErr(w, fmt.Errorf("welcome package store not available"), http.StatusServiceUnavailable)
		return
	}
	if req.FlsID == "" || req.PackageVersion == "" {
		jsonErr(w, fmt.Errorf("fls_id and package_version required"), http.StatusBadRequest)
		return
	}
	n, err := welcomeStoreDB.deleteFailed(req.FlsID, req.PackageVersion, req.AccountID)
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"cleared": n})
}

// @Summary Revoke a welcome-package grant (clears the ledger row so the same package can be granted again)
// @Tags welcome-package
// @Accept json
// @Produce json
// @Param body body object true "fls_id, package_version, account_id"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/welcome-package/revoke [post]
func handleRevokeWelcomeGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID          string `json:"fls_id"`
		PackageVersion string `json:"package_version"`
		AccountID      int64  `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if welcomeStoreDB == nil {
		jsonErr(w, fmt.Errorf("welcome package store not available"), http.StatusServiceUnavailable)
		return
	}
	if req.FlsID == "" || req.PackageVersion == "" {
		jsonErr(w, fmt.Errorf("fls_id and package_version required"), http.StatusBadRequest)
		return
	}
	n, err := welcomeStoreDB.deleteGrant(req.FlsID, req.PackageVersion, req.AccountID)
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"revoked": n})
}

// @Summary Manually grant a package to a chosen player, bypassing the already-granted guard
// @Tags welcome-package
// @Accept json
// @Produce json
// @Param body body object true "account_id, package_version"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/welcome-package/override [post]
func handleOverrideWelcomeGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID      int64  `json:"account_id"`
		PackageVersion string `json:"package_version"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if req.AccountID <= 0 || req.PackageVersion == "" {
		jsonErr(w, fmt.Errorf("account_id and package_version required"), http.StatusBadRequest)
		return
	}

	// Resolve the package items from the live runtime so the override always
	// uses the operator's current definition.
	rt := getWelcomeRuntime()
	idx := findPackage(rt.packages, req.PackageVersion)
	if idx < 0 {
		jsonErr(w, fmt.Errorf("package %q not found", req.PackageVersion), http.StatusBadRequest)
		return
	}
	pkg := rt.packages[idx]
	if err := validateWelcomeItems(pkg.Items); err != nil {
		jsonErr(w, fmt.Errorf("package %q: %w", req.PackageVersion, err), http.StatusBadRequest)
		return
	}

	if globalDB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	if welcomeStoreDB == nil {
		jsonErr(w, fmt.Errorf("welcome package store not available"), http.StatusServiceUnavailable)
		return
	}

	acc, err := cmdFetchWelcomeAccount(r.Context(), globalDB, req.AccountID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			jsonErr(w, fmt.Errorf("player not found"), http.StatusNotFound)
			return
		}
		log.Printf("handleOverrideWelcomeGrant: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}

	if err := overrideGrantToAccount(r.Context(), acc, pkg.Version, pkg.Items, welcomeScanDeps{
		grant: welcomeGrantViaGiveItems,
		store: welcomeStoreDB,
	}); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{"granted": true, "account_id": acc.AccountID, "character_name": acc.CharacterName})
}

// @Summary Run a welcome-package scan now (one-off, regardless of enabled)
// @Tags welcome-package
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/welcome-package/run [post]
func handleRunWelcomePackage(w http.ResponseWriter, _ *http.Request) {
	if welcomeStoreDB == nil {
		jsonErr(w, fmt.Errorf("welcome package store not available"), http.StatusServiceUnavailable)
		return
	}
	rt := getWelcomeRuntime()
	activePkgs := rt.activePackages()
	if len(activePkgs) == 0 {
		jsonErr(w, fmt.Errorf("no active package selected"), http.StatusBadRequest)
		return
	}
	var totalGranted, totalFailed, totalSkipped int
	for _, pkg := range activePkgs {
		if err := validateWelcomeItems(pkg.Items); err != nil {
			jsonErr(w, fmt.Errorf("package %q: %w", pkg.Version, err), http.StatusBadRequest)
			return
		}
		g, f, s, err := welcomePackageScanOnce(context.Background(), pkg.Version, pkg.Items, welcomeScanDeps{
			listAccounts: listWelcomeOnlineAccounts,
			grant:        welcomeGrantViaGiveItems,
			store:        welcomeStoreDB,
		})
		if err != nil {
			jsonErr(w, err, http.StatusInternalServerError)
			return
		}
		totalGranted += g
		totalFailed += f
		totalSkipped += s
	}
	jsonOK(w, map[string]any{"granted": totalGranted, "failed": totalFailed, "skipped": totalSkipped})
}
