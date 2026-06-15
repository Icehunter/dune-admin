package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// servers_columns.go stores each per-server ServerConfig as typed columns on the
// servers table instead of the legacy config_json blob. The ServerConfig struct
// and its json/yaml tags are unchanged; only storage moves to columns. The
// servers.config_json column is kept (written as '{}') but no longer read once
// migrated (see migrateServersColumns). ID maps to the existing id PK, Name to
// the existing name column, and LegacyID is import-only (never stored).

// serverColumnAlters adds one column per typed ServerConfig field. SQLite has no
// IF NOT EXISTS for ALTER TABLE, so each statement is attempted and "duplicate
// column" errors are tolerated (matching initWelcomeSchema).
var serverColumnAlters = []string{
	"ALTER TABLE servers ADD COLUMN ssh_host TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN ssh_user TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN ssh_key TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN ssh_mode TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN ssh_extra_opts TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN auto_discover INTEGER NOT NULL DEFAULT 0",
	"ALTER TABLE servers ADD COLUMN db_host TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN db_port INTEGER NOT NULL DEFAULT 0",
	"ALTER TABLE servers ADD COLUMN db_user TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN db_pass TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN db_name TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN db_schema TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN control TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN control_namespace TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN docker_gameserver TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN docker_broker_game TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN docker_broker_admin TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN docker_db TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN cmd_start TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN cmd_stop TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN cmd_restart TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN cmd_status TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN broker_game_addr TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN broker_admin_addr TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN broker_tls INTEGER NOT NULL DEFAULT 0",
	"ALTER TABLE servers ADD COLUMN broker_user TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN broker_pass TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN broker_jwt_secret TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN broker_exec_prefix TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN backup_dir TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN server_ini_dir TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN default_ini_dir TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_instance TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_container TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_user TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_log_path TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_use_container INTEGER",
	"ALTER TABLE servers ADD COLUMN amp_container_runtime TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_data_root TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_api_user TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_api_pass TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_api_port INTEGER NOT NULL DEFAULT 0",
	"ALTER TABLE servers ADD COLUMN amp_pg_bin TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_pg_lib TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN amp_backup_dir TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN director_url TEXT NOT NULL DEFAULT ''",
	"ALTER TABLE servers ADD COLUMN market_bot_enabled INTEGER",
}

// initServersColumnsSchema adds the typed ServerConfig columns to the servers
// table. Assumes initServersSchema has created the table. Idempotent.
func initServersColumnsSchema(db *sql.DB) error {
	for _, alter := range serverColumnAlters {
		if _, err := db.Exec(alter); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("init servers columns schema: %w", err)
		}
	}
	return nil
}

// serverColumnNames lists the typed columns in the canonical order shared by the
// UPDATE writer and the SELECT reader. ID/Name/LegacyID are excluded by design.
const serverColumnNames = `ssh_host, ssh_user, ssh_key, ssh_mode, ssh_extra_opts, auto_discover,
	db_host, db_port, db_user, db_pass, db_name, db_schema, control, control_namespace,
	docker_gameserver, docker_broker_game, docker_broker_admin, docker_db,
	cmd_start, cmd_stop, cmd_restart, cmd_status,
	broker_game_addr, broker_admin_addr, broker_tls, broker_user, broker_pass, broker_jwt_secret,
	broker_exec_prefix, backup_dir, server_ini_dir, default_ini_dir,
	amp_instance, amp_container, amp_user, amp_log_path, amp_use_container, amp_container_runtime,
	amp_data_root, amp_api_user, amp_api_pass, amp_api_port, amp_pg_bin, amp_pg_lib, amp_backup_dir,
	director_url, market_bot_enabled`

// writeServerColumns updates the typed columns for an existing server row.
// insertServer creates the row first, so this is always an UPDATE by id.
func writeServerColumns(db dbExecer, id int, cfg ServerConfig) error {
	_, err := db.Exec(`UPDATE servers SET
		ssh_host=?, ssh_user=?, ssh_key=?, ssh_mode=?, ssh_extra_opts=?, auto_discover=?,
		db_host=?, db_port=?, db_user=?, db_pass=?, db_name=?, db_schema=?, control=?, control_namespace=?,
		docker_gameserver=?, docker_broker_game=?, docker_broker_admin=?, docker_db=?,
		cmd_start=?, cmd_stop=?, cmd_restart=?, cmd_status=?,
		broker_game_addr=?, broker_admin_addr=?, broker_tls=?, broker_user=?, broker_pass=?,
		broker_jwt_secret=?, broker_exec_prefix=?, backup_dir=?, server_ini_dir=?, default_ini_dir=?,
		amp_instance=?, amp_container=?, amp_user=?, amp_log_path=?, amp_use_container=?,
		amp_container_runtime=?, amp_data_root=?, amp_api_user=?, amp_api_pass=?, amp_api_port=?,
		amp_pg_bin=?, amp_pg_lib=?, amp_backup_dir=?, director_url=?, market_bot_enabled=?
		WHERE id=?`,
		cfg.SSHHost, cfg.SSHUser, cfg.SSHKey, cfg.SSHMode, cfg.SSHExtraOpts, b2i(cfg.AutoDiscover),
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName, cfg.DBSchema, cfg.Control,
		cfg.ControlNamespace, cfg.DockerGameserver, cfg.DockerBrokerGame, cfg.DockerBrokerAdmin,
		cfg.DockerDB, cfg.CmdStart, cfg.CmdStop, cfg.CmdRestart, cfg.CmdStatus,
		cfg.BrokerGameAddr, cfg.BrokerAdminAddr, b2i(cfg.BrokerTLS), cfg.BrokerUser, cfg.BrokerPass,
		cfg.BrokerJWTSecret, cfg.BrokerExecPrefix, cfg.BackupDir, cfg.ServerIniDir, cfg.DefaultIniDir,
		cfg.AmpInstance, cfg.AmpContainer, cfg.AmpUser, cfg.AmpLogPath, boolPtrToNullInt(cfg.AmpUseContainer),
		cfg.AmpContainerRuntime, cfg.AmpDataRoot, cfg.AmpAPIUser, cfg.AmpAPIPass, cfg.AmpAPIPort,
		cfg.AmpPgBin, cfg.AmpPgLib, cfg.AmpBackupDir, cfg.DirectorURL, boolPtrToNullInt(cfg.MarketBotEnabled),
		id)
	if err != nil {
		return fmt.Errorf("write server columns %d: %w", id, err)
	}
	return nil
}

// readServerColumns loads the typed columns for one server and stamps the
// authoritative numeric id. Returns sql.ErrNoRows if the row is absent.
func readServerColumns(db dbRowQueryer, id int) (ServerConfig, error) {
	var cfg ServerConfig
	var autoDiscover, brokerTLS int
	var ampUseContainer, marketBotEnabled sql.NullInt64
	err := db.QueryRow(`SELECT `+serverColumnNames+` FROM servers WHERE id=?`, id).Scan(
		&cfg.SSHHost, &cfg.SSHUser, &cfg.SSHKey, &cfg.SSHMode, &cfg.SSHExtraOpts, &autoDiscover,
		&cfg.DBHost, &cfg.DBPort, &cfg.DBUser, &cfg.DBPass, &cfg.DBName, &cfg.DBSchema, &cfg.Control,
		&cfg.ControlNamespace, &cfg.DockerGameserver, &cfg.DockerBrokerGame, &cfg.DockerBrokerAdmin,
		&cfg.DockerDB, &cfg.CmdStart, &cfg.CmdStop, &cfg.CmdRestart, &cfg.CmdStatus,
		&cfg.BrokerGameAddr, &cfg.BrokerAdminAddr, &brokerTLS, &cfg.BrokerUser, &cfg.BrokerPass,
		&cfg.BrokerJWTSecret, &cfg.BrokerExecPrefix, &cfg.BackupDir, &cfg.ServerIniDir, &cfg.DefaultIniDir,
		&cfg.AmpInstance, &cfg.AmpContainer, &cfg.AmpUser, &cfg.AmpLogPath, &ampUseContainer,
		&cfg.AmpContainerRuntime, &cfg.AmpDataRoot, &cfg.AmpAPIUser, &cfg.AmpAPIPass, &cfg.AmpAPIPort,
		&cfg.AmpPgBin, &cfg.AmpPgLib, &cfg.AmpBackupDir, &cfg.DirectorURL, &marketBotEnabled)
	if err != nil {
		return ServerConfig{}, err
	}
	cfg.ID = id
	cfg.AutoDiscover = autoDiscover != 0
	cfg.BrokerTLS = brokerTLS != 0
	cfg.AmpUseContainer = nullIntToBoolPtr(ampUseContainer)
	cfg.MarketBotEnabled = nullIntToBoolPtr(marketBotEnabled)
	return cfg, nil
}

type legacyServerBlob struct {
	id  int
	cfg ServerConfig
}

// readLegacyServerBlobs decodes every servers.config_json blob into a typed
// ServerConfig keyed by row id. Empty blobs decode to a zero-value config.
func readLegacyServerBlobs(tx *sql.Tx) ([]legacyServerBlob, error) {
	rows, err := tx.Query(`SELECT id, config_json FROM servers`)
	if err != nil {
		return nil, fmt.Errorf("read legacy server blobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []legacyServerBlob
	for rows.Next() {
		var rec legacyServerBlob
		var blob string
		if err := rows.Scan(&rec.id, &blob); err != nil {
			return nil, fmt.Errorf("scan legacy server: %w", err)
		}
		if blob != "" {
			if err := json.Unmarshal([]byte(blob), &rec.cfg); err != nil {
				return nil, fmt.Errorf("unmarshal legacy server %d: %w", rec.id, err)
			}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// migrateServersColumns translates each legacy servers.config_json blob into the
// typed columns, once, guarded by the migrated:servers_columns marker. After
// this runs the blob column is never read again.
func migrateServersColumns(db *sql.DB) error {
	return runColumnMigrationOnce(db, "migrated:servers_columns", func(tx *sql.Tx) error {
		blobs, err := readLegacyServerBlobs(tx)
		if err != nil {
			return err
		}
		for _, rec := range blobs {
			if err := writeServerColumns(tx, rec.id, rec.cfg); err != nil {
				return err
			}
		}
		return nil
	})
}
