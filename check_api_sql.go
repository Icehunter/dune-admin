package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	body := []byte(`{"sql":"SELECT term_id, start_time, end_time, winning_faction_id, elected_decree_id FROM dune.landsraad_decree_term ORDER BY start_time DESC LIMIT 2"}`)
	req, _ := http.NewRequest("POST", "http://localhost:8080/api/v1/database/sql", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Println(string(b))
}
