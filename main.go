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

// InitDB handles the SQLite connection and schema setup
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

	// Read and execute schema.sql
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema.sql: %w", err)
	}

	if _, err := db.Exec(string(schema)); err != nil {
		return nil, fmt.Errorf("failed to execute schema.sql: %w", err)
	}

	return db, nil
}

// SeedDummyData populates local DB with sample players and stats if live import is skipped
func SeedDummyData(db *sql.DB) {
	log.Println("[DUMMY DATA] Seeding local database with mock players and match stats...")

	type MockPlayer struct {
		Handle  string
		Team    string
		Role    string
		Region  string
		Image   string
		Kills   int
		Deaths  int
		Assists int
		CS      int
		Points  float64
	}

	players := []MockPlayer{
		{"Faker", "T1", "Mid", "LCK", "https://lol.fandom.com/wiki/Special:FilePath/Faker_2024_Split_1.png", 6, 1, 8, 280, 24.5},
		{"Chovy", "Gen.G", "Mid", "LCK", "https://lol.fandom.com/wiki/Special:FilePath/Chovy_2024_Split_1.png", 8, 0, 5, 340, 28.2},
		{"Caps", "G2 Esports", "Mid", "LEC", "https://lol.fandom.com/wiki/Special:FilePath/Caps_2024_Split_1.png", 7, 3, 9, 290, 23.8},
		{"Blaber", "Cloud9", "Jungle", "LCS", "https://lol.fandom.com/wiki/Special:FilePath/Blaber_2024_Split_1.png", 4, 2, 11, 195, 19.4},
		{"JackeyLove", "Top Esports", "Bot", "LPL", "https://lol.fandom.com/wiki/Special:FilePath/JackeyLove_2024_Split_1.png", 10, 2, 4, 310, 31.0},
		{"Keria", "T1", "Support", "LCK", "https://lol.fandom.com/wiki/Special:FilePath/Keria_2024_Split_1.png", 1, 2, 16, 45, 18.5},
	}

	playerStmt, err := db.Prepare(`
		INSERT INTO pro_players (handle, pro_team, role, region, image_url)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(handle) DO UPDATE SET
			pro_team = excluded.pro_team,
			role = excluded.role,
			region = excluded.region,
			image_url = excluded.image_url;
	`)
	if err != nil {
		log.Printf("[DUMMY DATA ERROR] Failed to prepare player statement: %v", err)
		return
	}
	defer playerStmt.Close()

	statStmt, err := db.Prepare(`
		INSERT INTO pro_match_stats (
			player_id, match_date, week_number, win, kills, deaths, assists, creep_score, fantasy_points
		) VALUES (?, '2026-01-15', 1, 1, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		log.Printf("[DUMMY DATA ERROR] Failed to prepare stats statement: %v", err)
		return
	}
	defer statStmt.Close()

	for _, p := range players {
		_, err := playerStmt.Exec(p.Handle, p.Team, p.Role, p.Region, p.Image)
		if err != nil {
			continue
		}

		var playerID int
		if err := db.QueryRow("SELECT player_id FROM pro_players WHERE handle = ?", p.Handle).Scan(&playerID); err == nil {
			_, _ = db.Exec("DELETE FROM pro_match_stats WHERE player_id = ?", playerID)
			_, _ = statStmt.Exec(playerID, p.Kills, p.Deaths, p.Assists, p.CS, p.Points)
		}
	}

	log.Println("[DUMMY DATA] Successfully seeded database with mock rosters and statistics!")
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

	// --- Startup Prompt & Data Ingestion Decision ---
	if os.Getenv("INGEST_REMOTE") == "true" {
		fmt.Println("\n==================================================================")
		fmt.Println("⚠️  WARNING: Live remote data ingestion from Leaguepedia Cargo API")
		fmt.Println("   is enabled. Due to Fandom rate limits and retry pacing,")
		fmt.Println("   this process may take SEVERAL MINUTES to complete.")
		fmt.Println("==================================================================")
		fmt.Print("Would you like to run live data ingestion now? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "y" || input == "yes" {
			go func() {
				log.Println("Starting background live data ingestion...")
				if err := TriggerLiveIngestion(db); err != nil {
					log.Printf("[INGEST ERROR] Live data ingestion failed: %v", err)
				} else {
					log.Println("[INGEST COMPLETE] Live data ingestion completed successfully.")
				}
			}()
		} else {
			log.Println("Skipping live data ingestion. Seeding dummy data instead...")
			SeedDummyData(db)
		}
	} else {
		log.Println("INGEST_REMOTE is disabled. Seeding database with local/dummy content...")
		SeedDummyData(db)
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
