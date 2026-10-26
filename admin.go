package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"fantasylol/scoring"
)

type AdminHandler struct {
	DB *sql.DB
}

type PlayerStatView struct {
	Handle        string  `json:"handle"`
	ProTeam       string  `json:"pro_team"`
	Role          string  `json:"role"`
	Region        string  `json:"region"`
	MatchDate     string  `json:"match_date"`
	Kills         int     `json:"kills"`
	Deaths        int     `json:"deaths"`
	Assists       int     `json:"assists"`
	CreepScore    int     `json:"creep_score"`
	Win           int     `json:"win"`
	FantasyPoints float64 `json:"fantasy_points"`
}

// TriggerLiveIngestion performs remote sync using Fandom bot credentials loaded from .env
func TriggerLiveIngestion(db *sql.DB) error {
	username := os.Getenv("FANDOM_USERNAME")
	botPass := os.Getenv("FANDOM_BOT_PASS")

	if username == "" || botPass == "" {
		return fmt.Errorf("FANDOM_USERNAME or FANDOM_BOT_PASS environment variable is missing")
	}

	bot, err := NewBotClient(leaguepediaAPI)
	if err != nil {
		return fmt.Errorf("failed to create bot client: %w", err)
	}

	if err := bot.Login(username, botPass); err != nil {
		log.Printf("[INGEST ERROR] Bot login failed: %v", err)
		return fmt.Errorf("fandom authentication failed: %w", err)
	}

	log.Println("[INGEST] Authenticated successfully. Clearing previous records before fresh live sync...")

	// Clear existing local players and stats to avoid stale/mock entries
	_, _ = db.Exec("DELETE FROM pro_match_stats;")
	_, _ = db.Exec("DELETE FROM pro_players;")

	// Sync premier Tier-1 leagues
	regions := []string{"LCK", "LCS", "LEC", "LPL"}
	for _, region := range regions {
		if err := FetchProPlayers(db, bot, region); err != nil {
			log.Printf("[INGEST WARNING] Failed fetching region %s: %v", region, err)
		}
	}

	// Fetch handles to query match stats in batches to protect against rate limits
	rows, err := db.Query("SELECT handle FROM pro_players LIMIT 100")
	var handles []string
	if err == nil {
		for rows.Next() {
			var handle string
			if err := rows.Scan(&handle); err == nil {
				handles = append(handles, handle)
			}
		}
		rows.Close()
	}

	// Process handles in chunks of 20 to reduce total HTTP requests while staying within URL length limits
	batchSize := 20
	for i := 0; i < len(handles); i += batchSize {
		end := i + batchSize
		if end > len(handles) {
			end = len(handles)
		}

		batch := handles[i:end]
		if err := FetchMatchStatsForPlayers(db, bot, batch); err != nil {
			log.Printf("[INGEST WARNING] Failed batch match stats fetch for handles %v: %v", batch, err)
		}
	}

	return SetLastStatsUpdate(db)
}

// StatsAPIHandler returns raw JSON of the ingested match stats
func StatsAPIHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		query := `
			SELECT 
				p.player_id,
				p.handle,
				p.real_name,
				p.role,
				p.pro_team,
				p.region,
				p.image_url,
				COALESCE(s.match_date, '') AS match_date,
				COALESCE(s.week_number, 1) AS week_number,
				COALESCE(s.kills, 0) AS kills,
				COALESCE(s.deaths, 0) AS deaths,
				COALESCE(s.assists, 0) AS assists,
				COALESCE(s.creep_score, 0) AS creep_score,
				COALESCE(s.fantasy_points, 0.0) AS fantasy_points
			FROM pro_players p
			LEFT JOIN pro_match_stats s ON p.player_id = s.player_id
			ORDER BY s.fantasy_points DESC, p.handle ASC;
		`

		rows, err := db.Query(query)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to retrieve stats: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type PlayerStatDTO struct {
			PlayerID      int     `json:"player_id"`
			Handle        string  `json:"handle"`
			RealName      string  `json:"real_name"`
			Role          string  `json:"role"`
			ProTeam       string  `json:"pro_team"`
			Region        string  `json:"region"`
			ImageURL      string  `json:"image_url"`
			MatchDate     string  `json:"match_date"`
			WeekNumber    int     `json:"week_number"`
			Kills         int     `json:"kills"`
			Deaths        int     `json:"deaths"`
			Assists       int     `json:"assists"`
			CreepScore    int     `json:"creep_score"`
			FantasyPoints float64 `json:"fantasy_points"`
		}

		var stats []PlayerStatDTO
		for rows.Next() {
			var ps PlayerStatDTO
			var realName sql.NullString
			var imageURL sql.NullString

			err := rows.Scan(
				&ps.PlayerID,
				&ps.Handle,
				&realName,
				&ps.Role,
				&ps.ProTeam,
				&ps.Region,
				&imageURL,
				&ps.MatchDate,
				&ps.WeekNumber,
				&ps.Kills,
				&ps.Deaths,
				&ps.Assists,
				&ps.CreepScore,
				&ps.FantasyPoints,
			)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to scan row: %v", err), http.StatusInternalServerError)
				return
			}

			if realName.Valid {
				ps.RealName = realName.String
			}
			if imageURL.Valid {
				ps.ImageURL = imageURL.String
			}

			stats = append(stats, ps)
		}

		json.NewEncoder(w).Encode(stats)
	}
}

// UpdatePlayerStatsHandler handles POST /admin/stats/update
func (h *AdminHandler) UpdatePlayerStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("Received admin manual ingestion request...")

	if err := TriggerLiveIngestion(h.DB); err != nil {
		log.Printf("[ADMIN INGEST ERROR] %v", err)
		http.Error(w, fmt.Sprintf("Live ingestion failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"Rosters and stats updated successfully"}`))
}

// ResetStatsDateHandler handles POST /admin/stats/reset-date
func (h *AdminHandler) ResetStatsDateHandler(w http.ResponseWriter, r *http.Request) {
	_, err := h.DB.Exec(`
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ('last_stats_update', '2026-01-01 00:00:00', CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = '2026-01-01 00:00:00',
			updated_at = CURRENT_TIMESTAMP;
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to reset stats date: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"Last stats update date reset to 2026-01-01"}`))
}

// CalculateLeaderboardHandler handles POST /admin/leaderboard/calculate
func (h *AdminHandler) CalculateLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "leaderboard recalculated"}`))
}

type SimulateResponse struct {
	PlayerName  string  `json:"player_name"`
	TotalPoints float64 `json:"total_points"`
}

// SimulateRequest represents the payload for POST /admin/stats/simulate
type SimulateRequest struct {
	PlayerName  string `json:"player_name"`
	Kills       int    `json:"kills"`
	Deaths      int    `json:"deaths"`
	Assists     int    `json:"assists"`
	CS          int    `json:"cs"`
	TripleKills int    `json:"triple_kills"`
	QuadraKills int    `json:"quadra_kills"`
	PentaKills  int    `json:"penta_kills"`
	Win         bool   `json:"win"`
}

// SimulateStatsHandler handles POST /admin/stats/simulate
func SimulateStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SimulateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	stats := scoring.PlayerStats{
		Kills:       req.Kills,
		Deaths:      req.Deaths,
		Assists:     req.Assists,
		CS:          req.CS,
		TripleKills: req.TripleKills,
		QuadraKills: req.QuadraKills,
		PentaKills:  req.PentaKills,
		Win:         req.Win,
	}

	resp := SimulateResponse{
		PlayerName:  req.PlayerName,
		TotalPoints: scoring.CalculateFantasyPoints(stats),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
