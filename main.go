package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // <--- Blank import registers the "sqlite" driver
)

// initDB handles the SQLite connection and schema setup
func InitDB() (*sql.DB, error) {
	// Determine database path from environment variable or default
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/fantasylol.db"
	}

	// Ensure parent directory exists (e.g., ./data)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open connection (creates file if missing)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 0. Initialize SQLite Database
	database, err := InitDB()
	if err != nil {
		log.Fatalf("Database initialization error: %v", err)
	}
	defer database.Close()
	fmt.Println("SQLite Database initialized successfully!")

	// 1. Health check endpoint
	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 2. Sample API endpoint
	http.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello World from FantasyLoL Backend!")
	})

	http.HandleFunc("/admin/stats/simulate", SimulateStatsHandler)

	// 3. Serve static files from the ./public directory
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	fmt.Printf("Server listening on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		panic(err)
	}
}
