package main
import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)
func main() {
	pool, err := pgxpool.New(context.Background(), "postgres://dune:PhQLGpm6Q6AlJ8jDe4E7Ik3S@127.0.0.1:15432/dune?sslmode=disable")
	if err != nil { fmt.Println("err1", err); return }
	defer pool.Close()
	
	// Get active term
	var termID int
	err = pool.QueryRow(context.Background(), "SELECT term_id FROM dune.landsraad_decree_term ORDER BY term_id DESC LIMIT 1").Scan(&termID)
	if err != nil { fmt.Println("err2", err); return }
	
	fmt.Printf("Term ID: %d\n", termID)
	
	// Get total progress
	rows, err := pool.Query(context.Background(), "SELECT guild_id, SUM(amount) FROM dune.landsraad_task_guild_contributions WHERE task_id IN (SELECT id FROM dune.landsraad_tasks WHERE term_id = ) GROUP BY guild_id", termID)
	if err != nil { fmt.Println("err3", err); return }
	defer rows.Close()
	
	for rows.Next() {
		var guildID int
		var totalAmount int
		rows.Scan(&guildID, &totalAmount)
		fmt.Printf("Guild %d injected %d total XP\n", guildID, totalAmount)
	}
}
