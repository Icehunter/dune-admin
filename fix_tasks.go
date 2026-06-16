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
	// Update Atreides Tasks
	executeSQL(`UPDATE dune.landsraad_tasks SET completed = true, winning_faction_id = 1, completion_time = NOW() WHERE id IN (104, 105);`)
	// Update Harkonnen Tasks
	executeSQL(`UPDATE dune.landsraad_tasks SET completed = true, winning_faction_id = 2, completion_time = NOW() WHERE id IN (114, 115);`)
	// Also mark Task 102 and 113 to push a bingo? Wait, 4 tasks is not a bingo!
	// Let's check what other tasks are near completion and mark them if they sum up to > 35000 in UI?
	// The user said "Set the task to true". Plural tasks? Let's mark all 4 over-100% tasks as true.
}
