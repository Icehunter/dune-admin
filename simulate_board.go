package main

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

type Task struct {
	ID               int
	BoardIndex       int
	Completed        bool
	WinningFactionID *int
	GoalAmount       float64
	CurrentProgress  float64
}

type ScoredTask struct {
	ID    int
	Score float64
	ReqXP float64
}

var winPaths = [][]int{
	{0, 1, 2, 3, 4}, {5, 6, 7, 8, 9}, {10, 11, 12, 13, 14}, {15, 16, 17, 18, 19}, {20, 21, 22, 23, 24},
	{0, 5, 10, 15, 20}, {1, 6, 11, 16, 21}, {2, 7, 12, 17, 22}, {3, 8, 13, 18, 23}, {4, 9, 14, 19, 24},
	{0, 6, 12, 18, 24}, {4, 8, 12, 16, 20},
}

func calcDesirability(tasks map[int]*Task, myFactionID int) []ScoredTask {
	scores := make(map[int]float64)

	var bestPaths [][]int
	bestFriendlyCount := -1

	for _, path := range winPaths {
		friendlyCount := 0
		enemyCount := 0
		for _, idx := range path {
			t := tasks[idx]
			if t.Completed {
				if t.WinningFactionID != nil && *t.WinningFactionID == myFactionID {
					friendlyCount++
				} else if t.WinningFactionID != nil {
					enemyCount++
				}
			}
		}

		if enemyCount == 0 {
			if friendlyCount > bestFriendlyCount {
				bestFriendlyCount = friendlyCount
				bestPaths = [][]int{path}
			} else if friendlyCount == bestFriendlyCount {
				bestPaths = append(bestPaths, path)
			}
		}
	}

	var primaryPath []int
	if len(bestPaths) > 0 {
		// Use faction ID as tie-breaker seed to avoid bouncing
		seed := 5 + int64(myFactionID) // Hardcoded termID=5 for simulation
		r := rand.New(rand.NewSource(seed))
		primaryPath = bestPaths[r.Intn(len(bestPaths))]
	}

	if len(primaryPath) > 0 {
		for _, idx := range primaryPath {
			if !tasks[idx].Completed {
				scores[tasks[idx].ID] += 1000.0
				scores[tasks[idx].ID] += float64(bestFriendlyCount) * 200.0
			}
		}
	}

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
				blockScore = 800.0
			} else if oppCount == 4 {
				blockScore = 100000.0
			}
			for _, id := range uncompletedIDs {
				scores[id] += blockScore
			}
		}
	}

	var scoredTasks []ScoredTask
	for _, t := range tasks {
		if t.Completed { continue }
		score := scores[t.ID]
		pct := t.CurrentProgress / t.GoalAmount
		if pct > 1.0 { pct = 1.0 }
		score += pct * 500.0
		score += rand.Float64() * 10.0

		reqXP := t.GoalAmount - t.CurrentProgress
		if reqXP < 0 { reqXP = 0 }
		scoredTasks = append(scoredTasks, ScoredTask{ID: t.ID, Score: score, ReqXP: reqXP})
	}

	sort.Slice(scoredTasks, func(i, j int) bool {
		return scoredTasks[i].Score > scoredTasks[j].Score
	})
	return scoredTasks
}

func checkWin(tasks map[int]*Task) *int {
	for _, path := range winPaths {
		count1 := 0
		count2 := 0
		for _, idx := range path {
			t := tasks[idx]
			if t.Completed && t.WinningFactionID != nil {
				if *t.WinningFactionID == 1 { count1++ }
				if *t.WinningFactionID == 2 { count2++ }
			}
		}
		if count1 == 5 { v := 1; return &v }
		if count2 == 5 { v := 2; return &v }
	}
	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	atreidesBuff := 1.0
	harkonnenBuff := 1.0
	
	for sim := 1; sim <= 10; sim++ {
		tasks := make(map[int]*Task)
		for i := 0; i < 25; i++ {
			tasks[i] = &Task{ID: i, BoardIndex: i, GoalAmount: 35000}
		}

		totalTickXP := ((35000.0 * 10) / (7.0 * 1440)) // BUDGET OF 10!
		ticks := 0
		var winner *int

		for {
			ticks++
			if ticks > 10080 {
				break
			}

			// Randomize tick order to avoid first-mover advantage
			f1, f2 := 1, 2
			b1, b2 := atreidesBuff, harkonnenBuff
			if rand.Float64() > 0.5 {
				f1, f2 = 2, 1
				b1, b2 = harkonnenBuff, atreidesBuff
			}

			scored1 := calcDesirability(tasks, f1)
			budget1 := float64(int(totalTickXP * b1 * (0.6 + rand.Float64()*0.8)))
			for i := 0; i < 3 && i < len(scored1); i++ {
				t := tasks[scored1[i].ID]
				t.CurrentProgress += budget1 / 3.0
				if t.CurrentProgress >= t.GoalAmount {
					t.Completed = true
					fid := f1
					t.WinningFactionID = &fid
				}
			}

			scored2 := calcDesirability(tasks, f2)
			budget2 := float64(int(totalTickXP * b2 * (0.6 + rand.Float64()*0.8)))
			for i := 0; i < 3 && i < len(scored2); i++ {
				t := tasks[scored2[i].ID]
				t.CurrentProgress += budget2 / 3.0
				if t.CurrentProgress >= t.GoalAmount {
					t.Completed = true
					if t.WinningFactionID == nil {
						fid := f2
						t.WinningFactionID = &fid
					}
				}
			}

			winner = checkWin(tasks)
			if winner != nil {
				break
			}
		}

		if winner == nil {
			fmt.Printf("Sim %2d | DRAW      | Day 7.00 | Buffs - A: %3.0f%%, H: %3.0f%%\n", sim, (atreidesBuff-1)*100, (harkonnenBuff-1)*100)
			atreidesBuff += 0.10
			harkonnenBuff += 0.10
		} else if *winner == 1 {
			fmt.Printf("Sim %2d | ATREIDES  | Day %.2f | Buffs - A: %3.0f%%, H: %3.0f%%\n", sim, float64(ticks)/1440.0, (atreidesBuff-1)*100, (harkonnenBuff-1)*100)
			atreidesBuff = 1.0
			harkonnenBuff += 0.10
		} else {
			fmt.Printf("Sim %2d | HARKONNEN | Day %.2f | Buffs - A: %3.0f%%, H: %3.0f%%\n", sim, float64(ticks)/1440.0, (atreidesBuff-1)*100, (harkonnenBuff-1)*100)
			harkonnenBuff = 1.0
			atreidesBuff += 0.10
		}
	}
}
