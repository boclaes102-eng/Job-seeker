package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"job-seeker/server/internal/api"
	"job-seeker/server/internal/matcher"
	"job-seeker/server/internal/store"
)

func main() {
	// Try both relative paths: running from server/ or from server/cmd/server/
	if err := godotenv.Load("../.env"); err != nil {
		_ = godotenv.Load("../../.env")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "jobs.db"
	}

	s, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	m := matcher.New()
	profilePath := api.ProfilePath()
	h := api.NewHandler(s, m, profilePath)
	router := api.NewRouter(h)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Job Seeker API running on http://localhost:%s\n", port)
	fmt.Printf("Profile: %s\n", profilePath)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
