package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// welcomeConfigResponse is the shape returned by the config endpoints and
// consumed by the WelcomePackage tab. It carries the whole package library plus
// the active-version pointer.
type welcomeConfigResponse struct {
	Enabled          bool             `json:"enabled"`
	ScanIntervalSecs int              `json:"scan_interval_secs"`
	ActiveVersion    string           `json:"active_version"`
	Packages         []welcomePackage `json:"packages"`
}

func currentWelcomeConfig() welcomeConfigResponse {
	rt := getWelcomeRuntime()
	pkgs := rt.packages
	if pkgs == nil {
		pkgs = []welcomePackage{}
	}
	return welcomeConfigResponse{
		Enabled:          rt.enabled,
		ScanIntervalSecs: int(rt.interval / time.Second),
		ActiveVersion:    rt.activeVersion,
		Packages:         pkgs,
	}
}

// @Summary Get welcome-package config
// @Tags welcome-package
// @Produce json
// @Success 200 {object} welcomeConfigResponse
// @Router /api/v1/welcome-package/config [get]
func handleGetWelcomeConfig(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, currentWelcomeConfig())
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

	rt := buildWelcomeRuntime(req.Enabled, req.ActiveVersion, req.ScanIntervalSecs, req.Packages)

	// Only require a valid active package when enabling — an operator can save a
	// disabled draft (e.g. no packages yet, or a half-built one).
	if req.Enabled {
		pkg, ok := rt.active()
		if !ok {
			jsonErr(w, fmt.Errorf("select an active package version before enabling"), http.StatusBadRequest)
			return
		}
		if err := validateWelcomeItems(pkg.Items); err != nil {
			jsonErr(w, err, http.StatusBadRequest)
			return
		}
	}

	// Persist the welcome fields onto the in-memory config and write it back.
	// loadedConfig holds real (unmasked) secrets, so writing it preserves every
	// other field untouched.
	cfg := loadedConfig
	enabled := rt.enabled
	cfg.WelcomePackageEnabled = &enabled
	cfg.WelcomePackageScanSecs = int(rt.interval / time.Second)
	cfg.WelcomePackageActiveVersion = rt.activeVersion
	cfg.WelcomePackages = req.Packages
	// Clear the legacy single-package fields so they can't shadow the library.
	cfg.WelcomePackageVersion = ""
	cfg.WelcomePackageItems = nil
	if err := writeConfigFile(cfg); err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	loadedConfig = cfg

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
	pkg, ok := rt.active()
	if !ok {
		jsonErr(w, fmt.Errorf("no active package selected"), http.StatusBadRequest)
		return
	}
	if err := validateWelcomeItems(pkg.Items); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
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
	jsonOK(w, map[string]any{"granted": g, "failed": f, "skipped": s})
}
