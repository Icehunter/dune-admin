package main

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "C:/Users/diya0/.dune-admin/dune-admin.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var configJson string
	err = db.QueryRow("SELECT config_json FROM landsraad_bot_config WHERE id = 1").Scan(&configJson)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("CONFIG:", configJson)
}
