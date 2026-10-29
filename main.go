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

	// Page Routes
	// These pages are not implemented yet. Each currently renders a shared
	// "coming soon" placeholder.
	//
	// Exact-match patterns (no trailing slash) only match that precise path,
	// so they take precedence over the "/" static file server below.
	mux.HandleFunc("/login", comingSoonHandler("Log In / Sign Up"))
	mux.HandleFunc("/leagues-search", comingSoonHandler("Search Leagues"))
	mux.HandleFunc("/league", comingSoonHandler("League"))
	mux.HandleFunc("/create-league", comingSoonHandler("Create / Edit League"))
	mux.HandleFunc("/edit-team", comingSoonHandler("Edit Team"))
	mux.HandleFunc("/pro-player-search", comingSoonHandler("Pro Player Search"))
	mux.HandleFunc("/my-leagues", comingSoonHandler("My Leagues"))

	// Static File Server
	// Mounted at "/" so it serves index.html for the root and any other
	// static assets (CSS, images, etc.) not claimed by an explicit route above.
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("/", fs)

	return mux
}

// comingSoonHandler returns an http.HandlerFunc that renders a simple
// placeholder page for a not-yet-implemented route. The pageName is shown
// to the user so it's clear which feature is pending.
func comingSoonHandler(pageName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s — Coming Soon | FantasyLoL</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #0f1419;
            color: #e6e6e6;
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            text-align: center;
        }
        .card {
            padding: 2.5rem 3rem;
            border: 1px solid #2a2f3a;
            border-radius: 12px;
            background: #161b22;
            max-width: 480px;
        }
        h1 { margin: 0 0 0.5rem; font-size: 1.75rem; }
        .badge {
            display: inline-block;
            margin-bottom: 1rem;
            padding: 0.25rem 0.75rem;
            border-radius: 999px;
            background: #1f6feb;
            color: #fff;
            font-size: 0.8rem;
            letter-spacing: 0.05em;
            text-transform: uppercase;
        }
        p { color: #9da5b4; line-height: 1.5; }
        a { color: #58a6ff; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="card">
        <span class="badge">Coming Soon</span>
        <h1>%s</h1>
        <p>This page is under construction and will be available soon.</p>
        <p><a href="/">&larr; Back to home</a></p>
    </div>
</body>
</html>`, pageName, pageName)
	}
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
