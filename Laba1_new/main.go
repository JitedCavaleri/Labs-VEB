package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Response struct {
	DaysBeforeNewYear int `json:"days_before_new_year"`
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	nextYear := now.Year() + 1

	newYear := time.Date(
		nextYear,
		time.January,
		1,
		0,
		0,
		0,
		0,
		now.Location(),
	)

	days := int(newYear.Sub(now).Hours() / 24)

	response := Response{
		DaysBeforeNewYear: days,
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {
	http.HandleFunc("/info", infoHandler)

	port := ":4200"

	log.Printf("Server started on %s", port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatal(err)
	}
}
