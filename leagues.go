package main

import (
	"database/sql"
	"log"
	"net/http"
)

// LeaguesHandler bundles the DB connection for league-related endpoints.
// It reuses AuthHandler's session logic to identify the current user.
type LeaguesHandler struct {
	DB   *sql.DB
	Auth *AuthHandler
}

// leagueSummary is the public view of a league the user belongs to, along
// with the user's team in that league.
type leagueSummary struct {
	LeagueID    int64   `json:"league_id"`
	Name        string  `json:"name"`
	Privacy     string  `json:"privacy"`
	TeamName    string  `json:"team_name"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	TotalPoints float64 `json:"total_points"`
}

// MyLeaguesHandler handles GET /api/leagues/mine.
// Returns the leagues the logged-in user has a fantasy team in. Responds
// 401 when there is no valid session so the frontend can show a login prompt.
func (h *LeaguesHandler) MyLeaguesHandler(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.userFromSession(r)
	if err != nil {
		log.Printf("[LEAGUES ERROR] session lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read session")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	rows, err := h.DB.Query(`
		SELECT l.league_id, l.name, l.privacy,
		       t.team_name, t.wins, t.losses, t.total_points
		FROM fantasy_teams t
		JOIN leagues l ON l.league_id = t.league_id
		WHERE t.user_id = ?
		ORDER BY l.name ASC`, user.UserID,
	)
	if err != nil {
		log.Printf("[LEAGUES ERROR] query failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load leagues")
		return
	}
	defer rows.Close()

	leagues := make([]leagueSummary, 0)
	for rows.Next() {
		var ls leagueSummary
		if err := rows.Scan(
			&ls.LeagueID, &ls.Name, &ls.Privacy,
			&ls.TeamName, &ls.Wins, &ls.Losses, &ls.TotalPoints,
		); err != nil {
			log.Printf("[LEAGUES ERROR] scan failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load leagues")
			return
		}
		leagues = append(leagues, ls)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[LEAGUES ERROR] row iteration failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load leagues")
		return
	}

	writeJSON(w, http.StatusOK, leagues)
}
