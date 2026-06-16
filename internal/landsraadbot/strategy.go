package landsraadbot

import (
	"context"
	"math/rand"
	"sort"
)

type landsraadTaskData struct {
	ID               int
	BoardIndex       int
	Completed        bool
	WinningFactionID *int
	TermID           int64
	CurrentProgress  float64
	GoalAmount       float64
	Revealed         bool
}

type ScoredTask struct {
	ID    int
	Score float64
	ReqXP float64
}

// Win paths by board_index
var winPaths = [][]int{
	{0, 1, 2, 3, 4},
	{5, 6, 7, 8, 9},
	{10, 11, 12, 13, 14},
	{15, 16, 17, 18, 19},
	{20, 21, 22, 23, 24},
	{0, 5, 10, 15, 20},
	{1, 6, 11, 16, 21},
	{2, 7, 12, 17, 22},
	{3, 8, 13, 18, 23},
	{4, 9, 14, 19, 24},
	{0, 6, 12, 18, 24},
	{4, 8, 12, 16, 20},
}

func (i *Instance) calculateTaskDesirability(ctx context.Context, myFactionID int, strategy string) []ScoredTask {
	// First fetch the active term ID
	var activeTermID int64
	err := i.pool.QueryRow(ctx, "SELECT term_id FROM dune.landsraad_decree_term ORDER BY start_time DESC LIMIT 1").Scan(&activeTermID)
	if err != nil {
		return nil
	}

	query := `
		SELECT t.id, t.board_index, t.completed, t.winning_faction_id, t.term_id, t.goal_amount,
		       COALESCE((SELECT SUM(amount) FROM dune.landsraad_task_faction_contributions WHERE task_id = t.id AND faction_id = $2), 0),
		       COALESCE(r.revealed, false)
		FROM dune.landsraad_tasks t
		LEFT JOIN dune.landsraad_task_reveal_state r ON t.id = r.task_id AND r.faction_id = $2
		WHERE t.term_id = $1
	`
	rows, err := i.pool.Query(ctx, query, activeTermID, myFactionID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	tasks := make(map[int]landsraadTaskData)
	var termID int64
	for rows.Next() {
		var t landsraadTaskData
		err := rows.Scan(&t.ID, &t.BoardIndex, &t.Completed, &t.WinningFactionID, &t.TermID, &t.GoalAmount, &t.CurrentProgress, &t.Revealed)
		if err == nil {
			tasks[t.BoardIndex] = t
			termID = t.TermID
		}
	}

	// Build a reverse map: task ID -> board index (needed for cheapest-block lookup)
	taskIDToIdx := make(map[int]int)
	for idx, t := range tasks {
		taskIDToIdx[t.ID] = idx
	}

	scores := make(map[int]float64)

	// Fix #1: Board-state-aware hash so the tie-breaker naturally re-rolls when
	// the board changes (e.g. a task completes), but stays stable between
	// identical ticks.
	boardHash := int64(0)
	for _, t := range tasks {
		if t.Completed && t.WinningFactionID != nil {
			boardHash += int64(t.BoardIndex) * int64(*t.WinningFactionID)
		}
	}

	// 1. Dynamically evaluate all bingo paths to find the best viable primary path
	var bestPaths [][]int
	bestPathScore := -1
	bestFriendlyCount := -1

	for _, path := range winPaths {
		friendlyCount := 0
		enemyCount := 0
		revealedCount := 0
		hiddenCount := 0
		for _, idx := range path {
			t := tasks[idx]
			if t.Completed {
				if t.WinningFactionID != nil && *t.WinningFactionID == myFactionID {
					friendlyCount++
				} else if t.WinningFactionID != nil {
					enemyCount++
				}
			} else if t.Revealed {
				revealedCount++
			} else {
				hiddenCount++
			}
		}

		// All strategies require fully unblocked paths (zero enemy completions).
		// focus_aggressive focuses on finishing own tasks fastest, not contesting
		// enemy territory.
		if enemyCount == 0 {
			// Fix #2: Penalize paths with unrevealed tasks — the bot can't
			// actually contribute to hidden squares yet, so prefer actionable paths.
			pathScore := friendlyCount*100 + revealedCount*10 - hiddenCount*20
			if pathScore > bestPathScore {
				bestPathScore = pathScore
				bestFriendlyCount = friendlyCount
				bestPaths = [][]int{path}
			} else if pathScore == bestPathScore {
				bestPaths = append(bestPaths, path)
			}
		}
	}

	var primaryPath []int
	if len(bestPaths) > 0 {
		// Tie-breaker: deterministic but board-state-aware so the bot naturally
		// pivots when the board materially changes (Fix #1).
		seed := termID + int64(myFactionID) + boardHash
		r := rand.New(rand.NewSource(seed))
		primaryPath = bestPaths[r.Intn(len(bestPaths))]
	}

	// 2. Calculate Offensive Scores
	if strategy != "focus_blocking" {
		if len(primaryPath) > 0 {
			for _, idx := range primaryPath {
				if !tasks[idx].Completed {
					scores[tasks[idx].ID] += 1000.0 // Base primary path bias
					if strategy == "focus_aggressive" {
						scores[tasks[idx].ID] += float64(bestFriendlyCount) * 400.0 // Aggressive momentum bonus (double)
					} else {
						scores[tasks[idx].ID] += float64(bestFriendlyCount) * 200.0 // Momentum bonus
					}
				}
			}
		}

		// Fix #5: "Close it out" bonus — if ANY path (not just the primary) has
		// 4/5 friendly completions, the remaining task gets a massive bonus so
		// the bot doesn't miss an easy bingo win.
		for _, path := range winPaths {
			friendlyCount := 0
			var remainingIDs []int
			for _, idx := range path {
				t := tasks[idx]
				if t.Completed && t.WinningFactionID != nil && *t.WinningFactionID == myFactionID {
					friendlyCount++
				} else if !t.Completed && t.Revealed {
					remainingIDs = append(remainingIDs, t.ID)
				}
			}
			if friendlyCount == 4 && len(remainingIDs) > 0 {
				for _, id := range remainingIDs {
					scores[id] += 50000.0 // Close-it-out bonus: one task away from bingo
				}
			}
		}
	}

	// 3. Defensive Blocking Threat (Evaluate all paths)
	if strategy != "focus_aggressive" {
		for _, path := range winPaths {
			oppCount := 0
			uncompletedCount := 0
			var uncompletedIDs []int

			for _, idx := range path {
				t := tasks[idx]
				if t.Completed {
					if t.WinningFactionID != nil && *t.WinningFactionID != myFactionID {
						oppCount++
					}
				} else {
					uncompletedCount++
					uncompletedIDs = append(uncompletedIDs, t.ID)
				}
			}

			if uncompletedCount > 0 {
				blockScore := 0.0
				if oppCount == 2 {
					blockScore = 200.0
				} else if oppCount == 3 {
					// 3/5: Competitive block, might be worth it over our own path
					blockScore = 800.0
				} else if oppCount == 4 {
					// 4/5: Drop everything and block with all might
					blockScore = 100000.0 // Critical block
				}

				if strategy == "focus_blocking" {
					blockScore *= 10.0 // Massive multiplier to prioritize any threat immediately
				}

				// Fix #3: When the enemy has a serious threat (≥3), concentrate
				// ALL block effort on the single cheapest task to complete. The
				// bot only needs to finish ONE task to break their line — spending
				// any XP on the others is wasteful.
				if oppCount >= 3 && len(uncompletedIDs) > 1 && blockScore > 0 {
					cheapestID := uncompletedIDs[0]
					cheapestReq := tasks[taskIDToIdx[cheapestID]].GoalAmount - tasks[taskIDToIdx[cheapestID]].CurrentProgress

					for _, id := range uncompletedIDs[1:] {
						idx := taskIDToIdx[id]
						req := tasks[idx].GoalAmount - tasks[idx].CurrentProgress
						if req < cheapestReq {
							cheapestReq = req
							cheapestID = id
						}
					}

					// Only the cheapest task gets block score — zero for the rest
					scores[cheapestID] += blockScore * 2.0
				} else {
					// Standard: apply block score evenly to all uncompleted tasks in that path
					for _, id := range uncompletedIDs {
						scores[id] += blockScore
					}
				}
			}
		}
	}

	// Fix #7: If focus_blocking produced no blocking scores (enemy has zero
	// progress on any path), fall back to auto offensive logic so the bot
	// doesn't wander aimlessly.
	if strategy == "focus_blocking" {
		hasBlockScores := false
		for _, s := range scores {
			if s > 0 {
				hasBlockScores = true
				break
			}
		}
		if !hasBlockScores && len(primaryPath) > 0 {
			for _, idx := range primaryPath {
				if !tasks[idx].Completed {
					scores[tasks[idx].ID] += 1000.0                             // Base primary path bias
					scores[tasks[idx].ID] += float64(bestFriendlyCount) * 200.0 // Momentum bonus (auto-level)
				}
			}
		}
	}

	// 4. Bandwagon Bias & Final Assembly
	var scoredTasks []ScoredTask
	for _, t := range tasks {
		if t.Completed || !t.Revealed {
			continue
		}

		score := scores[t.ID]
		// Add bandwagon bias based on completion percentage.
		// Fix #4: Scale down bandwagon for focus_blocking so it doesn't
		// accidentally make the bot finish its own tasks instead of blocking.
		pct := t.CurrentProgress / t.GoalAmount
		if pct > 1.0 {
			pct = 1.0
		}
		if strategy == "focus_blocking" {
			score += pct * 50.0 // Minimal bandwagon to break ties only
		} else {
			score += pct * 500.0
		}

		// Add a tiny random jitter (0 to 10) to break ties organically
		score += rand.Float64() * 10.0

		reqXP := t.GoalAmount - t.CurrentProgress
		if reqXP < 0 {
			reqXP = 0
		}

		scoredTasks = append(scoredTasks, ScoredTask{
			ID:    t.ID,
			Score: score,
			ReqXP: reqXP,
		})
	}

	// Sort by score descending
	sort.Slice(scoredTasks, func(i, j int) bool {
		return scoredTasks[i].Score > scoredTasks[j].Score
	})

	return scoredTasks
}
