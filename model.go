package main

import (
	"sort"
	"strings"
)

// dbItemTemplates is the merged, sorted list of item templates.
var dbItemTemplates []string

// ── domain types ─────────────────────────────────────────────────────────────

type playerInfo struct {
	ID           int64  `json:"id"`
	AccountID    int64  `json:"account_id"`
	ControllerID int64  `json:"controller_id"`
	FLSID        string `json:"fls_id"`
	Name         string `json:"name"`
	Class        string `json:"class"`
	Map          string `json:"map"`
	FactionID    int16  `json:"faction_id"`
	OnlineStatus string `json:"online_status"`
}

type journeyNode struct {
	NodeID           string `json:"node_id"`
	IsComplete       bool   `json:"is_complete"`
	IsRevealed       bool   `json:"is_revealed"`
	HasPendingReward bool   `json:"has_pending_reward"`
}

type itemInfo struct {
	ID            int64  `json:"id"`
	TemplateID    string `json:"template_id"`
	Name          string `json:"name"`
	StackSize     int64  `json:"stack_size"`
	Quality       int64  `json:"quality"`
	Durability    string `json:"durability"`
	MaxDurability string `json:"max_durability"`
}

type currencyRow struct {
	PlayerID   int64 `json:"player_id"`
	CurrencyID int16 `json:"currency_id"`
	Balance    int64 `json:"balance"`
}

type factionRep struct {
	ActorID     int64  `json:"actor_id"`
	FactionID   int16  `json:"faction_id"`
	FactionName string `json:"faction_name"`
	Reputation  int32  `json:"reputation"`
	Scrips      int64  `json:"scrips"`
}

type specTrack struct {
	PlayerID  int64   `json:"player_id"`
	TrackType string  `json:"track_type"`
	XP        int32   `json:"xp"`
	Level     float32 `json:"level"`
}

type itemRule struct {
	Name          string   `json:"name"`
	StackMax      int64    `json:"stack_max"`
	Volume        float64  `json:"volume"`
	Tier          int      `json:"tier"`
	Rarity        string   `json:"rarity"`
	Category      string   `json:"category"`
	MaxDurability *float64 `json:"max_durability,omitempty"`
	Icon          *string  `json:"icon"`
}

type itemDataFile struct {
	DefaultStackMax int64               `json:"default_stack_max"`
	DefaultVolume   float64             `json:"default_volume"`
	Names           map[string]string   `json:"names"`
	Items           map[string]itemRule `json:"items"`
}

// tagsDataFile is the output of dune-item-data/build-tags-data.sh — maps from
// journey story node IDs / contract names to the gameplay tags those in-game
// completions would emit. The admin tool uses these to apply tags when an
// admin clicks Mark Complete (since the DB-only completion bypasses the
// in-game effects).
type tagsDataFile struct {
	JourneyNodeTags map[string][]string `json:"journey_node_tags"`
	ContractTags    map[string][]string `json:"contract_tags"`
	ContractAliases map[string]string   `json:"contract_aliases"`
	// ContractSkillGrants[contract_id] = skill-block tags this contract's
	// SkillsKeyRewards would unlock in-game (e.g. "Skills.Key.Trooper3").
	// Without applying these the trainer contract's tags land but the skill
	// tree branch stays locked.
	ContractSkillGrants map[string][]string `json:"contract_skill_grants"`
	// JobSkillBlocks["Trooper"] = every bExternal Skills.Key.* module in the
	// Trooper skill tree (tier 1/2/3 + capstones). Used by Unlock Trainer to
	// grant the *entire* job's block set, since only ~10 of 30 are
	// contract-granted (the rest are normally unlocked by dialogue or auto).
	JobSkillBlocks map[string][]string `json:"job_skill_blocks"`
	// JobAllModules["Trooper"] = every module (Key, Ability, Attribute,
	// Perk) whose SkillArea is ESkillTree::Trooper. Used by Reset Job Skills
	// to fully nuke a class tree — the trainer Key blocks alone aren't
	// enough because the game auto-grants the corresponding tier-1 ability
	// (e.g. Skills.Ability.SuspensorGrenade_Reduction) which then sticks
	// around as a refundable 1-SP phantom unless we remove it here.
	JobAllModules map[string][]string `json:"job_all_modules"`
}

type blueprintRow struct {
	ID         int64  `json:"id"`
	OwnerName  string `json:"owner_name"`
	ItemID     int64  `json:"item_id"`
	Pieces     int64  `json:"pieces"`
	Placeables int64  `json:"placeables"`
	Name       string `json:"name"`
}

type baseRow struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Pieces     int64  `json:"pieces"`
	Placeables int64  `json:"placeables"`
}

// ── message types (used by db.go cmd* functions) ──────────────────────────────

type msgConnect struct{ err error }
type msgPlayers struct {
	rows []playerInfo
	err  error
}
type msgInventory struct {
	rows []itemInfo
	err  error
}
type msgCurrency struct {
	rows []currencyRow
	err  error
}
type msgFactions struct {
	rows            []factionRep
	scripCurrencyID int16
	err             error
}
type msgSpecs struct {
	rows []specTrack
	err  error
}
type msgSQL struct {
	result string
	err    error
}
type msgMutate struct {
	ok  string
	err error
}
type msgPlayerPosition struct {
	pos playerPosition
	err error
}
type msgRepairGear struct {
	repaired int
	scanned  int
	err      error
}
type msgRepairVehicle struct {
	repaired int
	skipped  int
	total    int
	err      error
}
type msgJourney struct {
	rows []journeyNode
	err  error
}
type msgBlueprintList struct {
	rows []blueprintRow
	err  error
}
type msgBaseList struct {
	rows []baseRow
	err  error
}
type msgItemTemplates struct {
	templates []string
}

type vehicleRow struct {
	ID                int64   `json:"id"`
	Class             string  `json:"class"`
	Map               string  `json:"map"`
	ChassisDurability float64 `json:"chassis_durability"`
	VehicleName       string  `json:"vehicle_name"`
	IsRecovered       bool    `json:"is_recovered"`
	IsBackup          bool    `json:"is_backup"`
}

type cheatEntry struct {
	FLSID         string `json:"fls_id"`
	CheatType     string `json:"cheat_type"`
	EventTime     string `json:"event_time"`
	CharacterName string `json:"character_name"`
}

type gameEvent struct {
	ActorID      int64   `json:"actor_id"`
	UniverseTime string  `json:"universe_time"`
	Map          string  `json:"map"`
	EventType    int32   `json:"event_type"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Z            float64 `json:"z"`
	CustomData   string  `json:"custom_data"`
}

type dungeonRecord struct {
	DungeonID    string `json:"dungeon_id"`
	Difficulty   string `json:"difficulty"`
	DurationMs   int64  `json:"duration_ms"`
	PlayersNum   int    `json:"players_num"`
	CompletionID int64  `json:"completion_id"`
}

// ── market board types ────────────────────────────────────────────────────────

type marketItem struct {
	TemplateID   string  `json:"template_id"`
	Quality      int64   `json:"quality"`
	DisplayName  string  `json:"display_name"`
	Category     string  `json:"category"`
	Tier         int     `json:"tier"`
	Rarity       string  `json:"rarity"`
	LowestPrice  int64   `json:"lowest_price"`
	TotalStock   int64   `json:"total_stock"`
	BotStock     int64   `json:"bot_stock"`
	ListingCount int64   `json:"listing_count"`
	Icon         *string `json:"icon"`
}

type marketListing struct {
	OrderID    int64  `json:"order_id"`
	TemplateID string `json:"template_id"`
	OwnerType  string `json:"owner_type"` // "bot" or "player"
	OwnerName  string `json:"owner_name"`
	Price      int64  `json:"price"`
	Stock      int64  `json:"stock"`
	Quality    int64  `json:"quality"`
}

type marketSale struct {
	OrderID    int64  `json:"order_id"`
	TemplateID string `json:"template_id"`
	SellerType string `json:"seller_type"` // "bot" or "player"
	SellerName string `json:"seller_name"`
	Price      int64  `json:"price"`
	Quantity   int64  `json:"quantity"`
}

type marketStats struct {
	TotalListings  int64 `json:"total_listings"`
	BotListings    int64 `json:"bot_listings"`
	PlayerListings int64 `json:"player_listings"`
	TotalStock     int64 `json:"total_stock"`
	BotStock       int64 `json:"bot_stock"`
	PlayerStock    int64 `json:"player_stock"`
	UniqueItems    int64 `json:"unique_items"`
}

type msgMarketItems struct {
	rows []marketItem
	err  error
}
type msgMarketListings struct {
	rows []marketListing
	err  error
}
type msgMarketSales struct {
	rows []marketSale
	err  error
}
type msgMarketStats struct {
	stats marketStats
	err   error
}

type teleportLocation struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}

type msgVehicles struct {
	rows []vehicleRow
	err  error
}
type msgCheatLog struct {
	rows []cheatEntry
	err  error
}
type msgEvents struct {
	rows []gameEvent
	err  error
}
type msgDungeons struct {
	rows []dungeonRecord
	err  error
}
type msgPartitions struct {
	rows []teleportLocation
	err  error
}

type msgKeystones struct {
	ids []int16
	err error
}

type msgTags struct {
	rows []string
	err  error
}

// ── item template merge (called after connect) ────────────────────────────────

func mergeItemTemplates(dbTemplates []string) {
	seen := make(map[string]string)
	for _, t := range dbTemplates {
		seen[strings.ToLower(t)] = t
	}
	for k := range itemData.Names {
		if _, ok := seen[k]; !ok {
			seen[k] = k
		}
	}
	if itemData.Items != nil {
		for k := range itemData.Items {
			if _, ok := seen[k]; !ok {
				seen[k] = k
			}
		}
	}
	merged := make([]string, 0, len(seen))
	for _, v := range seen {
		merged = append(merged, v)
	}
	sort.Strings(merged)
	dbItemTemplates = merged
}
