package main

import (
	"context"
	"fmt"
	"log"

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
	err = pool.QueryRow(ctx, "SELECT term_id FROM dune.landsraad_decree_term ORDER BY start_time DESC LIMIT 1").Scan(&termID)
	if err != nil {
		log.Fatal("fetch term:", err)
	}

	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM dune.landsraad_tasks WHERE term_id =  AND completed = true", termID).Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Completed tasks left: %d\n", count)

	err = pool.QueryRow(ctx, "SELECT count(*) FROM dune.landsraad_task_faction_contributions WHERE task_id IN (SELECT id FROM dune.landsraad_tasks WHERE term_id = )", termID).Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Faction contributions left: %d\n", count)
}
