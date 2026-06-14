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

func (i *Instance) calculateTaskDesirability(ctx context.Context, myFactionID int) []ScoredTask {
	// First fetch the active term ID
	var activeTermID int64
	err := i.pool.QueryRow(ctx, "SELECT term_id FROM dune.landsraad_decree_term ORDER BY start_time DESC LIMIT 1").Scan(&activeTermID)
	if err != nil {
		return nil
	}

	query := `
		SELECT t.id, t.board_index, t.completed, t.winning_faction_id, t.term_id, t.goal_amount,
		       (COALESCE((SELECT SUM(amount) FROM dune.landsraad_task_guild_contributions WHERE task_id = t.id), 0) +
		        COALESCE((SELECT SUM(amount) FROM dune.landsraad_task_faction_contributions WHERE task_id = t.id), 0) +
		        COALESCE((SELECT SUM(amount) FROM dune.landsraad_task_player_contributions WHERE task_id = t.id), 0)) 
		FROM dune.landsraad_tasks t
		WHERE t.term_id = $1
	`
	rows, err := i.pool.Query(ctx, query, activeTermID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	tasks := make(map[int]landsraadTaskData)
	var termID int64
	for rows.Next() {
		var t landsraadTaskData
		err := rows.Scan(&t.ID, &t.BoardIndex, &t.Completed, &t.WinningFactionID, &t.TermID, &t.GoalAmount, &t.CurrentProgress)
		if err == nil {
			tasks[t.BoardIndex] = t
			termID = t.TermID
		}
	}

	scores := make(map[int]float64)

	// 1. Identify primary offensive path deterministically
	seed := termID + int64(myFactionID)
	r := rand.New(rand.NewSource(seed))
	primaryPathIdx := r.Intn(12)
	primaryPath := winPaths[primaryPathIdx]

	// 2. Calculate Offensive Scores
	myPrimaryCount := 0
	oppPrimaryCount := 0
	for _, idx := range primaryPath {
		t := tasks[idx]
		if t.Completed {
			if t.WinningFactionID != nil && *t.WinningFactionID == myFactionID {
				myPrimaryCount++
			} else if t.WinningFactionID != nil {
				oppPrimaryCount++
			}
		}
	}

	// Only apply offensive bias if path is still winnable
	if oppPrimaryCount == 0 {
		for _, idx := range primaryPath {
			if !tasks[idx].Completed {
				scores[tasks[idx].ID] += 1000.0                       // Base primary path bias
				scores[tasks[idx].ID] += float64(myPrimaryCount) * 200.0 // Momentum bias
			}
		}
	}

	// 3. Defensive Blocking Threat (Evaluate all paths)
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

			// Apply block score to all uncompleted tasks in that path
			for _, id := range uncompletedIDs {
				scores[id] += blockScore
			}
		}
	}

	// 4. Bandwagon Bias & Final Assembly
	var scoredTasks []ScoredTask
	for _, t := range tasks {
		if t.Completed {
			continue
		}

		score := scores[t.ID]
		// Add bandwagon bias (0 to 500) based on completion percentage
		pct := t.CurrentProgress / t.GoalAmount
		if pct > 1.0 {
			pct = 1.0
		}
		score += pct * 500.0

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
