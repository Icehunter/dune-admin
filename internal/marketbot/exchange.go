package marketbot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

const orderExpirySecs = int64(24 * 3600)

// statusSnap is returned by GET /status.
type statusSnap struct {
	Uptime       string `json:"uptime"`
	LastBuyTick  string `json:"last_buy_tick"`
	LastListTick string `json:"last_list_tick"`
	ListingCount int64  `json:"listing_count"`
	Balance      int64  `json:"balance"`
	ErrorCount   int64  `json:"error_count"`
}

func (e *Exchange) statusSnapshot(start time.Time) statusSnap {
	lastBuy := "never"
	if ns := e.lastBuyNano.Load(); ns > 0 {
		lastBuy = time.Unix(0, ns).UTC().Format(time.RFC3339)
	}
	lastList := "never"
	if ns := e.lastListNano.Load(); ns > 0 {
		lastList = time.Unix(0, ns).UTC().Format(time.RFC3339)
	}
	return statusSnap{
		Uptime:       time.Since(start).Round(time.Second).String(),
		LastBuyTick:  lastBuy,
		LastListTick: lastList,
		ListingCount: e.listingCount.Load(),
		Balance:      e.solariBalance.Load(),
		ErrorCount:   e.errCount.Load(),
	}
}

// gradeKey groups listings by template + quality grade for per-grade quota tracking.
type gradeKey struct {
	tmpl  string
	grade int64
}

// applicableGrades returns which quality levels to list an item at.
// Items whose schematic drops from overland testing stations (ecolabs) are gradeable 0–5
// (or from MinQualityLevel–5 for augments that only drop at higher grades).
// Stackables and items without an ecolab schematic get grade 0 only.
func applicableGrades(item CatalogItem) []int64 {
	if item.StackMax > 1 || !item.IsGradeable {
		return []int64{0}
	}
	min := item.MinQualityLevel
	if min < 0 || min > 5 {
		min = 0
	}
	grades := make([]int64, 0, 6-min)
	for g := min; g <= 5; g++ {
		grades = append(grades, int64(g))
	}
	return grades
}

type categoryEntry struct {
	mask  int32
	depth int16
}

type listingInfo struct {
	orderID   int64
	itemID    int64
	stackSize int64
	price     int64
	grade     int64
}

type Exchange struct {
	db            *pgxpool.Pool
	cache         *sql.DB // local SQLite category cache
	cfg           *Config
	segIdx        [4][]string
	botInvID      int64
	ownerID       int64 // actor ID of the market bot (Revy)
	exchangeID    int64
	accessPointID int64
	mapMu         sync.RWMutex // protects prices and catalogMap
	prices        map[string]int64
	marketPrices  map[string]marketPrice // real market prices from dune_exchange_get_item_price_stats
	categories    map[string]categoryEntry
	catalogMap    map[string]CatalogItem // template_id → catalog entry (for buyable check)
	gameEpochUnix int64                  // unix timestamp of the game server's time epoch; 0 = unknown
	nextPos       int64                  // position_index counter for item inserts

	// atomic counters — updated by BuyTick/ListTick, read by statusSnapshot.
	lastBuyNano   atomic.Int64 // UnixNano of last buy tick; 0 = never
	lastListNano  atomic.Int64 // UnixNano of last list tick; 0 = never
	listingCount  atomic.Int64 // current bot listing count
	errCount      atomic.Int64 // cumulative errors since process start
	solariBalance atomic.Int64 // Solari balance as of last list tick
}

// marketPrice holds real market stats from dune_exchange_get_item_price_stats.
type marketPrice struct {
	minimum int64
	average int64
}

func ensureCachePath(cachePath string) (string, error) {
	if strings.TrimSpace(cachePath) == "" {
		return "", fmt.Errorf("cache path is empty")
	}
	cleanPath := filepath.Clean(cachePath)
	dir := filepath.Dir(cleanPath)
	if dir == "." || dir == "" {
		return cleanPath, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory %s: %w", dir, err)
	}
	return cleanPath, nil
}

func NewExchange(db *pgxpool.Pool, cachePath string, catalog []CatalogItem, cfg *Config) (*Exchange, error) {
	cachePath, err := ensureCachePath(cachePath)
	if err != nil {
		return nil, fmt.Errorf("prepare category cache path: %w", err)
	}
	cache, err := sql.Open("sqlite", cachePath)
	if err != nil {
		return nil, fmt.Errorf("open category cache %s: %w", cachePath, err)
	}
	if _, err := cache.Exec(`
		CREATE TABLE IF NOT EXISTS categories (
			template_id    TEXT     PRIMARY KEY,
			category_mask  INTEGER  NOT NULL,
			category_depth INTEGER  NOT NULL
		)`); err != nil {
		_ = cache.Close()
		return nil, fmt.Errorf("init category cache: %w", err)
	}
	if _, err := cache.Exec(`
		CREATE TABLE IF NOT EXISTS metadata (
			key   TEXT    PRIMARY KEY,
			value INTEGER NOT NULL
		)`); err != nil {
		_ = cache.Close()
		return nil, fmt.Errorf("init metadata cache: %w", err)
	}

	ex := &Exchange{
		db:         db,
		cache:      cache,
		cfg:        cfg,
		segIdx:     buildSegmentIndex(catalog),
		prices:     make(map[string]int64),
		categories: make(map[string]categoryEntry),
	}
	if err := cache.QueryRow(`SELECT value FROM metadata WHERE key = 'game_epoch_unix'`).Scan(&ex.gameEpochUnix); err == nil {
		log.Printf("loaded game epoch from cache: unix %d (game time now: %d)", ex.gameEpochUnix, ex.gameNow())
	}
	return ex, nil
}

// epochSentinelCutoff is the exclusive upper bound for expiration_time values
// used in epoch detection. Sentinel listings are created with
// expiration_time = 999_999_999 when the epoch is unknown; these must be
// excluded so they cannot corrupt the epoch calculation.
const epochSentinelCutoff = int64(999_999_999)

func (e *Exchange) learnGameEpoch(ctx context.Context) {
	// Two-tier epoch detection:
	//
	// Tier 1 — bot's own non-sentinel listings (accurate: the bot always places
	// orders with expiration_time = gameNow + orderExpirySecs, so the offset is
	// exact). Sentinel listings (expiration_time = 999_999_999) are excluded via
	// epochSentinelCutoff so they cannot produce a bogus near-1e9 game time.
	//
	// Tier 2 — player listings (is_npc_order = FALSE). Used as a bootstrap
	// fallback when the only bot listings are sentinels (fresh install or cache
	// cleared). Players always have real expiration times. Using player listings
	// was historically unsafe as sole source because players may list for
	// durations other than 24 h, but as a fallback it is acceptable — the epoch
	// will self-correct once the bot places its first non-sentinel listing.
	applyLearnedEpochTwoTier(e,
		func() (int64, error) {
			var ref int64
			err := e.db.QueryRow(ctx, `
				SELECT expiration_time FROM dune.dune_exchange_orders
				WHERE owner_id = $1
				  AND is_npc_order = TRUE
				  AND expiration_time IS NOT NULL
				  AND expiration_time < $2
				ORDER BY expiration_time DESC LIMIT 1`, e.ownerID, epochSentinelCutoff).Scan(&ref)
			return ref, err
		},
		func() (int64, error) {
			var ref int64
			err := e.db.QueryRow(ctx, `
				SELECT expiration_time FROM dune.dune_exchange_orders
				WHERE is_npc_order = FALSE
				  AND expiration_time IS NOT NULL
				ORDER BY expiration_time DESC LIMIT 1`).Scan(&ref)
			return ref, err
		},
	)
}

// applyLearnedEpochTwoTier updates the Exchange epoch using a two-tier strategy:
// fetchBotRef queries the bot's own non-sentinel listings (tier 1, accurate);
// fetchPlayerRef queries player listings as a bootstrap fallback (tier 2).
// Tier 2 is only consulted when tier 1 returns no rows or an error.
func applyLearnedEpochTwoTier(e *Exchange, fetchBotRef, fetchPlayerRef func() (int64, error)) {
	applyLearnedEpoch(e, func() (int64, error) {
		ref, err := fetchBotRef()
		if (err != nil || ref == 0) && fetchPlayerRef != nil {
			return fetchPlayerRef()
		}
		return ref, err
	})
}

// applyLearnedEpoch updates the Exchange epoch from the value returned by
// fetchRef. Extracted so epoch-learning logic can be tested without a live DB.
func applyLearnedEpoch(e *Exchange, fetchRef func() (int64, error)) {
	ref, err := fetchRef()
	if err != nil || ref == 0 {
		return
	}
	gameNow := ref - orderExpirySecs
	epoch := time.Now().Unix() - gameNow
	if e.gameEpochUnix == epoch {
		return
	}
	e.gameEpochUnix = epoch
	_, _ = e.cache.Exec(`INSERT INTO metadata (key, value) VALUES ('game_epoch_unix', ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, epoch)
	log.Printf("game epoch learned: unix %d (current game time: %d)", epoch, gameNow)
}

func (e *Exchange) gameNow() int64 {
	if e.gameEpochUnix == 0 {
		return 0
	}
	return time.Now().Unix() - e.gameEpochUnix
}

// detectExchangeID resolves the exchange ID via a three-tier cascade:
//  1. An existing player order's exchange_id (most reliable).
//  2. Any row in dune_exchanges (works on fresh servers with no player trades).
//  3. An upsert via get_dune_exchange_id('Global') so a completely empty DB
//     still boots rather than crashing with "no rows in result set".
//
// Each tier is provided as a closure so the function can be unit-tested without
// a live Postgres connection.
func detectExchangeID(
	fromOrders func() (int64, error),
	fromTable func() (int64, error),
	autoCreate func() (int64, error),
) (int64, error) {
	if id, err := fromOrders(); err == nil {
		return id, nil
	}
	if id, err := fromTable(); err == nil {
		return id, nil
	}
	id, err := autoCreate()
	if err != nil {
		return 0, fmt.Errorf("detect exchange id: %w", err)
	}
	return id, nil
}

func (e *Exchange) Init(ctx context.Context, catalog []CatalogItem) error {
	id, err := detectExchangeID(
		func() (int64, error) {
			var id int64
			return id, e.db.QueryRow(ctx,
				`SELECT exchange_id FROM dune.dune_exchange_orders WHERE is_npc_order = FALSE LIMIT 1`).Scan(&id)
		},
		func() (int64, error) {
			var id int64
			return id, e.db.QueryRow(ctx,
				`SELECT id FROM dune.dune_exchanges ORDER BY id LIMIT 1`).Scan(&id)
		},
		func() (int64, error) {
			var id int64
			return id, e.db.QueryRow(ctx,
				`SELECT dune.get_dune_exchange_id('Global')`).Scan(&id)
		},
	)
	if err != nil {
		return err
	}
	e.exchangeID = id
	log.Printf("exchange id: %d", e.exchangeID)

	if err := e.db.QueryRow(ctx,
		`SELECT DISTINCT access_point_id FROM dune.dune_exchange_orders WHERE exchange_id = $1 LIMIT 1`,
		e.exchangeID).Scan(&e.accessPointID); err != nil {
		e.accessPointID = 1
		log.Printf("access point: no existing orders, defaulting to 1")
	} else {
		log.Printf("access point id: %d", e.accessPointID)
	}

	if err := e.db.QueryRow(ctx,
		`SELECT dune.get_exchange_inventory_id($1)`, e.exchangeID).Scan(&e.botInvID); err != nil {
		return fmt.Errorf("exchange inventory: %w", err)
	}
	log.Printf("exchange inventory id: %d", e.botInvID)

	if err := e.initBotUser(ctx); err != nil {
		return fmt.Errorf("bot user: %w", err)
	}

	newCatalog := make(map[string]CatalogItem, len(catalog))
	for _, item := range catalog {
		newCatalog[item.TemplateID] = item
	}

	e.mapMu.Lock()
	for _, item := range catalog {
		e.prices[item.TemplateID] = item.ListPrice
	}
	e.catalogMap = newCatalog
	e.mapMu.Unlock()

	// Start position counter after existing items.
	_ = e.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(position_index), -1) + 1 FROM dune.items WHERE inventory_id = $1`,
		e.botInvID).Scan(&e.nextPos)

	e.learnGameEpoch(ctx)
	e.refreshCategoryCache(ctx)

	if err := e.poisonCategoryHash(ctx); err != nil {
		log.Printf("warn: category hash poison: %v", err)
	}
	return nil
}

func (e *Exchange) initBotUser(ctx context.Context) error {
	err := e.db.QueryRow(ctx,
		`SELECT id FROM dune.actors WHERE class = 'Revy' LIMIT 1`).Scan(&e.ownerID)
	if err == pgx.ErrNoRows {
		// Use a valid world partition so the bot actor satisfies the actors FK.
		var partitionID int64
		partitionArg := any(nil)
		partitionErr := e.db.QueryRow(ctx,
			`SELECT partition_id FROM dune.world_partition ORDER BY partition_id LIMIT 1`).Scan(&partitionID)
		if partitionErr == nil {
			partitionArg = partitionID
		} else if partitionErr != pgx.ErrNoRows {
			return fmt.Errorf("bot actor partition: %w", partitionErr)
		}
		err = e.db.QueryRow(ctx,
			`INSERT INTO dune.actors (class, serial, gas_attributes, properties, dimension_index, partition_id)
			 VALUES ('Revy', 0, '{}', '{}', 0, $1) RETURNING id`, partitionArg).Scan(&e.ownerID)
	}
	if err != nil {
		return fmt.Errorf("bot actor: %w", err)
	}
	log.Printf("bot actor id: %d (Revy)", e.ownerID)

	var userID int64
	if err := e.db.QueryRow(ctx,
		`SELECT dune.dune_exchange_get_user_id($1)`, e.ownerID).Scan(&userID); err != nil {
		return err
	}

	// Check current balance before seeding — dune_exchange_modify_user_solari_balance
	// adds a delta, not sets an absolute. Only seed if below a reasonable floor.
	const seedFloor int64 = 1_000_000_000_000  // 1T
	const seedAmount int64 = 9_000_000_000_000 // 9T
	var currentBalance int64
	_ = e.db.QueryRow(ctx,
		`SELECT dune.dune_exchange_retrieve_solari_balance($1)`, e.ownerID).Scan(&currentBalance)
	if currentBalance < seedFloor {
		_, err = e.db.Exec(ctx,
			`SELECT dune.dune_exchange_modify_user_solari_balance($1, $2)`,
			e.ownerID, seedAmount-currentBalance) // top up to 9T
		if err != nil {
			return err
		}
		log.Printf("seeded bot balance: %d → %d", currentBalance, seedAmount)
	} else {
		log.Printf("bot balance OK: %d (floor %d)", currentBalance, seedFloor)
	}
	return nil
}

func (e *Exchange) poisonCategoryHash(ctx context.Context) error {
	_, err := e.db.Exec(ctx,
		`INSERT INTO dune.dune_exchange_categories_hash (id, hash) VALUES (1, 0)
		 ON CONFLICT (id) DO UPDATE SET hash = 0`)
	return err
}

func (e *Exchange) refreshCategoryCache(ctx context.Context) {
	rows, err := e.cache.Query(
		`SELECT template_id, category_mask, category_depth FROM categories`)
	if err != nil {
		log.Printf("warn: load category cache: %v", err)
	} else {
		for rows.Next() {
			var tmpl string
			var mask int32
			var depth int16
			if err := rows.Scan(&tmpl, &mask, &depth); err != nil {
				continue
			}
			e.categories[strings.ToLower(tmpl)] = categoryEntry{mask: mask, depth: depth}
		}
		_ = rows.Close()
	}

	liveRows, err := e.db.Query(ctx, `
		SELECT DISTINCT template_id, category_mask, category_depth
		FROM dune.dune_exchange_orders
		WHERE category_mask != 0`)
	if err != nil {
		log.Printf("warn: live category scan: %v", err)
		return
	}
	defer liveRows.Close()

	type entry struct {
		tmpl  string
		mask  int32
		depth int16
	}
	var toWrite []entry
	for liveRows.Next() {
		var tmpl string
		var mask int32
		var depth int16
		if err := liveRows.Scan(&tmpl, &mask, &depth); err != nil {
			continue
		}
		key := strings.ToLower(tmpl)
		if _, known := e.categories[key]; !known {
			e.categories[key] = categoryEntry{mask: mask, depth: depth}
			toWrite = append(toWrite, entry{tmpl, mask, depth})
		}
	}

	for _, en := range toWrite {
		if _, err := e.cache.Exec(`
			INSERT INTO categories (template_id, category_mask, category_depth)
			VALUES (?, ?, ?)
			ON CONFLICT (template_id) DO UPDATE
			  SET category_mask  = excluded.category_mask,
			      category_depth = excluded.category_depth`,
			en.tmpl, en.mask, en.depth); err != nil {
			log.Printf("warn: persist category %s: %v", en.tmpl, err)
		}
	}

	if len(toWrite) > 0 {
		log.Printf("category cache: +%d new (total %d)", len(toWrite), len(e.categories))
	}
}

func (e *Exchange) categoryFor(item CatalogItem) (mask int32, depth int16) {
	if item.IsSchematic && item.Category != "" {
		if m, d, ok := UniqueSchematicsMask(item.Category); ok {
			return m, d
		}
		// Schematic whose category has no unique-schematics section — fall through.
		return CategoryMask(item.Category, e.segIdx)
	}
	// Catalog items always use a freshly computed mask so stale cache values
	// can never poison category filters.
	if item.Category != "" {
		return CategoryMask(item.Category, e.segIdx)
	}
	if c, ok := e.categories[strings.ToLower(item.TemplateID)]; ok {
		return c.mask, c.depth
	}
	return 0, 0
}

func (e *Exchange) buyPlayerListings(ctx context.Context, orderExpiry int64, snap configValues) {
	if snap.BuyThreshold <= 0 {
		return
	}
	if orderExpiry <= 0 {
		orderExpiry = 999_999_999
	}

	// Use actual current stack_size from items so partial fills pay the right amount.
	// quality_level is needed to compare the player's grade-adjusted price against the
	// bot's grade-adjusted reference price rather than the raw base price.
	rows, err := e.db.Query(ctx, `
		SELECT o.id, o.template_id, o.item_price, o.item_id, o.owner_id,
		       COALESCE(i.stack_size, s.initial_stack_size) AS actual_stack,
		       COALESCE(o.quality_level, 0) AS quality_level
		FROM dune.dune_exchange_orders o
		JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
		LEFT JOIN dune.items i ON i.id = o.item_id
		WHERE o.is_npc_order = FALSE AND o.exchange_id = $1
		LIMIT $2`, e.exchangeID, snap.MaxBuys*10)
	if err != nil {
		log.Printf("buy: query: %v", err)
		return
	}
	defer rows.Close()

	purchased, skippedPrice, skippedUnknown, errs := 0, 0, 0, 0

	for rows.Next() {
		if purchased >= snap.MaxBuys {
			break
		}

		var orderID, price, itemID, sellerActorID, stackSize, grade int64
		var tmpl string
		if err := rows.Scan(&orderID, &tmpl, &price, &itemID, &sellerActorID, &stackSize, &grade); err != nil {
			errs++
			continue
		}

		botPrice, known := e.prices[tmpl]
		if !known || botPrice <= 0 {
			skippedUnknown++
			continue
		}

		// Skip items the operator has marked as non-buyable.
		if item, ok := e.catalogMap[tmpl]; ok && !item.Buyable {
			skippedUnknown++
			continue
		}

		// Skip items disabled via runtime config.
		if snap.isDisabled(tmpl) {
			skippedUnknown++
			continue
		}

		refPrice := gradedPrice(botPrice, grade, snap.GradeMultipliers)
		if price > int64(float64(refPrice)*snap.BuyThreshold) {
			log.Printf("buy: skip %s price=%d ref=%d(grade%d) threshold=%.2f", tmpl, price, refPrice, grade, snap.BuyThreshold)
			skippedPrice++
			continue
		}

		totalCost := price * stackSize

		tx, err := e.db.Begin(ctx)
		if err != nil {
			errs++
			continue
		}

		// Create a payment log entry for the seller (item_id omitted → NULL).
		// completion_type=4 + item_id=NULL is what the game engine uses for the
		// seller side of a fulfilled sale, causing the client to show "Take Solari"
		// in the Completed tab and fire the "X SOLARIS CLAIMED" toast on collection.
		var logOrderID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO dune.dune_exchange_orders
			  (exchange_id, access_point_id, owner_id, template_id, expiration_time,
			   durability_cur, durability_max, item_price, category_mask, category_depth, is_npc_order)
			VALUES ($1,$2,$3,$4,$5,1.0,1.0,$6,0,0,FALSE) RETURNING id`,
			e.exchangeID, e.accessPointID, sellerActorID, tmpl, orderExpiry, price,
		).Scan(&logOrderID); err != nil {
			log.Printf("buy: log order for %s: %v", tmpl, err)
			_ = tx.Rollback(ctx)
			errs++
			continue
		}

		ok := true
		for _, q := range []struct {
			sql  string
			args []any
		}{
			// Fulfilled-order record: source_order_id=NULL (original listing will be
			// deleted), original_order_id kept for telemetry linkage (no FK constraint).
			{`INSERT INTO dune.dune_exchange_fulfilled_orders
			    (order_id, source_order_id, completion_type, stack_size, original_order_id)
			    VALUES ($1, NULL, 4, $2, $3)`, []any{logOrderID, stackSize, orderID}},
			// Debit bot's exchange balance for the purchase.
			{`UPDATE dune.dune_exchange_users
			    SET solari_balance = solari_balance - $1
			    WHERE owner_id = $2`, []any{totalCost, e.ownerID}},
			// Remove the original sell listing.
			{`DELETE FROM dune.dune_exchange_sell_orders WHERE order_id = $1`, []any{orderID}},
			{`DELETE FROM dune.dune_exchange_orders WHERE id = $1`, []any{orderID}},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				log.Printf("buy: %s: %v", tmpl, err)
				ok = false
				break
			}
		}
		if ok && itemID > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM dune.items WHERE id = $1`, itemID); err != nil {
				log.Printf("buy: delete item %d: %v", itemID, err)
				ok = false
			}
		}
		if !ok {
			_ = tx.Rollback(ctx)
			errs++
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			errs++
			continue
		}
		purchased++
	}

	if errs > 0 {
		e.errCount.Add(int64(errs))
	}
	if purchased+errs+skippedPrice+skippedUnknown > 0 {
		log.Printf("buy: %d purchased, %d skipped-price, %d skipped-unknown, %d errors",
			purchased, skippedPrice, skippedUnknown, errs)
	}
}

// pendingListing holds the data needed to batch-insert a new bot listing.
type pendingListing struct {
	item      CatalogItem
	basePrice int64
	stackMax  int64
	expiry    int64
	grade     int64
}

// createListingsBatch inserts up to batchSize listings per transaction.
// Returns (created, errors).
func (e *Exchange) createListingsBatch(ctx context.Context, listings []pendingListing, snap configValues) (int, int) {
	const batchSize = 100
	created, errs := 0, 0
	for i := 0; i < len(listings); i += batchSize {
		end := i + batchSize
		if end > len(listings) {
			end = len(listings)
		}
		batch := listings[i:end]

		tx, err := e.db.Begin(ctx)
		if err != nil {
			errs += len(batch)
			continue
		}
		ok := true
		for _, pl := range batch {
			catMask, catDepth := e.categoryFor(pl.item)
			qualityLevel := pl.grade
			listPrice := gradeFloor(pl.item, pl.grade, snap)
			if pl.item.MaterialCost <= 0 {
				listPrice = gradedPrice(pl.basePrice, pl.grade, snap.GradeMultipliers)
			}
			var itemID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO dune.items (inventory_id, stack_size, position_index, template_id, quality_level, stats)
				VALUES ($1, $2, $3, $4, $5, '{}') RETURNING id`,
				e.botInvID, pl.stackMax, e.nextPos, pl.item.TemplateID, qualityLevel).Scan(&itemID); err != nil {
				log.Printf("batch insert item %s grade %d: %v", pl.item.TemplateID, pl.grade, err)
				ok = false
				errs++
				break
			}
			e.nextPos++
			var orderID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO dune.dune_exchange_orders
				  (exchange_id, access_point_id, owner_id, is_npc_order, expiration_time,
				   template_id, durability_cur, durability_max, category_mask, category_depth,
				   item_price, quality_level, item_id)
				VALUES ($1,$2,$3,TRUE,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
				e.exchangeID, e.accessPointID, e.ownerID, pl.expiry,
				pl.item.TemplateID, float32(1.0), float32(1.0),
				catMask, catDepth, listPrice, qualityLevel, itemID).Scan(&orderID); err != nil {
				log.Printf("batch insert order %s grade %d: %v", pl.item.TemplateID, pl.grade, err)
				ok = false
				errs++
				break
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO dune.dune_exchange_sell_orders (order_id, initial_stack_size, wear_normalized_price)
				VALUES ($1, $2, $3)`,
				orderID, pl.stackMax, listPrice); err != nil {
				log.Printf("batch insert sell order %s grade %d: %v", pl.item.TemplateID, pl.grade, err)
				ok = false
				errs++
				break
			}
			created++
		}
		if ok {
			_ = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
	}
	return created, errs
}

// BuyTick runs the buy-side operations: learn game epoch and purchase player listings.
func (e *Exchange) BuyTick(ctx context.Context) {
	e.learnGameEpoch(ctx)

	snap := e.cfg.Snapshot()

	gameNow := e.gameNow()
	var orderExpiry int64
	if gameNow > 0 {
		orderExpiry = gameNow + orderExpirySecs
	} else {
		orderExpiry = 999_999_999
	}

	e.buyPlayerListings(ctx, orderExpiry, snap)
	e.lastBuyNano.Store(time.Now().UnixNano())
}

// ListTick runs the listing/pruning operations: refresh caches, update prices,
// prune stale listings, top up depleted stacks, and create new listings.
func (e *Exchange) ListTick(ctx context.Context, catalog []CatalogItem) {
	snap := e.cfg.Snapshot()

	e.learnGameEpoch(ctx)
	e.refreshCategoryCache(ctx)
	e.fetchMarketPrices(ctx, catalog) // fetch real market prices via proc
	e.updatePrices(ctx, catalog, snap)
	e.expireAndPurgeOrders(ctx) // use server procs for expiration

	gameNow := e.gameNow()
	var orderExpiry int64
	if gameNow > 0 {
		orderExpiry = gameNow + orderExpirySecs
	} else {
		orderExpiry = 999_999_999
	}

	// Load only non-expired bot listings grouped by (template, grade).
	// Excluding expired rows means the bot refills those slots on this tick
	// rather than counting them toward the quota while players see them as gone.
	rows, err := e.db.Query(ctx, `
		SELECT o.id, o.template_id, o.item_id, o.item_price, i.stack_size, o.quality_level
		FROM dune.dune_exchange_orders o
		JOIN dune.items i ON i.id = o.item_id
		WHERE o.owner_id = $1 AND o.is_npc_order = TRUE
		  AND (o.expiration_time IS NULL OR o.expiration_time > $2)`,
		e.ownerID, e.gameNow())
	if err != nil {
		log.Printf("load listings: %v", err)
		return
	}
	current := make(map[gradeKey][]listingInfo)
	for rows.Next() {
		var orderID, itemID, price, stack, grade int64
		var tmpl string
		if err := rows.Scan(&orderID, &tmpl, &itemID, &price, &stack, &grade); err != nil {
			continue
		}
		k := gradeKey{tmpl, grade}
		current[k] = append(current[k], listingInfo{orderID, itemID, stack, price, grade})
	}
	rows.Close()

	// Slices accumulated across the full catalog loop, flushed in bulk at the end.
	var staleOrderIDs, staleItemIDs []int64
	type topUp struct{ itemID, stackMax int64 }
	var topUps []topUp
	var pending []pendingListing

	created, topped, pruned, errs := 0, 0, 0, 0

	for _, item := range catalog {
		stackMax := item.StackMax
		if stackMax <= 0 {
			stackMax = 1
		}
		basePrice := e.prices[item.TemplateID]
		if basePrice <= 0 {
			basePrice = item.ListPrice
		}
		if basePrice <= 0 {
			continue
		}

		for _, grade := range applicableGrades(item) {
			key := gradeKey{item.TemplateID, grade}
			listings := current[key]

			// If this item is disabled, prune all existing bot listings for it
			// and skip creating new ones.
			if snap.isDisabled(item.TemplateID) {
				for _, l := range listings {
					staleOrderIDs = append(staleOrderIDs, l.orderID)
					staleItemIDs = append(staleItemIDs, l.itemID)
					pruned++
				}
				continue
			}

			price := gradeFloor(item, grade, snap)
			if item.MaterialCost <= 0 {
				price = gradedPrice(basePrice, grade, snap.GradeMultipliers)
			}

			// Collect stale listings (wrong price) for bulk delete.
			var valid []listingInfo
			for _, l := range listings {
				if l.price != price {
					staleOrderIDs = append(staleOrderIDs, l.orderID)
					staleItemIDs = append(staleItemIDs, l.itemID)
					pruned++
				} else {
					valid = append(valid, l)
				}
			}

			// Collect depleted stacks for bulk update.
			for _, l := range valid {
				if l.stackSize < stackMax {
					topUps = append(topUps, topUp{l.itemID, stackMax})
					topped++
				}
			}

			// Accumulate listings to create to reach the configured quota per grade.
			for i := len(valid); i < snap.ListingsPerGrade; i++ {
				pending = append(pending, pendingListing{
					item:      item,
					basePrice: basePrice,
					stackMax:  stackMax,
					expiry:    orderExpiry,
					grade:     grade,
				})
			}
		}
	}

	// Bulk delete stale orders and their items.
	if len(staleOrderIDs) > 0 {
		_, _ = e.db.Exec(ctx, `DELETE FROM dune.dune_exchange_orders WHERE id = ANY($1)`, staleOrderIDs)
		_, _ = e.db.Exec(ctx, `DELETE FROM dune.items WHERE id = ANY($1)`, staleItemIDs)
	}

	// Bulk update depleted stacks.
	if len(topUps) > 0 {
		ids := make([]int64, len(topUps))
		sizes := make([]int64, len(topUps))
		for i, t := range topUps {
			ids[i] = t.itemID
			sizes[i] = t.stackMax
		}
		_, _ = e.db.Exec(ctx, `
			UPDATE dune.items SET stack_size = u.s
			FROM unnest($1::bigint[], $2::bigint[]) AS u(id, s)
			WHERE dune.items.id = u.id`, ids, sizes)
	}

	// Batch insert new listings.
	if len(pending) > 0 {
		c, e2 := e.createListingsBatch(ctx, pending, snap)
		created += c
		errs += e2
	}

	log.Printf("list-tick: %d created, %d topped up, %d pruned, %d errors", created, topped, pruned, errs)

	e.lastListNano.Store(time.Now().UnixNano())
	if errs > 0 {
		e.errCount.Add(int64(errs))
	}

	// Refresh balance and listing count for /status endpoint.
	var balance, count int64
	_ = e.db.QueryRow(ctx, `SELECT dune.dune_exchange_retrieve_solari_balance($1)`, e.ownerID).Scan(&balance)
	_ = e.db.QueryRow(ctx, `SELECT COUNT(*) FROM dune.dune_exchange_orders WHERE owner_id = $1 AND is_npc_order = TRUE`, e.ownerID).Scan(&count)
	e.solariBalance.Store(balance)
	e.listingCount.Store(count)
}

// CleanupListings deletes every active bot-owned listing (and the backing
// item rows) for Revy. Player listings, fulfilled order history, and the bot's
// Solari balance are left untouched. Returns the number of orders and items
// removed.
func (e *Exchange) CleanupListings(ctx context.Context) (orders int64, items int64, err error) {
	if e.ownerID == 0 {
		return 0, 0, fmt.Errorf("bot owner id not initialised")
	}
	tx, err := e.db.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	itemRes, err := tx.Exec(ctx, `
		DELETE FROM dune.items
		WHERE id IN (
			SELECT item_id FROM dune.dune_exchange_orders
			WHERE owner_id = $1 AND is_npc_order = TRUE AND item_id IS NOT NULL
		)`, e.ownerID)
	if err != nil {
		return 0, 0, fmt.Errorf("delete items: %w", err)
	}
	items = itemRes.RowsAffected()

	// dune_exchange_sell_orders rows cascade via FK; if not, delete them too.
	if _, err = tx.Exec(ctx, `
		DELETE FROM dune.dune_exchange_sell_orders
		WHERE order_id IN (
			SELECT id FROM dune.dune_exchange_orders
			WHERE owner_id = $1 AND is_npc_order = TRUE
		)`, e.ownerID); err != nil {
		return 0, 0, fmt.Errorf("delete sell orders: %w", err)
	}

	orderRes, err := tx.Exec(ctx, `
		DELETE FROM dune.dune_exchange_orders
		WHERE owner_id = $1 AND is_npc_order = TRUE`, e.ownerID)
	if err != nil {
		return 0, 0, fmt.Errorf("delete orders: %w", err)
	}
	orders = orderRes.RowsAffected()

	if err = tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}
	e.listingCount.Store(0)
	return orders, items, nil
}

// Tick runs both BuyTick and ListTick. Used for the initial run on startup.
func (e *Exchange) Tick(ctx context.Context, catalog []CatalogItem) {
	e.BuyTick(ctx)
	e.ListTick(ctx, catalog)
}

func (e *Exchange) updatePrices(ctx context.Context, catalog []CatalogItem, snap configValues) {
	catalogMap := make(map[string]CatalogItem, len(catalog))
	for _, item := range catalog {
		catalogMap[item.TemplateID] = item
	}

	rows, err := e.db.Query(ctx, `
		SELECT o.template_id,
		       COALESCE(SUM(f.stack_size), 0)         AS sold,
		       COALESCE(MAX(s.initial_stack_size), 0) AS listed
		FROM dune.dune_exchange_orders o
		JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
		LEFT JOIN dune.dune_exchange_fulfilled_orders f ON f.order_id = o.id
		WHERE o.owner_id = $1 AND o.is_npc_order = TRUE
		GROUP BY o.template_id`, e.ownerID)
	if err != nil {
		log.Printf("price stats: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tmpl string
		var sold, listed int64
		if err := rows.Scan(&tmpl, &sold, &listed); err != nil {
			continue
		}
		item, ok := catalogMap[tmpl]
		if !ok {
			continue
		}
		current := e.prices[tmpl]
		if current <= 0 {
			current = item.ListPrice
		}
		var frac float64
		if listed > 0 {
			frac = float64(sold) / float64(listed)
		}
		adjusted := adjustPrice(item, current, frac, snap)

		// Factor in real market prices: if players are undercutting us significantly,
		// consider lowering our price toward the market minimum.
		if mp, ok := e.marketPrices[tmpl]; ok && mp.minimum > 0 {
			// If market min is below our adjusted price by >10%, move toward it.
			if mp.minimum < int64(float64(adjusted)*0.9) {
				// Don't go below our floor, but trend toward market.
				adjusted = (adjusted + mp.minimum) / 2
			}
		}

		e.mapMu.Lock()
		e.prices[tmpl] = adjusted
		e.mapMu.Unlock()
	}
}

// fetchMarketPrices uses dune_exchange_get_item_price_stats to get real market
// prices (minimum and weighted average) across ALL active listings, not just bot's.
func (e *Exchange) fetchMarketPrices(ctx context.Context, catalog []CatalogItem) {
	if len(catalog) == 0 {
		return
	}

	// Collect all template IDs.
	templateIDs := make([]string, 0, len(catalog))
	for _, item := range catalog {
		templateIDs = append(templateIDs, item.TemplateID)
	}

	rows, err := e.db.Query(ctx,
		`SELECT * FROM dune.dune_exchange_get_item_price_stats($1)`, templateIDs)
	if err != nil {
		log.Printf("market price stats: %v", err)
		return
	}
	defer rows.Close()

	if e.marketPrices == nil {
		e.marketPrices = make(map[string]marketPrice)
	}

	count := 0
	for rows.Next() {
		var tmpl string
		var minPrice, avgPrice int64
		if err := rows.Scan(&tmpl, &minPrice, &avgPrice); err != nil {
			continue
		}
		e.marketPrices[tmpl] = marketPrice{minimum: minPrice, average: avgPrice}
		count++
	}
	log.Printf("market prices: fetched %d items from real market", count)
}

// expireBotOrders deletes bot NPC orders whose game-time expiry has passed.
// It only touches rows owned by ownerID with is_npc_order = TRUE, so player
// listings are never affected. Returns the delete error, if any.
// gameNow <= 0 means the epoch is not yet known; nothing is deleted.
func expireBotOrders(gameNow, ownerID int64, deleteFn func(ownerID, cutoff int64) error) error {
	if gameNow <= 0 {
		return nil
	}
	return deleteFn(ownerID, gameNow)
}

// expireAndPurgeOrders removes only the bot's own NPC listings that have
// passed their game-time expiry. The game server's stored procs are NOT used
// because they operate on all orders in the exchange and would expire player
// listings that the bot has no business touching.
func (e *Exchange) expireAndPurgeOrders(ctx context.Context) {
	err := expireBotOrders(e.gameNow(), e.ownerID, func(ownerID, cutoff int64) error {
		_, err := e.db.Exec(ctx, `
			DELETE FROM dune.dune_exchange_orders
			WHERE owner_id = $1
			  AND is_npc_order = TRUE
			  AND expiration_time IS NOT NULL
			  AND expiration_time < $2`,
			ownerID, cutoff)
		return err
	})
	if err != nil {
		log.Printf("expire bot orders: %v", err)
	}
}
