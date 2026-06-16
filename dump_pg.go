package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	connStr := "host=127.0.0.1 port=15432 user=dune password=PhQLGpm6Q6AlJ8jDe4E7Ik3S dbname=dune sslmode=disable"
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	rows, err := pool.Query(context.Background(), "SELECT task_id, amount FROM dune.landsraad_task_faction_contributions WHERE amount > 0")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var taskId int
		var amount float64
		rows.Scan(&taskId, &amount)
		fmt.Printf("Faction Contribution - Task %d: %.2f XP\n", taskId, amount)
	}
}
