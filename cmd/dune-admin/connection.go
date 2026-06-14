package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"
)

var (
	// Legacy globals kept for K8s path (globalSSH/globalPod*) and for the
	// shared DB pool (globalDB). New code should use globalExecutor/globalControl.
	globalSSH   *ssh.Client
	globalDB    *pgxpool.Pool
	globalPodIP string
	globalPodNS string
	globalPod   string

	globalExecutor Executor
	globalControl  ControlPlane
)

// resolveControl returns the effective control plane name based on config,
// defaulting to "kubectl" when SSH is configured and "local" otherwise.
func resolveControl() string {
	if controlPlane != "" {
		return controlPlane
	}
	if sshHost != "" {
		return "kubectl"
	}
	return "local"
}

// connectAll creates the executor, control plane, and DB connection, then sets
// all globals. Called from main() and handleReconnect.
func connectAll() error {
	ctrl := resolveControl()

	// Start from the full loaded config so provider-specific fields
	// (docker_*, cmd_*) that have no flag/env equivalent are preserved.
	cfg := loadedConfig
	cfg.SSHHost = sshHost
	cfg.SSHUser = sshUser
	cfg.SSHKey = resolveKeyPath()
	cfg.SSHMode = sshMode
	cfg.SSHExtraOpts = sshExtraOpts
	cfg.AutoDiscover = autoDiscover
	cfg.DBHost = dbHost
	cfg.DBPort = dbPort
	cfg.DBUser = dbUser
	cfg.DBPass = dbPass
	cfg.DBName = dbName
	cfg.DBSchema = dbSchema
	cfg.Control = ctrl
	cfg.ControlNamespace = controlNS
	cfg.BrokerGameAddr = brokerGameAddr
	cfg.BrokerAdminAddr = brokerAdminAddr
	cfg.BrokerTLS = brokerTLS
	cfg.BackupDir = backupDir
	cfg.ServerIniDir = serverIniDir

	exec, err := newExecutor(cfg.SSHHost, cfg.SSHUser, cfg.SSHKey, cfg.SSHMode, cfg.SSHExtraOpts)
	if err != nil {
		return fmt.Errorf("executor: %w", err)
	}
	// AMP mode wraps the executor to elevate WriteFile through sudo.
	// Applies regardless of whether the inner executor is local or SSH.
	if ctrl == "amp" {
		user := cfg.AmpUser
		if user == "" {
			user = "amp"
		}
		exec = &ampExecutor{Executor: exec, ampUser: user}
	}
	globalExecutor = exec

	// Auto-discovery: fill empty DB/RMQ/Director fields from the running
	// game-server args. Best-effort and self-contained in applyAutoDiscovery.
	if cfg.AutoDiscover {
		applyAutoDiscovery(&cfg, exec, ctrl)
	}

	// kubectl needs DB-pod discovery (via the executor, not the DB) to learn the
	// namespace before the control plane and DB connect. A discovery failure is
	// fatal — without a namespace there is nothing to drive the control plane.
	if ctrl == "kubectl" {
		ns, pod, podIP, err := discoverDBPod(exec)
		if err != nil {
			exec.Close()
			globalExecutor = nil
			return fmt.Errorf("DB pod discovery: %w", err)
		}
		globalPodNS = ns
		globalPod = pod
		globalPodIP = podIP
		// Propagate discovered namespace so kubectlControl can use it.
		if cfg.ControlNamespace == "" {
			cfg.ControlNamespace = ns
			controlNS = ns
		}
		if s, ok := exec.(*sshExecutor); ok {
			globalSSH = s.client
		}
	}

	// The control plane (logs, battlegroup, server control) does not depend on
	// the database. Establish it before connecting the DB so a DB outage never
	// disables it — the DB can be re-established later via /api/v1/reconnect
	// without losing control-plane functionality.
	globalControl = newControlPlane(ctrl, cfg)

	// DB connect is best-effort: on failure keep the executor + control plane
	// intact and return the error so the caller can surface it (main starts the
	// server anyway; the systemd watchdog or a manual reconnect retries the DB).
	var pool *pgxpool.Pool
	if ctrl == "kubectl" {
		pool, err = connectDB(context.Background(), cfg.DBUser, cfg.DBPass)
	} else {
		pool, err = connectDBDirect(context.Background(), cfg)
	}
	if err != nil {
		globalDB = nil
		return fmt.Errorf("DB connect: %w", err)
	}
	globalDB = pool
	ensureDBSchema(pool)
	return nil
}

// ensureDBSchema runs best-effort idempotent schema init after DB connect.
// Failures are logged and swallowed — they must never block startup.
func ensureDBSchema(pool *pgxpool.Pool) {
	ctx := context.Background()
	if err := cmdEnsureGMIdentity(ctx); err != nil {
		log.Printf("connectAll: ensure GM identity: %v", err)
	}
	if err := cmdEnsureDiscordLinksTable(ctx, pool); err != nil {
		log.Printf("connectAll: ensure discord_links table: %v", err)
	}
}

// applyAutoDiscovery runs game-server discovery and gap-fills cfg in place,
// propagating the filled values into the connection globals the DB/broker
// connect paths read. Best-effort: a discovery failure is logged and ignored
// (the operator may have set everything explicitly). The password is never
// logged in clear text. Extracted from connectAll to keep its cognitive
// complexity within the project gate.
func applyAutoDiscovery(cfg *appConfig, exec Executor, ctrl string) {
	g, err := discoverGameConfig(exec)
	if err != nil {
		log.Printf("connectAll: auto-discover: %v", err)
		return
	}
	applyDiscovered(cfg, g)
	// Propagate into the globals the DB connect path reads.
	dbUser, dbPass, dbName = cfg.DBUser, cfg.DBPass, cfg.DBName
	log.Printf("connectAll: auto-discover filled DB user=%s name=%s (pass %s)",
		cfg.DBUser, cfg.DBName, maskSecret(cfg.DBPass))
	if ctrl == "kubectl" {
		pods := fetchClusterPodIPs(exec)
		gameIP := podIPByPattern(pods, "mq-game")
		adminIP := podIPByPattern(pods, "mq-admin")
		directorIP := podIPByPattern(pods, "bgd")
		applyDiscoveredEndpoints(cfg, g, gameIP, adminIP, directorIP)
		brokerGameAddr, brokerAdminAddr, brokerTLS = cfg.BrokerGameAddr, cfg.BrokerAdminAddr, cfg.BrokerTLS
	}
	if discoverWrite {
		if werr := writeConfigFile(*cfg); werr != nil {
			log.Printf("connectAll: discover-write: %v", werr)
		} else {
			loadedConfig = *cfg
			log.Printf("connectAll: discover-write persisted config.yaml")
		}
	}
}

// cmdConnect wraps connectAll in the legacy Msg return type.
func cmdConnect() Msg {
	if err := connectAll(); err != nil {
		return msgConnect{err: err}
	}
	return msgConnect{}
}

// connectDBDirect opens a pgxpool without SSH tunnelling, routing TCP through
// the executor's Dial (which is net.Dial for local, SSH tunnel for SSH).
func connectDBDirect(ctx context.Context, cfg appConfig) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName)
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{cfg.DBSchema}.Sanitize()))
		return err
	}
	if globalExecutor != nil {
		addr := fmt.Sprintf("%s:%d", cfg.DBHost, cfg.DBPort)
		poolCfg.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return globalExecutor.Dial("tcp", addr)
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	dbUser = cfg.DBUser
	dbPass = cfg.DBPass
	return pool, nil
}

func connectDB(ctx context.Context, user, pass string) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf(
		"host=127.0.0.1 port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbPort, user, pass, dbName)
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{dbSchema}.Sanitize()))
		return err
	}
	poolCfg.ConnConfig.LookupFunc = func(_ context.Context, _ string) ([]string, error) {
		return []string{globalPodIP}, nil
	}
	if globalExecutor == nil {
		return nil, fmt.Errorf("cannot connect to DB: globalExecutor is nil (DB pod discovery likely failed)")
	}
	poolCfg.ConnConfig.DialFunc = func(_ context.Context, _, _ string) (net.Conn, error) {
		return globalExecutor.Dial("tcp", fmt.Sprintf("%s:%d", globalPodIP, dbPort))
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	dbUser = user
	dbPass = pass
	return pool, nil
}
