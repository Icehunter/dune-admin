package main
import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)
func main() {
	pool, _ := pgxpool.New(context.Background(), "postgres://dune:PhQLGpm6Q6AlJ8jDe4E7Ik3S@127.0.0.1:15432/dune?sslmode=disable")
	defer pool.Close()
	
	rows, _ := pool.Query(context.Background(), "SELECT typname FROM pg_type WHERE typname = 'landsraadtask'")
	for rows.Next() {
		var typ string
		rows.Scan(&typ)
		fmt.Printf("Type found: %s\n", typ)
	}
}
