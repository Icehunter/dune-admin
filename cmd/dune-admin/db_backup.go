package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ── Database backups (#150) ─────────────────────────────────────────────────
// AMP-native Postgres backups. The existing handleBGBackup* family targets
// kubectl/k8s pod paths and does nothing on AMP, so this is a separate,
// control-plane-aware path: pg_dump (-Fc) runs inside the AMP container and its
// stdout is redirected to a host file the dune-admin service user owns, so the
// list/download/delete operations are plain os.* calls on the host. Restore
// (pg_restore --clean) is destructive and guarded at the handler layer.

// dbConn is the Postgres connection target for backup/restore.
type dbConn struct {
	Host string
	Port int
	User string
	Pass string
	Name string
}

type dbBackupFile struct {
	Name     string `json:"name"`
	SizeB    int64  `json:"size_bytes"`
	Modified string `json:"modified"`
}

// dbBackupProvider is the optional control-plane capability for native database
// backup/restore. Only the AMP control plane implements it; other planes get a
// 501 via the handler's type assertion.
type dbBackupProvider interface {
	BackupDatabase(exec Executor, conn dbConn, destPath string) (string, error)
	RestoreDatabase(exec Executor, conn dbConn, srcPath string) (string, error)
}

var backupNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+\.dump$`)

// validateBackupName guards against path traversal and shell metacharacters: a
// backup filename must be a bare name (no separators or "..") matching a strict
// charset and ending in .dump.
func validateBackupName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid backup name")
	}
	if !backupNameRe.MatchString(name) {
		return fmt.Errorf("invalid backup name")
	}
	return nil
}

// backupsToPrune returns the names to delete to satisfy a keep-N retention
// policy, given names sorted newest-first. keepN <= 0 disables pruning.
func backupsToPrune(newestFirst []string, keepN int) []string {
	if keepN <= 0 || len(newestFirst) <= keepN {
		return nil
	}
	return append([]string(nil), newestFirst[keepN:]...)
}

// dbBackupFilename is the canonical timestamped name for a new backup.
func dbBackupFilename(t time.Time) string {
	return "dune-" + t.UTC().Format("20060102-150405") + ".dump"
}

// dbBackupDir resolves (and creates) the host directory where dumps live for the
// active server's config. amp_backup_dir is per-server (servers table) after the
// remodel, so resolve it from the active ServerConfig, not the cleared global.
func dbBackupDir() (string, error) {
	return dbBackupDirFor(activeServerCfg())
}

// dbBackupDirFor resolves (and creates) the host directory where dumps live for
// the given server config.
func dbBackupDirFor(cfg ServerConfig) (string, error) {
	return resolveBackupDir(cfg.AmpBackupDir)
}

func resolveBackupDir(ampBackupDir string) (string, error) {
	dir := ampBackupDir
	if dir == "" {
		dir = filepath.Join(configDir(), "db-backups")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	return dir, nil
}

// dbBackupConn builds the Postgres connection target from the active server's
// config. The connection lives on the per-server ServerConfig (servers table)
// after the storage remodel — the global loadedConfig flat fields are cleared,
// so reading them here would dump the wrong DB (e.g. default :5432 instead of
// the AMP :15432). Falls back to loadedConfig only when no server is registered.
func dbBackupConn() dbConn {
	return dbBackupConnFor(activeServerCfg())
}

// dbBackupConnFor builds the Postgres connection target from a server config. The
// AMP Postgres listens on 127.0.0.1:<db_port> both inside and outside the container.
func dbBackupConnFor(cfg ServerConfig) dbConn {
	return resolveBackupConn(cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPass)
}

func resolveBackupConn(port int, name, user, pass string) dbConn {
	if port == 0 {
		port = 5432
	}
	if name == "" {
		name = "dune"
	}
	if user == "" {
		user = "dune"
	}
	return dbConn{Host: "127.0.0.1", Port: port, User: user, Pass: pass, Name: name}
}

// listDBBackups lists the .dump files in the active server's backup dir.
func listDBBackups() ([]dbBackupFile, error) {
	dir, err := dbBackupDir()
	if err != nil {
		return nil, err
	}
	return listDBBackupsInDir(dir)
}

// listDBBackupsIn lists the .dump files in cfg's backup dir, newest first.
func listDBBackupsIn(cfg ServerConfig) ([]dbBackupFile, error) {
	dir, err := dbBackupDirFor(cfg)
	if err != nil {
		return nil, err
	}
	return listDBBackupsInDir(dir)
}

func listDBBackupsInDir(dir string) ([]dbBackupFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	files := make([]dbBackupFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".dump") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, dbBackupFile{
			Name:     e.Name(),
			SizeB:    info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	// RFC3339 UTC strings sort lexicographically in chronological order.
	sort.Slice(files, func(i, j int) bool { return files[i].Modified > files[j].Modified })
	return files, nil
}

// deleteDBBackup removes a backup file from the active server's dir, after
// validating the name. Used by manual delete.
func deleteDBBackup(name string) error {
	dir, err := dbBackupDir()
	if err != nil {
		return err
	}
	return deleteDBBackupInDir(dir, name)
}

// deleteDBBackupIn removes a backup file from cfg's dir, after validating the
// name. Used by retention pruning.
func deleteDBBackupIn(cfg ServerConfig, name string) error {
	dir, err := dbBackupDirFor(cfg)
	if err != nil {
		return err
	}
	return deleteDBBackupInDir(dir, name)
}

func deleteDBBackupInDir(dir, name string) error {
	if err := validateBackupName(name); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	// #nosec G304 G703 -- name validated by validateBackupName above (no separators/..)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete backup %q: %w", name, err)
	}
	return nil
}
