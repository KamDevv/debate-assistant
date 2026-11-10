package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/yourname/british-debate-assistant/internal/api"
	"github.com/yourname/british-debate-assistant/internal/config"
	"github.com/yourname/british-debate-assistant/internal/store"
)

func main() {
	if envPath, err := config.LoadDotEnv(); err != nil {
		log.Printf("Warning: could not load .env: %v", err)
	} else if envPath != "" {
		log.Printf("Loaded configuration from %s", envPath)
	}

	dbPath := getenv("DB_PATH", "./debates.db")
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			log.Fatal(err)
		}
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	handler := api.New(db)
	addr := getenv("ADDR", ":8080")
	server := &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Debate Assistant API listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
