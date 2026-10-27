package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

// InitDB handles the SQLite connection and schema setup, then automatically
// seeds the database from seed.sql on every startup. Because all seed
// statements use INSERT OR IGNORE, running against an already-populated
// database is safe — existing rows are never overwritten.
func InitDB() (*sql.DB, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/fantasylol.db"
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply schema (all CREATE TABLE IF NOT EXISTS — safe to re-run)
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema.sql: %w", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		return nil, fmt.Errorf("failed to execute schema.sql: %w", err)
	}

	// Seed demo data (INSERT OR IGNORE — safe to re-run, never overwrites)
	SeedData(db)

	return db, nil
}

// SeedData reads seed.sql and executes its contents against the database.
// Existing records (matching handles) are skipped automatically via INSERT OR IGNORE.
func SeedData(db *sql.DB) {
	log.Println("[SEED DATA] Reading seed.sql file...")

	sqlContent, err := os.ReadFile("seed.sql")
	if err != nil {
		log.Printf("[SEED DATA ERROR] Failed to read seed.sql file: %v", err)
		return
	}

	log.Println("[SEED DATA] Seeding database with SQL statements...")

	_, err = db.Exec(string(sqlContent))
	if err != nil {
		log.Printf("[SEED DATA ERROR] Failed to execute seed SQL statements: %v", err)
		return
	}

	log.Println("[SEED DATA] Successfully seeded database from seed.sql!")
}

// setupRoutes registers all application endpoints on a new ServeMux
func setupRoutes(db *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()

	adminHandler := &AdminHandler{DB: db}

	mux.HandleFunc("/api/stats", StatsAPIHandler(db))

	// Health & Utility Endpoints
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello World from FantasyLoL Backend!")
	})

	// Admin Endpoints
	mux.HandleFunc("/admin/stats/simulate", SimulateStatsHandler)
	mux.HandleFunc("/admin/stats/update", adminHandler.UpdatePlayerStatsHandler)
	mux.HandleFunc("/admin/stats/reset-date", adminHandler.ResetStatsDateHandler)
	mux.HandleFunc("/admin/leaderboard/calculate", adminHandler.CalculateLeaderboardHandler)

	// Static File Server
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("/", fs)

	return mux
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] No .env file found or failed to load, falling back to system environment variables")
	}

	db, err := InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// --- Startup Ingestion Prompt ---
	// The database is already seeded inside InitDB(). This prompt only runs
	// when INGEST_REMOTE=true and asks whether to kick off a live sync
	// against the Leaguepedia API in the background. Saying no (or pressing
	// Enter) starts the server immediately with the existing seeded data.
	if os.Getenv("INGEST_REMOTE") == "true" {
		fmt.Println("\n==================================================================")
		fmt.Println("⚠️  Live remote data ingestion from Leaguepedia Cargo API is")
		fmt.Println("   enabled. Due to Fandom rate limits and retry pacing, this")
		fmt.Println("   process may take SEVERAL MINUTES to complete.")
		fmt.Println("==================================================================")
		fmt.Print("Run live data ingestion now? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "y" || input == "yes" {
			go func() {
				log.Println("[INGEST] Starting background live data ingestion...")
				if err := TriggerLiveIngestion(db); err != nil {
					log.Printf("[INGEST ERROR] Live data ingestion failed: %v", err)
				} else {
					log.Println("[INGEST COMPLETE] Live data ingestion completed successfully.")
				}
			}()
		} else {
			log.Println("[STARTUP] Live ingestion skipped — server starting with seeded data.")
		}
	}

	mux := setupRoutes(db)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server listening on port %s...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
