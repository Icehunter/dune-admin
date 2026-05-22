package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

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
		       COALESCE(convert_from(e.encrypted_funcom_id, 'UTF8'), ''),
		       a.class,
		       COALESCE(a.map, ''),
		       COALESCE(pf.faction_id, 0),
		       COALESCE(ps.online_status::text, 'Offline')
		FROM dune.actors a
		LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
		LEFT JOIN dune.encrypted_accounts e ON e.id = a.owner_account_id
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
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		template = strings.TrimSpace(template)
		if template == "" {
			return msgMutate{err: fmt.Errorf("item template required")}
		}
		if qty <= 0 {
			return msgMutate{err: fmt.Errorf("quantity must be > 0")}
		}
		ctx := context.Background()

		// Prefer the backpack (inventory_type=0) — that's where resources live.
		// Fall back to the first available inventory if not found.
		var invID int64
		var maxSlots int
		var maxVolume float64
		err := globalDB.QueryRow(ctx, `
			SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
			FROM dune.inventories
			WHERE actor_id = $1::bigint AND inventory_type = 0
			LIMIT 1`, playerID).Scan(&invID, &maxSlots, &maxVolume)
		if err != nil {
			err = globalDB.QueryRow(ctx,
				`SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
				 FROM dune.inventories WHERE actor_id = $1::bigint LIMIT 1`, playerID).Scan(&invID, &maxSlots, &maxVolume)
			if err != nil {
				return msgMutate{err: fmt.Errorf("find inventory: %w", err)}
			}
		}

		hasSlotCap := maxSlots > 0
		hasVolumeCap := maxVolume > 0

		type stackSlot struct {
			id   int64
			size int64
		}
		var stacks []stackSlot
		usedSlots := 0
		usedVolume := 0.0
		maxPos := int64(-1)

		rows, err := globalDB.Query(ctx, `
			SELECT id, template_id, stack_size, quality_level, volume_override, position_index
			FROM dune.items
			WHERE inventory_id = $1::bigint`, invID)
		if err != nil {
			return msgMutate{err: err}
		}
		defer rows.Close()
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
			usedSlots++
			if pos > maxPos {
				maxPos = pos
			}
			if qLevel == quality && tmpl == template {
				stacks = append(stacks, stackSlot{id: id, size: stackSize})
			}
			if hasVolumeCap {
				itemVol := 0.0
				if vol.Valid && vol.Float64 > 0 {
					itemVol = vol.Float64
				} else if itemData.Items != nil {
					if rule, ok := itemData.Items[strings.ToLower(tmpl)]; ok {
						itemVol = rule.Volume // 0 is valid — item takes no volume
					} else if itemData.DefaultVolume > 0 {
						itemVol = itemData.DefaultVolume
					}
					// Unknown volume: treat as 0 (no space consumed).
				} else if itemData.DefaultVolume > 0 {
					itemVol = itemData.DefaultVolume
				}
				usedVolume += itemVol * float64(stackSize)
			}
		}
		if rows.Err() != nil {
			return msgMutate{err: rows.Err()}
		}
		stackMax, err := resolveStackMax(ctx, template, quality)
		if err != nil {
			return msgMutate{err: err}
		}
		if stackMax < 1 {
			stackMax = 1
		}

		if hasVolumeCap {
			perItemVol, err := resolveItemVolume(ctx, template)
			if err != nil {
				return msgMutate{err: err}
			}
			if perItemVol > 0 {
				availableVol := maxVolume - usedVolume
				if availableVol < 0 {
					availableVol = 0
				}
				maxByVolume := int64(math.Floor(availableVol / perItemVol))
				if maxByVolume < qty {
					return msgMutate{err: fmt.Errorf(
						"over weight limit: room for %d more %s (%.2f/%.2f volume used)",
						maxByVolume, template, usedVolume, maxVolume)}
				}
			}
			// perItemVol == 0: item takes no volume, always fits.
		}

		sort.Slice(stacks, func(i, j int) bool {
			return stacks[i].size > stacks[j].size
		})

		remaining := qty
		type stackUpdate struct {
			id  int64
			add int64
		}
		var updates []stackUpdate
		if stackMax > 1 {
			for _, st := range stacks {
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
				updates = append(updates, stackUpdate{id: st.id, add: add})
				remaining -= add
			}
		}

		var newStacks []int64
		for remaining > 0 {
			size := stackMax
			if size > remaining {
				size = remaining
			}
			newStacks = append(newStacks, size)
			remaining -= size
		}

		if hasSlotCap {
			freeSlots := maxSlots - usedSlots
			if freeSlots < len(newStacks) {
				return msgMutate{err: fmt.Errorf(
					"inventory full: need %d free slots, have %d",
					len(newStacks), freeSlots)}
			}
		}

		tx, err := globalDB.Begin(ctx)
		if err != nil {
			return msgMutate{err: err}
		}
		defer tx.Rollback(ctx)

		for _, u := range updates {
			_, err = tx.Exec(ctx, `
				UPDATE dune.items
				SET stack_size = stack_size + $1::bigint
				WHERE id = $2::bigint`, u.add, u.id)
			if err != nil {
				return msgMutate{err: err}
			}
		}

		nextPos := maxPos + 1
		for _, size := range newStacks {
			_, err = tx.Exec(ctx, `
				INSERT INTO dune.items (inventory_id, stack_size, position_index, template_id, quality_level, stats)
				VALUES ($1::bigint, $2::bigint, $3::bigint, $4::text, $5::bigint, '{}'::jsonb)`,
				invID, size, nextPos, template, quality)
			if err != nil {
				return msgMutate{err: err}
			}
			nextPos++
		}

		if err := tx.Commit(ctx); err != nil {
			return msgMutate{err: err}
		}

		msg := fmt.Sprintf("Added %d × %s to player %d", qty, template, playerID)
		if len(updates) > 0 || len(newStacks) > 0 {
			msg = fmt.Sprintf(
				"Added %d × %s to player %d (%d stack(s) topped up, %d new stack(s))",
				qty, template, playerID, len(updates), len(newStacks))
		}
		return msgMutate{ok: msg}
	}
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
			SET xp_amount = LEAST(xp_amount + $1::integer, $4::integer)
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

// structureCount holds building + totem counts for a player.
type structureCount struct {
	PlayerAccountID int64
	Buildings       int64
	Totems          int64
}

type msgStructures struct {
	counts map[int64]structureCount
	err    error
}

func cmdFetchStructureCounts() Msg {
	if globalDB == nil {
		return msgStructures{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT a.owner_account_id,
		       SUM(CASE WHEN b.id IS NOT NULL THEN 1 ELSE 0 END) AS buildings,
		       SUM(CASE WHEN t.id IS NOT NULL THEN 1 ELSE 0 END) AS totems
		FROM dune.actors a
		LEFT JOIN dune.buildings b ON b.id = a.id
		LEFT JOIN dune.totems t ON t.id = a.id
		WHERE a.owner_account_id IS NOT NULL
		  AND (b.id IS NOT NULL OR t.id IS NOT NULL)
		GROUP BY a.owner_account_id`)
	if err != nil {
		return msgStructures{err: err}
	}
	defer rows.Close()

	counts := make(map[int64]structureCount)
	for rows.Next() {
		var accountID, bld, tot int64
		if err := rows.Scan(&accountID, &bld, &tot); err != nil {
			continue
		}
		counts[accountID] = structureCount{accountID, bld, tot}
	}
	if err := rows.Err(); err != nil {
		return msgStructures{err: err}
	}
	return msgStructures{counts: counts}
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

func describeMissingTemplates(m map[string]struct{}) string {
	if len(m) == 0 {
		return ""
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) > 5 {
		return strings.Join(names[:5], ", ") + ", …"
	}
	return strings.Join(names, ", ")
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
		rep := factionTierThresholds[tier]
		ctx := context.Background()
		_, err := globalDB.Exec(ctx, `SELECT dune.set_player_faction_reputation($1, $2, $3)`,
			actorID, factionID, rep)
		if err != nil {
			return msgMutate{err: fmt.Errorf("set_player_faction_reputation: %w", err)}
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
		return msgJourney{rows: nodes}
	}
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
		return msgMutate{ok: fmt.Sprintf("Completed %s + %d node(s) — takes effect on next login", nodeID, updated)}
	}
}

func cmdResetJourneyNode(accountID int64, nodeID string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		_, err := globalDB.Exec(context.Background(), `
			UPDATE dune.journey_story_node
			SET complete_condition_state = 'false'::jsonb,
			    has_pending_reward       = false
			WHERE account_id = $1
			  AND (story_node_id = $2 OR story_node_id LIKE $2 || '.%')`,
			accountID, nodeID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset node: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Reset %s", nodeID)}
	}
}

func cmdWipeJourneyNodes(accountID int64) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		_, err := globalDB.Exec(context.Background(),
			`SELECT dune.delete_all_journey_story_nodes($1)`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("wipe journey: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Wiped all journey nodes for account %d", accountID)}
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

// factionJourneyNodes returns the journey node IDs to complete for a preset.
// Both presets complete the same Rank5To20 onboarding nodes (faction-independent).
// The presets differ only in which tags and rep tier are written.
func factionJourneyNodes(_ string) []string {
	return climbTheRanksNodes
}

// cmdProgressionUnlock completes all prerequisite faction story journey nodes
// and writes the corresponding gameplay tags, optionally setting faction tier.
//
// faction: "atreides" | "harkonnen"
// preset:  "ch3_start" (through Rank03) | "rank19_eligible" (through Rank04 + tier 19)
func cmdProgressionUnlock(actorID int64, faction, preset string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		var factionID int16
		var dialogueFlag string
		switch faction {
		case "atreides":
			factionID = 1
			dialogueFlag = "DialogueFlags.Factions.SentToMeetHawat"
		case "harkonnen":
			factionID = 2
			dialogueFlag = "DialogueFlags.Factions.SentToPiterDeVries"
		default:
			return msgMutate{err: fmt.Errorf("faction must be atreides or harkonnen")}
		}

		setTier := false
		switch preset {
		case "ch3_start":
		case "rank19_eligible":
			setTier = true
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

		journeyNodes := factionJourneyNodes(faction)

		factionName := factionDisplayName(factionID)
		maxTier := 5
		if setTier {
			maxTier = 19
		}
		allTags := []string{dialogueFlag}
		if setTier {
			allTags = append(allTags, "Journey.LandsraadContractsUnlocked")
		}
		for t := 0; t <= maxTier; t++ {
			allTags = append(allTags, fmt.Sprintf("Faction.%s.Tier%d", factionName, t))
		}

		tx, err := globalDB.Begin(ctx)
		if err != nil {
			return msgMutate{err: err}
		}
		defer tx.Rollback(ctx)

		if _, err = tx.Exec(ctx,
			`SELECT dune.complete_journey_story_nodes_for_player($1, $2::text[])`,
			flsID, journeyNodes); err != nil {
			return msgMutate{err: fmt.Errorf("complete journey nodes: %w", err)}
		}

		if _, err = tx.Exec(ctx,
			`SELECT dune.update_player_tags($1, $2::text[], '{}'::text[])`, accountID, allTags); err != nil {
			return msgMutate{err: fmt.Errorf("update player tags: %w", err)}
		}

		if setTier {
			if _, err = tx.Exec(ctx,
				`SELECT dune.set_player_faction_reputation($1, $2, $3)`,
				controllerID, factionID, factionTierThresholds[19]); err != nil {
				return msgMutate{err: fmt.Errorf("set faction tier: %w", err)}
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return msgMutate{err: err}
		}

		tierMsg := fmt.Sprintf("%s tier tags 0–%d", factionName, maxTier)
		if setTier {
			tierMsg += fmt.Sprintf(" + rep tier 19 on controller %d", controllerID)
		}
		return msgMutate{ok: fmt.Sprintf(
			"Progression unlock (%s/%s): %d journey nodes completed + %s — takes effect on next login",
			preset, faction, len(journeyNodes), tierMsg)}
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

		newXP := currentXP + amount
		if newXP > maxCharXP {
			newXP = maxCharXP
		}
		newLevel := int64(xpToLevel(newXP))
		newTotalSP := newLevel
		newUnspentSP := newTotalSP - spentSP
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
	BuildingType string  `json:"building_type"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Z            float64 `json:"z"`
	Rotation     float64 `json:"rotation"`
}

type blueprintPlaceable struct {
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

func cmdExportBlueprint(blueprintID int64, outputPath string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgBlueprintExport{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		// Fetch instances.
		iRows, err := globalDB.Query(ctx, `
			SELECT building_type, transform
			FROM dune.building_blueprint_instances
			WHERE building_blueprint_id = $1
			ORDER BY instance_id`, blueprintID)
		if err != nil {
			return msgBlueprintExport{err: fmt.Errorf("query instances: %w", err)}
		}
		defer iRows.Close()

		var instances []blueprintInstance
		for iRows.Next() {
			var btype string
			var t []float32
			if err := iRows.Scan(&btype, &t); err != nil {
				continue
			}
			if len(t) < 4 {
				continue
			}
			instances = append(instances, blueprintInstance{
				BuildingType: btype,
				X:            float64(t[0]),
				Y:            float64(t[1]),
				Z:            float64(t[2]),
				Rotation:     float64(t[3]),
			})
		}
		if err := iRows.Err(); err != nil {
			return msgBlueprintExport{err: fmt.Errorf("read instances: %w", err)}
		}

		// Fetch placeables.
		pRows, err := globalDB.Query(ctx, `
			SELECT building_type, transform
			FROM dune.building_blueprint_placeables
			WHERE building_blueprint_id = $1
			ORDER BY placeable_id`, blueprintID)
		if err != nil {
			return msgBlueprintExport{err: fmt.Errorf("query placeables: %w", err)}
		}
		defer pRows.Close()

		var placeables []blueprintPlaceable
		for pRows.Next() {
			var btype string
			var t []float32
			if err := pRows.Scan(&btype, &t); err != nil {
				continue
			}
			if len(t) < 6 {
				continue
			}
			placeables = append(placeables, blueprintPlaceable{
				BuildingType: btype,
				X:            float64(t[0]),
				Y:            float64(t[1]),
				Z:            float64(t[2]),
				RX:           float64(t[3]),
				RY:           float64(t[4]),
				RZ:           float64(t[5]),
			})
		}
		if err := pRows.Err(); err != nil {
			return msgBlueprintExport{err: fmt.Errorf("read placeables: %w", err)}
		}

		// Fetch pentashield scale data.
		psRows, err := globalDB.Query(ctx, `
			SELECT placeable_id, scale
			FROM dune.building_blueprint_pentashields
			WHERE building_blueprint_id = $1
			ORDER BY placeable_id`, blueprintID)
		if err != nil {
			return msgBlueprintExport{err: fmt.Errorf("query pentashields: %w", err)}
		}
		defer psRows.Close()

		var pentashields []blueprintPentashield
		for psRows.Next() {
			var pid int
			var scale []int16
			if err := psRows.Scan(&pid, &scale); err != nil {
				continue
			}
			if len(scale) < 3 {
				continue
			}
			pentashields = append(pentashields, blueprintPentashield{
				PlaceableID: pid,
				Scale:       [3]int{int(scale[0]), int(scale[1]), int(scale[2])},
			})
		}
		if err := psRows.Err(); err != nil {
			return msgBlueprintExport{err: fmt.Errorf("read pentashields: %w", err)}
		}

		bf := blueprintFile{Instances: instances, Placeables: placeables, Pentashields: pentashields}
		data, err := json.MarshalIndent(bf, "", "  ")
		if err != nil {
			return msgBlueprintExport{err: fmt.Errorf("marshal json: %w", err)}
		}
		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return msgBlueprintExport{err: fmt.Errorf("write file: %w", err)}
		}
		return msgBlueprintExport{path: outputPath}
	}
}

func cmdImportBlueprint(playerPawnID int64, filename string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		// Read and parse JSON.
		data, err := os.ReadFile(filename)
		if err != nil {
			return msgMutate{err: fmt.Errorf("read %s: %w", filename, err)}
		}
		var bf blueprintFile
		if err := json.Unmarshal(data, &bf); err != nil {
			return msgMutate{err: fmt.Errorf("parse json: %w", err)}
		}
		if len(bf.Instances) == 0 && len(bf.Placeables) == 0 {
			return msgMutate{err: fmt.Errorf("blueprint file has no instances or placeables")}
		}

		ctx := context.Background()

		// Player must be offline.
		if err := checkPlayerOffline(ctx, playerPawnID); err != nil {
			return msgMutate{err: err}
		}

		tx, err := globalDB.Begin(ctx)
		if err != nil {
			return msgMutate{err: fmt.Errorf("begin tx: %w", err)}
		}
		defer tx.Rollback(ctx)

		// Get backpack inventory.
		var invID int64
		err = tx.QueryRow(ctx, `
			SELECT id FROM dune.inventories
			WHERE actor_id = $1 AND inventory_type = 0
			LIMIT 1`, playerPawnID).Scan(&invID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("find inventory: %w", err)}
		}

		// Next free position index.
		var nextPos int64
		_ = tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(position_index), -1) + 1
			FROM dune.items WHERE inventory_id = $1`, invID).Scan(&nextPos)

		// Placeholder stats — will be updated with blueprint ID after insert.
		placeholderStats := `{"FCustomizationStats":[[], {}],"FBuildingBlueprintItemStats":[[], {"PlayerBlueprintId":"!!bbp#0","PlayerBaseBackupId":{}}],"FItemStackAndDurabilityStats":[[], {"DecayedMaxDurability":0.0}]}`

		var itemID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO dune.items
				(inventory_id, stack_size, position_index, template_id, quality_level, stats)
			VALUES ($1, 1, $2, 'BuildingBlueprint_CopyDevice', 0, $3::jsonb)
			RETURNING id`,
			invID, nextPos, placeholderStats).Scan(&itemID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("create item: %w", err)}
		}

		// Insert blueprint master record.
		var blueprintID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO dune.building_blueprints (item_id, player_id, building_blueprint_map)
			VALUES ($1, null, '')
			RETURNING id`, itemID).Scan(&blueprintID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("create blueprint: %w", err)}
		}

		// Update item stats with real blueprint ID.
		fullStats := fmt.Sprintf(
			`{"FCustomizationStats":[[], {}],"FBuildingBlueprintItemStats":[[], {"PlayerBlueprintId":"!!bbp#%d","PlayerBaseBackupId":{}}],"FItemStackAndDurabilityStats":[[], {"DecayedMaxDurability":0.0}]}`,
			blueprintID)
		if _, err = tx.Exec(ctx, `UPDATE dune.items SET stats = $1::jsonb WHERE id = $2`,
			fullStats, itemID); err != nil {
			return msgMutate{err: fmt.Errorf("update item stats: %w", err)}
		}

		// Insert instances in batches of 50.
		// Arrays use explicit [0:3] bounds to match UE's 0-indexed storage convention.
		const batchSize = 50
		for start := 0; start < len(bf.Instances); start += batchSize {
			end := start + batchSize
			if end > len(bf.Instances) {
				end = len(bf.Instances)
			}
			batch := &pgx.Batch{}
			for i, inst := range bf.Instances[start:end] {
				transform := fmt.Sprintf("[0:3]={%g,%g,%g,%g}",
					float32(inst.X), float32(inst.Y), float32(inst.Z), float32(inst.Rotation))
				batch.Queue(`
					INSERT INTO dune.building_blueprint_instances
						(building_blueprint_id, instance_id, building_type, transform, hologram, provides_stability, health)
					VALUES ($1, $2, $3, $4::real[], true, false, 1.0)`,
					blueprintID, start+i, inst.BuildingType, transform)
			}
			br := tx.SendBatch(ctx, batch)
			for i := start; i < end; i++ {
				if _, err := br.Exec(); err != nil {
					br.Close()
					return msgMutate{err: fmt.Errorf("insert instance %d: %w", i, err)}
				}
			}
			br.Close()
		}

		// Insert placeables in batches of 50.
		// Arrays use explicit [0:5] bounds to match UE's 0-indexed storage convention.
		for start := 0; start < len(bf.Placeables); start += batchSize {
			end := start + batchSize
			if end > len(bf.Placeables) {
				end = len(bf.Placeables)
			}
			batch := &pgx.Batch{}
			for i, pl := range bf.Placeables[start:end] {
				transform := fmt.Sprintf("[0:5]={%g,%g,%g,%g,%g,%g}",
					float32(pl.X), float32(pl.Y), float32(pl.Z),
					float32(pl.RX), float32(pl.RY), float32(pl.RZ))
				batch.Queue(`
					INSERT INTO dune.building_blueprint_placeables
						(building_blueprint_id, placeable_id, building_type, transform, hologram)
					VALUES ($1, $2, $3, $4::real[], true)`,
					blueprintID, start+i, pl.BuildingType, transform)
			}
			br := tx.SendBatch(ctx, batch)
			for i := start; i < end; i++ {
				if _, err := br.Exec(); err != nil {
					br.Close()
					return msgMutate{err: fmt.Errorf("insert placeable %d: %w", i, err)}
				}
			}
			br.Close()
		}

		// Insert pentashield scale data (1-indexed array, standard PostgreSQL default).
		for _, ps := range bf.Pentashields {
			if _, err = tx.Exec(ctx, `
				INSERT INTO dune.building_blueprint_pentashields
					(building_blueprint_id, placeable_id, scale)
				VALUES ($1, $2, ARRAY[$3,$4,$5]::smallint[])`,
				blueprintID, ps.PlaceableID,
				int16(ps.Scale[0]), int16(ps.Scale[1]), int16(ps.Scale[2])); err != nil {
				return msgMutate{err: fmt.Errorf("insert pentashield %d: %w", ps.PlaceableID, err)}
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return msgMutate{err: fmt.Errorf("commit: %w", err)}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Imported %d pieces + %d placeables + %d pentashields → blueprint #%d (item %d) in player inventory",
			len(bf.Instances), len(bf.Placeables), len(bf.Pentashields), blueprintID, itemID)}
	}
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
			       COALESCE(rv.vehicle_name, ''),
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
		res, err := globalDB.Exec(context.Background(), `
			UPDATE dune.items
			SET stats = jsonb_set(
				stats,
				'{FItemStackAndDurabilityStats,1,CurrentDurability}',
				(stats->'FItemStackAndDurabilityStats'->1->'MaxDurability')
			)
			WHERE id = $1::bigint
			  AND stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability' IS NOT NULL
			  AND (stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability')::float > 0`, itemID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("repair item: %w", err)}
		}
		if res.RowsAffected() == 0 {
			return msgMutate{err: fmt.Errorf("item %d not found or has no repairable durability", itemID)}
		}
		return msgMutate{ok: fmt.Sprintf("Repaired item %d", itemID)}
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

// ── storage container commands ────────────────────────────────────────────────

type storageContainerRow struct {
	ID        int64  `json:"id"`
	Class     string `json:"class"`
	Map       string `json:"map"`
	ItemCount int64  `json:"item_count"`
}

type msgStorageContainers struct {
	rows []storageContainerRow
	err  error
}

func cmdListStorageContainers() Msg {
	if globalDB == nil {
		return msgStorageContainers{err: fmt.Errorf("not connected")}
	}
	rows, err := globalDB.Query(context.Background(), `
		SELECT a.id,
		       a.class,
		       COALESCE(a.map, ''),
		       COUNT(i.id) AS item_count
		FROM dune.actors a
		LEFT JOIN dune.inventories inv ON inv.actor_id = a.id
		LEFT JOIN dune.items i ON i.inventory_id = inv.id
		WHERE a.class ILIKE '%StorageContainer%'
		GROUP BY a.id, a.class, a.map
		ORDER BY a.id`)
	if err != nil {
		return msgStorageContainers{err: err}
	}
	defer rows.Close()
	var out []storageContainerRow
	for rows.Next() {
		var r storageContainerRow
		if err := rows.Scan(&r.ID, &r.Class, &r.Map, &r.ItemCount); err != nil {
			continue
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
		globalDB.QueryRow(ctx, `SELECT COUNT(*) FROM dune.items WHERE inventory_id = $1`, invID).Scan(&currentCount)
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