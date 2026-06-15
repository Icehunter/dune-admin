package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"dune-admin/internal/marketbot"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"
)

// ServerConfig holds the per-server subset of appConfig: every field that
// varies between game servers. Global fields (listen_addr, auth_*, market bot,
// Discord bot, etc.) remain in appConfig and are not duplicated here.
type ServerConfig struct {
	ID   string `yaml:"id"   json:"id"`
	Name string `yaml:"name" json:"name"`

	// Transport — SSH fields.
	SSHHost      string `yaml:"ssh_host"       json:"ssh_host"`
	SSHUser      string `yaml:"ssh_user"       json:"ssh_user"`
	SSHKey       string `yaml:"ssh_key"        json:"ssh_key"`
	SSHMode      string `yaml:"ssh_mode"       json:"ssh_mode"`
	SSHExtraOpts string `yaml:"ssh_extra_opts" json:"ssh_extra_opts"`
	AutoDiscover bool   `yaml:"auto_discover"  json:"auto_discover"`

	// Database — always required per server.
	DBHost   string `yaml:"db_host"   json:"db_host"`
	DBPort   int    `yaml:"db_port"   json:"db_port"`
	DBUser   string `yaml:"db_user"   json:"db_user"`
	DBPass   string `yaml:"db_pass"   json:"db_pass"`
	DBName   string `yaml:"db_name"   json:"db_name"`
	DBSchema string `yaml:"db_schema" json:"db_schema"`

	// Control plane.
	Control          string `yaml:"control"           json:"control"`
	ControlNamespace string `yaml:"control_namespace" json:"control_namespace"`

	// docker-specific container names.
	DockerGameserver  string `yaml:"docker_gameserver"   json:"docker_gameserver"`
	DockerBrokerGame  string `yaml:"docker_broker_game"  json:"docker_broker_game"`
	DockerBrokerAdmin string `yaml:"docker_broker_admin" json:"docker_broker_admin"`
	DockerDB          string `yaml:"docker_db"           json:"docker_db"`

	// local-specific shell commands.
	CmdStart   string `yaml:"cmd_start"   json:"cmd_start"`
	CmdStop    string `yaml:"cmd_stop"    json:"cmd_stop"`
	CmdRestart string `yaml:"cmd_restart" json:"cmd_restart"`
	CmdStatus  string `yaml:"cmd_status"  json:"cmd_status"`

	// Broker — RabbitMQ for capture / notification.
	BrokerGameAddr   string `yaml:"broker_game_addr"   json:"broker_game_addr"`
	BrokerAdminAddr  string `yaml:"broker_admin_addr"  json:"broker_admin_addr"`
	BrokerTLS        bool   `yaml:"broker_tls"         json:"broker_tls"`
	BrokerUser       string `yaml:"broker_user"        json:"broker_user"`
	BrokerPass       string `yaml:"broker_pass"        json:"broker_pass"`
	BrokerJWTSecret  string `yaml:"broker_jwt_secret"  json:"broker_jwt_secret"`
	BrokerExecPrefix string `yaml:"broker_exec_prefix" json:"broker_exec_prefix"`

	// Paths.
	BackupDir     string `yaml:"backup_dir"     json:"backup_dir"`
	ServerIniDir  string `yaml:"server_ini_dir" json:"server_ini_dir"`
	DefaultIniDir string `yaml:"default_ini_dir" json:"default_ini_dir"`

	// AMP-specific.
	AmpInstance         string `yaml:"amp_instance"          json:"amp_instance"`
	AmpContainer        string `yaml:"amp_container"         json:"amp_container"`
	AmpUser             string `yaml:"amp_user"              json:"amp_user"`
	AmpLogPath          string `yaml:"amp_log_path"          json:"amp_log_path"`
	AmpUseContainer     *bool  `yaml:"amp_use_container"     json:"amp_use_container"`
	AmpContainerRuntime string `yaml:"amp_container_runtime" json:"amp_container_runtime"`
	AmpDataRoot         string `yaml:"amp_data_root"         json:"amp_data_root"`
	AmpAPIUser          string `yaml:"amp_api_user"          json:"amp_api_user"`
	AmpAPIPass          string `yaml:"amp_api_pass"          json:"amp_api_pass"`
	AmpAPIPort          int    `yaml:"amp_api_port"          json:"amp_api_port"`
	AmpPgBin            string `yaml:"amp_pg_bin"            json:"amp_pg_bin"`
	AmpPgLib            string `yaml:"amp_pg_lib"            json:"amp_pg_lib"`
	AmpBackupDir        string `yaml:"amp_backup_dir"        json:"amp_backup_dir"`

	// Director proxy.
	DirectorURL string `yaml:"director_url" json:"director_url"`

	// Market bot enable toggle — PER SERVER. The rest of the market-bot config
	// (intervals, thresholds, cache base, item data) is global/shared in
	// appConfig. nil means "not set" → OFF (explicit opt-in per server).
	MarketBotEnabled *bool `yaml:"market_bot_enabled" json:"market_bot_enabled"`
}

// ServerContext is the fully-connected runtime for one game server. It is the
// per-server analogue of the process-wide singletons (globalDB, globalControl,
// globalExecutor). During the shim period those globals alias reg.Active().
type ServerContext struct {
	ID         string
	Name       string
	Cfg        ServerConfig
	DB         *pgxpool.Pool // nil if DB connect failed; control plane still usable
	Control    ControlPlane
	Executor   Executor
	PodIP      string
	PodNS      string
	Pod        string
	SSH        *ssh.Client
	StoreScope string // == ID; scopes every SQLite query for this server

	// Per-server embedded market bot. Bot is nil unless the server's toggle is
	// on AND it has a live DB. BotConfigured records that the toggle is on even
	// when the bot isn't running (e.g. DB down) so status can report it.
	Bot           *marketbot.Instance
	BotCancel     context.CancelFunc
	BotConfigured bool
}

// serverRegistry holds all connected ServerContexts and tracks the active one.
// It is the single authoritative source of per-server state. All methods are
// safe for concurrent use.
type serverRegistry struct {
	mu      sync.RWMutex
	servers map[string]*ServerContext
	order   []string // registration order for stable All() output
	active  string   // ID of the currently active server
	store   *sql.DB  // the shared SQLite handle (today's globalStore)
}

// globalRegistry is the process-wide server registry. It is pre-initialised
// with a nil store; initUnifiedStoreOnce wires the store in before connectAll
// runs (or connectAll updates it lazily).
var globalRegistry = newServerRegistry(nil)

// newServerRegistry constructs an empty registry backed by the given SQLite
// store. store may be nil in tests or before the store is opened.
func newServerRegistry(store *sql.DB) *serverRegistry {
	return &serverRegistry{
		servers: make(map[string]*ServerContext),
		store:   store,
	}
}

// Register adds sc to the registry. If a server with the same ID already
// exists it is replaced in-place (preserving its position in order). The first
// server registered becomes the active server.
func (r *serverRegistry) Register(sc *ServerContext) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.servers[sc.ID]; !exists {
		r.order = append(r.order, sc.ID)
		if r.active == "" {
			r.active = sc.ID
		}
	}
	r.servers[sc.ID] = sc
}

// Get returns the ServerContext for id, or nil if no server with that id is
// registered.
func (r *serverRegistry) Get(id string) *ServerContext {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.servers[id]
}

// Active returns the currently active ServerContext, or nil if the registry is
// empty.
func (r *serverRegistry) Active() *ServerContext {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.servers[r.active]
}

// ActiveID returns the ID of the currently active server.
func (r *serverRegistry) ActiveID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// SetActive switches the active server to id. Returns an error if no server
// with that id is registered; in that case the active server is unchanged.
func (r *serverRegistry) SetActive(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.servers[id]; !ok {
		return fmt.Errorf("server %q not found in registry", id)
	}
	r.active = id
	return nil
}

// Remove deletes the server with the given id from the registry. If the
// removed server was the active one, active is reassigned to the next server
// in registration order (or cleared if the registry is now empty).
// Returns false when no server with that id exists.
func (r *serverRegistry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.servers[id]; !ok {
		return false
	}
	delete(r.servers, id)
	newOrder := r.order[:0]
	for _, oid := range r.order {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}
	r.order = newOrder
	if r.active == id {
		if len(r.order) > 0 {
			r.active = r.order[0]
		} else {
			r.active = ""
		}
	}
	return true
}

// All returns all registered ServerContexts in registration order. The
// returned slice is a snapshot; callers must not modify it.
func (r *serverRegistry) All() []*ServerContext {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ServerContext, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.servers[id])
	}
	return out
}
