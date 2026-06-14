package landsraadbot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BotConfig struct {
	Enabled              bool
	ProgressRate         float64
	SimultaneousTargets  int
	TargetCompletionDays float64
	AtreidesGuildID      int64
	HarkonnenGuildID     int64
	AtreidesStrategy     string
	HarkonnenStrategy    string
	AtreidesTargetTask   int
	HarkonnenTargetTask  int
	AtreidesTargetDecree int
	HarkonnenTargetDecree int
	TickIntervalSeconds   int
	TickJitterSeconds     int
}

type Instance struct {
	pool   *pgxpool.Pool
	mu     sync.Mutex
	cfg    BotConfig
	cancel context.CancelFunc
}

func Run(ctx context.Context, pool *pgxpool.Pool, cfg BotConfig) (*Instance, error) {
	if pool == nil {
		return nil, fmt.Errorf("landsraadbot requires db pool")
	}
	ctx, cancel := context.WithCancel(ctx)
	inst := &Instance{
		pool:   pool,
		cfg:    cfg,
		cancel: cancel,
	}

	go inst.runLoop(ctx)
	return inst, nil
}

func (i *Instance) ReloadConfig(cfg BotConfig) {
	i.mu.Lock()
	i.cfg = cfg
	i.mu.Unlock()

	// Always trigger an immediate tick on reload for responsiveness during testing
	if cfg.Enabled {
		go i.tick(context.Background())
	}
}

func (i *Instance) Stop() {
	if i.cancel != nil {
		i.cancel()
	}
}

func (i *Instance) runLoop(ctx context.Context) {
	// Perform an initial tick right when the bot starts
	i.tick(ctx)

	for {
		i.mu.Lock()
		cfg := i.cfg
		i.mu.Unlock()

		interval := cfg.TickIntervalSeconds
		if interval <= 0 {
			interval = 300 // default fallback
		}
		jitter := cfg.TickJitterSeconds
		if jitter < 0 {
			jitter = 0
		}
		
		delaySec := interval
		if jitter > 0 {
			// jitter is +/- jitter
			j := rand.Intn(jitter*2 + 1) - jitter
			delaySec += j
		}
		if delaySec < 1 {
			delaySec = 1
		}
		
		timer := time.NewTimer(time.Duration(delaySec) * time.Second)
		
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			i.tick(ctx)
		}
	}
}

func (i *Instance) tick(ctx context.Context) {
	i.mu.Lock()
	cfg := i.cfg
	i.mu.Unlock()

	if !cfg.Enabled {
		return
	}

	log.Printf("landsraadbot: running simulation tick")

	// Dynamic XP Scaling with Jitter
	interval := cfg.TickIntervalSeconds
	if interval <= 0 {
		interval = 300
	}
	ticksPerDay := 86400.0 / float64(interval)
	
	// Fetch the actual game term start and end times to perfectly pace the progress
	var termStart time.Time
	var termEnd time.Time
	err := i.pool.QueryRow(ctx, "SELECT start_time, end_time FROM dune.landsraad_decree_term ORDER BY start_time DESC LIMIT 1").Scan(&termStart, &termEnd)
	if err != nil {
		log.Printf("landsraadbot: failed to fetch active term for pacing calculation: %v", err)
		return
	}

	// Calculate the actual total duration of the term in days
	actualTermDays := termEnd.Sub(termStart).Hours() / 24.0
	if actualTermDays <= 0 {
		actualTermDays = 7.0 // Safe fallback
	}

	// Scale the budget so the bot finishes 8 full tasks (5 for a winning bingo, plus ~3 buffer tasks for blocking the opponent) over the length of the term
	totalTickXP := ((35000.0 * 8) / (actualTermDays * ticksPerDay)) * (cfg.ProgressRate / 100.0)
	
	if totalTickXP <= 0 {
		return
	}

	// Jitter: +/- 40%
	jitter := 0.6 + rand.Float64()*0.8
	actualTotalXP := int(totalTickXP * jitter)

	if actualTotalXP <= 0 {
		actualTotalXP = 1
	}

	i.simulateFaction(ctx, 1, cfg.AtreidesGuildID, cfg.AtreidesStrategy, cfg.AtreidesTargetTask, cfg.AtreidesTargetDecree, actualTotalXP, cfg.SimultaneousTargets)
	i.simulateFaction(ctx, 2, cfg.HarkonnenGuildID, cfg.HarkonnenStrategy, cfg.HarkonnenTargetTask, cfg.HarkonnenTargetDecree, actualTotalXP, cfg.SimultaneousTargets)
}

func (i *Instance) simulateFaction(ctx context.Context, factionID int, guildID int64, strategy string, targetTask int, targetDecree int, xp int, numTargets int) {
	if guildID == 0 {
		return
	}
	
	// Fetch current term status
	var termID int64
	var winningFactionID *int
	var electedDecreeID *int64
	err := i.pool.QueryRow(ctx, `
		SELECT term_id, winning_faction_id, elected_decree_id 
		FROM dune.landsraad_decree_term 
		ORDER BY start_time DESC LIMIT 1
	`).Scan(&termID, &winningFactionID, &electedDecreeID)
	
	if err != nil {
		log.Printf("landsraadbot: failed to fetch term: %v", err)
		return
	}

	// Stop task progress injection if the term has been won by someone
	if winningFactionID != nil {
		// Vote for decree if voting window is still open and bot's faction won
		votingOpen := electedDecreeID == nil
		if votingOpen && targetDecree != 0 && *winningFactionID == factionID {
			// Calculate total influence earned during the term
			var influence *int
			err = i.pool.QueryRow(ctx, "SELECT FLOOR(dune.landsraad_load_guild_contribution($1, $2, $3))::INTEGER", termID, guildID, factionID).Scan(&influence)
			if err == nil && influence != nil && *influence > 0 {
				
				finalDecree := targetDecree
				if finalDecree == -1 {
					// Auto-Vote: Randomly pick one of the active decrees
					err = i.pool.QueryRow(ctx, `
						SELECT r.decree_id 
						FROM dune.landsraad_decree_rotation r 
						ORDER BY RANDOM() LIMIT 1
					`).Scan(&finalDecree)
					if err != nil {
						log.Printf("landsraadbot: failed to auto-pick decree: %v", err)
						finalDecree = 0
					}
				}
				
				if finalDecree > 0 {
					_, err = i.pool.Exec(ctx, `
						INSERT INTO dune.landsraad_decree_votes (decree_id, guild_id, player_id, influence)
						VALUES ($1, $2, -1, $3)
						ON CONFLICT(decree_id, guild_id, player_id) DO UPDATE SET influence = excluded.influence
					`, finalDecree, guildID, *influence)
					if err != nil {
						log.Printf("landsraadbot: failed to cast vote for guild %d: %v", guildID, err)
					}
				}
			}
		}
		
		// If the board is won, we do not inject any more task progress!
		return
	}
	
	// Task Progress Injection
	if strategy == "manual" && targetTask > 0 {
		i.injectXP(ctx, factionID, guildID, targetTask, xp)
	} else if strategy == "auto" {
		scoredTasks := i.calculateTaskDesirability(ctx, factionID)
		if len(scoredTasks) == 0 {
			return
		}
		
		if len(scoredTasks) > numTargets {
			scoredTasks = scoredTasks[:numTargets]
		}
		
		budgetRemaining := float64(xp)
		for idx, st := range scoredTasks {
			if budgetRemaining <= 0 {
				break
			}
			
			// Recalculate score of remaining targets to distribute remaining budget
			remScore := 0.0
			for j := idx; j < len(scoredTasks); j++ {
				remScore += scoredTasks[j].Score
			}
			if remScore <= 0 {
				remScore = 1.0
			}
			
			splitXP := budgetRemaining * (st.Score / remScore)
			
			if splitXP > st.ReqXP {
				splitXP = st.ReqXP
			}
			
			intXP := int(splitXP)
			if intXP > 0 {
				budgetRemaining -= float64(intXP)
				i.injectXP(ctx, factionID, guildID, st.ID, intXP)
			}
		}
	} else {
		// random
		query := `
			SELECT l.id 
			FROM dune.landsraad_tasks l
			WHERE l.completed = false AND l.term_id = $1
			ORDER BY RANDOM() LIMIT $2
		`
		rows, err := i.pool.Query(ctx, query, termID, numTargets)
		if err != nil {
			return
		}
		defer rows.Close()

		var taskIDs []int
		for rows.Next() {
			var tid int
			if err := rows.Scan(&tid); err == nil {
				taskIDs = append(taskIDs, tid)
			}
		}
		
		if len(taskIDs) > 0 {
			splitXP := xp / len(taskIDs)
			if splitXP > 0 {
				for _, tid := range taskIDs {
					i.injectXP(ctx, factionID, guildID, tid, splitXP)
				}
			}
		}
	}
}

func (i *Instance) injectXP(ctx context.Context, factionID int, guildID int64, tid int, xp int) {
	var progressID int64
	err := i.pool.QueryRow(ctx, `
		INSERT INTO dune.landsraad_task_progress (faction_id, task_id, faction_progress, guild_progress, player_progress, timestamp)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id
	`, factionID, tid, xp, xp, 0).Scan(&progressID)
	if err != nil {
		log.Printf("landsraadbot: failed to insert progress: %v", err)
		return
	}
	
	_, err = i.pool.Exec(ctx, `
		INSERT INTO dune.landsraad_task_progress_guild (progress_id, guild_id)
		VALUES ($1, $2)
	`, progressID, guildID)
	if err != nil {
		log.Printf("landsraadbot: failed to insert progress: %v", err)
		return
	}
	
	// Force game server aggregation logic
	_, err = i.pool.Exec(ctx, `SELECT dune.landsraad_process_task_progress(100)`)
	if err != nil {
		log.Printf("landsraadbot: failed to process progress: %v", err)
	}
}
