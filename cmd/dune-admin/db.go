package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ── journey-node fetch cache ────────────────────────────────────────────────
// Journey fetches return ~300-800 rows per character through an SSH tunnel,
// which makes them visibly slow. A short TTL cache keeps the common
// "open modal → close → reopen" loop snappy. Mutations call
// invalidateJourneyCache(accountID) to drop stale entries.

const journeyCacheTTL = 30 * time.Second

type journeyCacheEntry struct {
	nodes  []journeyNode
	cached time.Time
}

var (
	journeyCacheMu sync.RWMutex
	journeyCache   = map[int64]journeyCacheEntry{}
)

func invalidateJourneyCache(accountID int64) {
	journeyCacheMu.Lock()
	delete(journeyCache, accountID)
	journeyCacheMu.Unlock()
}

// Used by mutations keyed on player_id where we don't have the account_id handy
// (progression unlock, contract completion). A single-user admin tool, so
// dropping every entry is cheap.
func invalidateAllJourneyCache() {
	journeyCacheMu.Lock()
	journeyCache = map[int64]journeyCacheEntry{}
	journeyCacheMu.Unlock()
}

// ── data fetch commands ───────────────────────────────────────────────────────

func cmdFetchPlayers() Msg {
	if globalDB == nil {
		return msgPlayers{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT a.id,
		       COALESCE(a.owner_account_id, 0),
		       COALESCE(ps.character_name, convert_from(e.encrypted_funcom_id, 'UTF8'), ''),
		       COALESCE(ps.player_controller_id, 0),
		       COALESCE(ac."user", ''),
		       a.class,
		       COALESCE(a.map, ''),
		       COALESCE(pf.faction_id, 0),
		       COALESCE(ps.online_status::text, 'Offline')
		FROM dune.actors a
		LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
		LEFT JOIN dune.encrypted_accounts e ON e.id = a.owner_account_id
		LEFT JOIN dune.accounts ac ON ac.id = a.owner_account_id
		LEFT JOIN dune.player_faction pf ON pf.actor_id = a.id
		WHERE a.class ILIKE '%PlayerCharacter%'
		ORDER BY a.id`)
	if err != nil {
		return msgPlayers{err: err}
	}
	defer rows.Close()

	var players []playerInfo
	for rows.Next() {
		var p playerInfo
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.ControllerID, &p.FLSID, &p.Class, &p.Map, &p.FactionID, &p.OnlineStatus); err != nil {
			continue
		}
		p.Class = shortClass(p.Class)
		players = append(players, p)
	}
	if rows.Err() != nil {
		return msgPlayers{err: rows.Err()}
	}
	return msgPlayers{rows: players}
}

func cmdFetchInventory(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgInventory{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT i.id, i.template_id, i.stack_size, i.quality_level,
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'), 'N/A'),
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'), 'N/A')
			FROM dune.items i
			JOIN dune.inventories inv ON i.inventory_id = inv.id
			WHERE inv.actor_id = $1::bigint
			ORDER BY i.template_id`, playerID)
		if err != nil {
			return msgInventory{err: err}
		}
		defer rows.Close()

		var items []itemInfo
		for rows.Next() {
			var it itemInfo
			if err := rows.Scan(&it.ID, &it.TemplateID, &it.StackSize, &it.Quality, &it.Durability, &it.MaxDurability); err != nil {
				continue
			}
			it.Name = itemData.Names[strings.ToLower(it.TemplateID)]
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			return msgInventory{err: err}
		}
		return msgInventory{rows: items}
	}
}

func cmdFetchCurrency() Msg {
	if globalDB == nil {
		return msgCurrency{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT player_controller_id, currency_id, balance
		FROM dune.player_virtual_currency_balances
		ORDER BY player_controller_id, currency_id`)
	if err != nil {
		return msgCurrency{err: err}
	}
	defer rows.Close()

	var out []currencyRow
	for rows.Next() {
		var r currencyRow
		if err := rows.Scan(&r.PlayerID, &r.CurrencyID, &r.Balance); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgCurrency{err: err}
	}
	return msgCurrency{rows: out}
}

func cmdFetchFactions() Msg {
	if globalDB == nil {
		return msgFactions{err: fmt.Errorf("not connected")}
	}
	ctx := context.Background()
	scripID, err := resolveScripCurrencyID(ctx)
	if err != nil {
		return msgFactions{err: err}
	}
	rows, err := globalDB.Query(ctx, `
		SELECT pfr.actor_id, pfr.faction_id, f.name, pfr.reputation_amount,
		       COALESCE(vcb.balance, 0)
		FROM dune.player_faction_reputation pfr
		JOIN dune.factions f ON f.id = pfr.faction_id
		LEFT JOIN dune.player_virtual_currency_balances vcb
			ON vcb.player_controller_id = pfr.actor_id
			AND vcb.currency_id = $1::smallint
		ORDER BY pfr.actor_id, pfr.faction_id`, scripID)
	if err != nil {
		return msgFactions{err: err}
	}
	defer rows.Close()

	var out []factionRep
	for rows.Next() {
		var r factionRep
		if err := rows.Scan(&r.ActorID, &r.FactionID, &r.FactionName, &r.Reputation, &r.Scrips); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgFactions{err: err}
	}
	return msgFactions{rows: out, scripCurrencyID: scripID}
}

func cmdFetchSpecs() Msg {
	if globalDB == nil {
		return msgSpecs{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT player_id, track_type::text, xp_amount, level
		FROM dune.specialization_tracks
		ORDER BY player_id, track_type`)
	if err != nil {
		return msgSpecs{err: err}
	}
	defer rows.Close()

	var out []specTrack
	for rows.Next() {
		var r specTrack
		if err := rows.Scan(&r.PlayerID, &r.TrackType, &r.XP, &r.Level); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgSpecs{err: err}
	}
	return msgSpecs{rows: out}
}

func cmdRunSQL(sql string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgSQL{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), sql)
		if err != nil {
			return msgSQL{err: err}
		}
		defer rows.Close()

		var sb strings.Builder
		descs := rows.FieldDescriptions()
		headers := make([]string, len(descs))
		for i, d := range descs {
			headers[i] = string(d.Name)
		}
		sb.WriteString(strings.Join(headers, " │ "))
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat("─", 80))
		sb.WriteString("\n")

		count := 0
		for rows.Next() && count < 200 {
			vals, err := rows.Values()
			if err != nil {
				continue
			}
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = fmt.Sprintf("%v", v)
			}
			sb.WriteString(strings.Join(parts, " │ "))
			sb.WriteString("\n")
			count++
		}
		if count == 200 {
			sb.WriteString("… (limited to 200 rows)\n")
		}
		return msgSQL{result: sb.String()}
	}
}

func cmdGiveItem(playerID int64, template string, qty, quality int64) Cmd {
	return func() Msg {
		return runGiveItem(playerID, template, qty, quality)
	}
}

func runGiveItem(playerID int64, template string, qty, quality int64) Msg {
	if globalDB == nil {
		return msgMutate{err: fmt.Errorf("not connected")}
	}
	trimmedTemplate, err := validateGiveItemInput(playerID, template, qty)
	if err != nil {
		return msgMutate{err: err}
	}
	template = trimmedTemplate

	ctx := context.Background()
	inv, err := findGiveItemInventory(ctx, playerID)
	if err != nil {
		return msgMutate{err: err}
	}
	state, err := loadGiveItemInventoryState(ctx, inv.id, template, quality, inv.hasVolumeCap)
	if err != nil {
		return msgMutate{err: err}
	}
	stackMax, err := resolveStackMax(ctx, template, quality)
	if err != nil {
		return msgMutate{err: err}
	}
	if stackMax < 1 {
		stackMax = 1
	}
	if err := ensureGiveItemVolumeCapacity(ctx, inv, state, template, qty); err != nil {
		return msgMutate{err: err}
	}

	updates, newStacks := planGiveItemStacks(qty, stackMax, state.stacks)
	if err := ensureGiveItemSlotCapacity(inv, state, len(newStacks)); err != nil {
		return msgMutate{err: err}
	}
	if err := applyGiveItemChanges(ctx, inv.id, template, quality, state.maxPos, updates, newStacks); err != nil {
		return msgMutate{err: err}
	}
	return msgMutate{ok: formatGiveItemResult(playerID, template, qty, len(updates), len(newStacks))}
}

type giveItemInventory struct {
	id           int64
	maxSlots     int
	maxVolume    float64
	hasSlotCap   bool
	hasVolumeCap bool
}

type giveItemStackSlot struct {
	id   int64
	size int64
}

type giveItemInventoryState struct {
	stacks     []giveItemStackSlot
	usedSlots  int
	usedVolume float64
	maxPos     int64
}

type giveItemStackUpdate struct {
	id  int64
	add int64
}

func validateGiveItemInput(playerID int64, template string, qty int64) (string, error) {
	if playerID == 0 {
		return "", fmt.Errorf("player ID required")
	}
	template = strings.TrimSpace(template)
	if template == "" {
		return "", fmt.Errorf("item template required")
	}
	if qty <= 0 {
		return "", fmt.Errorf("quantity must be > 0")
	}
	return template, nil
}

func findGiveItemInventory(ctx context.Context, playerID int64) (giveItemInventory, error) {
	var inv giveItemInventory
	err := globalDB.QueryRow(ctx, `
		SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
		FROM dune.inventories
		WHERE actor_id = $1::bigint AND inventory_type = 0
		LIMIT 1`, playerID).Scan(&inv.id, &inv.maxSlots, &inv.maxVolume)
	if err == nil {
		inv.hasSlotCap = inv.maxSlots > 0
		inv.hasVolumeCap = inv.maxVolume > 0
		return inv, nil
	}
	err = globalDB.QueryRow(ctx, `
		SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
		FROM dune.inventories
		WHERE actor_id = $1::bigint
		LIMIT 1`, playerID).Scan(&inv.id, &inv.maxSlots, &inv.maxVolume)
	if err != nil {
		return giveItemInventory{}, fmt.Errorf("find inventory: %w", err)
	}
	inv.hasSlotCap = inv.maxSlots > 0
	inv.hasVolumeCap = inv.maxVolume > 0
	return inv, nil
}

func loadGiveItemInventoryState(ctx context.Context, invID int64, template string, quality int64, includeVolume bool) (giveItemInventoryState, error) {
	rows, err := globalDB.Query(ctx, `
		SELECT id, template_id, stack_size, quality_level, volume_override, position_index
		FROM dune.items
		WHERE inventory_id = $1::bigint`, invID)
	if err != nil {
		return giveItemInventoryState{}, err
	}
	defer rows.Close()

	state := giveItemInventoryState{maxPos: -1}
	for rows.Next() {
		var id int64
		var tmpl string
		var stackSize int64
		var qLevel int64
		var vol pgtype.Float8
		var pos int64
		if err := rows.Scan(&id, &tmpl, &stackSize, &qLevel, &vol, &pos); err != nil {
			continue
		}
		state.usedSlots++
		if pos > state.maxPos {
			state.maxPos = pos
		}
		if qLevel == quality && tmpl == template {
			state.stacks = append(state.stacks, giveItemStackSlot{id: id, size: stackSize})
		}
		if includeVolume {
			state.usedVolume += inventoryItemVolume(tmpl, vol) * float64(stackSize)
		}
	}
	if err := rows.Err(); err != nil {
		return giveItemInventoryState{}, err
	}
	return state, nil
}

func inventoryItemVolume(template string, vol pgtype.Float8) float64 {
	if vol.Valid && vol.Float64 > 0 {
		return vol.Float64
	}
	if itemData.Items != nil {
		if rule, ok := itemData.Items[strings.ToLower(template)]; ok {
			return rule.Volume // 0 is valid — item takes no volume
		}
		if itemData.DefaultVolume > 0 {
			return itemData.DefaultVolume
		}
		// Unknown volume: treat as 0 (no space consumed).
		return 0
	}
	if itemData.DefaultVolume > 0 {
		return itemData.DefaultVolume
	}
	return 0
}

func ensureGiveItemVolumeCapacity(
	ctx context.Context,
	inv giveItemInventory,
	state giveItemInventoryState,
	template string,
	qty int64,
) error {
	if !inv.hasVolumeCap {
		return nil
	}
	perItemVol, err := resolveItemVolume(ctx, template)
	if err != nil {
		return err
	}
	if perItemVol <= 0 {
		return nil
	}
	availableVol := inv.maxVolume - state.usedVolume
	if availableVol < 0 {
		availableVol = 0
	}
	maxByVolume := int64(math.Floor(availableVol / perItemVol))
	if maxByVolume < qty {
		return fmt.Errorf(
			"over weight limit: room for %d more %s (%.2f/%.2f volume used)",
			maxByVolume, template, state.usedVolume, inv.maxVolume)
	}
	return nil
}

func planGiveItemStacks(qty, stackMax int64, stacks []giveItemStackSlot) ([]giveItemStackUpdate, []int64) {
	sorted := make([]giveItemStackSlot, len(stacks))
	copy(sorted, stacks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].size > sorted[j].size
	})
	remaining := qty
	updates := make([]giveItemStackUpdate, 0, len(sorted))
	if stackMax > 1 {
		for _, st := range sorted {
			if remaining == 0 {
				break
			}
			space := stackMax - st.size
			if space <= 0 {
				continue
			}
			add := space
			if add > remaining {
				add = remaining
			}
			updates = append(updates, giveItemStackUpdate{id: st.id, add: add})
			remaining -= add
		}
	}
	newStackCap := 0
	if stackMax > 0 {
		newStackCap = int((remaining + stackMax - 1) / stackMax)
	}
	newStacks := make([]int64, 0, newStackCap)
	for remaining > 0 {
		size := stackMax
		if size > remaining {
			size = remaining
		}
		newStacks = append(newStacks, size)
		remaining -= size
	}
	return updates, newStacks
}

func ensureGiveItemSlotCapacity(inv giveItemInventory, state giveItemInventoryState, newStackCount int) error {
	if !inv.hasSlotCap {
		return nil
	}
	freeSlots := inv.maxSlots - state.usedSlots
	if freeSlots < newStackCount {
		return fmt.Errorf("inventory full: need %d free slots, have %d", newStackCount, freeSlots)
	}
	return nil
}

func applyGiveItemChanges(
	ctx context.Context,
	invID int64,
	template string,
	quality int64,
	maxPos int64,
	updates []giveItemStackUpdate,
	newStacks []int64,
) error {
	tx, err := globalDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, u := range updates {
		if _, err := tx.Exec(ctx, `
			UPDATE dune.items
			SET stack_size = stack_size + $1::bigint
			WHERE id = $2::bigint`, u.add, u.id); err != nil {
			return err
		}
	}

	nextPos := maxPos + 1
	for _, size := range newStacks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO dune.items (inventory_id, stack_size, position_index, template_id, quality_level, stats)
			VALUES ($1::bigint, $2::bigint, $3::bigint, $4::text, $5::bigint, '{}'::jsonb)`,
			invID, size, nextPos, template, quality); err != nil {
			return err
		}
		nextPos++
	}

	return tx.Commit(ctx)
}

func formatGiveItemResult(playerID int64, template string, qty int64, toppedUp, created int) string {
	msg := fmt.Sprintf("Added %d × %s to player %d", qty, template, playerID)
	if toppedUp > 0 || created > 0 {
		return fmt.Sprintf(
			"Added %d × %s to player %d (%d stack(s) topped up, %d new stack(s))",
			qty, template, playerID, toppedUp, created)
	}
	return msg
}

// checkInventoryCapacity verifies that qty items of template fit in the player's
// backpack (inventory_type=0). Returns an error if the inventory is over volume
// or slot limits. Used to pre-validate RMQ give-item commands since the game
// server's cheat function bypasses these checks.
func checkInventoryCapacity(ctx context.Context, playerID int64, template string, qty int64) error {
	if globalDB == nil {
		return fmt.Errorf("not connected")
	}
	var invID int64
	var maxSlots int
	var maxVolume float64
	err := globalDB.QueryRow(ctx, `
		SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
		FROM dune.inventories
		WHERE actor_id = $1::bigint AND inventory_type = 0
		LIMIT 1`, playerID).Scan(&invID, &maxSlots, &maxVolume)
	if err != nil {
		// No inventory found — cannot validate; let the game server decide.
		return nil
	}

	hasSlotCap := maxSlots > 0
	hasVolumeCap := maxVolume > 0
	if !hasSlotCap && !hasVolumeCap {
		return nil
	}

	usedSlots := 0
	usedVolume := 0.0
	rows, err := globalDB.Query(ctx, `
		SELECT template_id, stack_size, volume_override
		FROM dune.items
		WHERE inventory_id = $1::bigint`, invID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var tmpl string
		var stackSize int64
		var vol pgtype.Float8
		if err := rows.Scan(&tmpl, &stackSize, &vol); err != nil {
			continue
		}
		usedSlots++
		if hasVolumeCap {
			itemVol := 0.0
			if vol.Valid && vol.Float64 > 0 {
				itemVol = vol.Float64
			} else if itemData.Items != nil {
				if rule, ok := itemData.Items[strings.ToLower(tmpl)]; ok {
					itemVol = rule.Volume
				} else if itemData.DefaultVolume > 0 {
					itemVol = itemData.DefaultVolume
				}
			} else if itemData.DefaultVolume > 0 {
				itemVol = itemData.DefaultVolume
			}
			usedVolume += itemVol * float64(stackSize)
		}
	}

	if hasVolumeCap {
		perItemVol, err := resolveItemVolume(ctx, template)
		if err == nil && perItemVol > 0 {
			availableVol := maxVolume - usedVolume
			if availableVol < 0 {
				availableVol = 0
			}
			maxByVolume := int64(math.Floor(availableVol / perItemVol))
			if maxByVolume < qty {
				return fmt.Errorf(
					"over weight limit: room for %d more %s (%.2f/%.2f volume used)",
					maxByVolume, template, usedVolume, maxVolume)
			}
		}
	}

	if hasSlotCap {
		stackMax, err := resolveStackMax(ctx, template, 0)
		if err != nil || stackMax < 1 {
			stackMax = 1
		}
		newStacks := int((qty + stackMax - 1) / stackMax)
		freeSlots := maxSlots - usedSlots
		if freeSlots < newStacks {
			return fmt.Errorf(
				"inventory full: need %d free slots, have %d",
				newStacks, freeSlots)
		}
	}
	return nil
}

// cmdGrantLive inserts into landsraad_house_rewards which fires a pg_notify trigger.
// The game server receives the notification immediately and shows "Claim Rewards" to the player.
func cmdGrantLive(controllerID int64, templateID string, amount int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		_, err := globalDB.Exec(context.Background(), `
			DELETE FROM dune.landsraad_house_rewards
			WHERE player_id = $1 AND house_name = 'AdminGrant'`,
			controllerID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("grant live clear: %w", err)}
		}
		_, err = globalDB.Exec(context.Background(), `
			INSERT INTO dune.landsraad_house_rewards (player_id, house_name, amount, template_id, last_updated)
			VALUES ($1, 'AdminGrant', $2, $3, NOW())`,
			controllerID, amount, templateID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("grant live: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Queued live grant: %dx %s — player will see Claim Rewards", amount, templateID)}
	}
}

func cmdGiveCurrency(playerID int64, amount int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		// Route through adjust_player_virtual_currency_balance for audit logging
		// and negative-balance guards. The casts match the live function signature.
		_, err := globalDB.Exec(ctx, `
			SELECT dune.adjust_player_virtual_currency_balance(
				$1::bigint,
				dune.get_solaris_id(),
				$2::bigint
			)`,
			playerID, amount)
		if err != nil {
			return msgMutate{err: err}
		}
		var balance int64
		_ = globalDB.QueryRow(ctx, `
			SELECT balance FROM dune.player_virtual_currency_balances
			WHERE player_controller_id = $1::bigint AND currency_id = dune.get_solaris_id()`,
			playerID).Scan(&balance)
		return msgMutate{ok: fmt.Sprintf(
			"Added %d Solaris to player %d — new balance %d",
			amount, playerID, balance)}
	}
}

func cmdGiveFactionRep(actorID int64, factionID int16, delta int32) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		return applyFactionRepDelta(ctx, actorID, factionID, delta)
	}
}

func cmdGiveLandsraadScrip(actorID int64, delta int32) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		if actorID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		currencyID, err := resolveScripCurrencyID(ctx)
		if err != nil {
			return msgMutate{err: err}
		}
		_, err = globalDB.Exec(ctx, `
			SELECT dune.adjust_player_virtual_currency_balance($1::bigint, $2::smallint, $3::bigint)`,
			actorID, currencyID, int64(delta))
		if err != nil {
			return msgMutate{err: err}
		}
		var balance int64
		_ = globalDB.QueryRow(ctx, `
			SELECT balance FROM dune.player_virtual_currency_balances
			WHERE player_controller_id = $1::bigint AND currency_id = $2::smallint`,
			actorID, currencyID).Scan(&balance)
		return msgMutate{ok: fmt.Sprintf(
			"Added %d scrips (currency %d) to player %d — new balance %d",
			delta, currencyID, actorID, balance)}
	}
}

func cmdAwardXP(playerID int64, trackType string, delta int32) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		const maxXP int32 = 44182
		if delta > maxXP {
			delta = maxXP
		}
		res, err := globalDB.Exec(context.Background(), `
			UPDATE dune.specialization_tracks
			SET xp_amount = GREATEST(LEAST(xp_amount + $1::integer, $4::integer), 0)
			WHERE player_id = $2::bigint AND track_type::text = $3::text`,
			delta, playerID, trackType, maxXP)
		if err != nil {
			return msgMutate{err: err}
		}
		if res.RowsAffected() == 0 {
			_, err = globalDB.Exec(context.Background(), `
				INSERT INTO dune.specialization_tracks (player_id, track_type, xp_amount, level)
				VALUES ($1::bigint, $2::dune.specializationtracktype, LEAST($3::integer, $4::integer), 0::real)`,
				playerID, trackType, delta, maxXP)
			if err != nil {
				return msgMutate{err: err}
			}
		}
		return msgMutate{ok: fmt.Sprintf("Awarded %d XP (%s) to player %d", delta, trackType, playerID)}
	}
}

func cmdRenameCharacter(accountID int64, name string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return msgMutate{err: fmt.Errorf("name required")}
		}
		_, err := globalDB.Exec(context.Background(), `SELECT dune.set_character_name($1, $2)`, accountID, name)
		if err != nil {
			return msgMutate{err: fmt.Errorf("rename character: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Renamed to %s", name)}
	}
}

func cmdGetPlayerTags(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgTags{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(),
			`SELECT tag FROM dune.player_tags WHERE account_id=$1 ORDER BY tag`, accountID)
		if err != nil {
			return msgTags{err: err}
		}
		defer rows.Close()
		var tags []string
		for rows.Next() {
			var tag string
			if err := rows.Scan(&tag); err != nil {
				continue
			}
			tags = append(tags, tag)
		}
		if err := rows.Err(); err != nil {
			return msgTags{err: err}
		}
		return msgTags{rows: tags}
	}
}

func cmdUpdatePlayerTags(accountID int64, add []string, remove []string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		var addArg, removeArg interface{}
		if len(add) > 0 {
			addArg = add
		} else {
			addArg = []string{}
		}
		if len(remove) > 0 {
			removeArg = remove
		} else {
			removeArg = []string{}
		}
		_, err := globalDB.Exec(context.Background(),
			`SELECT dune.update_player_tags($1, $2::text[], $3::text[])`, accountID, addArg, removeArg)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update player tags: %w", err)}
		}
		return msgMutate{ok: "Tags updated"}
	}
}

// rawFuncomID returns the accounts."user" value (hex Funcom ID) for a given
// account_id. This is the ID format expected by character_transfer_export,
// complete_journey_story_nodes_for_player, update_returning_player_status,
// delete_account, and other procs — distinct from encrypted_funcom_id which
// stores the human-readable display name (e.g. "Icehunter#55381").
func rawFuncomID(ctx context.Context, accountID int64) (string, error) {
	var id string
	err := globalDB.QueryRow(ctx, `SELECT "user" FROM dune.accounts WHERE id = $1`, accountID).Scan(&id)
	return id, err
}

func cmdGrantReturningPlayerAward(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		rawID, err := rawFuncomID(ctx, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("look up funcom id: %w", err)}
		}
		_, err = globalDB.Exec(ctx, `
			UPDATE dune.encrypted_player_state
			SET last_returning_player_awarded_time = NULL,
			    last_returning_player_event_time = NULL
			WHERE account_id = $1`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset returning player timestamps: %w", err)}
		}
		_, err = globalDB.Exec(ctx, `SELECT dune.update_returning_player_status($1, 0)`, rawID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update_returning_player_status: %w", err)}
		}
		return msgMutate{ok: "Returning player award reset — will trigger on next login"}
	}
}

func cmdDismissReturningPlayerAward(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		_, err := globalDB.Exec(ctx, `
			UPDATE dune.encrypted_player_state
			SET last_returning_player_awarded_time = NOW(),
			    last_returning_player_event_time = NOW()
			WHERE account_id = $1`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("dismiss returning player award: %w", err)}
		}
		return msgMutate{ok: "Returning player popup dismissed"}
	}
}

func cmdDeleteAccount(accountID int64, reason string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		rawID, err := rawFuncomID(ctx, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("look up funcom id: %w", err)}
		}
		var result bool
		err = globalDB.QueryRow(ctx, `SELECT dune.delete_account($1, $2)`, rawID, reason).Scan(&result)
		if err != nil {
			return msgMutate{err: fmt.Errorf("delete account: %w", err)}
		}
		return msgMutate{ok: "Account deleted"}
	}
}

func cmdDeleteItem(itemID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if itemID == 0 {
			return msgMutate{err: fmt.Errorf("item ID required")}
		}
		_, err := globalDB.Exec(context.Background(), `SELECT dune.delete_item($1::bigint)`, itemID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("delete item: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Deleted item %d", itemID)}
	}
}

func cmdResetSpecializations(playerID int64, trackType string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()

		if trackType == "" || strings.EqualFold(trackType, "all") {
			if _, err := globalDB.Exec(ctx, `SELECT dune.reset_specialization_tracks($1)`, playerID); err != nil {
				return msgMutate{err: fmt.Errorf("reset tracks: %w", err)}
			}
			if _, err := globalDB.Exec(ctx, `SELECT dune.reset_specialization_keystones($1)`, playerID); err != nil {
				return msgMutate{err: fmt.Errorf("reset keystones: %w", err)}
			}
			return msgMutate{ok: fmt.Sprintf("Reset all spec tracks + keystones for player %d", playerID)}
		}

		res, err := globalDB.Exec(ctx, `
			DELETE FROM dune.specialization_tracks
			WHERE player_id = $1::bigint AND track_type::text = $2::text`, playerID, trackType)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset track: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf(
			"Reset %s track for player %d (%d row(s) cleared)", trackType, playerID, res.RowsAffected())}
	}
}

// onlineStateRow holds a single row from the player online state query.
type onlineStateRow struct {
	PlayerID int64
	Name     string
	Map      string
	Status   string
	LastSeen string
}

type msgOnlineState struct {
	rows []onlineStateRow
	err  error
}

func cmdFetchOnlineState() Msg {
	if globalDB == nil {
		return msgOnlineState{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT ps.player_controller_id,
		       COALESCE(ps.character_name, ''),
		       COALESCE(a.map, ''),
		       ps.online_status::text,
		       COALESCE(to_char(ps.last_avatar_activity AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '')
		FROM dune.player_state ps
		LEFT JOIN dune.actors a ON a.id = ps.player_controller_id
		ORDER BY ps.online_status DESC, ps.last_avatar_activity DESC`)
	if err != nil {
		return msgOnlineState{err: err}
	}
	defer rows.Close()

	var out []onlineStateRow
	for rows.Next() {
		var r onlineStateRow
		if err := rows.Scan(&r.PlayerID, &r.Name, &r.Map, &r.Status, &r.LastSeen); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgOnlineState{err: err}
	}
	return msgOnlineState{rows: out}
}

// ── private helpers ───────────────────────────────────────────────────────────

func resolveStackMax(ctx context.Context, template string, quality int64) (int64, error) {
	if itemData.Items != nil {
		if rule, ok := itemData.Items[strings.ToLower(template)]; ok && rule.StackMax > 0 {
			return rule.StackMax, nil
		}
	}
	var maxStack int64
	err := globalDB.QueryRow(ctx, `
		SELECT COALESCE(MAX(stack_size), 0)
		FROM dune.items
		WHERE template_id = $1::text AND quality_level = $2::bigint`, template, quality).Scan(&maxStack)
	if err != nil {
		return 0, err
	}
	if maxStack > 0 {
		return maxStack, nil
	}
	if itemData.DefaultStackMax > 0 {
		return itemData.DefaultStackMax, nil
	}
	return 1, nil
}

func resolveItemVolume(ctx context.Context, template string) (float64, error) {
	if itemData.Items != nil {
		if rule, ok := itemData.Items[strings.ToLower(template)]; ok {
			// volume=0 is valid (item takes no inventory space).
			return rule.Volume, nil
		}
	}
	var vol pgtype.Float8
	err := globalDB.QueryRow(ctx, `
		SELECT MAX(volume_override)
		FROM dune.items
		WHERE template_id = $1::text AND volume_override IS NOT NULL`, template).Scan(&vol)
	if err != nil {
		return 0, err
	}
	if vol.Valid && vol.Float64 > 0 {
		return vol.Float64, nil
	}
	if itemData.DefaultVolume > 0 {
		return itemData.DefaultVolume, nil
	}
	return 0, nil // unknown volume — treat as zero (no space consumed)
}

func formatCurrencyIDs(ids []int16) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ", ")
}

func resolveScripCurrencyID(ctx context.Context) (int16, error) {
	if scripCurrencyID >= 0 {
		return int16(scripCurrencyID), nil
	}
	rows, err := globalDB.Query(ctx, `
		SELECT currency_id, COALESCE(SUM(balance), 0) AS total
		FROM dune.player_virtual_currency_balances
		WHERE currency_id <> dune.get_solaris_id()
		GROUP BY currency_id
		ORDER BY total DESC, currency_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []int16
	for rows.Next() {
		var id int16
		var total int64
		if err := rows.Scan(&id, &total); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return 0, rows.Err()
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("no non-solaris currency rows found; pass -scripcurrency")
	}
	return 0, fmt.Errorf("multiple non-solaris currency IDs found (%s); pass -scripcurrency", formatCurrencyIDs(ids))
}

// factionPlayerComponentRepSQL updates ReputationAmount inside
// actors.properties.FactionPlayerComponent.m_FactionDataArray for the matching
// faction name. set_player_faction_reputation only updates the table; the
// in-game faction UI reads rank from this jsonb component, so any rep write
// that needs to be reflected in-game must also run this update.
// $1 = controller actor id, $2 = faction name ("Atreides"/"Harkonnen"),
// $3 = new ReputationAmount.
const factionPlayerComponentRepSQL = `
	UPDATE dune.actors a
	SET properties = jsonb_set(
		a.properties,
		ARRAY['FactionPlayerComponent','m_FactionDataArray', (sub.idx - 1)::text, 'ReputationAmount'],
		to_jsonb($3::int))
	FROM (
		SELECT ord AS idx
		FROM dune.actors aa,
		     jsonb_array_elements(aa.properties->'FactionPlayerComponent'->'m_FactionDataArray')
		         WITH ORDINALITY AS arr(elem, ord)
		WHERE aa.id = $1 AND elem->'Faction'->>'Name' = $2
	) sub
	WHERE a.id = $1`

func applyFactionRepDelta(ctx context.Context, actorID int64, factionID int16, delta int32) msgMutate {
	// Route through set_player_faction_reputation which handles tier tags correctly.
	// First get current rep to compute the new absolute value.
	var currentRep int32
	_ = globalDB.QueryRow(ctx, `
		SELECT COALESCE(reputation_amount, 0) FROM dune.player_faction_reputation
		WHERE actor_id = $1::bigint AND faction_id = $2::smallint`, actorID, factionID).Scan(&currentRep)

	newRep := currentRep + delta
	if newRep < 0 {
		newRep = 0
	}
	if newRep > factionRepCap {
		newRep = factionRepCap
	}

	_, err := globalDB.Exec(ctx, `
		SELECT dune.set_player_faction_reputation($1::bigint, $2::smallint, $3::integer)`,
		actorID, factionID, newRep)
	if err != nil {
		return msgMutate{err: fmt.Errorf("set_player_faction_reputation: %w", err)}
	}
	if _, err = globalDB.Exec(ctx, factionPlayerComponentRepSQL,
		actorID, factionDisplayName(factionID), newRep); err != nil {
		return msgMutate{err: fmt.Errorf("update FactionPlayerComponent rep: %w", err)}
	}

	tier := repToTier(newRep)
	fName := factionDisplayName(factionID)
	return msgMutate{ok: fmt.Sprintf(
		"Set %s rep to %d → tier %d (%s) for actor %d",
		fName, newRep, tier, factionTierName(factionID, tier), actorID)}
}

// factionRepCap is the maximum reputation for any faction (tier 20).
const factionRepCap = int32(12474)

// factionTierThresholds[i] = cumulative rep required to reach tier i (0–20).
// Both Atreides and Harkonnen share identical thresholds.
var factionTierThresholds = [21]int32{
	0, 99, 249, 499, 999, 1999, 2224, 2524, 2899, 3349, 3874,
	4474, 5149, 5899, 6724, 7624, 8599, 9649, 10774, 11974, 12474,
}

// repToTier returns the tier (0–20) for a given reputation amount.
func repToTier(rep int32) int {
	tier := 0
	for i := 1; i <= 20; i++ {
		if rep >= factionTierThresholds[i] {
			tier = i
		} else {
			break
		}
	}
	return tier
}

// factionTierName returns the named tier string for a faction+tier combination.
func factionTierName(factionID int16, tier int) string {
	named := map[int]string{
		0: "Outsider", 1: "Mercenary", 2: "Recruit", 3: "Contractor",
		4: "Agent", 5: "House Operator",
	}
	if tier20 := map[int16]string{1: "Envoy", 2: "Enforcer"}; tier == 20 {
		if n, ok := tier20[factionID]; ok {
			return n
		}
	}
	if n, ok := named[tier]; ok {
		return n
	}
	return fmt.Sprintf("Tier %d", tier)
}

func factionDisplayName(id int16) string {
	switch id {
	case 1:
		return "Atreides"
	case 2:
		return "Harkonnen"
	case 3:
		return "None"
	case 4:
		return "Smuggler"
	default:
		return fmt.Sprintf("Faction%d", id)
	}
}

func cmdSetFactionTier(actorID int64, factionID int16, tier int) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if tier < 0 || tier > 20 {
			return msgMutate{err: fmt.Errorf("tier must be 0–20")}
		}
		// Nudge +1 over the threshold — the game UI floors at the threshold
		// (rep == threshold shows the tier below), except at tier 0 where 0 is
		// the legitimate minimum.
		rep := factionTierThresholds[tier]
		if tier > 0 {
			rep++
		}
		ctx := context.Background()
		_, err := globalDB.Exec(ctx, `SELECT dune.set_player_faction_reputation($1, $2, $3)`,
			actorID, factionID, rep)
		if err != nil {
			return msgMutate{err: fmt.Errorf("set_player_faction_reputation: %w", err)}
		}
		if _, err = globalDB.Exec(ctx, factionPlayerComponentRepSQL,
			actorID, factionDisplayName(factionID), rep); err != nil {
			return msgMutate{err: fmt.Errorf("update FactionPlayerComponent rep: %w", err)}
		}
		fName := factionDisplayName(factionID)
		return msgMutate{ok: fmt.Sprintf(
			"Set %s to tier %d (%s) — rep %d for actor %d",
			fName, tier, factionTierName(factionID, tier), rep, actorID)}
	}
}

func cmdFetchItemTemplates() Msg {
	if globalDB == nil {
		return msgItemTemplates{}
	}
	rows, err := globalDB.Query(context.Background(),
		`SELECT DISTINCT template_id FROM dune.items ORDER BY template_id`)
	if err != nil {
		return msgItemTemplates{}
	}
	defer rows.Close()
	var templates []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			templates = append(templates, t)
		}
	}
	return msgItemTemplates{templates: templates}
}

// ── database tab types and fetch functions ────────────────────────────────────

type tableRow struct {
	Name     string
	RowCount int64
}

type columnInfo struct {
	Name     string
	DataType string
	Nullable string
}

type msgTables struct {
	rows []tableRow
	err  error
}

type msgDescribe struct {
	table string
	cols  []columnInfo
	err   error
}

type msgSample struct {
	table   string
	headers []string
	rows    [][]string
	err     error
}

type msgSearchCols struct {
	headers []string
	rows    [][]string
	err     error
}

func cmdFetchTables() Msg {
	if globalDB == nil {
		return msgTables{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT relname, COALESCE(n_live_tup, 0)
		FROM pg_stat_user_tables
		ORDER BY relname`)
	if err != nil {
		return msgTables{err: err}
	}
	defer rows.Close()
	var result []tableRow
	for rows.Next() {
		var r tableRow
		if err := rows.Scan(&r.Name, &r.RowCount); err != nil {
			return msgTables{err: err}
		}
		result = append(result, r)
	}
	return msgTables{rows: result}
}

func cmdDescribeTable(tbl string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgDescribe{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT column_name, data_type,
			       CASE is_nullable WHEN 'YES' THEN 'null' ELSE 'not null' END
			FROM information_schema.columns
			WHERE table_schema = $1::text AND table_name = $2::text
			ORDER BY ordinal_position`, dbSchema, tbl)
		if err != nil {
			return msgDescribe{table: tbl, err: err}
		}
		defer rows.Close()
		var cols []columnInfo
		for rows.Next() {
			var c columnInfo
			if err := rows.Scan(&c.Name, &c.DataType, &c.Nullable); err != nil {
				return msgDescribe{table: tbl, err: err}
			}
			cols = append(cols, c)
		}
		if err := rows.Err(); err != nil {
			return msgDescribe{table: tbl, err: err}
		}
		return msgDescribe{table: tbl, cols: cols}
	}
}

func cmdSampleTable(tbl string, limit int) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgSample{err: fmt.Errorf("not connected")}
		}
		// Sanitize table name defensively even though tbl comes from pg_stat_user_tables.
		// pgx.Identifier handles quoting and escaping to prevent SQL injection.
		safeTable := pgx.Identifier{dbSchema, tbl}.Sanitize()
		rows, err := globalDB.Query(context.Background(),
			fmt.Sprintf("SELECT * FROM %s LIMIT %d", safeTable, limit))
		if err != nil {
			return msgSample{table: tbl, err: err}
		}
		defer rows.Close()
		var headers []string
		for _, fd := range rows.FieldDescriptions() {
			headers = append(headers, fd.Name)
		}
		var result [][]string
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return msgSample{table: tbl, err: err}
			}
			var row []string
			for _, v := range vals {
				row = append(row, fmt.Sprintf("%v", v))
			}
			result = append(result, row)
		}
		if err := rows.Err(); err != nil {
			return msgSample{table: tbl, err: err}
		}
		return msgSample{table: tbl, headers: headers, rows: result}
	}
}

func cmdSearchColumns(term string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgSearchCols{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT table_name, column_name, data_type
			FROM information_schema.columns
			WHERE table_schema = $1::text
			  AND (column_name ILIKE $2::text OR table_name ILIKE $2::text)
			ORDER BY table_name, column_name`, dbSchema, "%"+term+"%")
		if err != nil {
			return msgSearchCols{err: err}
		}
		defer rows.Close()
		headers := []string{"table", "column", "type"}
		var result [][]string
		for rows.Next() {
			var table, col, dtype string
			if err := rows.Scan(&table, &col, &dtype); err != nil {
				return msgSearchCols{err: err}
			}
			result = append(result, []string{table, col, dtype})
		}
		if err := rows.Err(); err != nil {
			return msgSearchCols{err: err}
		}
		return msgSearchCols{headers: headers, rows: result}
	}
}

// ── journey / progression commands ───────────────────────────────────────────

func cmdFetchJourneyNodes(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgJourney{err: fmt.Errorf("not connected")}
		}

		// Cache hit?
		journeyCacheMu.RLock()
		entry, ok := journeyCache[accountID]
		journeyCacheMu.RUnlock()
		if ok && time.Since(entry.cached) < journeyCacheTTL {
			return msgJourney{rows: entry.nodes}
		}

		rows, err := globalDB.Query(context.Background(), `
			SELECT story_node_id,
			       (complete_condition_state = 'true'::jsonb) AS is_complete,
			       (reveal_condition_state   = 'true'::jsonb) AS is_revealed,
			       has_pending_reward
			FROM dune.journey_story_node
			WHERE account_id = $1
			ORDER BY story_node_id`, accountID)
		if err != nil {
			return msgJourney{err: err}
		}
		defer rows.Close()

		var nodes []journeyNode
		for rows.Next() {
			var n journeyNode
			var isComplete, isRevealed pgtype.Bool
			if err := rows.Scan(&n.NodeID, &isComplete, &isRevealed, &n.HasPendingReward); err != nil {
				continue
			}
			n.IsComplete = isComplete.Bool
			n.IsRevealed = isRevealed.Bool
			nodes = append(nodes, n)
		}
		if err := rows.Err(); err != nil {
			return msgJourney{err: err}
		}
		journeyCacheMu.Lock()
		journeyCache[accountID] = journeyCacheEntry{nodes: nodes, cached: time.Now()}
		journeyCacheMu.Unlock()
		return msgJourney{rows: nodes}
	}
}

// tagsForJourneyNodeSubtree returns the union of m_TagsToAdd for the named
// node and every descendant (matching the SQL completion behavior in
// cmdCompleteJourneyNode which flips children too). Order preserved, deduped.
func tagsForJourneyNodeSubtree(nodeID string) []string {
	if tagsData.JourneyNodeTags == nil {
		return nil
	}
	prefix := nodeID + "."
	seen := map[string]bool{}
	var out []string
	add := func(tags []string) {
		for _, t := range tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	add(tagsData.JourneyNodeTags[nodeID])
	for id, tags := range tagsData.JourneyNodeTags {
		if strings.HasPrefix(id, prefix) {
			add(tags)
		}
	}
	return out
}

// tierBumpFromTags scans applied tags for Faction.<X>.Tier<N> (N ∈ [0,5]) and
// returns the highest implied reputation per faction. Used to fire the rep
// promotion side effect when admin completion applies a tier tag.
func tierBumpFromTags(tags []string) map[string]int32 {
	out := map[string]int32{}
	// e.g. "Faction.Atreides.Tier3"
	for _, t := range tags {
		const prefix = "Faction."
		if !strings.HasPrefix(t, prefix) {
			continue
		}
		rest := t[len(prefix):]
		dot := strings.IndexByte(rest, '.')
		if dot <= 0 {
			continue
		}
		faction := rest[:dot]
		tail := rest[dot+1:]
		if !strings.HasPrefix(tail, "Tier") {
			continue
		}
		n, err := strconv.Atoi(tail[len("Tier"):])
		if err != nil || n < 0 || n > 5 {
			continue
		}
		// +1 over the tier threshold so the in-game UI doesn't floor a tier low
		// (rep == threshold displays the tier below). Tier 0 stays at 0 — it's
		// the legitimate starting state.
		rep := factionTierThresholds[n]
		if n > 0 {
			rep++
		}
		if rep > out[faction] {
			out[faction] = rep
		}
	}
	return out
}

func factionIDByName(name string) int16 {
	switch name {
	case "Atreides":
		return 1
	case "Harkonnen":
		return 2
	case "None":
		return 3
	case "Smuggler":
		return 4
	}
	return 0
}

func cmdCompleteJourneyNode(accountID int64, nodeID string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		// Complete the node itself plus all child nodes (nodeID + ".anything").
		// The game checks sub-nodes to determine quest completion state.
		res, err := globalDB.Exec(ctx, `
			UPDATE dune.journey_story_node
			SET complete_condition_state = 'true'::jsonb,
			    reveal_condition_state   = 'true'::jsonb
			WHERE account_id = $1
			  AND (story_node_id = $2 OR story_node_id LIKE $2 || '.%')`,
			accountID, nodeID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("complete node: %w", err)}
		}
		updated := res.RowsAffected()
		if updated == 0 {
			// Node doesn't exist yet — insert it.
			_, err = globalDB.Exec(ctx, `
				INSERT INTO dune.journey_story_node
					(account_id, story_node_id, has_pending_reward,
					 complete_condition_state, reveal_condition_state,
					 fail_condition_state, metadata_state, reset_group)
				VALUES ($1, $2, false,
					'true'::jsonb, 'true'::jsonb,
					'{}'::jsonb, '{}'::jsonb,
					'Default'::dune.JourneyStoryResetGroup)`,
				accountID, nodeID)
			if err != nil {
				return msgMutate{err: fmt.Errorf("insert node: %w", err)}
			}
			updated = 1
		}

		// Apply tags that in-game completion of the node + its descendants
		// would emit (via m_TagsToAdd). Without this the DB row is flipped
		// but the player is missing the side effects the game would have
		// written — which is why journey-only completion historically did not
		// "stick" without login/logout cycles.
		appliedTags := tagsForJourneyNodeSubtree(nodeID)
		extra, err := applyTagsWithTierBump(ctx, accountID, appliedTags)
		if err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: fmt.Sprintf("Completed %s + %d node(s)%s — takes effect on next login", nodeID, updated, extra)}
	}
}

// applyTagsWithTierBump writes `tags` via dune.update_player_tags and, for any
// Faction.<X>.Tier<N> (N ∈ 0–5) it sees, also raises that faction's rep + the
// FactionPlayerComponent ReputationAmount on the controller actor so the
// in-game rank UI reflects the promotion. Never lowers existing rep.
// Returns a short " , +K tag(s), bumped rep for N faction(s)" fragment for
// inclusion in the caller's success message (empty when no tags applied).
func applyTagsWithTierBump(ctx context.Context, accountID int64, tags []string) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	if _, err := globalDB.Exec(ctx,
		`SELECT dune.update_player_tags($1, $2::text[], '{}'::text[])`,
		accountID, tags); err != nil {
		return "", fmt.Errorf("apply tags: %w", err)
	}

	extra := fmt.Sprintf(", +%d tag(s)", len(tags))

	bumps := tierBumpFromTags(tags)
	if len(bumps) == 0 {
		return extra, nil
	}

	var controllerID int64
	_ = globalDB.QueryRow(ctx, `
		SELECT player_controller_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, accountID).Scan(&controllerID)
	if controllerID == 0 {
		// Fresh character without a player_state row — can't bump rep yet.
		// Tags landed, the rep side effect will have to wait until the
		// character first logs in. Surface in the message.
		return extra + ", rep bump skipped (no controller yet)", nil
	}

	bumped := 0
	for faction, rep := range bumps {
		fid := factionIDByName(faction)
		if fid == 0 {
			continue
		}
		var current int32
		_ = globalDB.QueryRow(ctx, `
			SELECT COALESCE(reputation_amount, 0)
			FROM dune.player_faction_reputation
			WHERE actor_id = $1 AND faction_id = $2`,
			controllerID, fid).Scan(&current)
		if current >= rep {
			continue
		}
		if _, err := globalDB.Exec(ctx,
			`SELECT dune.set_player_faction_reputation($1::bigint, $2::smallint, $3::integer)`,
			controllerID, fid, rep); err != nil {
			return "", fmt.Errorf("bump %s rep: %w", faction, err)
		}
		if _, err := globalDB.Exec(ctx, factionPlayerComponentRepSQL,
			controllerID, faction, rep); err != nil {
			return "", fmt.Errorf("bump %s FactionPlayerComponent: %w", faction, err)
		}
		bumped++
	}
	if bumped > 0 {
		extra += fmt.Sprintf(", bumped rep for %d faction(s)", bumped)
	}
	return extra, nil
}

// resolveContractTags resolves a contract id (full DA_CT_ name or short alias)
// to its AddedFlagsOnCompletion list. Returns the resolved canonical name and
// the tags, or ("", nil, err) if unknown.
func resolveContractTags(contractID string) (string, []string, error) {
	name := contractID
	if full, ok := tagsData.ContractAliases[contractID]; ok {
		name = full
	}
	tags, ok := tagsData.ContractTags[name]
	if !ok || len(tags) == 0 {
		return "", nil, fmt.Errorf("unknown contract %q (check tags-data.json)", contractID)
	}
	return name, tags, nil
}

// cmdCompleteContract applies the AddedFlagsOnCompletion tags for one contract.
func cmdCompleteContract(accountID int64, contractID string) Cmd {
	return cmdCompleteContracts(accountID, []string{contractID})
}

// cmdCompleteContracts applies the union of AddedFlagsOnCompletion across
// multiple contracts in one go — one update_player_tags call, one tier-bump
// pass, plus any SkillsKeyRewards skill-block unlocks. Unknown contracts
// cause the whole batch to fail before any write so the operation is
// all-or-nothing.
func cmdCompleteContracts(accountID int64, contractIDs []string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		if len(contractIDs) == 0 {
			return msgMutate{err: fmt.Errorf("at least one contract required")}
		}

		seenTag := map[string]bool{}
		var allTags []string
		seenSkill := map[string]bool{}
		var allSkillGrants []string
		var resolved []string
		for _, id := range contractIDs {
			name, tags, err := resolveContractTags(id)
			if err != nil {
				return msgMutate{err: err}
			}
			resolved = append(resolved, name)
			for _, t := range tags {
				if !seenTag[t] {
					seenTag[t] = true
					allTags = append(allTags, t)
				}
			}
			for _, sk := range tagsData.ContractSkillGrants[name] {
				if !seenSkill[sk] {
					seenSkill[sk] = true
					allSkillGrants = append(allSkillGrants, sk)
				}
			}
		}

		ctx := context.Background()
		extra, err := applyTagsWithTierBump(ctx, accountID, allTags)
		if err != nil {
			return msgMutate{err: err}
		}

		if len(allSkillGrants) > 0 {
			grantedExtra, err := grantSkillBlocks(ctx, accountID, allSkillGrants)
			if err != nil {
				return msgMutate{err: err}
			}
			extra += grantedExtra
		}

		// Strip any in-progress ContractItem rows so the in-game quest
		// tracker doesn't keep showing the conditions for a contract we just
		// force-completed. ContractName.Name uses the short alias form
		// (no DA_CT_ prefix).
		shortNames := make([]string, 0, len(resolved))
		for _, full := range resolved {
			shortNames = append(shortNames, strings.TrimPrefix(full, "DA_CT_"))
		}
		dismissedExtra, err := dismissActiveContracts(ctx, accountID, shortNames)
		if err != nil {
			return msgMutate{err: err}
		}
		extra += dismissedExtra

		summary := resolved[0]
		if len(resolved) > 1 {
			summary = fmt.Sprintf("%d contracts", len(resolved))
		}
		return msgMutate{ok: fmt.Sprintf("Applied %s%s — takes effect on next login", summary, extra)}
	}
}

// cmdReverseContracts removes the AddedFlagsOnCompletion tags and strips the
// Skills.Key.* ModuleData entries that cmdCompleteContracts wrote. Skill blocks
// are only removed when SkillPointsSpent <= 1 — branches the player genuinely
// levelled beyond the admin grant are left intact.
func cmdReverseContracts(accountID int64, contractIDs []string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		if len(contractIDs) == 0 {
			return msgMutate{err: fmt.Errorf("at least one contract required")}
		}

		seenTag := map[string]bool{}
		var removeTags []string
		seenSkill := map[string]bool{}
		var removeSkills []string
		var resolved []string

		for _, id := range contractIDs {
			name, tags, err := resolveContractTags(id)
			if err != nil {
				return msgMutate{err: err}
			}
			resolved = append(resolved, name)
			for _, t := range tags {
				if !seenTag[t] {
					seenTag[t] = true
					removeTags = append(removeTags, t)
				}
			}
			for _, sk := range tagsData.ContractSkillGrants[name] {
				if !seenSkill[sk] {
					seenSkill[sk] = true
					removeSkills = append(removeSkills, sk)
				}
			}
		}

		ctx := context.Background()

		if len(removeTags) > 0 {
			if _, err := globalDB.Exec(ctx,
				`SELECT dune.update_player_tags($1, '{}'::text[], $2::text[])`,
				accountID, removeTags); err != nil {
				return msgMutate{err: fmt.Errorf("remove tags: %w", err)}
			}
		}

		stripped := 0
		if len(removeSkills) > 0 {
			var pawnID int64
			_ = globalDB.QueryRow(ctx,
				`SELECT player_pawn_id FROM dune.player_state WHERE account_id = $1 LIMIT 1`,
				accountID).Scan(&pawnID)
			if pawnID != 0 {
				for _, sk := range removeSkills {
					key := fmt.Sprintf(`(TagName="%s")`, sk)
					tag, err := globalDB.Exec(ctx, `
						UPDATE dune.fgl_entities fe
						SET components = jsonb_set(
							fe.components,
							ARRAY['FLevelComponent','1','ModuleData'],
							(fe.components->'FLevelComponent'->1->'ModuleData') - $2::text)
						WHERE fe.entity_id = (
							SELECT entity_id FROM dune.actor_fgl_entities
							WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
						)
						AND COALESCE(
							(fe.components->'FLevelComponent'->1->'ModuleData'->$2->>'SkillPointsSpent')::int,
							0
						) <= 1`,
						pawnID, key)
					if err != nil {
						return msgMutate{err: fmt.Errorf("strip %s: %w", sk, err)}
					}
					if tag.RowsAffected() > 0 {
						stripped++
					}
				}
			}
		}

		summary := resolved[0]
		if len(resolved) > 1 {
			summary = fmt.Sprintf("%d contracts", len(resolved))
		}
		return msgMutate{ok: fmt.Sprintf(
			"Reversed %s: removed %d tag(s), stripped %d skill block(s) — takes effect on next login",
			summary, len(removeTags), stripped)}
	}
}

// cmdResetJobSkills removes every ModuleData entry whose SkillArea matches
// the named job — Key blocks, Abilities, Attributes, Perks — fully nuking
// that class's skill tree. Key-block removal alone leaves orphaned ability
// rows (e.g. SuspensorGrenade_Reduction lingers after Skills.Key.Trooper1
// is gone) which the game still treats as refundable for 1 SP each (the
// "phantom SP" bug). Removing every SkillArea-matching module avoids that.
func cmdResetJobSkills(accountID int64, job string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		modules := tagsData.JobAllModules[job]
		if len(modules) == 0 {
			return msgMutate{err: fmt.Errorf("unknown job %q (check tags-data.json job_all_modules)", job)}
		}
		ctx := context.Background()

		var pawnID int64
		_ = globalDB.QueryRow(ctx, `
			SELECT player_pawn_id FROM dune.player_state
			WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
		if pawnID == 0 {
			return msgMutate{err: fmt.Errorf("no pawn for account %d", accountID)}
		}

		// Build the (TagName="...") keyed names in one pass and use the
		// jsonb minus-text[] operator to drop them all in a single UPDATE.
		keys := make([]string, len(modules))
		for i, m := range modules {
			keys[i] = fmt.Sprintf(`(TagName="%s")`, m)
		}
		tag, err := globalDB.Exec(ctx, `
			UPDATE dune.fgl_entities fe
			SET components = jsonb_set(
				fe.components,
				ARRAY['FLevelComponent','1','ModuleData'],
				(fe.components->'FLevelComponent'->1->'ModuleData') - $2::text[])
			WHERE fe.entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)`,
			pawnID, keys)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset %s tree: %w", job, err)}
		}
		if tag.RowsAffected() == 0 {
			return msgMutate{ok: fmt.Sprintf("Reset %s skill tree — no ModuleData on pawn", job)}
		}
		return msgMutate{ok: fmt.Sprintf("Reset %s skill tree — scanned %d module slot(s)", job, len(modules))}
	}
}

// starterAbilityByJob is the canonical tier-1 starter ability the game
// auto-grants on character creation for each class — empirically observed
// for BG (VoiceCompel) and Trooper (SuspensorGrenade_Reduction); the others
// derived from DT_TrainingModules.json by picking the unique
// PrereqModuleTags_And = [Skills.Key.<Job>1] ability at GridPosition (3,0),
// which is the slot the game uses for the "middle of the first row" starter.
var starterAbilityByJob = map[string]string{
	"BeneGesserit":  "Skills.Ability.VoiceCompel",
	"Mentat":        "Skills.Ability.PoisonCapsuleLauncher",
	"Planetologist": "Skills.Ability.SuspensorPad",
	"Swordmaster":   "Skills.Ability.DeflectionSlow",
	"Trooper":       "Skills.Ability.SuspensorGrenade_Reduction",
}

// cmdSetStarterClass swaps the player's starter class:
//  1. removes the previous starter's Skills.Key.<Old>1 block + its starter
//     ability from ModuleData (so you don't end up with two starters
//     stacked after switching), then
//  2. writes the new StarterSkillTreeTag pointer,
//  3. activates the new Skills.Key.<Job>1 block at SpSpent: 1,
//  4. grants the new tier-1 starter ability at SpSpent: 1.
//
// Result on next login: only one class is recognised as starter, with its
// canonical first ability already learned.
func cmdSetStarterClass(accountID int64, job string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		if _, ok := tagsData.JobSkillBlocks[job]; !ok {
			return msgMutate{err: fmt.Errorf("unknown job %q", job)}
		}
		newAbility, ok := starterAbilityByJob[job]
		if !ok {
			return msgMutate{err: fmt.Errorf("no starter ability mapping for %q", job)}
		}
		ctx := context.Background()

		var pawnID int64
		_ = globalDB.QueryRow(ctx, `
			SELECT player_pawn_id FROM dune.player_state
			WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
		if pawnID == 0 {
			return msgMutate{err: fmt.Errorf("no pawn for account %d", accountID)}
		}

		// Look up the current starter so we can deactivate it. Format is
		// "Skills.Key.<Job>1"; we strip the prefix/suffix to recover the
		// job name and look up its starter-ability for removal.
		var oldStarterTag string
		_ = globalDB.QueryRow(ctx, `
			SELECT fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName'
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.actor_id = $1 AND afe.slot_name = 'DuneCharacter'`,
			pawnID).Scan(&oldStarterTag)

		var keysToRemove []string
		if strings.HasPrefix(oldStarterTag, "Skills.Key.") && strings.HasSuffix(oldStarterTag, "1") {
			oldJob := strings.TrimSuffix(strings.TrimPrefix(oldStarterTag, "Skills.Key."), "1")
			if oldJob != "" && oldJob != job {
				keysToRemove = append(keysToRemove, fmt.Sprintf(`(TagName="%s")`, oldStarterTag))
				if oldAb, ok := starterAbilityByJob[oldJob]; ok {
					keysToRemove = append(keysToRemove, fmt.Sprintf(`(TagName="%s")`, oldAb))
				}
			}
		}

		newStarterTag := fmt.Sprintf("Skills.Key.%s1", job)
		newStarterKey := fmt.Sprintf(`(TagName="%s")`, newStarterTag)
		newAbilityKey := fmt.Sprintf(`(TagName="%s")`, newAbility)

		// One chained jsonb update: strip old keys, write new tag, activate
		// new starter block, grant new starter ability. - operator on an
		// empty text[] is a no-op so it's safe when there's no old starter
		// to clean up (e.g. fresh character with StarterSkillTreeTag=None).
		_, err := globalDB.Exec(ctx, `
			UPDATE dune.fgl_entities fe
			SET components = jsonb_set(
				jsonb_set(
					jsonb_set(
						jsonb_set(
							fe.components,
							ARRAY['FLevelComponent','1','ModuleData'],
							(fe.components->'FLevelComponent'->1->'ModuleData') - $4::text[]),
						ARRAY['FLevelComponent','1','StarterSkillTreeTag','TagName'],
						to_jsonb($2::text)),
					ARRAY['FLevelComponent','1','ModuleData',$3],
					'{"SkillPointsSpent": 1}'::jsonb,
					true),
				ARRAY['FLevelComponent','1','ModuleData',$5],
				'{"SkillPointsSpent": 1}'::jsonb,
				true)
			WHERE fe.entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)`, pawnID, newStarterTag, newStarterKey, keysToRemove, newAbilityKey)
		if err != nil {
			return msgMutate{err: fmt.Errorf("set starter tag: %w", err)}
		}

		msg := fmt.Sprintf("Starter class set to %s (%s + %s active)", job, newStarterTag, newAbility)
		if len(keysToRemove) > 0 {
			msg += fmt.Sprintf(", cleared previous starter (%d module(s))", len(keysToRemove))
		}
		return msgMutate{ok: msg}
	}
}

// cmdGrantJobSkills unlocks every bExternal Skills.Key.* module in the named
// job's skill tree (e.g. "Trooper" → Trooper1/2/3 + CapstoneGadgets +
// CapstoneWeaponry + CapstoneSuspensorTech). Only ~⅓ of these blocks are
// contract-granted via SkillsKeyRewards; the rest are normally unlocked by
// trainer dialogue or auto on level progression, so the admin Unlock Trainer
// action calls this after the contract batch to bypass those gates.
func cmdGrantJobSkills(accountID int64, job string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		blocks := tagsData.JobSkillBlocks[job]
		if len(blocks) == 0 {
			return msgMutate{err: fmt.Errorf("unknown job %q (check tags-data.json job_skill_blocks)", job)}
		}
		ctx := context.Background()
		extra, err := grantSkillBlocks(ctx, accountID, blocks)
		if err != nil {
			return msgMutate{err: err}
		}
		return msgMutate{ok: fmt.Sprintf("Unlocked %s skill tree%s — takes effect on next login", job, extra)}
	}
}

// dismissActiveContracts deletes any ContractItem inventory entries whose
// stats.FContractItemStats.ContractName.Name matches one of shortNames.
// Active contract items drive the in-game quest tracker, so after force-
// completing a contract via tags we need to remove the live instance
// otherwise the player keeps seeing "Deploy Assault Seekers" / etc as
// outstanding. No-op if the player never had the contract active.
func dismissActiveContracts(ctx context.Context, accountID int64, shortNames []string) (string, error) {
	if len(shortNames) == 0 {
		return "", nil
	}
	var pawnID int64
	_ = globalDB.QueryRow(ctx, `
		SELECT player_pawn_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
	if pawnID == 0 {
		return "", nil
	}
	tag, err := globalDB.Exec(ctx, `
		DELETE FROM dune.items
		WHERE template_id = 'ContractItem'
		  AND inventory_id IN (
		      SELECT id FROM dune.inventories
		      WHERE actor_id = $1 AND inventory_type = 29
		  )
		  AND stats->'FContractItemStats'->1->'ContractName'->>'Name' = ANY($2::text[])`,
		pawnID, shortNames)
	if err != nil {
		return "", fmt.Errorf("dismiss active contracts: %w", err)
	}
	n := tag.RowsAffected()
	if n == 0 {
		return "", nil
	}
	return fmt.Sprintf(", dismissed %d active contract(s)", n), nil
}

// grantSkillBlocks ensures each Skills.Key.<X> entry exists in the player's
// FLevelComponent.ModuleData with SkillPointsSpent: 1 (the format the game
// itself writes when a trainer's SkillsKeyRewards fires). If an entry already
// exists it's left alone — preserves any further SP the player may have
// already spent on that branch's child nodes. Returns a short fragment to
// append to the caller's success message.
func grantSkillBlocks(ctx context.Context, accountID int64, skillKeys []string) (string, error) {
	var pawnID int64
	_ = globalDB.QueryRow(ctx, `
		SELECT player_pawn_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
	if pawnID == 0 {
		return ", skill grants skipped (no pawn yet)", nil
	}

	granted := 0
	for _, sk := range skillKeys {
		key := fmt.Sprintf(`(TagName="%s")`, sk)
		// Set ModuleData[key] = {"SkillPointsSpent": 1} when:
		//   - key doesn't exist yet (game never created a placeholder), OR
		//   - key exists with SpSpent <= 0 (game-created placeholder that
		//     means "available but not yet purchased").
		// SpSpent >= 1 is left alone so any further SP the player has
		// already spent on child nodes survives.
		tag, err := globalDB.Exec(ctx, `
			UPDATE dune.fgl_entities fe
			SET components = jsonb_set(
				fe.components,
				ARRAY['FLevelComponent','1','ModuleData',$2],
				'{"SkillPointsSpent": 1}'::jsonb,
				true)
			WHERE fe.entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)
			AND COALESCE(
				(fe.components->'FLevelComponent'->1->'ModuleData'->$2->>'SkillPointsSpent')::int,
				0
			) < 1`,
			pawnID, key)
		if err != nil {
			return "", fmt.Errorf("grant %s: %w", sk, err)
		}
		if tag.RowsAffected() > 0 {
			granted++
		}
	}
	if granted == 0 {
		return ", no skill blocks needed (all already unlocked)", nil
	}
	return fmt.Sprintf(", unlocked %d skill block(s)", granted), nil
}

// allJourneyTags returns the union of every tag any journey node would emit
// on completion. Used by Wipe All to also strip tags that prior completions
// may have applied. Rep is intentionally not touched — natural progression
// is monotonic and we don't try to roll it back.
func allJourneyTags() []string {
	if tagsData.JourneyNodeTags == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, tags := range tagsData.JourneyNodeTags {
		for _, t := range tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func cmdResetJourneyNode(accountID int64, nodeID string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		_, err := globalDB.Exec(ctx, `
			UPDATE dune.journey_story_node
			SET complete_condition_state = 'false'::jsonb,
			    has_pending_reward       = false
			WHERE account_id = $1
			  AND (story_node_id = $2 OR story_node_id LIKE $2 || '.%')`,
			accountID, nodeID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset node: %w", err)}
		}

		// Also strip any tags this node + its descendants would have emitted
		// on completion. The proc accepts (add, remove) text[] pairs.
		removeTags := tagsForJourneyNodeSubtree(nodeID)
		extra := ""
		if len(removeTags) > 0 {
			if _, err = globalDB.Exec(ctx,
				`SELECT dune.update_player_tags($1, '{}'::text[], $2::text[])`,
				accountID, removeTags); err != nil {
				return msgMutate{err: fmt.Errorf("remove node tags: %w", err)}
			}
			extra = fmt.Sprintf(", removed %d tag(s)", len(removeTags))
		}
		return msgMutate{ok: fmt.Sprintf("Reset %s%s", nodeID, extra)}
	}
}

func cmdWipeJourneyNodes(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		_, err := globalDB.Exec(ctx,
			`SELECT dune.delete_all_journey_story_nodes($1)`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("wipe journey: %w", err)}
		}

		// Strip every tag any journey node could have emitted, so the
		// player's tag state matches the post-wipe journey state.
		removeTags := allJourneyTags()
		extra := ""
		if len(removeTags) > 0 {
			if _, err = globalDB.Exec(ctx,
				`SELECT dune.update_player_tags($1, '{}'::text[], $2::text[])`,
				accountID, removeTags); err != nil {
				return msgMutate{err: fmt.Errorf("remove journey tags: %w", err)}
			}
			extra = fmt.Sprintf(", removed %d journey tag(s)", len(removeTags))
		}
		return msgMutate{ok: fmt.Sprintf("Wiped all journey nodes for account %d%s", accountID, extra)}
	}
}

// climbTheRanksNodes are the journey nodes that gate access to Landsraad
// rank 5–20 progression (DA_FQ = Dune Awakening Faction Quest).
// Both parent and child nodes must be completed — confirmed by in-game observation.
// These are faction-independent.
var climbTheRanksNodes = []string{
	"DA_FQ_ClimbTheRanks.Rank5To20.MeetSponsor",
	"DA_FQ_ClimbTheRanks.Rank5To20.MeetSponsor.TalkToSponsor",
	"DA_FQ_ClimbTheRanks.Rank5To20.StartLandsraadOnboarding",
	"DA_FQ_ClimbTheRanks.Rank5To20.StartLandsraadOnboarding.ReportToMasterOfAssassins",
	"DA_FQ_ClimbTheRanks.Rank5To20.CompleteLandsraadMission",
	"DA_FQ_ClimbTheRanks.Rank5To20.CompleteLandsraadMission.CompleteOnboardingJourney1",
	"DA_FQ_ClimbTheRanks.Rank5To20.CraftAugmentation",
	"DA_FQ_ClimbTheRanks.Rank5To20.CraftAugmentation.CompleteOnboardingJourney2",
}

// climbTheRanksStoryNodes are the faction-neutral storyline beats observed
// completed on both rank-up reference characters (rank 19 Atreides and rank 8
// Harkonnen). These cover the chapter-2 → rank-5-onboarding journey beats.
var climbTheRanksStoryNodes = []string{
	"DA_FQ_ClimbTheRanks.HuntingSkorda",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda.SkordaInArrakeen",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda.SkordaInMysaTarrill",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda.SkordaInOodham",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.FindCanister",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.InvestigateSandflies",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.TrackDownPilot",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.TrackDownRedScorpion",
	"DA_FQ_ClimbTheRanks.JoinAHouse",
	"DA_FQ_ClimbTheRanks.JoinAHouse.ProveYourself",
	"DA_FQ_ClimbTheRanks.JoinAHouse.ProveYourself.ChooseASide",
	"DA_FQ_ClimbTheRanks.JoinAHouse.ProveYourself.Rank1Contracts",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal.FindTheSpy",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal.GetSpyMission",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal.TalkToARecruiter",
	"DA_FQ_ClimbTheRanks.ClimbTheRanksR2",
	"DA_FQ_ClimbTheRanks.ClimbTheRanksR2.ContributeToWarEffort_Atreides",
	"DA_FQ_ClimbTheRanks.ClimbTheRanksR2.ContributeToWarEffort_Atreides.CompleteContractsR2",
}

// climbTheRanksStoryNodesAtreides are the Atreides-side storyline beats
// (Ch2→Ch3 transition + Test of Loyalty + Atreides investigations).
var climbTheRanksStoryNodesAtreides = []string{
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Atre",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Atre.TheCall",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Atre.TheCall.AnswerTheCall",
	"DA_FQ_ClimbTheRanks.ATestOfLoyalty",
	"DA_FQ_ClimbTheRanks.ATestOfLoyalty.GetMaximToBackOff",
	"DA_FQ_ClimbTheRanks.ATestOfLoyalty.GetMaximToBackOff.FindSemuta",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides.InvestigateWreck_Atreides",
	`DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides.InvestigateWreck_Atreides.Complete "Track Down Skorda" Contract`,
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides.InvestigateWreck_Atreides.MeetAndreaGanan",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.DeviseAPlan_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.DeviseAPlan_Atreides.TellThufirAboutDelphis",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.PledgeAllegiance_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.PledgeAllegiance_Atreides.PledgeAllegiance_Atreides_Sub",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.SecureLastContainer_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.SecureLastContainer_Atreides.RecoverSheolContainer_Atreides",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.ChoosePoisonOrSpare",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.CompleteWarProfiteerContract",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.FindBusinessman",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.TalkToThufirAgain",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest.MeetThufir",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest.ReturnToGanan",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest.SpeakWithGanan",
}

// climbTheRanksStoryNodesHarkonnen are the Harkonnen-side storyline beats
// (Ch2→Ch3 transition + Test of Treachery + Harkonnen investigations).
var climbTheRanksStoryNodesHarkonnen = []string{
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Hark",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Hark.TheCall",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Hark.TheCall.AnswerTheCall",
	"DA_FQ_ClimbTheRanks.ATestOfTreachery",
	"DA_FQ_ClimbTheRanks.ATestOfTreachery.GetAntonToBackOff",
	"DA_FQ_ClimbTheRanks.ATestOfTreachery.GetAntonToBackOff.FindCounterfeitEvidence",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen.InvestigateWreck_Harkonnen",
	`DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen.InvestigateWreck_Harkonnen.Complete "Track Down Skorda" Contract`,
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen.InvestigateWreck_Harkonnen.MeetSimoneVonKonig",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.DeviseAPlan_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.DeviseAPlan_Harkonnen.TellPiterAboutEuporia",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.PledgeAllegiance_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.PledgeAllegiance_Harkonnen.PledgeAllegiance_Harkonnen_Sub",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.SecureLastContainer_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.SecureLastContainer_Harkonnen.RecoverSheolContainer_Harkonnen",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.DeliverResults",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.MeetPiter",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.ReturnToVonKonig",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.SpeakWithVonKonig",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.TakeALeap",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.TakeALeap.PoisonOrWarnPiter",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.TakeALeap.TalkToPiterAgain",
}

// landsraadMissionNodes* are the weekly Landsraad mission journey nodes (DA_SQ =
// Dune Awakening Side Quest). Completed naturally by doing one Landsraad mission
// in-game; required alongside climbTheRanksNodes for rank 5→20 progression.
var landsraadMissionNodesAtreides = []string{
	"DA_SQ_OverlandMap.AtreLandsraadMission",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreAccept",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreKeyStone",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreComplete",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreReturn",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreClaimReward",
}

var landsraadMissionNodesHarkonnen = []string{
	"DA_SQ_OverlandMap.HarkLandsraadMission",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkAccept",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkKeyStone",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkComplete",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkReturn",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkClaimReward",
}

// nodesForPreset returns the journey node IDs to complete for a faction+preset.
// ch3_start: Rank5To20 onboarding + faction-neutral chapter-2 storyline + chosen
// faction's Ch2→Ch3 transition / Test of Loyalty(Treachery) / investigations /
// poisoned spice arc — i.e. everything required for a fresh character to land
// at rank 5 (House Operator), so rank 6-19 can be earned organically.
// rank19_eligible: same set + the weekly Landsraad mission tree, fast-forwarded
// to tier 19.
func nodesForPreset(faction, preset string) []string {
	nodes := append([]string{}, climbTheRanksNodes...)
	nodes = append(nodes, climbTheRanksStoryNodes...)
	switch faction {
	case "atreides":
		nodes = append(nodes, climbTheRanksStoryNodesAtreides...)
	case "harkonnen":
		nodes = append(nodes, climbTheRanksStoryNodesHarkonnen...)
	}
	if preset == "rank19_eligible" {
		switch faction {
		case "atreides":
			nodes = append(nodes, landsraadMissionNodesAtreides...)
		case "harkonnen":
			nodes = append(nodes, landsraadMissionNodesHarkonnen...)
		}
	}
	return nodes
}

// cmdProgressionUnlock completes all prerequisite faction story journey nodes,
// writes the corresponding gameplay tags, and sets reputation to the preset's
// target tier.
//
// faction: "atreides" | "harkonnen"
// preset:  "ch3_start" (rank 5 — House Operator) | "rank19_eligible" (rank 19)
func cmdProgressionUnlock(actorID int64, faction, preset string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		var factionID int16
		var dialogueFlag, alignedFlag, metRecruiterFlag, factionUnlocked, recruitmentDone string
		switch faction {
		case "atreides":
			factionID = 1
			dialogueFlag = "DialogueFlags.Factions.SentToMeetHawat"
			alignedFlag = "DialogueFlags.Factions.AlignedAtreides"
			metRecruiterFlag = "DialogueFlags.Factions.MetHawat"
			factionUnlocked = "Contract.Tracking.AtreidesFactionUnlocked"
			recruitmentDone = "Contract.Tracking.AtreidesRecruitmentCompleted"
		case "harkonnen":
			factionID = 2
			dialogueFlag = "DialogueFlags.Factions.SentToPiterDeVries"
			alignedFlag = "DialogueFlags.Factions.AlignedHarkonnen"
			metRecruiterFlag = "DialogueFlags.Factions.MetPiterDeVries"
			factionUnlocked = "Contract.Tracking.HarkonnenFactionUnlocked"
			recruitmentDone = "Contract.Tracking.HarkonnenRecruitmentCompleted"
		default:
			return msgMutate{err: fmt.Errorf("faction must be atreides or harkonnen")}
		}

		var targetTier int
		switch preset {
		case "ch3_start":
			targetTier = 5
		case "rank19_eligible":
			targetTier = 19
		default:
			return msgMutate{err: fmt.Errorf("preset must be ch3_start or rank19_eligible")}
		}

		ctx := context.Background()

		var accountID, controllerID int64
		err := globalDB.QueryRow(ctx, `
			SELECT COALESCE(a.owner_account_id, 0),
			       COALESCE(ps.player_controller_id, 0)
			FROM dune.actors a
			LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
			WHERE a.id = $1`, actorID,
		).Scan(&accountID, &controllerID)
		if err != nil || accountID == 0 {
			return msgMutate{err: fmt.Errorf("player %d not found or has no account", actorID)}
		}
		if controllerID == 0 {
			return msgMutate{err: fmt.Errorf("player %d has no controller actor", actorID)}
		}
		flsID, err := rawFuncomID(ctx, accountID)
		if err != nil || flsID == "" {
			return msgMutate{err: fmt.Errorf("player %d has no FLS ID", actorID)}
		}

		journeyNodes := nodesForPreset(faction, preset)

		factionName := factionDisplayName(factionID)
		// Faction.<X>.TierN is only a real gameplay tag for N ∈ [0,5] — see
		// DA_Atreides.json / DA_Harkonnen.json m_FactionTiers, where Tier 6+
		// all have m_FactionTierTag.TagName == "None". Tier 5 flips
		// m_bAllowPromotionThroughReputation to true, after which rep alone
		// advances the displayed rank. So Tier0–5 + a rep >= threshold[19] is
		// enough to display rank 19 — no need to write phantom Tier6..19 tags.
		const maxTier = 5
		// Baseline faction-progression tags observed on both rank-up reference
		// characters (rank 19 Atreides + rank 8 Harkonnen). MetARecruiter,
		// PlayedAllegianceCinematic, SeenAnvilCinematic, FactionRank1/3 are
		// faction-neutral; the faction-specific flags are picked above.
		allTags := []string{
			dialogueFlag, alignedFlag, metRecruiterFlag,
			factionUnlocked, recruitmentDone,
			"DialogueFlags.Factions.FactionIntro",
			"DialogueFlags.Factions.FactionRank1",
			"DialogueFlags.Factions.FactionRank3",
			"DialogueFlags.Factions.MetARecruiter",
			"DialogueFlags.Factions.PlayedAllegianceCinematic",
			"DialogueFlags.Factions.SeenAnvilCinematic",
		}
		if targetTier >= 19 {
			allTags = append(allTags, "Journey.LandsraadContractsUnlocked")
		}
		for t := 0; t <= maxTier; t++ {
			allTags = append(allTags, fmt.Sprintf("Faction.%s.Tier%d", factionName, t))
		}

		tx, err := globalDB.Begin(ctx)
		if err != nil {
			return msgMutate{err: err}
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err = tx.Exec(ctx,
			`SELECT dune.complete_journey_story_nodes_for_player($1, $2::text[])`,
			flsID, journeyNodes); err != nil {
			return msgMutate{err: fmt.Errorf("complete journey nodes: %w", err)}
		}

		// Align the player with the chosen faction. Required for fresh / unaligned
		// characters (no player_faction row) — without this the rank UI doesn't
		// reflect tier changes because the game treats the player as unaligned.
		// neutral_faction_id = 3 ("None") so this proc takes the upsert branch.
		if _, err = tx.Exec(ctx,
			`SELECT dune.change_player_faction($1::bigint, $2::smallint, 3::smallint, NOW()::timestamp)`,
			controllerID, factionID); err != nil {
			return msgMutate{err: fmt.Errorf("change_player_faction: %w", err)}
		}

		if _, err = tx.Exec(ctx,
			`SELECT dune.update_player_tags($1, $2::text[], '{}'::text[])`, accountID, allTags); err != nil {
			return msgMutate{err: fmt.Errorf("update player tags: %w", err)}
		}

		// +1 over the tier threshold: the game UI floors at the threshold
		// (rep == threshold shows the tier below), so we nudge just over.
		targetRep := factionTierThresholds[targetTier] + 1
		if _, err = tx.Exec(ctx,
			`SELECT dune.set_player_faction_reputation($1, $2, $3)`,
			controllerID, factionID, targetRep); err != nil {
			return msgMutate{err: fmt.Errorf("set faction rep: %w", err)}
		}
		if _, err = tx.Exec(ctx, factionPlayerComponentRepSQL,
			controllerID, factionName, targetRep); err != nil {
			return msgMutate{err: fmt.Errorf("update FactionPlayerComponent rep: %w", err)}
		}

		if err := tx.Commit(ctx); err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Progression unlock (%s/%s): %d journey nodes completed + %s tier tags 0–%d + rep tier %d on controller %d — takes effect on next login",
			preset, faction, len(journeyNodes), factionName, maxTier, targetTier, controllerID)}
	}
}

// cmdReverseProgressionUnlock undoes cmdProgressionUnlock: resets the journey
// nodes from nodesForPreset back to not-complete and removes all tags the
// forward function wrote. Reputation and faction alignment are not touched —
// matching the existing per-node reset behaviour.
func cmdReverseProgressionUnlock(actorID int64, faction, preset string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		var factionID int16
		var dialogueFlag, alignedFlag, metRecruiterFlag, factionUnlocked, recruitmentDone string
		switch faction {
		case "atreides":
			factionID = 1
			dialogueFlag = "DialogueFlags.Factions.SentToMeetHawat"
			alignedFlag = "DialogueFlags.Factions.AlignedAtreides"
			metRecruiterFlag = "DialogueFlags.Factions.MetHawat"
			factionUnlocked = "Contract.Tracking.AtreidesFactionUnlocked"
			recruitmentDone = "Contract.Tracking.AtreidesRecruitmentCompleted"
		case "harkonnen":
			factionID = 2
			dialogueFlag = "DialogueFlags.Factions.SentToPiterDeVries"
			alignedFlag = "DialogueFlags.Factions.AlignedHarkonnen"
			metRecruiterFlag = "DialogueFlags.Factions.MetPiterDeVries"
			factionUnlocked = "Contract.Tracking.HarkonnenFactionUnlocked"
			recruitmentDone = "Contract.Tracking.HarkonnenRecruitmentCompleted"
		default:
			return msgMutate{err: fmt.Errorf("faction must be atreides or harkonnen")}
		}

		var targetTier int
		switch preset {
		case "ch3_start":
			targetTier = 5
		case "rank19_eligible":
			targetTier = 19
		default:
			return msgMutate{err: fmt.Errorf("preset must be ch3_start or rank19_eligible")}
		}

		ctx := context.Background()

		var accountID int64
		if err := globalDB.QueryRow(ctx,
			`SELECT COALESCE(owner_account_id, 0) FROM dune.actors WHERE id = $1`,
			actorID).Scan(&accountID); err != nil || accountID == 0 {
			return msgMutate{err: fmt.Errorf("player %d not found or has no account", actorID)}
		}

		// Build the exact same tag set the forward function writes so we can remove it.
		factionName := factionDisplayName(factionID)
		const maxTier = 5
		allTags := []string{
			dialogueFlag, alignedFlag, metRecruiterFlag,
			factionUnlocked, recruitmentDone,
			"DialogueFlags.Factions.FactionIntro",
			"DialogueFlags.Factions.FactionRank1",
			"DialogueFlags.Factions.FactionRank3",
			"DialogueFlags.Factions.MetARecruiter",
			"DialogueFlags.Factions.PlayedAllegianceCinematic",
			"DialogueFlags.Factions.SeenAnvilCinematic",
		}
		if targetTier >= 19 {
			allTags = append(allTags, "Journey.LandsraadContractsUnlocked")
		}
		for t := 0; t <= maxTier; t++ {
			allTags = append(allTags, fmt.Sprintf("Faction.%s.Tier%d", factionName, t))
		}

		// Also collect any tags the journey nodes themselves emit on completion.
		nodes := nodesForPreset(faction, preset)
		seen := map[string]bool{}
		for _, t := range allTags {
			seen[t] = true
		}
		for _, node := range nodes {
			for _, t := range tagsForJourneyNodeSubtree(node) {
				if !seen[t] {
					seen[t] = true
					allTags = append(allTags, t)
				}
			}
		}

		tx, err := globalDB.Begin(ctx)
		if err != nil {
			return msgMutate{err: err}
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err = tx.Exec(ctx,
			`SELECT dune.update_player_tags($1, '{}'::text[], $2::text[])`,
			accountID, allTags); err != nil {
			return msgMutate{err: fmt.Errorf("remove tags: %w", err)}
		}

		result, err := tx.Exec(ctx, `
			UPDATE dune.journey_story_node
			SET complete_condition_state = 'false'::jsonb,
			    has_pending_reward       = false
			WHERE account_id = $1
			  AND story_node_id = ANY($2::text[])`,
			accountID, nodes)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset journey nodes: %w", err)}
		}

		if err := tx.Commit(ctx); err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Reversed progression unlock (%s/%s): reset %d node(s), removed %d tag(s) — takes effect on next login",
			preset, faction, result.RowsAffected(), len(allTags))}
	}
}

func cmdDeleteAllTutorials(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		_, err := globalDB.Exec(context.Background(),
			`SELECT dune.delete_all_tutorial_entries($1)`, playerID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("delete tutorials: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Deleted all tutorial entries for player %d", playerID)}
	}
}

func cmdWipeCodex(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		_, err := globalDB.Exec(context.Background(),
			`SELECT dune.delete_mnemonic_recall_lesson_all($1)`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("wipe codex: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Wiped all codex entries for account %d", accountID)}
	}
}

const maxCharXP = int64(344440) // XP required for level 200 (hard cap)

// cumulativeXPByLevel[i] = total XP needed to reach level i (from SkillXPPerLevel.json).
var cumulativeXPByLevel = [201]int64{
	0, 40, 215, 440, 740, 1240, 1790, 2390, 2990, 3590, 4190, // 0-10
	4790, 5390, 5990, 6590, 7190, 7790, 8390, 8990, 9590, 10190, // 11-20
	10790, 11390, 11990, 12590, 13190, 13790, 14390, 14990, 15590, 16190, // 21-30
	16790, 17390, 17990, 18590, 19190, 19790, 20390, 20990, 21590, 22190, // 31-40
	22790, 23390, 23990, 24590, 25190, 25790, 26390, 26990, 27590, 28190, // 41-50
	28790, 29390, 29990, 30590, 31190, 31790, 32390, 32990, 33590, 34190, // 51-60
	34790, 35390, 35990, 36590, 37190, 37790, 38390, 38990, 39590, 40190, // 61-70
	40790, 41390, 41990, 42590, 43190, 43790, 44390, 44990, 45590, 46190, // 71-80
	46790, 47390, 47990, 48590, 49190, 49790, 50390, 50990, 51590, 52190, // 81-90
	52790, 53390, 53990, 54590, 55190, 55790, 56390, 56990, 57590, 58190, // 91-100
	58840, 59490, 60140, 60790, 61440, 62090, 62740, 63390, 64040, 64690, // 101-110
	65340, 65990, 66640, 67290, 67940, 68590, 69240, 69890, 70540, 71190, // 111-120
	71840, 72490, 73140, 73790, 74440, 75090, 75740, 76391, 77044, 77699, // 121-130
	78357, 79018, 79683, 80353, 81030, 81714, 82407, 83110, 83825, 84554, // 131-140
	85298, 86060, 86842, 87646, 88475, 89332, 90220, 91141, 92100, 93099, // 141-150
	94143, 95235, 96380, 97582, 98845, 100175, 101576, 103054, 104614, 106263, // 151-160
	108006, 109849, 111799, 113862, 116046, 118358, 120806, 123397, 126139, 129041, // 161-170
	132112, 135360, 138795, 142426, 146263, 150316, 154596, 159114, 163880, 168906, // 171-180
	174203, 179784, 185661, 191846, 198353, 205195, 212385, 219938, 227868, 236190, // 181-190
	244918, 254069, 263657, 273700, 284213, 295214, 306719, 318746, 331314, 344440, // 191-200
}

// xpToLevel returns the character level for the given cumulative XP (1–200).
func xpToLevel(xp int64) int {
	if xp <= 0 {
		return 0
	}
	lo, hi := 1, 200
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if cumulativeXPByLevel[mid] <= xp {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// intelAtLevel returns cumulative intel points earned through a given level.
// Based on IntelPointsRewarded curve in SkillXPPerLevel.json:
//
//	L1=4, L2-3=+2, L4-15=+3, L16-30=+5, L31-50=+10,
//	L51-69=+20, L70-85=+30, L86-125=+40, L126+=0 (cap 2779)
func intelAtLevel(level int) int64 {
	switch {
	case level <= 0:
		return 0
	case level == 1:
		return 4
	case level <= 3:
		return 4 + int64(level-1)*2
	case level <= 15:
		return 8 + int64(level-3)*3
	case level <= 30:
		return 44 + int64(level-15)*5
	case level <= 50:
		return 119 + int64(level-30)*10
	case level <= 69:
		return 319 + int64(level-50)*20
	case level <= 85:
		return 699 + int64(level-69)*30
	case level <= 125:
		return 1179 + int64(level-85)*40
	default:
		return 2779
	}
}

// checkPlayerOffline returns an error if the player is currently online.
// playerID is the pawn actor ID (PlayerCharacter).
func checkPlayerOffline(ctx context.Context, playerID int64) error {
	var status string
	err := globalDB.QueryRow(ctx, `
		SELECT online_status::text FROM dune.player_state
		WHERE player_pawn_id = $1`, playerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		// No player_state row means the player has never connected or their
		// session record was cleaned up — treat as offline.
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not check online status: %w", err)
	}
	if status != "Offline" {
		return fmt.Errorf("player is currently %s — log out first, then apply the edit", status)
	}
	return nil
}

type msgCharXP struct {
	xp    int64
	level int
	err   error
}

func cmdFetchCharXP(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgCharXP{err: fmt.Errorf("not connected")}
		}
		var xp int64
		err := globalDB.QueryRow(context.Background(), `
			SELECT COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.actor_id = $1 AND afe.slot_name = 'DuneCharacter'`, playerID).Scan(&xp)
		if err != nil {
			return msgCharXP{err: fmt.Errorf("read char xp: %w", err)}
		}
		return msgCharXP{xp: xp, level: xpToLevel(xp)}
	}
}

func cmdAwardCharXP(playerID int64, amount int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgMutate{err: err}
		}

		// Read current XP and count of skill points already spent in the tree.
		var currentXP, spentSP int64
		err := globalDB.QueryRow(ctx, `
			SELECT
				(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
				COALESCE((
					SELECT SUM((v->>'SkillPointsSpent')::int)
					FROM jsonb_each(fe.components->'FLevelComponent'->1->'ModuleData') AS kv(k, v)
					WHERE k != format('(TagName="%s")',
						fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName')
				), 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.actor_id = $1 AND afe.slot_name = 'DuneCharacter'`, playerID).Scan(&currentXP, &spentSP)
		if err != nil {
			return msgMutate{err: fmt.Errorf("read current state: %w", err)}
		}

		// Resolve controller id from the pawn id so we can read purchased
		// keystones (which are keyed by controller id). A missing player_state
		// row means no keystones could have been purchased — treat bonus as 0.
		var keystoneBonus int64
		var controllerID int64
		err = globalDB.QueryRow(ctx, `
			SELECT player_controller_id FROM dune.player_state
			WHERE player_pawn_id = $1 LIMIT 1`, playerID).Scan(&controllerID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return msgMutate{err: fmt.Errorf("resolve controller id: %w", err)}
		}
		if controllerID != 0 {
			rows, err := globalDB.Query(ctx, `
				SELECT keystone_id FROM dune.purchased_specialization_keystones
				WHERE player_id = $1::bigint`, controllerID)
			if err != nil {
				return msgMutate{err: fmt.Errorf("read keystones: %w", err)}
			}
			var ids []int16
			for rows.Next() {
				var id int16
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return msgMutate{err: fmt.Errorf("scan keystone: %w", err)}
				}
				ids = append(ids, id)
			}
			rows.Close()
			keystoneBonus = keystoneSPBonus(ids)
		}

		newXP := currentXP + amount
		if newXP > maxCharXP {
			newXP = maxCharXP
		}
		newLevel := int64(xpToLevel(newXP))
		newTotalSP := newLevel + keystoneBonus
		// Starter job always occupies 1 SP that is excluded from spentSP.
		newUnspentSP := newTotalSP - spentSP - 1
		if newUnspentSP < 0 {
			newUnspentSP = 0
		}
		newIntel := intelAtLevel(int(newLevel))

		// Update FLevelComponent: XP + both skill point fields.
		_, err = globalDB.Exec(ctx, `
			UPDATE dune.fgl_entities
			SET components = jsonb_set(jsonb_set(jsonb_set(
				components,
				'{FLevelComponent,1,TotalXPEarned}',    to_jsonb($2::bigint)),
				'{FLevelComponent,1,TotalSkillPoints}',  to_jsonb($3::bigint)),
				'{FLevelComponent,1,UnspentSkillPoints}', to_jsonb($4::bigint))
			WHERE entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)`, playerID, newXP, newTotalSP, newUnspentSP)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update fgl xp/sp: %w", err)}
		}

		// Update intel points on the PlayerCharacter actor.
		_, err = globalDB.Exec(ctx, `
			UPDATE dune.actors
			SET properties = jsonb_set(
				properties,
				'{TechKnowledgePlayerComponent,m_TechKnowledgePoints}',
				to_jsonb($2::bigint))
			WHERE id = $1 AND properties ? 'TechKnowledgePlayerComponent'`,
			playerID, newIntel)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update intel: %w", err)}
		}

		capped := ""
		if newXP == maxCharXP {
			capped = " (capped at level 200)"
		}
		return msgMutate{ok: fmt.Sprintf(
			"Player %d → level %d%s | XP %d | SP %d unspent (%d spent) | Intel %d",
			playerID, newLevel, capped, newXP, newUnspentSP, spentSP, newIntel)}
	}
}

func cmdAwardIntel(playerID int64, amount int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgMutate{err: err}
		}
		res, err := globalDB.Exec(ctx, `
			UPDATE dune.actors
			SET properties = jsonb_set(
				properties,
				'{TechKnowledgePlayerComponent,m_TechKnowledgePoints}',
				to_jsonb((properties->'TechKnowledgePlayerComponent'->>'m_TechKnowledgePoints')::bigint + $2)
			)
			WHERE id = $1
			  AND properties ? 'TechKnowledgePlayerComponent'`, playerID, amount)
		if err != nil {
			return msgMutate{err: fmt.Errorf("award intel: %w", err)}
		}
		if res.RowsAffected() == 0 {
			return msgMutate{err: fmt.Errorf("TechKnowledgePlayerComponent not found for player %d — ensure player is a PlayerCharacter actor", playerID)}
		}
		return msgMutate{ok: fmt.Sprintf("Awarded %d intel points to player %d", amount, playerID)}
	}
}

// ── blueprint JSON types ──────────────────────────────────────────────────────

type blueprintInstance struct {
	InstanceID        *int    `json:"instance_id,omitempty"`
	BuildingType      string  `json:"building_type"`
	X                 float64 `json:"x"`
	Y                 float64 `json:"y"`
	Z                 float64 `json:"z"`
	Rotation          float64 `json:"rotation"`
	ProvidesStability *bool   `json:"provides_stability,omitempty"`
}

type blueprintPlaceable struct {
	PlaceableID  *int    `json:"placeable_id,omitempty"`
	BuildingType string  `json:"building_type"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Z            float64 `json:"z"`
	RX           float64 `json:"rx,omitempty"`
	RY           float64 `json:"ry"`
	RZ           float64 `json:"rz,omitempty"`
}

type blueprintPentashield struct {
	PlaceableID int    `json:"placeable_id"`
	Scale       [3]int `json:"scale"` // [width, height, depth] stored as SMALLINT[3]
}

type blueprintFile struct {
	Name         string                 `json:"name,omitempty"`
	Instances    []blueprintInstance    `json:"instances"`
	Placeables   []blueprintPlaceable   `json:"placeables"`
	Pentashields []blueprintPentashield `json:"pentashields,omitempty"`
}

// ── blueprint commands ────────────────────────────────────────────────────────

func cmdListBlueprints() Msg {
	if globalDB == nil {
		return msgBlueprintList{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT bb.id,
		       COALESCE(ps.character_name, '') AS owner,
		       COALESCE(bb.item_id, 0),
		       COALESCE(inst.cnt, 0) AS pieces,
		       COALESCE(plac.cnt, 0) AS placeables,
		       COALESCE(i.stats->'FBuildingBlueprintItemStats'->1->>'BuildingBlueprintName', '') AS name
		FROM dune.building_blueprints bb
		LEFT JOIN dune.items i ON i.id = bb.item_id
		LEFT JOIN dune.inventories inv ON inv.id = i.inventory_id
		LEFT JOIN dune.actors a ON a.id = inv.actor_id
		LEFT JOIN dune.player_state ps ON ps.player_pawn_id = a.id
		LEFT JOIN (
		    SELECT building_blueprint_id, COUNT(*) AS cnt
		    FROM dune.building_blueprint_instances
		    GROUP BY building_blueprint_id
		) inst ON inst.building_blueprint_id = bb.id
		LEFT JOIN (
		    SELECT building_blueprint_id, COUNT(*) AS cnt
		    FROM dune.building_blueprint_placeables
		    GROUP BY building_blueprint_id
		) plac ON plac.building_blueprint_id = bb.id
		ORDER BY bb.id`)
	if err != nil {
		return msgBlueprintList{err: err}
	}
	defer rows.Close()
	var out []blueprintRow
	for rows.Next() {
		var r blueprintRow
		if err := rows.Scan(&r.ID, &r.OwnerName, &r.ItemID, &r.Pieces, &r.Placeables, &r.Name); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgBlueprintList{err: err}
	}
	return msgBlueprintList{rows: out}
}

func cmdGrantMaxSpec(playerID int64, trackType string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		_, err := globalDB.Exec(context.Background(),
			`SELECT dune.set_specialization_xp_and_level($1, $2::dune.specializationtracktype, $3, $4)`,
			playerID, trackType, 44182, 100.0)
		if err != nil {
			return msgMutate{err: err}
		}
		return msgMutate{ok: fmt.Sprintf("Granted max %s spec to player %d", trackType, playerID)}
	}
}

func cmdFetchPlayerSpecs(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgSpecs{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT player_id, track_type::text, xp_amount, level
			FROM dune.specialization_tracks
			WHERE player_id = $1::bigint
			ORDER BY track_type`, playerID)
		if err != nil {
			return msgSpecs{err: err}
		}
		defer rows.Close()
		var out []specTrack
		for rows.Next() {
			var r specTrack
			if err := rows.Scan(&r.PlayerID, &r.TrackType, &r.XP, &r.Level); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgSpecs{err: err}
		}
		return msgSpecs{rows: out}
	}
}

// keystoneSPBonus returns the total extra skill points granted by a set of keystone IDs.
// SkillPoint = +1, SkillPoint_Major = +3, SkillPoint_Super = +5 (Combat track only).
func keystoneSPBonus(ids []int16) int64 {
	var total int64
	for _, id := range ids {
		info, ok := keystoneMap[id]
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(info.Name, "_SkillPoint_Super"):
			total += 5
		case strings.HasSuffix(info.Name, "_SkillPoint_Major"):
			total += 3
		case strings.HasSuffix(info.Name, "_SkillPoint"):
			total += 1
		}
	}
	return total
}

func cmdGrantAllKeystones(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgMutate{err: err}
		}

		var err error
		_, err = globalDB.Exec(ctx, `
			INSERT INTO dune.purchased_specialization_keystones (player_id, keystone_id)
			SELECT $1::bigint, generate_series(1, 205)
			ON CONFLICT DO NOTHING`, playerID)
		if err != nil {
			return msgMutate{err: err}
		}

		// Compute the SP bonus all 205 purchased keystones should give.
		allIDs := make([]int16, 205)
		for i := range allIDs {
			allIDs[i] = int16(i + 1)
		}
		keystoneBonus := keystoneSPBonus(allIDs)

		// Read XP, current TotalSkillPoints, and SP spent in non-starter modules.
		// Uses pawn actor id (purchased_specialization_keystones uses controller id).
		var xp, currentTotal, spentSP int64
		err = globalDB.QueryRow(ctx, `
			SELECT
				(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
				(fe.components->'FLevelComponent'->1->>'TotalSkillPoints')::bigint,
				COALESCE((
					SELECT SUM((v->>'SkillPointsSpent')::int)
					FROM jsonb_each(fe.components->'FLevelComponent'->1->'ModuleData') AS kv(k, v)
					WHERE k != format('(TagName="%s")',
						fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName')
				), 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.slot_name = 'DuneCharacter'
			  AND afe.actor_id = (
				SELECT player_pawn_id FROM dune.player_state
				WHERE player_controller_id = $1 LIMIT 1
			  )`, playerID).Scan(&xp, &currentTotal, &spentSP)
		if err != nil {
			return msgMutate{err: fmt.Errorf("read FLevelComponent: %w", err)}
		}

		level := int64(xpToLevel(xp))
		expectedTotal := level + keystoneBonus
		// UnspentSkillPoints = total - non-starter spent - 1 (starter job always occupies 1 SP).
		expectedUnspent := expectedTotal - spentSP - 1
		if expectedUnspent < 0 {
			expectedUnspent = 0
		}

		if currentTotal >= expectedTotal {
			return msgMutate{ok: fmt.Sprintf(
				"Granted all keystones to player %d — SP already correct (%d total, %d unspent)",
				playerID, currentTotal, expectedUnspent)}
		}

		_, err = globalDB.Exec(ctx, `
			UPDATE dune.fgl_entities
			SET components = jsonb_set(jsonb_set(
				components,
				'{FLevelComponent,1,TotalSkillPoints}',
				to_jsonb($2::bigint)),
				'{FLevelComponent,1,UnspentSkillPoints}',
				to_jsonb($3::bigint))
			WHERE entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE slot_name = 'DuneCharacter'
				  AND actor_id = (
					SELECT player_pawn_id FROM dune.player_state
					WHERE player_controller_id = $1 LIMIT 1
				  )
			)`, playerID, expectedTotal, expectedUnspent)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update skill points: %w", err)}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Granted all keystones to player %d — SP %d → %d total, %d unspent (+%d keystone bonus)",
			playerID, currentTotal, expectedTotal, expectedUnspent, keystoneBonus)}
	}
}

// cmdResetAllKeystones is the inverse of cmdGrantAllKeystones: it deletes all
// purchased keystones and rolls TotalSkillPoints/UnspentSkillPoints back to
// the XP-derived baseline (no keystone bonus). Requires the player to be offline.
func cmdResetAllKeystones(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		ctx := context.Background()

		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgMutate{err: err}
		}

		// Read XP, current total SP, and non-starter spent SP — same query as
		// cmdGrantAllKeystones so the arithmetic is symmetric.
		var xp, currentTotal, spentSP int64
		err := globalDB.QueryRow(ctx, `
			SELECT
				(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
				(fe.components->'FLevelComponent'->1->>'TotalSkillPoints')::bigint,
				COALESCE((
					SELECT SUM((v->>'SkillPointsSpent')::int)
					FROM jsonb_each(fe.components->'FLevelComponent'->1->'ModuleData') AS kv(k, v)
					WHERE k != format('(TagName="%s")',
						fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName')
				), 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.slot_name = 'DuneCharacter'
			  AND afe.actor_id = (
				SELECT player_pawn_id FROM dune.player_state
				WHERE player_controller_id = $1 LIMIT 1
			  )`, playerID).Scan(&xp, &currentTotal, &spentSP)
		if err != nil {
			return msgMutate{err: fmt.Errorf("read FLevelComponent: %w", err)}
		}

		if _, err := globalDB.Exec(ctx,
			`DELETE FROM dune.purchased_specialization_keystones WHERE player_id = $1`,
			playerID); err != nil {
			return msgMutate{err: fmt.Errorf("delete keystones: %w", err)}
		}

		level := int64(xpToLevel(xp))
		newTotal := level
		newUnspent := newTotal - spentSP - 1
		if newUnspent < 0 {
			newUnspent = 0
		}

		if _, err := globalDB.Exec(ctx, `
			UPDATE dune.fgl_entities
			SET components = jsonb_set(jsonb_set(
				components,
				'{FLevelComponent,1,TotalSkillPoints}',
				to_jsonb($2::bigint)),
				'{FLevelComponent,1,UnspentSkillPoints}',
				to_jsonb($3::bigint))
			WHERE entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE slot_name = 'DuneCharacter'
				  AND actor_id = (
					SELECT player_pawn_id FROM dune.player_state
					WHERE player_controller_id = $1 LIMIT 1
				  )
			)`, playerID, newTotal, newUnspent); err != nil {
			return msgMutate{err: fmt.Errorf("update skill points: %w", err)}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Reset all keystones for player %d — SP %d → %d total, %d unspent",
			playerID, currentTotal, newTotal, newUnspent)}
	}
}

func cmdFetchPlayerKeystones(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgKeystones{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT keystone_id FROM dune.purchased_specialization_keystones
			WHERE player_id = $1::bigint ORDER BY keystone_id`, playerID)
		if err != nil {
			return msgKeystones{err: err}
		}
		defer rows.Close()
		var ids []int16
		for rows.Next() {
			var id int16
			if err := rows.Scan(&id); err != nil {
				continue
			}
			ids = append(ids, id)
		}
		return msgKeystones{ids: ids}
	}
}

func cmdGetPlayerVehicles(controllerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgVehicles{err: fmt.Errorf("not connected")}
		}
		// Look up account_id from controller_id — vehicle actors don't use owner_account_id.
		var accountID int64
		err := globalDB.QueryRow(context.Background(),
			`SELECT ps.account_id FROM dune.player_state ps WHERE ps.player_controller_id = $1 LIMIT 1`,
			controllerID).Scan(&accountID)
		if err != nil {
			return msgVehicles{err: fmt.Errorf("look up account: %w", err)}
		}

		rows, err := globalDB.Query(context.Background(), `
			SELECT pa.actor_id, a.class, COALESCE(a.map, ''),
			       COALESCE(rv.chassis_durability::float8, 1.0),
			       COALESCE(pa.actor_name, rv.vehicle_name, ''),
			       (rv.vehicle_id IS NOT NULL) AS is_recovered,
			       false AS is_backup
			FROM dune.permission_actor pa
			JOIN dune.permission_actor_rank par ON par.permission_actor_id = pa.actor_id
			JOIN dune.actors a ON a.id = pa.actor_id
			LEFT JOIN dune.recovered_vehicles rv ON rv.vehicle_id = pa.actor_id AND rv.account_id = $2
			WHERE par.player_id = $1 AND pa.actor_type = 2

			UNION ALL

			SELECT a.id, a.class, '' AS map,
			       1.0 AS chassis_durability,
			       '' AS vehicle_name,
			       false AS is_recovered,
			       true AS is_backup
			FROM dune.backup_vehicles bv
			JOIN dune.actors a ON a.id = bv.vehicle_id
			WHERE bv.account_id = $2

			ORDER BY class`, controllerID, accountID)
		if err != nil {
			return msgVehicles{err: err}
		}
		defer rows.Close()
		var out []vehicleRow
		for rows.Next() {
			var r vehicleRow
			if err := rows.Scan(&r.ID, &r.Class, &r.Map, &r.ChassisDurability, &r.VehicleName, &r.IsRecovered, &r.IsBackup); err != nil {
				continue
			}
			r.Class = shortClass(r.Class)
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgVehicles{err: err}
		}
		return msgVehicles{rows: out}
	}
}

func cmdRepairItem(itemID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		// Derive owning player from item → inventory → actor (pawn) so we can gate Offline.
		var pawnID int64
		err := globalDB.QueryRow(ctx, `
			SELECT inv.actor_id
			FROM dune.items i
			JOIN dune.inventories inv ON inv.id = i.inventory_id
			WHERE i.id = $1::bigint`, itemID).Scan(&pawnID)
		if errors.Is(err, pgx.ErrNoRows) {
			return msgMutate{err: fmt.Errorf("item %d not found", itemID)}
		}
		if err != nil {
			return msgMutate{err: fmt.Errorf("look up item owner: %w", err)}
		}
		if err := checkPlayerOffline(ctx, pawnID); err != nil {
			return msgMutate{err: err}
		}

		// Write both fields: Current-only gets clamped to surviving Decayed on reload.
		// Fallback target = 100.0 covers the 0-100 gear scale when MaxDurability is absent.
		res, err := globalDB.Exec(ctx, `
			UPDATE dune.items i
			SET stats = jsonb_set(
				jsonb_set(i.stats,
					'{FItemStackAndDurabilityStats,1,CurrentDurability}',
					to_jsonb(t.val), true),
				'{FItemStackAndDurabilityStats,1,DecayedMaxDurability}',
				to_jsonb(t.val), true)
			FROM (
				SELECT COALESCE(
					(stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability')::float8,
					100.0
				) AS val
				FROM dune.items
				WHERE id = $1::bigint
				  AND stats ? 'FItemStackAndDurabilityStats'
			) AS t
			WHERE i.id = $1::bigint
			  AND (
				abs(COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability')::float8, 0) - t.val) > 0.01
				OR abs(COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'DecayedMaxDurability')::float8, 0) - t.val) > 0.01
			  )`, itemID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("repair item: %w", err)}
		}
		if res.RowsAffected() == 0 {
			// Item exists (owner lookup succeeded). Either no durability field, or already at ceiling.
			var hasDur bool
			if err := globalDB.QueryRow(ctx, `
				SELECT stats ? 'FItemStackAndDurabilityStats'
				FROM dune.items WHERE id = $1::bigint`, itemID).Scan(&hasDur); err != nil {
				return msgMutate{err: fmt.Errorf("check item: %w", err)}
			}
			if !hasDur {
				return msgMutate{err: fmt.Errorf("item %d has no durability field", itemID)}
			}
			return msgMutate{ok: fmt.Sprintf("Item %d already at full durability", itemID)}
		}
		return msgMutate{ok: fmt.Sprintf("Repaired item %d — relog to see in-game", itemID)}
	}
}

// Carried inventories: backpack, equipment, emote wheel, equipped weapons, action wheel, bank.
var repairGearInventoryTypes = []int32{0, 1, 14, 15, 27, 30}

func cmdRepairPlayerGear(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgRepairGear{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgRepairGear{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgRepairGear{err: err}
		}

		rows, err := globalDB.Query(ctx, `
			SELECT i.id, i.template_id,
			       (i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'),
			       (i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'),
			       (i.stats->'FItemStackAndDurabilityStats'->1->>'DecayedMaxDurability')
			FROM dune.items i
			JOIN dune.inventories inv ON inv.id = i.inventory_id
			WHERE inv.actor_id = $1::bigint
			  AND inv.inventory_type = ANY($2::int[])
			  AND i.stats ? 'FItemStackAndDurabilityStats'`,
			playerID, repairGearInventoryTypes)
		if err != nil {
			return msgRepairGear{err: fmt.Errorf("scan items: %w", err)}
		}
		defer rows.Close()

		type repairCandidate struct {
			id     int64
			target float64
		}
		var toRepair []repairCandidate
		scanned := 0
		for rows.Next() {
			scanned++
			var id int64
			var templateID string
			var maxStr, curStr, decayedStr pgtype.Text
			if err := rows.Scan(&id, &templateID, &maxStr, &curStr, &decayedStr); err != nil {
				return msgRepairGear{scanned: scanned, err: fmt.Errorf("scan item: %w", err)}
			}

			// Target priority: catalog (vehicle modules stored as items) → stats.MaxDurability → 100.
			var target float64
			if v, ok := itemMaxDurability(templateID); ok && v > 0 {
				target = v
			} else if maxStr.Valid {
				if v, perr := strconv.ParseFloat(maxStr.String, 64); perr == nil && v > 0 {
					target = v
				} else {
					target = 100.0
				}
			} else {
				target = 100.0
			}

			cur := 0.0
			if curStr.Valid {
				cur, _ = strconv.ParseFloat(curStr.String, 64)
			}
			decayed := 0.0
			if decayedStr.Valid {
				decayed, _ = strconv.ParseFloat(decayedStr.String, 64)
			}
			if math.Abs(cur-target) < 0.01 && math.Abs(decayed-target) < 0.01 {
				continue
			}
			toRepair = append(toRepair, repairCandidate{id: id, target: target})
		}
		if err := rows.Err(); err != nil {
			return msgRepairGear{err: fmt.Errorf("scan rows: %w", err)}
		}

		tx, err := globalDB.Begin(ctx)
		if err != nil {
			return msgRepairGear{scanned: scanned, err: fmt.Errorf("begin tx: %w", err)}
		}
		defer func() { _ = tx.Rollback(ctx) }()

		repaired := 0
		for _, rc := range toRepair {
			_, err := tx.Exec(ctx, `
				UPDATE dune.items
				SET stats = jsonb_set(
					jsonb_set(stats,
						'{FItemStackAndDurabilityStats,1,CurrentDurability}',
						to_jsonb($2::float8), true),
					'{FItemStackAndDurabilityStats,1,DecayedMaxDurability}',
					to_jsonb($2::float8), true)
				WHERE id = $1::bigint`, rc.id, rc.target)
			if err != nil {
				return msgRepairGear{repaired: repaired, scanned: scanned, err: fmt.Errorf("repair item %d: %w", rc.id, err)}
			}
			repaired++
		}
		if err := tx.Commit(ctx); err != nil {
			return msgRepairGear{scanned: scanned, err: fmt.Errorf("commit: %w", err)}
		}
		return msgRepairGear{repaired: repaired, scanned: scanned}
	}
}

func cmdRepairVehicle(playerID, vehicleID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgRepairVehicle{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgRepairVehicle{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgRepairVehicle{err: err}
		}

		rows, err := globalDB.Query(ctx, `
			SELECT id, template_id
			FROM dune.vehicle_modules
			WHERE vehicle_id = $1::bigint`, vehicleID)
		if err != nil {
			return msgRepairVehicle{err: fmt.Errorf("scan modules: %w", err)}
		}
		defer rows.Close()

		type moduleRow struct {
			id         int64
			templateID string
		}
		var modules []moduleRow
		for rows.Next() {
			var m moduleRow
			if err := rows.Scan(&m.id, &m.templateID); err != nil {
				return msgRepairVehicle{err: fmt.Errorf("scan module: %w", err)}
			}
			modules = append(modules, m)
		}
		if err := rows.Err(); err != nil {
			return msgRepairVehicle{err: err}
		}

		total := len(modules)
		repaired := 0
		skipped := 0
		for _, m := range modules {
			target, ok := itemMaxDurability(m.templateID)
			if !ok || target <= 0 {
				skipped++
				continue
			}
			// Both fields must be written — Current-only is clamped to surviving Decayed on reload.
			_, err := globalDB.Exec(ctx, `
				UPDATE dune.vehicle_modules
				SET stats = jsonb_set(
					jsonb_set(stats,
						'{FVehicleModuleDurabilityStats,1,CurrentDurability}',
						to_jsonb($2::float8), true),
					'{FVehicleModuleDurabilityStats,1,DecayedMaxDurability}',
					to_jsonb($2::float8), true)
				WHERE id = $1::bigint`, m.id, target)
			if err != nil {
				return msgRepairVehicle{repaired: repaired, skipped: skipped, total: total, err: fmt.Errorf("repair module %d: %w", m.id, err)}
			}
			repaired++
		}
		return msgRepairVehicle{repaired: repaired, skipped: skipped, total: total}
	}
}

func cmdRefuelVehicle(playerID, vehicleID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		if err := checkPlayerOffline(ctx, playerID); err != nil {
			return msgMutate{err: err}
		}

		// Each vehicle BP has its own properties bag keyed by the class basename
		// (e.g. "BP_Sandbike_CHOAM_C"), so we derive it before writing m_InitialFuel.
		var class string
		err := globalDB.QueryRow(ctx, `SELECT class FROM dune.actors WHERE id = $1::bigint`, vehicleID).Scan(&class)
		if errors.Is(err, pgx.ErrNoRows) {
			return msgMutate{err: fmt.Errorf("vehicle %d not found", vehicleID)}
		}
		if err != nil {
			return msgMutate{err: fmt.Errorf("look up vehicle class: %w", err)}
		}
		bpClass := class
		if idx := strings.LastIndex(class, "."); idx >= 0 {
			bpClass = class[idx+1:]
		}

		_, err = globalDB.Exec(ctx, `
			UPDATE dune.actors
			SET properties = jsonb_set(
				COALESCE(properties, '{}'::jsonb),
				ARRAY[$2::text, 'm_InitialFuel'],
				to_jsonb(1.0::float8),
				true)
			WHERE id = $1::bigint`, vehicleID, bpClass)
		if err != nil {
			return msgMutate{err: fmt.Errorf("refuel vehicle: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Refueled vehicle %d — relog to see in-game", vehicleID)}
	}
}

func cmdFetchCheatLog() Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgCheatLog{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT ct.fls_id, ct.cheat_type::text,
			       to_char(ct.event_time AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
			       COALESCE(ps.character_name, ct.fls_id)
			FROM dune.cheater_tracking ct
			LEFT JOIN dune.encrypted_accounts e ON convert_from(e.encrypted_funcom_id, 'UTF8') = ct.fls_id
			LEFT JOIN dune.player_state ps ON ps.account_id = e.id
			WHERE ct.event_time > NOW() - INTERVAL '7 days'
			ORDER BY ct.event_time DESC
			LIMIT 500`)
		if err != nil {
			return msgCheatLog{err: err}
		}
		defer rows.Close()
		var out []cheatEntry
		for rows.Next() {
			var r cheatEntry
			if err := rows.Scan(&r.FLSID, &r.CheatType, &r.EventTime, &r.CharacterName); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgCheatLog{err: err}
		}
		return msgCheatLog{rows: out}
	}
}

func cmdFetchEventLog(actorID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgEvents{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT actor_id,
			       to_char(universe_time AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
			       COALESCE(map, ''),
			       event_type,
			       COALESCE(x, 0)::float8, COALESCE(y, 0)::float8, COALESCE(z, 0)::float8,
			       COALESCE(custom_data::text, '{}')
			FROM dune.game_events
			WHERE actor_id = $1::bigint AND player_facing_event = true
			ORDER BY universe_time DESC
			LIMIT 200`, actorID)
		if err != nil {
			return msgEvents{err: err}
		}
		defer rows.Close()
		var out []gameEvent
		for rows.Next() {
			var r gameEvent
			if err := rows.Scan(&r.ActorID, &r.UniverseTime, &r.Map, &r.EventType, &r.X, &r.Y, &r.Z, &r.CustomData); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgEvents{err: err}
		}
		return msgEvents{rows: out}
	}
}

func cmdFetchPlayerDungeons(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgDungeons{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT dc.dungeon_id, dc.difficulty::text, dc.duration_ms, dc.players_num, dc.completion_id
			FROM dune.dungeon_completion_players dcp
			JOIN dune.dungeon_completion dc ON dc.completion_id = dcp.completion_id
			WHERE dcp.player_id = $1::bigint
			ORDER BY dc.completion_id DESC
			LIMIT 100`, playerID)
		if err != nil {
			return msgDungeons{err: err}
		}
		defer rows.Close()
		var out []dungeonRecord
		for rows.Next() {
			var r dungeonRecord
			if err := rows.Scan(&r.DungeonID, &r.Difficulty, &r.DurationMs, &r.PlayersNum, &r.CompletionID); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgDungeons{err: err}
		}
		return msgDungeons{rows: out}
	}
}

var cheatLocations = []teleportLocation{
	{Name: "Windsack", X: 974276.75, Y: 20084.312, Z: 5112.283},
	{Name: "EcoLabs", X: 826879.3, Y: -925967.2, Z: 4974.4277},
	{Name: "CrashSite", X: 330284.22, Y: 205236.98, Z: 2251.008},
	{Name: "MediumStarter", X: 268515.8, Y: 207559.39, Z: 5000.0},
	{Name: "ConvoyAmbush", X: -920080.0, Y: 909620.0, Z: 300.0},
	{Name: "SpiceRaid", X: 271590.0, Y: -493122.0, Z: 8471.0},
	{Name: "PS5_ESW_0", X: -113881.4, Y: -305252.1, Z: 20864.5},
	{Name: "PS5_ESW_1", X: -109861.8, Y: -307020.0, Z: 21192.9},
	{Name: "PS5_ESW_2", X: -129029.6, Y: -312757.8, Z: 21099.6},
	{Name: "PS5_ESW_3", X: -117312.0, Y: -305453.9, Z: 21649.8},
}

func cmdListPartitions() Cmd {
	return func() Msg {
		return msgPartitions{rows: cheatLocations}
	}
}

func cmdTeleportPlayer(flsID string, locationName string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		var loc teleportLocation
		for _, l := range cheatLocations {
			if l.Name == locationName {
				loc = l
				break
			}
		}
		if loc.Name == "" {
			return msgMutate{err: fmt.Errorf("unknown location: %s", locationName)}
		}
		ctx := context.Background()
		// Use the player's current partition so the zone server is correct.
		var partitionID int64
		err := globalDB.QueryRow(ctx, `
			SELECT COALESCE(a.partition_id, 0)
			FROM dune.encrypted_accounts e
			JOIN dune.player_state ps ON ps.account_id = e.id
			JOIN dune.actors a ON a.id = ps.player_pawn_id
			WHERE convert_from(e.encrypted_funcom_id, 'UTF8') = $1`, flsID).Scan(&partitionID)
		if err != nil || partitionID == 0 {
			_ = globalDB.QueryRow(ctx,
				`SELECT id FROM dune.world_partition WHERE blocked = false LIMIT 1`).Scan(&partitionID)
		}
		_, err = globalDB.Exec(ctx, `
			SELECT dune.admin_move_offline_player_to_partition($1::text, $2::bigint, ROW($3::float8,$4::float8,$5::float8)::dune.Vector)`,
			flsID, partitionID, loc.X, loc.Y, loc.Z)
		if err != nil {
			return msgMutate{err: fmt.Errorf("teleport: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Moved %s to %s", flsID, locationName)}
	}
}

// playerPosition is the live world position of a player's character.
type playerPosition struct {
	PartitionID int64   `json:"partition_id"`
	Map         string  `json:"map"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
}

// cmdGetPlayerPosition reads a player's current world position from the
// actors table. The transform column is a composite type holding a vector
// location plus a quaternion rotation; we only need the vector.
// playerID is the actor id (dune.actors.id) — matches playerInfo.ID.
func cmdGetPlayerPosition(playerID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgPlayerPosition{err: fmt.Errorf("not connected")}
		}
		var pos playerPosition
		err := globalDB.QueryRow(context.Background(), `
			SELECT
				COALESCE(a.partition_id, 0),
				COALESCE(a.map, ''),
				((a.transform).location).x,
				((a.transform).location).y,
				((a.transform).location).z
			FROM dune.actors a
			WHERE a.id = $1`, playerID).Scan(&pos.PartitionID, &pos.Map, &pos.X, &pos.Y, &pos.Z)
		if err != nil {
			return msgPlayerPosition{err: fmt.Errorf("read position: %w", err)}
		}
		return msgPlayerPosition{pos: pos}
	}
}

// cmdTeleportPlayerToCoords moves an offline player to a specific
// (partition_id, x, y, z). For online players this should be skipped in
// favour of rmqTeleportTo, which has immediate effect.
func cmdTeleportPlayerToCoords(flsID string, partitionID int64, x, y, z float64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if partitionID == 0 {
			_ = globalDB.QueryRow(context.Background(),
				`SELECT id FROM dune.world_partition WHERE blocked = false LIMIT 1`).Scan(&partitionID)
		}
		_, err := globalDB.Exec(context.Background(), `
			SELECT dune.admin_move_offline_player_to_partition($1::text, $2::bigint, ROW($3::float8,$4::float8,$5::float8)::dune.Vector)`,
			flsID, partitionID, x, y, z)
		if err != nil {
			return msgMutate{err: fmt.Errorf("teleport: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Moved %s to (%.0f, %.0f, %.0f)", flsID, x, y, z)}
	}
}

// ── storage container commands ────────────────────────────────────────────────

type storageContainerRow struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Class         string   `json:"class"`
	Map           string   `json:"map"`
	ItemCount     int64    `json:"item_count"`
	ItemTemplates []string `json:"item_templates"`
	ItemNames     []string `json:"item_names"`
	OwnerName     string   `json:"owner_name"`
}

type msgStorageContainers struct {
	rows []storageContainerRow
	err  error
}

func cmdListStorageContainers() Msg {
	if globalDB == nil {
		return msgStorageContainers{err: fmt.Errorf("not connected")}
	}
	// Drive from dune.placeables so we catch player-built containers regardless
	// of whether they've been promoted to an actor row yet (the game creates the
	// actor lazily on first interaction). building_type is the in-data identity
	// of the placeable kind; the four below cover the storage-container tiers,
	// noting that "Small Storage Container" registers as SpiceSilo_Placeable
	// despite sharing the type name with world POI silos — owner_entity_id
	// distinguishes player-built from world-spawned.
	// User-given container names live on dune.permission_actor.actor_name.
	// Unnamed containers default to 'None' or '##<PlaceableType>_Placeable' —
	// filter both out so only real custom names surface.
	rows, err := globalDB.Query(context.Background(), `
		SELECT p.id,
		       COALESCE(MAX(CASE
		           WHEN pa.actor_name NOT LIKE '##%' AND pa.actor_name <> 'None'
		           THEN pa.actor_name
		       END), '') AS name,
		       p.building_type AS class,
		       COALESCE(a.map, '') AS map,
		       COUNT(i.id) AS item_count,
		       COALESCE(array_agg(DISTINCT i.template_id) FILTER (WHERE i.template_id IS NOT NULL), '{}') AS item_templates,
		       COALESCE(MAX(ps.character_name), MAX(convert_from(e.encrypted_funcom_id, 'UTF8')), '') AS owner_name
		FROM dune.placeables p
		LEFT JOIN dune.actors a            ON a.id = p.id
		LEFT JOIN dune.permission_actor pa ON pa.actor_id = p.id
		LEFT JOIN dune.inventories inv     ON inv.actor_id = p.id
		LEFT JOIN dune.items i             ON i.inventory_id = inv.id
		LEFT JOIN dune.actor_fgl_entities afe  ON afe.entity_id = p.owner_entity_id
		LEFT JOIN dune.permission_actor_rank par ON par.permission_actor_id = afe.actor_id
		LEFT JOIN dune.actors player_a          ON player_a.id = par.player_id
		LEFT JOIN dune.encrypted_accounts e     ON e.id = player_a.owner_account_id
		LEFT JOIN dune.player_state ps          ON ps.account_id = player_a.owner_account_id
		WHERE p.building_type IN (
		    'SpiceSilo_Placeable',
		    'GenericContainer_Placeable',
		    'StorageContainer_Placeable',
		    'MediumStorageContainer_Placeable'
		  )
		  AND p.is_hologram = false
		  AND p.owner_entity_id IS NOT NULL
		  AND p.owner_entity_id != 0
		GROUP BY p.id, p.building_type, a.map
		ORDER BY p.id`)
	if err != nil {
		return msgStorageContainers{err: err}
	}
	defer rows.Close()
	var out []storageContainerRow
	for rows.Next() {
		var r storageContainerRow
		var templates []string
		if err := rows.Scan(&r.ID, &r.Name, &r.Class, &r.Map, &r.ItemCount, &templates, &r.OwnerName); err != nil {
			continue
		}
		if templates != nil {
			r.ItemTemplates = templates
		} else {
			r.ItemTemplates = []string{}
		}
		r.ItemNames = []string{}
		for _, t := range templates {
			if name := itemData.Names[strings.ToLower(t)]; name != "" {
				r.ItemNames = append(r.ItemNames, name)
			}
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return msgStorageContainers{err: rows.Err()}
	}
	return msgStorageContainers{rows: out}
}

type msgContainerInventory struct {
	rows []itemInfo
	err  error
}

func cmdGetContainerInventory(actorID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgContainerInventory{err: fmt.Errorf("not connected")}
		}
		rows, err := globalDB.Query(context.Background(), `
			SELECT i.id, i.template_id, i.stack_size, i.quality_level,
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'), 'N/A'),
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'), 'N/A')
			FROM dune.items i
			JOIN dune.inventories inv ON i.inventory_id = inv.id
			WHERE inv.actor_id = $1
			ORDER BY i.template_id`, actorID)
		if err != nil {
			return msgContainerInventory{err: err}
		}
		defer rows.Close()
		var items []itemInfo
		for rows.Next() {
			var it itemInfo
			if err := rows.Scan(&it.ID, &it.TemplateID, &it.StackSize, &it.Quality, &it.Durability, &it.MaxDurability); err != nil {
				continue
			}
			it.Name = itemData.Names[strings.ToLower(it.TemplateID)]
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			return msgContainerInventory{err: err}
		}
		return msgContainerInventory{rows: items}
	}
}

func cmdGiveItemToContainer(actorID int64, templateID string, qty, quality int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		// Find the container's inventory (any type).
		var invID int64
		var maxCount int
		var maxVol float32
		err := globalDB.QueryRow(ctx, `
			SELECT id, max_item_count, max_item_volume
			FROM dune.inventories
			WHERE actor_id = $1
			LIMIT 1`, actorID).Scan(&invID, &maxCount, &maxVol)
		if err != nil {
			return msgMutate{err: fmt.Errorf("find container inventory: %w", err)}
		}

		// Count current items.
		var currentCount int64
		if err := globalDB.QueryRow(ctx, `SELECT COUNT(*) FROM dune.items WHERE inventory_id = $1`, invID).Scan(&currentCount); err != nil {
			return msgMutate{err: fmt.Errorf("count items: %w", err)}
		}
		if maxCount > 0 && currentCount >= int64(maxCount) {
			return msgMutate{err: fmt.Errorf("container inventory full (%d/%d)", currentCount, maxCount)}
		}

		// Insert item with minimal valid stats matching game-generated items.
		_, err = globalDB.Exec(ctx, `
			INSERT INTO dune.items (inventory_id, template_id, stack_size, quality_level, position_index, stats)
			VALUES ($1, $2, $3, $4, $5, '{"FCustomizationStats":[[],{}],"FItemStackAndDurabilityStats":[[],{}]}')`,
			invID, templateID, qty, quality, currentCount)
		if err != nil {
			return msgMutate{err: fmt.Errorf("insert item: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Added %dx %s (quality %d) to container %d", qty, templateID, quality, actorID)}
	}
}

func cmdListBases() Msg {
	if globalDB == nil {
		return msgBaseList{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT b.id,
		       COALESCE(pa.actor_name, '') AS name,
		       COALESCE(inst.cnt, 0) AS pieces,
		       COALESCE(plac.cnt, 0) AS placeables
		FROM dune.buildings b
		LEFT JOIN (
		    SELECT building_id, MIN(owner_entity_id) AS owner_entity_id, COUNT(*) AS cnt
		    FROM dune.building_instances
		    GROUP BY building_id
		) inst ON inst.building_id = b.id
		LEFT JOIN dune.actor_fgl_entities afe ON afe.entity_id = inst.owner_entity_id
		LEFT JOIN dune.actors t ON t.id = afe.actor_id AND t.class ILIKE '%Totem%'
		LEFT JOIN dune.permission_actor pa ON pa.actor_id = t.id
		LEFT JOIN (
		    SELECT bi.building_id, COUNT(*) AS cnt
		    FROM dune.building_instances bi
		    JOIN dune.placeables p ON p.owner_entity_id = bi.owner_entity_id
		    GROUP BY bi.building_id
		) plac ON plac.building_id = b.id
		ORDER BY b.id`)
	if err != nil {
		return msgBaseList{err: err}
	}
	defer rows.Close()
	var out []baseRow
	for rows.Next() {
		var r baseRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Pieces, &r.Placeables); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgBaseList{err: err}
	}
	return msgBaseList{rows: out}
}
