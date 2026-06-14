package main

import (
	"fmt"
	"net/http"
)

// progressionPreset is a bundle of journey nodes to mark complete together.
// Children of each node are auto-completed via cmdCompleteJourneyNode.
type progressionPreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	NodeCount   int      `json:"node_count"` // approximate total nodes affected
	Nodes       []string `json:"nodes"`
}

// progressionPresets is the catalog of curated progression bundles.
var progressionPresets = []progressionPreset{
	{
		ID:          "skip_npe",
		Name:        "Skip NPE (New Player Experience)",
		Description: "Marks the tutorial/NPE as complete. Useful for experienced players who don't need the intro.",
		NodeCount:   4,
		Nodes:       []string{"DA_MQ_NPEAutocompleted"},
	},
	{
		ID:          "a_new_beginning",
		Name:        "Complete: A New Beginning",
		Description: "Main story intro — crafting Stillsuit, harvesting resources, fabricator research. Unlocks early game.",
		NodeCount:   132,
		Nodes:       []string{"DA_MQ_ANewBeginning"},
	},
	{
		ID:          "find_the_fremen",
		Name:        "Complete: Find the Fremen (Trials of Aql)",
		Description: "All 7 trials, the Sietch, and Epilogue. Unlocks the Fremkit (Stillsuit, Thumper, Cryss Knife, etc.) and completes Act 1.",
		NodeCount:   46,
		Nodes:       []string{"DA_MQ_FindTheFremen"},
	},
	{
		ID:          "act1_complete",
		Name:        "Complete: All of Act 1",
		Description: "A New Beginning + Find the Fremen combined. Player starts ready for Act 2.",
		NodeCount:   178,
		Nodes:       []string{"DA_MQ_ANewBeginning", "DA_MQ_FindTheFremen"},
	},
	{
		ID:          "vermillius_intro",
		Name:        "Skip: Vermillius Gap Tutorials",
		Description: "Completes all Vermillius Gap tutorial quests (research, crafting, exploration). No tag side effects — pure tutorial skip.",
		NodeCount:   101,
		Nodes:       []string{"DA_SQ_VermiliusGap", "DA_Dunipedia_Landmarks.VermiliusGap"},
	},
	{
		ID:          "deep_desert_intro",
		Name:        "Skip: Deep Desert Intro",
		Description: "Completes the Deep Desert side quest chain.",
		NodeCount:   34,
		Nodes:       []string{"DA_SQ_DeepDesert"},
	},
	{
		ID:          "taxation_intro",
		Name:        "Skip: Taxation / Exchange Tutorial",
		Description: "Completes the exchange/travel tutorial chain.",
		NodeCount:   27,
		Nodes:       []string{"DA_SQ_Taxation"},
	},
	{
		ID:          "overland_intro",
		Name:        "Skip: Overland Map Intro",
		Description: "Completes the Overland map side quest chain (Hark Landsraad missions, keystones).",
		NodeCount:   49,
		Nodes:       []string{"DA_SQ_OverlandMap"},
	},
	{
		ID:          "unlock_all_lore",
		Name:        "Unlock All Lore (Dunipedia)",
		Description: "Reveals all Dunipedia entries: Known Universe, Landmarks, Manual of the Friendly Desert, War for Arrakis.",
		NodeCount:   322,
		Nodes: []string{
			"DA_Dunipedia_KnownUniverse",
			"DA_Dunipedia_Landmarks",
			"DA_Dunipedia_ManualOfTheFriendlyDesert",
			"DA_Dunipedia_WarForArrakis",
		},
	},
}

// cmdApplyProgressionPreset applies a preset by completing each node (and its
// children + tags) via the existing cmdCompleteJourneyNode logic.
func progressionPresetByID(presetID string) *progressionPreset {
	for i := range progressionPresets {
		if progressionPresets[i].ID == presetID {
			return &progressionPresets[i]
		}
	}
	return nil
}

func completionError(presetID, nodeID string, msg msgMutate, ok bool) error {
	if ok && msg.err == nil {
		return nil
	}
	if ok && msg.err != nil {
		return fmt.Errorf("apply %s (node %s): %w", presetID, nodeID, msg.err)
	}
	return fmt.Errorf("apply %s (node %s): internal error", presetID, nodeID)
}

func applyProgressionPresetNodes(
	accountID int64,
	presetID string,
	nodeIDs []string,
	complete func(accountID int64, nodeID string) (msgMutate, bool),
) (int, int, error) {
	totalNodes := 0
	totalTags := 0
	for _, nodeID := range nodeIDs {
		msg, ok := complete(accountID, nodeID)
		if err := completionError(presetID, nodeID, msg, ok); err != nil {
			return totalNodes, totalTags, err
		}
		n, t := parseCompletionCounts(msg.ok)
		totalNodes += n
		totalTags += t
	}
	return totalNodes, totalTags, nil
}

func cmdApplyProgressionPreset(accountID int64, presetID string) Cmd {
	return func() Msg {
		if globalDB == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		preset := progressionPresetByID(presetID)
		if preset == nil {
			return msgMutate{err: fmt.Errorf("unknown preset: %s", presetID)}
		}

		totalNodes, totalTags, err := applyProgressionPresetNodes(accountID, presetID, preset.Nodes, func(id int64, nodeID string) (msgMutate, bool) {
			msg, ok := cmdCompleteJourneyNode(globalDB, id, nodeID)().(msgMutate)
			return msg, ok
		})
		if err != nil {
			return msgMutate{err: err}
		}
		return msgMutate{ok: fmt.Sprintf(
			"Applied preset '%s': %d node(s), %d tag(s) — takes effect on next login",
			preset.Name, totalNodes, totalTags)}
	}
}

// parseCompletionCounts extracts node and tag counts from a cmdCompleteJourneyNode
// success message like "Completed X + 4 node(s), +1 tag(s) — ..."
func parseCompletionCounts(s string) (nodes int, tags int) {
	// Lightweight parse — we just want approximate totals for the summary message.
	// Format: "Completed <node> + N node(s)[, +M tag(s)] — takes effect..."
	for i := 0; i < len(s); i++ {
		if i+8 < len(s) && s[i:i+2] == "+ " && isDigit(s[i+2]) {
			n := 0
			j := i + 2
			for j < len(s) && isDigit(s[j]) {
				n = n*10 + int(s[j]-'0')
				j++
			}
			nodes += n + 1 // +1 for the root node itself
		}
		if i+7 < len(s) && s[i:i+2] == ", " && i+3 < len(s) && s[i+2] == '+' && isDigit(s[i+3]) {
			t := 0
			j := i + 3
			for j < len(s) && isDigit(s[j]) {
				t = t*10 + int(s[j]-'0')
				j++
			}
			tags += t
		}
	}
	return
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// HTTP handlers below.

func handleListProgressionPresets(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, progressionPresets)
}

func handleApplyProgressionPreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		PresetID  string `json:"preset_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.AccountID == 0 || req.PresetID == "" {
		jsonErr(w, fmt.Errorf("account_id and preset_id required"), 400)
		return
	}
	msg, ok := cmdApplyProgressionPreset(req.AccountID, req.PresetID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}
