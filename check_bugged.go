package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	resp, err := http.Get("http://localhost:8080/api/v1/landsraad")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	
	b, _ := io.ReadAll(resp.Body)
	var data struct {
		Term struct {
			EndTime string `json:"end_time"`
		} `json:"term"`
		Tasks []struct {
			ID              int    `json:"id"`
			House           string `json:"house"`
			Completed       bool   `json:"completed"`
			GoalAmount      int    `json:"goal_amount"`
			CurrentProgress int    `json:"current_progress"`
		} `json:"tasks"`
	}
	
	if err := json.Unmarshal(b, &data); err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("End Time: %s\n", data.Term.EndTime)
	fmt.Println("Bugged Tasks:")
	for _, t := range data.Tasks {
		if t.CurrentProgress >= 35000 && !t.Completed {
			fmt.Printf("- %d %s (Progress: %d)\n", t.ID, t.House, t.CurrentProgress)
		}
	}
}
