package main
import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)
func main() {
	pool, _ := pgxpool.New(context.Background(), "postgres://dune:PhQLGpm6Q6AlJ8jDe4E7Ik3S@127.0.0.1:15432/dune?sslmode=disable")
	defer pool.Close()
	_, err := pool.Exec(context.Background(), "SELECT dune.landsraad_collect_votes(4)")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Success")
	}
}
