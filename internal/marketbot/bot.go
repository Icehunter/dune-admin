package marketbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// BotConfig holds all settings needed to start the embedded market bot.
// Fields correspond 1:1 to the market_bot_* keys in config.yaml.
type BotConfig struct {
	DBHost       string
	DBPort       int
	DBUser       string
	DBPass       string
	DBName       string
	DBSchema     string
	DBPool       *pgxpool.Pool // optional shared dune-admin pool (preserves SSH/tunnel routing)
	CacheDB      string
	StatePath    string // optional path for persisted runtime config (UI-tunable fields)
	ItemDataPath string
	BuyInterval  time.Duration
	ListInterval time.Duration
	BuyThreshold float64
	MaxBuys      int
	APIAddr      string // empty = disable HTTP sub-API
	APIToken     string
	// Logger is the structured logger the host attaches per server (carrying
	// component/server_id/control_plane fields). Run wires this logger to ALSO
	// fan out to the LogSink (live WebSocket view). The zero value is fine — Run
	// builds a fallback when no level/output is configured.
	Logger zerolog.Logger
}

// Instance holds live handles to the running bot so the host process can
// call lifecycle methods and stream logs without HTTP round-trips.
type Instance struct {
	API     *APIServer
	Sink    *LogSink
	cfg     *Config
	catalog []CatalogItem
	ex      *Exchange
	pool    *pgxpool.Pool
	started time.Time
}

// Pause disables the tick loop without terminating the process.
func (i *Instance) Pause() {
	_ = i.cfg.Apply(map[string]json.RawMessage{"enabled": json.RawMessage("false")})
}

// Resume re-enables the tick loop.
func (i *Instance) Resume() {
	_ = i.cfg.Apply(map[string]json.RawMessage{"enabled": json.RawMessage("true")})
}

// Restart re-initialises the exchange (reloads catalog, re-pings DB) then
// re-enables the tick loop.
func (i *Instance) Restart(ctx context.Context) error {
	i.Pause()
	if err := i.ex.Init(ctx, i.catalog); err != nil {
		return fmt.Errorf("marketbot restart: %w", err)
	}
	i.Resume()
	return nil
}

// ConfigJSON returns the current runtime config encoded with duration strings.
func (i *Instance) ConfigJSON() ([]byte, error) {
	return i.cfg.MarshalJSON()
}

// ApplyConfig applies a partial runtime config patch.
func (i *Instance) ApplyConfig(patch map[string]json.RawMessage) error {
	return i.cfg.Apply(patch)
}

// Enabled reports whether the bot tick loop is currently enabled.
func (i *Instance) Enabled() bool {
	return i.cfg.Snapshot().Enabled
}

// CleanupListings deletes every active bot-owned listing. The tick loop is
// paused for the duration and resumed only if it was running before. The next
// list tick will rebuild listings from the catalog.
func (i *Instance) CleanupListings(ctx context.Context) (orders int64, items int64, err error) {
	wasEnabled := i.Enabled()
	if wasEnabled {
		i.Pause()
	}
	orders, items, err = i.ex.CleanupListings(ctx)
	if wasEnabled {
		i.Resume()
	}
	return orders, items, err
}

// Run starts the market bot. It blocks until ctx is cancelled.
// The returned *Instance is valid as soon as Run returns a non-nil value
// in the first return position; callers should check err for startup errors.
//
//	inst, err := marketbot.Start(ctx, cfg)  // non-blocking wrapper below
func Run(ctx context.Context, cfg BotConfig) (*Instance, error) {
	sink := NewLogSink()
	logger := botLogger(cfg.Logger, sink)
	started := time.Now()

	if cfg.BuyInterval == 0 {
		cfg.BuyInterval = 5 * time.Minute
	}
	if cfg.ListInterval == 0 {
		cfg.ListInterval = 30 * time.Minute
	}
	if cfg.BuyThreshold == 0 {
		cfg.BuyThreshold = 1.05
	}
	if cfg.MaxBuys == 0 {
		cfg.MaxBuys = 50
	}
	if cfg.CacheDB == "" {
		cfg.CacheDB = "/data/market-bot-cache.db"
	}
	var (
		pool   *pgxpool.Pool
		ownsDB bool
		err    error
		schema = cfg.DBSchema
	)
	if schema == "" {
		schema = "dune"
	}
	if cfg.DBPool != nil {
		pool = cfg.DBPool
		logger.Info().Msg("using shared dune-admin database pool")
	} else {
		if cfg.DBHost == "" {
			return nil, fmt.Errorf("marketbot: DBHost is required")
		}
		if cfg.DBPort == 0 {
			cfg.DBPort = 15432
		}
		connStr := fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName,
		)
		poolConfig, cfgErr := pgxpool.ParseConfig(connStr)
		if cfgErr != nil {
			return nil, fmt.Errorf("marketbot: db config: %w", cfgErr)
		}
		poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, "SET search_path TO "+schema+", public")
			return err
		}
		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			return nil, fmt.Errorf("marketbot: db connect: %w", err)
		}
		ownsDB = true
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("marketbot: db ping: %w", err)
		}
		logger.Info().Str("db_host", cfg.DBHost).Int("db_port", cfg.DBPort).
			Str("db_name", cfg.DBName).Msg("connected to database")
	}

	botCfg := &Config{config: defaultConfig()}
	// Seed from BotConfig (CLI/yaml). These act as initial defaults; if a
	// persisted state file exists, its values win below.
	botCfg.config.BuyInterval = cfg.BuyInterval
	botCfg.config.ListInterval = cfg.ListInterval
	botCfg.config.BuyThreshold = cfg.BuyThreshold
	botCfg.config.MaxBuys = cfg.MaxBuys

	// Restore UI-tunable runtime config from disk if present.
	if cfg.StatePath != "" {
		if state, err := LoadState(cfg.StatePath); err != nil {
			logger.Warn().Str("state_path", cfg.StatePath).Err(err).Msg("load persisted state failed; using defaults")
		} else if state.MaxBuys > 0 || len(state.DisabledItems) > 0 || state.BuyThreshold > 0 {
			// Non-zero state indicates the file existed and had real content.
			botCfg.config = state
			logger.Info().Str("state_path", cfg.StatePath).Int("disabled_items", len(state.DisabledItems)).Msg("loaded persisted runtime state")
		}
		// Register persistence hook so every successful Apply writes through.
		statePath := cfg.StatePath
		botCfg.OnChange(func(v configValues) {
			if err := SaveState(statePath, v); err != nil {
				logger.Warn().Str("state_path", statePath).Err(err).Msg("persist state failed")
			}
		})
	}

	logger.Info().Msg("loading catalog")
	catalog, err := loadCatalog(cfg.ItemDataPath)
	if err != nil {
		if ownsDB {
			pool.Close()
		}
		return nil, fmt.Errorf("marketbot: load catalog: %w", err)
	}
	logger.Info().Int("listable_items", len(catalog)).Msg("catalog loaded")

	ex, err := NewExchange(pool, cfg.CacheDB, catalog, botCfg, logger)
	if err != nil {
		if ownsDB {
			pool.Close()
		}
		return nil, fmt.Errorf("marketbot: init exchange: %w", err)
	}

	logger.Info().Msg("initializing exchange")
	if err := ex.Init(ctx, catalog); err != nil {
		if ownsDB {
			pool.Close()
		}
		return nil, fmt.Errorf("marketbot: init: %w", err)
	}
	logger.Info().Msg("exchange ready")

	var api *APIServer
	if cfg.APIAddr != "" {
		api = newAPIServer(botCfg, ex, nil, cfg.APIToken)
		// inst is wired in below after construction.
	}

	inst := &Instance{
		API:     api,
		Sink:    sink,
		cfg:     botCfg,
		catalog: catalog,
		ex:      ex,
		pool:    pool,
		started: started,
	}

	if api != nil {
		api.inst = inst
		go api.ListenAndServe(cfg.APIAddr)
	}

	go func() {
		if ownsDB {
			defer pool.Close()
		}
		runLoop(ctx, botCfg, ex, catalog)
		logger.Info().Msg("shutting down")
	}()

	return inst, nil
}

// Start is a non-blocking convenience wrapper around Run.
// The *Instance is delivered on the channel once startup completes (or nil on error).
func Start(ctx context.Context, cfg BotConfig) <-chan *Instance {
	ch := make(chan *Instance, 1)
	go func() {
		inst, err := Run(ctx, cfg)
		if err != nil {
			l := cfg.Logger
			if l.GetLevel() == zerolog.Disabled {
				l = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).
					With().Timestamp().Str("component", "market_bot").Logger()
			}
			l.Error().Err(err).Msg("startup failed")
			ch <- nil
			return
		}
		ch <- inst
	}()
	return ch
}

// botLogger builds the logger the bot uses internally. It preserves the host
// logger's context fields (component/server_id/control_plane) while fanning
// every line out to BOTH stderr (in the process log format) and the LogSink
// (live WebSocket view, always human-readable console format so the UI stays
// legible regardless of LOG_FORMAT). A zero-value host logger (tests,
// standalone) yields a fresh sink+stderr console logger.
func botLogger(host zerolog.Logger, sink *LogSink) zerolog.Logger {
	sinkConsole := zerolog.ConsoleWriter{Out: trimWriter{sink}, TimeFormat: "15:04:05", NoColor: true}
	if host.GetLevel() == zerolog.Disabled {
		return sink.Logger(os.Stderr)
	}
	mw := zerolog.MultiLevelWriter(stderrWriter(), sinkConsole)
	return host.Output(mw)
}

// stderrWriter mirrors initLogging's destination choice: JSON to stderr when
// LOG_FORMAT=json, otherwise a human-readable console writer.
func stderrWriter() io.Writer {
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "json") {
		return os.Stderr
	}
	return zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}
}

func runLoop(ctx context.Context, cfg *Config, ex *Exchange, catalog []CatalogItem) {
	ex.Tick(ctx, catalog)

	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	snap0 := cfg.Snapshot()
	nextBuy := time.Now().Add(snap0.BuyInterval)
	nextList := time.Now().Add(snap0.ListInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			snap := cfg.Snapshot()
			if !snap.Enabled {
				continue
			}
			if now.After(nextBuy) {
				ex.BuyTick(ctx)
				nextBuy = now.Add(snap.BuyInterval)
			}
			if now.After(nextList) {
				ex.ListTick(ctx, catalog)
				nextList = now.Add(snap.ListInterval)
			}
		}
	}
}

// statusSnapshot is exported so dune-admin handlers can call it directly.
func (i *Instance) StatusSnapshot() any {
	return i.ex.statusSnapshot(i.started)
}

// ensure LogSink satisfies io.Writer (compile-time check)
var _ io.Writer = (*LogSink)(nil)
