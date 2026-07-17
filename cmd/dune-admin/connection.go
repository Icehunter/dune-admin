package main

import (
	"context"
	"fmt"
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

// resolveDBPort returns port if non-zero, otherwise the standard dune-admin
// default of 15432 (the port AMP exposes PostgreSQL on via pgBouncer). This
// matches the -dbport flag default so that a zero-valued ServerConfig.DBPort
// (wizard left blank) behaves the same as the flat-config path.
func resolveDBPort(port int) int {
	if port == 0 {
		return 15432
	}
	return port
}

// resolveDBHost returns host if non-empty, otherwise "127.0.0.1".
// pgx v5's key-value DSN parser reads "host= port=N" as host="port=N" because it
// skips the space after the equals sign and reads the next token as the value.
// An empty host always means localhost, so substituting 127.0.0.1 avoids the
// parse ambiguity without changing behaviour.
func resolveDBHost(host string) string {
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

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

	// Build the default ServerContext now — control plane + executor are set
	// regardless of whether the DB connect succeeds. DB is filled in below.
	defaultSC := &ServerContext{
		ID:         "default",
		Name:       defaultServerName(),
		Cfg:        legacyServerFromFlat(loadedConfig),
		Control:    globalControl,
		Executor:   globalExecutor,
		PodIP:      globalPodIP,
		PodNS:      globalPodNS,
		Pod:        globalPod,
		SSH:        globalSSH,
		StoreScope: defaultServerID,
	}

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
		globalRegistry.Register(defaultSC)
		return fmt.Errorf("DB connect: %w", err)
	}
	globalDB = pool
	defaultSC.DB = pool
	globalRegistry.Register(defaultSC)
	ensureDBSchema(pool)
	return nil
}

// ensureDBSchema runs best-effort idempotent schema init after DB connect.
// Failures are logged and swallowed — they must never block startup.
func ensureDBSchema(pool *pgxpool.Pool) {
	ctx := context.Background()
	if err := cmdEnsureGMIdentity(ctx, pool); err != nil {
		componentLog("connection").Error().Err(err).Msg("ensure GM identity")
	}
	// Discord character links now live in the unified SQLite store
	// (discord_user_links), not Postgres — no per-pool table to ensure.
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
		componentLog("connection").Warn().Err(err).Msg("auto-discover failed")
		return
	}
	applyDiscovered(cfg, g)
	// Propagate into the globals the DB connect path reads.
	dbUser, dbPass, dbName = cfg.DBUser, cfg.DBPass, cfg.DBName
	componentLog("connection").Info().
		Str("db_user", cfg.DBUser).
		Str("db_name", cfg.DBName).
		Str("db_pass", maskSecret(cfg.DBPass)).
		Msg("auto-discover filled DB credentials")
	if ctrl == "kubectl" {
		pods := fetchClusterPodIPs(exec)
		gameIP := podIPByPattern(pods, "mq-game")
		adminIP := podIPByPattern(pods, "mq-admin")
		directorIP := podIPByPattern(pods, "bgd")
		applyDiscoveredEndpoints(cfg, g, gameIP, adminIP, directorIP)
		brokerGameAddr, brokerAdminAddr, brokerTLS = cfg.BrokerGameAddr, cfg.BrokerAdminAddr, cfg.BrokerTLS
	}
	if discoverWrite {
		loadedConfig = *cfg
		// In DB-backed mode config.yaml is import-seed-only; discovered values
		// live in memory and are re-derived on each connect. Only the legacy
		// (store-unavailable) path persists them back to config.yaml.
		if globalSettingsStore == nil {
			if werr := writeConfigFile(*cfg); werr != nil {
				componentLog("connection").Error().Err(werr).Msg("discover-write failed")
			} else {
				componentLog("connection").Info().Msg("discover-write persisted config.yaml")
			}
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
		resolveDBHost(cfg.DBHost), cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName)
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{cfg.DBSchema}.Sanitize()))
		return err
	}
	if globalExecutor != nil {
		addr := fmt.Sprintf("%s:%d", resolveDBHost(cfg.DBHost), cfg.DBPort)
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

// activeServerCfg returns the active server's ServerConfig. Per-server
// connection/provider fields (db_*, broker_*, amp_*, director_url, ini/backup
// dirs) live on the ServerConfig after the storage remodel — the global
// loadedConfig flat fields are cleared — so consumers of those fields must
// resolve through here (or a request's server) rather than reading loadedConfig.
// Falls back to the flat-config view (flag-globals) when no server is registered
// yet (pre-setup), matching the legacy behaviour.
func activeServerCfg() ServerConfig {
	if a := globalRegistry.Active(); a != nil {
		return a.Cfg
	}
	return legacyServerFromFlat(loadedConfig)
}

// legacyServerFromFlat synthesises a default ServerConfig from the process-wide
// flag-globals and provider-specific fields in ac. It mirrors the cfg.X = ...
// block in connectAll so that the "default" server registered in globalRegistry
// carries the same values that the legacy connection path would use.
func legacyServerFromFlat(ac appConfig) ServerConfig {
	name := ac.DefaultServerName
	if name == "" {
		name = "Default"
	}
	return ServerConfig{
		LegacyID: "default",
		Name:     name,
		// Transport — read from the flag-globals that connectAll uses.
		SSHHost:      sshHost,
		SSHUser:      sshUser,
		SSHKey:       resolveKeyPath(),
		SSHMode:      sshMode,
		SSHExtraOpts: sshExtraOpts,
		AutoDiscover: autoDiscover,
		// Database
		DBHost:   dbHost,
		DBPort:   dbPort,
		DBUser:   dbUser,
		DBPass:   dbPass,
		DBName:   dbName,
		DBSchema: dbSchema,
		// Control
		Control:          resolveControl(),
		ControlNamespace: controlNS,
		// Broker
		BrokerGameAddr:   brokerGameAddr,
		BrokerAdminAddr:  brokerAdminAddr,
		BrokerTLS:        brokerTLS,
		BrokerUser:       ac.BrokerUser,
		BrokerPass:       ac.BrokerPass,
		BrokerJWTSecret:  ac.BrokerJWTSecret,
		BrokerExecPrefix: ac.BrokerExecPrefix,
		// Paths
		BackupDir:     backupDir,
		ServerIniDir:  serverIniDir,
		DefaultIniDir: ac.DefaultIniDir,
		// docker-specific container names
		DockerGameserver:  ac.DockerGameserver,
		DockerBrokerGame:  ac.DockerBrokerGame,
		DockerBrokerAdmin: ac.DockerBrokerAdmin,
		DockerDB:          ac.DockerDB,
		// local-specific shell commands
		CmdStart:   ac.CmdStart,
		CmdStop:    ac.CmdStop,
		CmdRestart: ac.CmdRestart,
		CmdStatus:  ac.CmdStatus,
		// AMP-specific
		AmpInstance:         ac.AmpInstance,
		AmpContainer:        ac.AmpContainer,
		AmpUser:             ac.AmpUser,
		AmpLogPath:          ac.AmpLogPath,
		AmpUseContainer:     ac.AmpUseContainer,
		AmpContainerRuntime: ac.AmpContainerRuntime,
		AmpDataRoot:         ac.AmpDataRoot,
		AmpAPIUser:          ac.AmpAPIUser,
		AmpAPIPass:          ac.AmpAPIPass,
		AmpAPIPort:          ac.AmpAPIPort,
		AmpPgBin:            ac.AmpPgBin,
		AmpPgLib:            ac.AmpPgLib,
		AmpBackupDir:        ac.AmpBackupDir,

		AmpContainerStopTimeout: ac.AmpContainerStopTimeout,
		AmpUpdateAutoRestart:    ac.AmpUpdateAutoRestart,
		// Director proxy
		DirectorURL: ac.DirectorURL,
		// Migrate the legacy global market-bot toggle onto the default server so
		// existing installs keep their enabled/disabled choice.
		MarketBotEnabled: ac.MarketBotEnabled,
	}
}

// serverCfgToAppConfig converts a ServerConfig back into an appConfig for
// functions that still take appConfig (newControlPlane, applyAutoDiscovery, etc.).
// It starts from loadedConfig so global-only fields (auth_*, market bot, Discord
// bot) are preserved, then overrides every per-server field.
func serverCfgToAppConfig(sc ServerConfig) appConfig {
	ac := loadedConfig
	ac.SSHHost = sc.SSHHost
	ac.SSHUser = sc.SSHUser
	ac.SSHKey = sc.SSHKey
	ac.SSHMode = sc.SSHMode
	ac.SSHExtraOpts = sc.SSHExtraOpts
	ac.AutoDiscover = sc.AutoDiscover
	ac.DBHost = sc.DBHost
	ac.DBPort = sc.DBPort
	ac.DBUser = sc.DBUser
	ac.DBPass = sc.DBPass
	ac.DBName = sc.DBName
	ac.DBSchema = sc.DBSchema
	ac.Control = sc.Control
	ac.ControlNamespace = sc.ControlNamespace
	ac.DockerGameserver = sc.DockerGameserver
	ac.DockerBrokerGame = sc.DockerBrokerGame
	ac.DockerBrokerAdmin = sc.DockerBrokerAdmin
	ac.DockerDB = sc.DockerDB
	ac.CmdStart = sc.CmdStart
	ac.CmdStop = sc.CmdStop
	ac.CmdRestart = sc.CmdRestart
	ac.CmdStatus = sc.CmdStatus
	ac.BrokerGameAddr = sc.BrokerGameAddr
	ac.BrokerAdminAddr = sc.BrokerAdminAddr
	ac.BrokerTLS = sc.BrokerTLS
	ac.BrokerUser = sc.BrokerUser
	ac.BrokerPass = sc.BrokerPass
	ac.BrokerJWTSecret = sc.BrokerJWTSecret
	ac.BrokerExecPrefix = sc.BrokerExecPrefix
	ac.BackupDir = sc.BackupDir
	ac.ServerIniDir = sc.ServerIniDir
	ac.DefaultIniDir = sc.DefaultIniDir
	ac.AmpInstance = sc.AmpInstance
	ac.AmpContainer = sc.AmpContainer
	ac.AmpUser = sc.AmpUser
	ac.AmpLogPath = sc.AmpLogPath
	ac.AmpUseContainer = sc.AmpUseContainer
	ac.AmpContainerRuntime = sc.AmpContainerRuntime
	ac.AmpDataRoot = sc.AmpDataRoot
	ac.AmpAPIUser = sc.AmpAPIUser
	ac.AmpAPIPass = sc.AmpAPIPass
	ac.AmpAPIPort = sc.AmpAPIPort
	ac.AmpPgBin = sc.AmpPgBin
	ac.AmpPgLib = sc.AmpPgLib
	ac.AmpBackupDir = sc.AmpBackupDir
	ac.AmpContainerStopTimeout = sc.AmpContainerStopTimeout
	ac.AmpUpdateAutoRestart = sc.AmpUpdateAutoRestart
	ac.DirectorURL = sc.DirectorURL
	ac.MarketBotEnabled = sc.MarketBotEnabled
	ac.WebInterfaceHostOverride = sc.WebInterfaceHostOverride
	ac.Timezone = sc.Timezone
	return ac
}

// connectDBDirectWithExecutor is the per-server analogue of connectDBDirect. It
// opens a pgxpool using an explicit executor for Dial instead of globalExecutor,
// so per-server connections don't share the global dial path.
func connectDBDirectWithExecutor(ctx context.Context, exec Executor, cfg ServerConfig) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		resolveDBHost(cfg.DBHost), resolveDBPort(cfg.DBPort), cfg.DBUser, cfg.DBPass, cfg.DBName)
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{cfg.DBSchema}.Sanitize()))
		return err
	}
	if exec != nil {
		addr := fmt.Sprintf("%s:%d", resolveDBHost(cfg.DBHost), resolveDBPort(cfg.DBPort))
		poolCfg.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return exec.Dial("tcp", addr)
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
	return pool, nil
}

// connectDBViaSSH is the per-server analogue of connectDB (the kubectl/SSH path).
// It routes DB traffic through an explicit executor and pod IP instead of
// globalExecutor / globalPodIP so per-server pools don't cross-wire.
func connectDBViaSSH(ctx context.Context, exec Executor, podIP string, cfg ServerConfig) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf(
		"host=127.0.0.1 port=%d user=%s password=%s dbname=%s sslmode=disable",
		resolveDBPort(cfg.DBPort), cfg.DBUser, cfg.DBPass, cfg.DBName)
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{cfg.DBSchema}.Sanitize()))
		return err
	}
	poolCfg.ConnConfig.LookupFunc = func(_ context.Context, _ string) ([]string, error) {
		return []string{podIP}, nil
	}
	if exec == nil {
		return nil, fmt.Errorf("cannot connect to DB: executor is nil")
	}
	poolCfg.ConnConfig.DialFunc = func(_ context.Context, _, _ string) (net.Conn, error) {
		return exec.Dial("tcp", fmt.Sprintf("%s:%d", podIP, resolveDBPort(cfg.DBPort)))
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// connectServer is the per-server analogue of connectAll. It creates an executor
// and control plane from cfg, then attempts the DB connection. The returned
// *ServerContext is never nil: the control plane is established even when the DB
// connect fails, so callers retain control-plane access while the DB is down.
func connectServer(cfg ServerConfig) (*ServerContext, error) {
	// Gap-fill blanks with control-plane defaults so a minimally-configured
	// server (e.g. AMP with only an SSH key) connects like the console wizard.
	applyServerConfigDefaults(&cfg)
	scope := serverScope(cfg.ID)
	sc := &ServerContext{
		ID:         scope,
		Name:       cfg.Name,
		Cfg:        cfg,
		StoreScope: storeScopeForID(cfg.ID),
	}

	ctrl := cfg.Control
	if ctrl == "" {
		ctrl = "local"
	}

	exec, err := newExecutor(cfg.SSHHost, cfg.SSHUser, cfg.SSHKey, cfg.SSHMode, cfg.SSHExtraOpts)
	if err != nil {
		return sc, fmt.Errorf("executor: %w", err)
	}
	if ctrl == "amp" {
		user := cfg.AmpUser
		if user == "" {
			user = "amp"
		}
		exec = &ampExecutor{Executor: exec, ampUser: user}
	}
	sc.Executor = exec

	if ctrl == "kubectl" {
		ns, pod, podIP, discErr := discoverDBPod(exec)
		if discErr != nil {
			exec.Close()
			sc.Executor = nil
			return sc, fmt.Errorf("DB pod discovery: %w", discErr)
		}
		sc.PodNS = ns
		sc.Pod = pod
		sc.PodIP = podIP
		if s, ok := exec.(*sshExecutor); ok {
			sc.SSH = s.client
		}
	}

	sc.Control = newControlPlane(ctrl, serverCfgToAppConfig(cfg))

	var pool *pgxpool.Pool
	if ctrl == "kubectl" {
		pool, err = connectDBViaSSH(context.Background(), exec, sc.PodIP, cfg)
	} else {
		pool, err = connectDBDirectWithExecutor(context.Background(), exec, cfg)
	}
	if err != nil {
		return sc, fmt.Errorf("DB connect: %w", err)
	}
	sc.DB = pool
	return sc, nil
}

// connectMultiServer connects all servers listed in cfg.Servers, registers them
// in globalRegistry, and aliases the active server's pool/control/executor to
// the process-wide globals for backward compatibility.
func connectMultiServer(cfg appConfig) error {
	var firstErr error
	for _, sc := range cfg.Servers {
		ctx, err := connectServer(sc)
		if err != nil {
			componentLog("connection").Error().Err(err).
				Int("server_id", sc.ID).
				Str("control_plane", controlOrDefault(sc.Control)).
				Msg("connect server failed")
			if firstErr == nil {
				firstErr = err
			}
		}
		globalRegistry.Register(ctx)
		if ctx.DB != nil {
			ensureDBSchema(ctx.DB)
		}
	}

	// Activate the configured default, or fall through to the first server.
	activeID := cfg.DefaultServer
	if activeID == "" && len(cfg.Servers) > 0 {
		activeID = serverScope(cfg.Servers[0].ID)
	}
	if activeID != "" {
		if err := globalRegistry.SetActive(activeID); err != nil {
			componentLog("connection").Error().Err(err).
				Str("active_id", activeID).
				Msg("set active server failed")
		}
	}

	// Alias globals from the active server so existing single-server code keeps working.
	if active := globalRegistry.Active(); active != nil {
		globalDB = active.DB
		globalControl = active.Control
		globalExecutor = active.Executor
	}
	return firstErr
}
