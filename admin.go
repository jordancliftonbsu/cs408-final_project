package main

import (
	"database/sql"
	"encoding/json"
	"fantasylol/scoring"
	"net/http"
)

type AdminHandler struct {
	DB *sql.DB
}

// UpdatePlayerStatsHandler handles POST /admin/stats/update
func (h *AdminHandler) UpdatePlayerStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Trigger pro player stats update logic here (checkpoint 3)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "stats updated"}`))
}

// CalculateLeaderboardHandler handles POST /admin/leaderboard/calculate
func (h *AdminHandler) CalculateLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Recalculate leaderboard totals here (checkpoint 3)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "leaderboard recalculated"}`))
}

type SimulateResponse struct {
	PlayerName  string  `json:"player_name"`
	TotalPoints float64 `json:"total_points"`
}

// POST /admin/stats/simulate
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
