package main
import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)
func main() {
	pool, _ := pgxpool.New(context.Background(), "postgres://dune:PhQLGpm6Q6AlJ8jDe4E7Ik3S@127.0.0.1:15432/dune?sslmode=disable")
	defer pool.Close()
	rows, _ := pool.Query(context.Background(), "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'landsraad_decree_votes'")
	for rows.Next() {
		var col, typ string
		rows.Scan(&col, &typ)
		fmt.Printf("%s %s\n", col, typ)
	}
}
