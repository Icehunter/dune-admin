package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	connStr := "postgres://dune:PhQLGpm6Q6AlJ8jDe4E7Ik3S@127.0.0.1:15432/dune?sslmode=disable"
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	var termID int64
	var start time.Time
	var end time.Time
	var winFaction *int
	var electedDecree *int
	err = pool.QueryRow(ctx, "SELECT term_id, start_time, end_time, winning_faction_id, elected_decree_id FROM dune.landsraad_decree_term ORDER BY start_time DESC LIMIT 1").Scan(&termID, &start, &end, &winFaction, &electedDecree)
	if err != nil {
		log.Fatal("fetch term:", err)
	}

	fmt.Printf("Term ID: %d\n", termID)
	fmt.Printf("Start: %v\n", start)
	fmt.Printf("End: %v\n", end)
	
	if winFaction != nil {
		fmt.Printf("Winning Faction: %d\n", *winFaction)
	} else {
		fmt.Printf("Winning Faction: NONE\n")
	}

	if electedDecree != nil {
		fmt.Printf("Elected Decree: %d\n", *electedDecree)
	} else {
		fmt.Printf("Elected Decree: NONE\n")
	}
}
