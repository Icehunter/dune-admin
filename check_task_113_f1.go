package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
)

func executeSQL(sql string) {
	body := []byte(fmt.Sprintf(`{"sql":"%s"}`, sql))
	req, _ := http.NewRequest("POST", "http://localhost:8080/api/v1/database/sql", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("Query: %s\nResponse: %s\n\n", sql, string(b))
}

func main() {
	executeSQL(`SELECT task_id, faction_id, amount FROM dune.landsraad_task_faction_contributions WHERE task_id = 113 AND faction_id = 1`)
}
