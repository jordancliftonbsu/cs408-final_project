package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
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

// ---------------------------------------------------------------------------
// Migration
// ---------------------------------------------------------------------------

// leagueColumnMigrations lists columns added to leagues after its first
// release. CREATE TABLE IF NOT EXISTS does not alter existing tables, so each
// is added here when missing.
var leagueColumnMigrations = []struct{ name, ddl string }{
	{"password_hash", `ALTER TABLE leagues ADD COLUMN password_hash VARCHAR(255)`},
	{"end_date", `ALTER TABLE leagues ADD COLUMN end_date DATE`},
}

// ensureLeagueColumns adds any missing columns from leagueColumnMigrations to
// the leagues table. It is a no-op when they all exist.
func ensureLeagueColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(leagues)`)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var (
			cid         int
			name, typ   string
			notnull, pk int
			dflt        sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, m := range leagueColumnMigrations {
		if have[m.name] {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return fmt.Errorf("adding leagues.%s: %w", m.name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

const (
	leaguePageSize = 10
	minMaxTeams    = 2
	maxMaxTeams    = 16
	dateLayout     = "2006-01-02"
)

// leagueRow is one row in the league search results.
type leagueRow struct {
	LeagueID  int64  `json:"league_id"`
	Name      string `json:"name"`
	Privacy   string `json:"privacy"`
	Owner     string `json:"owner"`
	OpenSlots int    `json:"open_slots"`
	MaxTeams  int    `json:"max_teams"`
	StartDate string `json:"start_date"`
	Joined    bool   `json:"joined"` // current user already has a team here
	IsOwner   bool   `json:"is_owner"`
	DaysUntil *int   `json:"days_until"` // days until start; zero or negative once started, null if no date
	Concluded bool   `json:"concluded"`  // end date has passed
}

type leagueSearchResponse struct {
	Leagues    []leagueRow `json:"leagues"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Total      int         `json:"total"`
	TotalPages int         `json:"total_pages"`
}

// leagueDetail is what the edit form loads. It never includes the password.
type leagueDetail struct {
	LeagueID  int64          `json:"league_id"`
	Name      string         `json:"name"`
	StartDate string         `json:"start_date"`
	EndDate   string         `json:"end_date"`
	MaxTeams  int            `json:"max_teams"`
	Private   bool           `json:"private"`
	TeamCount int            `json:"team_count"`
	OwnerID   int64          `json:"owner_id"`
	Started   bool           `json:"started"` // start date passed: start date, privacy, max teams locked
	Members   []leagueMember `json:"members,omitempty"`
}

// leagueMember is a user enrolled in a league via a fantasy team.
type leagueMember struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
}

type leagueRequest struct {
	Name       string `json:"name"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Password   string `json:"password"`
	MakePublic bool   `json:"make_public"` // edit only: remove the existing password
	MaxTeams   int    `json:"max_teams"`
	AddMe      bool   `json:"add_me"` // create only: enroll the creator
}

// ---------------------------------------------------------------------------
// Search + create (/api/leagues)
// ---------------------------------------------------------------------------

// LeaguesCollectionHandler handles GET (search) and POST (create) on /api/leagues.
func (h *LeaguesHandler) LeaguesCollectionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.searchLeagues(w, r)
	case http.MethodPost:
		h.createLeague(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// escapeLike escapes LIKE wildcards so user text is matched literally.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

// searchLeagues handles GET /api/leagues.
// Query params: page, id, name, privacy (public|private), open_slots=1, not_started=1.
// When id is supplied, every other filter is ignored.
func (h *LeaguesHandler) searchLeagues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page := 1
	if p := q.Get("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid page")
			return
		}
		page = n
	}

	var (
		conds []string
		args  []any
	)

	if idStr := strings.TrimSpace(q.Get("id")); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "league ID must be a positive number")
			return
		}
		conds = append(conds, "l.league_id = ?")
		args = append(args, id)
	} else {
		switch strings.ToLower(q.Get("privacy")) {
		case "public":
			conds = append(conds, "(l.password_hash IS NULL OR l.password_hash = '')")
		case "private":
			conds = append(conds, "(l.password_hash IS NOT NULL AND l.password_hash != '')")
		case "":
		default:
			writeError(w, http.StatusBadRequest, "privacy must be public or private")
			return
		}
		if q.Get("open_slots") == "1" {
			conds = append(conds, "(SELECT COUNT(*) FROM fantasy_teams t WHERE t.league_id = l.league_id) < l.max_teams")
		}
		if q.Get("include_concluded") != "1" {
			conds = append(conds, "(l.end_date IS NULL OR l.end_date >= date('now','localtime'))")
		}
		if q.Get("not_started") == "1" {
			conds = append(conds, "(l.start_date IS NULL OR l.start_date > ?)")
			args = append(args, time.Now().Format(dateLayout))
		}
		if name := strings.TrimSpace(q.Get("name")); name != "" {
			conds = append(conds, `l.name LIKE ? ESCAPE '\'`)
			args = append(args, "%"+escapeLike(name)+"%")
		}
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM leagues l "+where, args...).Scan(&total); err != nil {
		log.Printf("[LEAGUES ERROR] count failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load leagues")
		return
	}

	// Current user (0 when logged out matches no rows) to flag joined leagues.
	var viewerID int64
	if u, err := h.Auth.userFromSession(r); err != nil {
		log.Printf("[LEAGUES ERROR] session lookup failed: %v", err)
	} else if u != nil {
		viewerID = u.UserID
	}

	listArgs := append([]any{viewerID, viewerID}, args...)
	listArgs = append(listArgs, leaguePageSize, (page-1)*leaguePageSize)
	rows, err := h.DB.Query(`
		SELECT l.league_id, l.name,
		       CASE WHEN l.password_hash IS NOT NULL AND l.password_hash != '' THEN 'Private' ELSE 'Public' END,
		       u.username,
		       l.max_teams - (SELECT COUNT(*) FROM fantasy_teams t WHERE t.league_id = l.league_id),
		       l.max_teams,
		       COALESCE(l.start_date, ''),
		       EXISTS(SELECT 1 FROM fantasy_teams t WHERE t.league_id = l.league_id AND t.user_id = ?),
		       l.commissioner_id = ?,
		       CAST(julianday(l.start_date) - julianday(date('now','localtime')) AS INTEGER),
		       COALESCE(l.end_date, '9999-12-31') < date('now','localtime')
		FROM leagues l
		JOIN users u ON u.user_id = l.commissioner_id
		`+where+`
		ORDER BY COALESCE(l.start_date, '9999-12-31') <= date('now','localtime') ASC,
		         l.start_date IS NULL ASC, l.start_date ASC, l.league_id ASC
		LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		log.Printf("[LEAGUES ERROR] search failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load leagues")
		return
	}
	defer rows.Close()

	leagues := make([]leagueRow, 0)
	for rows.Next() {
		var lr leagueRow
		if err := rows.Scan(&lr.LeagueID, &lr.Name, &lr.Privacy, &lr.Owner, &lr.OpenSlots, &lr.MaxTeams, &lr.StartDate, &lr.Joined, &lr.IsOwner, &lr.DaysUntil, &lr.Concluded); err != nil {
			log.Printf("[LEAGUES ERROR] scan failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load leagues")
			return
		}
		if lr.OpenSlots < 0 {
			lr.OpenSlots = 0
		}
		leagues = append(leagues, lr)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[LEAGUES ERROR] row iteration failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load leagues")
		return
	}

	totalPages := (total + leaguePageSize - 1) / leaguePageSize
	writeJSON(w, http.StatusOK, leagueSearchResponse{
		Leagues: leagues, Page: page, PageSize: leaguePageSize,
		Total: total, TotalPages: totalPages,
	})
}

// validateLeagueRequest trims and validates the shared create/edit fields.
func validateLeagueRequest(req *leagueRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 255 {
		return errors.New("name is required (255 characters max)")
	}
	start, err := time.Parse(dateLayout, req.StartDate)
	if err != nil {
		return errors.New("please choose a valid start date")
	}
	end, err := time.Parse(dateLayout, req.EndDate)
	if err != nil {
		return errors.New("please choose a valid end date")
	}
	if end.Before(start) {
		return errors.New("end date can't be before the start date")
	}
	if req.MaxTeams < minMaxTeams || req.MaxTeams > maxMaxTeams {
		return fmt.Errorf("max teams must be between %d and %d", minMaxTeams, maxMaxTeams)
	}
	if len(req.Password) > 72 {
		return errors.New("password must be 72 characters or fewer")
	}
	return nil
}

// requireUser resolves the session user or writes the error response.
func (h *LeaguesHandler) requireUser(w http.ResponseWriter, r *http.Request) *userResponse {
	user, err := h.Auth.userFromSession(r)
	if err != nil {
		log.Printf("[LEAGUES ERROR] session lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read session")
		return nil
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return nil
	}
	return user
}

// createLeague handles POST /api/leagues.
func (h *LeaguesHandler) createLeague(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req leagueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateLeagueRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	privacy, hash := "Public", ""
	if req.Password != "" {
		b, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("[LEAGUES ERROR] hash failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not create league")
			return
		}
		privacy, hash = "Private", string(b)
	}

	tx, err := h.DB.Begin()
	if err != nil {
		log.Printf("[LEAGUES ERROR] begin failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create league")
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO leagues (commissioner_id, name, max_teams, start_date, end_date, privacy, password_hash)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		user.UserID, req.Name, req.MaxTeams, req.StartDate, req.EndDate, privacy, hash)
	if err != nil {
		log.Printf("[LEAGUES ERROR] insert failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create league")
		return
	}
	id, _ := res.LastInsertId()

	if req.AddMe {
		if _, err := tx.Exec(
			`INSERT INTO fantasy_teams (league_id, user_id, team_name) VALUES (?, ?, ?)`,
			id, user.UserID, user.Username+"'s Team"); err != nil {
			log.Printf("[LEAGUES ERROR] enroll failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not create league")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[LEAGUES ERROR] commit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create league")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"league_id": id})
}

// ---------------------------------------------------------------------------
// Single league (/api/leagues/{id})
// ---------------------------------------------------------------------------

// LeagueItemHandler routes /api/leagues/{id}[/members/{userId} | /transfer]:
//
//	GET    /api/leagues/{id}                  league + members (owner only)
//	PUT    /api/leagues/{id}                  update (owner only)
//	DELETE /api/leagues/{id}                  delete (owner only)
//	DELETE /api/leagues/{id}/members/{userId} kick a member (owner only)
//	POST   /api/leagues/{id}/transfer         transfer ownership (owner only)
func (h *LeaguesHandler) LeagueItemHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/leagues/"), "/"), "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "league not found")
		return
	}

	switch {
	case len(parts) == 1:
		switch r.Method {
		case http.MethodGet:
			h.getLeague(w, r, id)
		case http.MethodPut:
			h.updateLeague(w, r, id)
		case http.MethodDelete:
			h.deleteLeague(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case len(parts) == 3 && parts[1] == "members" && r.Method == http.MethodDelete:
		uid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || uid < 1 {
			writeError(w, http.StatusNotFound, "member not found")
			return
		}
		h.kickMember(w, r, id, uid)
	case len(parts) == 2 && parts[1] == "join" && r.Method == http.MethodPost:
		h.joinLeague(w, r, id)
	case len(parts) == 2 && parts[1] == "leave" && r.Method == http.MethodPost:
		h.leaveLeague(w, r, id)
	case len(parts) == 2 && parts[1] == "transfer" && r.Method == http.MethodPost:
		h.transferLeague(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// deleteTeamsTx removes rosters and teams for the given league (and optionally
// only one user) inside tx.
func deleteTeamsTx(tx *sql.Tx, leagueID int64, userID int64) error {
	filter, args := "", []any{leagueID}
	if userID != 0 {
		filter, args = " AND user_id = ?", append(args, userID)
	}
	if _, err := tx.Exec(`DELETE FROM roster_slots WHERE team_id IN
		(SELECT team_id FROM fantasy_teams WHERE league_id = ?`+filter+`)`, args...); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM fantasy_teams WHERE league_id = ?`+filter, args...)
	return err
}

// deleteLeague handles DELETE /api/leagues/{id} (owner only).
func (h *LeaguesHandler) deleteLeague(w http.ResponseWriter, r *http.Request, id int64) {
	if _, ok := h.loadOwnedLeague(w, r, id); !ok {
		return
	}
	tx, err := h.DB.Begin()
	if err == nil {
		defer tx.Rollback()
		if err = deleteTeamsTx(tx, id, 0); err == nil {
			if _, err = tx.Exec(`DELETE FROM leagues WHERE league_id = ?`, id); err == nil {
				err = tx.Commit()
			}
		}
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] delete failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete league")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// kickMember handles DELETE /api/leagues/{id}/members/{userId} (owner only).
// The owner cannot be kicked; transfer ownership first.
func (h *LeaguesHandler) kickMember(w http.ResponseWriter, r *http.Request, id, userID int64) {
	d, ok := h.loadOwnedLeague(w, r, id)
	if !ok {
		return
	}
	if userID == d.OwnerID {
		writeError(w, http.StatusBadRequest, "the owner can't be kicked; transfer ownership first")
		return
	}
	tx, err := h.DB.Begin()
	if err == nil {
		defer tx.Rollback()
		if err = deleteTeamsTx(tx, id, userID); err == nil {
			err = tx.Commit()
		}
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] kick failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not remove member")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// transferLeague handles POST /api/leagues/{id}/transfer with {"user_id": N}.
// The new owner must already be a member of the league.
func (h *LeaguesHandler) transferLeague(w http.ResponseWriter, r *http.Request, id int64) {
	d, ok := h.loadOwnedLeague(w, r, id)
	if !ok {
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID < 1 {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == d.OwnerID {
		writeError(w, http.StatusBadRequest, "you already own this league")
		return
	}
	var n int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM fantasy_teams WHERE league_id = ? AND user_id = ?`,
		id, req.UserID).Scan(&n); err != nil {
		log.Printf("[LEAGUES ERROR] transfer lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not transfer league")
		return
	}
	if n == 0 {
		writeError(w, http.StatusBadRequest, "new owner must be a member of the league")
		return
	}
	if _, err := h.DB.Exec(`UPDATE leagues SET commissioner_id = ? WHERE league_id = ?`, req.UserID, id); err != nil {
		log.Printf("[LEAGUES ERROR] transfer failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not transfer league")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"league_id": id})
}

// loadOwnedLeague loads a league for editing. It writes the error response and
// returns ok=false when the user is not logged in, the league does not exist,
// or the user is not the commissioner.
func (h *LeaguesHandler) loadOwnedLeague(w http.ResponseWriter, r *http.Request, id int64) (leagueDetail, bool) {
	var d leagueDetail
	user := h.requireUser(w, r)
	if user == nil {
		return d, false
	}

	var commissioner int64
	var hash sql.NullString
	err := h.DB.QueryRow(`
		SELECT l.league_id, l.commissioner_id, l.name, COALESCE(l.start_date, ''), COALESCE(l.end_date, ''), l.max_teams, l.password_hash,
		       (SELECT COUNT(*) FROM fantasy_teams t WHERE t.league_id = l.league_id),
		       COALESCE(l.start_date, '9999-12-31') <= date('now','localtime')
		FROM leagues l WHERE l.league_id = ?`, id,
	).Scan(&d.LeagueID, &commissioner, &d.Name, &d.StartDate, &d.EndDate, &d.MaxTeams, &hash, &d.TeamCount, &d.Started)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "league not found")
		return d, false
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] load failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load league")
		return d, false
	}
	if commissioner != user.UserID {
		writeError(w, http.StatusForbidden, "only the league owner can edit this league")
		return d, false
	}
	d.Private = hash.Valid && hash.String != ""
	d.OwnerID = commissioner
	return d, true
}

// getLeague handles GET /api/leagues/{id} for the edit form (owner only).
func (h *LeaguesHandler) getLeague(w http.ResponseWriter, r *http.Request, id int64) {
	d, ok := h.loadOwnedLeague(w, r, id)
	if !ok {
		return
	}
	rows, err := h.DB.Query(`
		SELECT u.user_id, u.username, t.team_name
		FROM fantasy_teams t JOIN users u ON u.user_id = t.user_id
		WHERE t.league_id = ? ORDER BY u.username`, id)
	if err != nil {
		log.Printf("[LEAGUES ERROR] members query failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load league")
		return
	}
	defer rows.Close()
	d.Members = []leagueMember{}
	for rows.Next() {
		var m leagueMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.TeamName); err != nil {
			log.Printf("[LEAGUES ERROR] member scan failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load league")
			return
		}
		d.Members = append(d.Members, m)
	}
	writeJSON(w, http.StatusOK, d)
}

// updateLeague handles PUT /api/leagues/{id} (owner only).
// A non-empty password sets/changes the password; make_public removes it;
// otherwise the existing password is kept.
func (h *LeaguesHandler) updateLeague(w http.ResponseWriter, r *http.Request, id int64) {
	current, ok := h.loadOwnedLeague(w, r, id)
	if !ok {
		return
	}

	var req leagueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateLeagueRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if current.Started {
		// After the start date only the name and end date may change.
		if req.StartDate != current.StartDate || req.MaxTeams != current.MaxTeams ||
			req.Password != "" || req.MakePublic {
			writeError(w, http.StatusForbidden,
				"this league has started: start date, privacy and max teams can no longer be changed")
			return
		}
		if _, err := h.DB.Exec(`UPDATE leagues SET name=?, end_date=? WHERE league_id=?`, req.Name, req.EndDate, id); err != nil {
			log.Printf("[LEAGUES ERROR] update failed: %v", err)
			writeError(w, http.StatusInternalServerError, "could not update league")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"league_id": id})
		return
	}
	if req.MaxTeams < current.TeamCount {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("max teams cannot be lower than the %d teams already in the league", current.TeamCount))
		return
	}

	var err error
	switch {
	case req.Password != "":
		var b []byte
		b, err = bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err == nil {
			_, err = h.DB.Exec(`UPDATE leagues SET name=?, start_date=?, end_date=?, max_teams=?, privacy='Private', password_hash=? WHERE league_id=?`,
				req.Name, req.StartDate, req.EndDate, req.MaxTeams, string(b), id)
		}
	case req.MakePublic:
		_, err = h.DB.Exec(`UPDATE leagues SET name=?, start_date=?, end_date=?, max_teams=?, privacy='Public', password_hash=NULL WHERE league_id=?`,
			req.Name, req.StartDate, req.EndDate, req.MaxTeams, id)
	default:
		_, err = h.DB.Exec(`UPDATE leagues SET name=?, start_date=?, end_date=?, max_teams=? WHERE league_id=?`,
			req.Name, req.StartDate, req.EndDate, req.MaxTeams, id)
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update league")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"league_id": id})
}

// ---------------------------------------------------------------------------
// Join / leave
// ---------------------------------------------------------------------------

// joinLeague handles POST /api/leagues/{id}/join with {"password": "..."}.
// Public leagues need no password. Fails if already joined or the league is full.
func (h *LeaguesHandler) joinLeague(w http.ResponseWriter, r *http.Request, id int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	// An empty body is fine for public leagues.
	_ = json.NewDecoder(r.Body).Decode(&req)

	tx, err := h.DB.Begin()
	if err != nil {
		log.Printf("[LEAGUES ERROR] begin failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not join league")
		return
	}
	defer tx.Rollback()

	var (
		maxTeams, teams, mine int
		hash                  sql.NullString
		started               bool
	)
	err = tx.QueryRow(`
		SELECT l.max_teams, l.password_hash,
		       (SELECT COUNT(*) FROM fantasy_teams t WHERE t.league_id = l.league_id),
		       (SELECT COUNT(*) FROM fantasy_teams t WHERE t.league_id = l.league_id AND t.user_id = ?)
		       ,COALESCE(l.start_date, '9999-12-31') <= date('now','localtime')
		FROM leagues l WHERE l.league_id = ?`, user.UserID, id,
	).Scan(&maxTeams, &hash, &teams, &mine, &started)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "league not found")
		return
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] join lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not join league")
		return
	}
	if mine > 0 {
		writeError(w, http.StatusConflict, "you are already in this league")
		return
	}
	if started {
		writeError(w, http.StatusConflict, "this league has already started and is closed to new players")
		return
	}
	if hash.Valid && hash.String != "" {
		if bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(req.Password)) != nil {
			writeError(w, http.StatusForbidden, "incorrect league password")
			return
		}
	}
	if teams >= maxTeams {
		writeError(w, http.StatusConflict, "this league is full")
		return
	}

	if _, err := tx.Exec(
		`INSERT INTO fantasy_teams (league_id, user_id, team_name) VALUES (?, ?, ?)`,
		id, user.UserID, user.Username+"'s Team"); err != nil {
		log.Printf("[LEAGUES ERROR] join insert failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not join league")
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[LEAGUES ERROR] join commit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not join league")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "joined"})
}

// leaveLeague handles POST /api/leagues/{id}/leave.
// The owner can't leave; they must transfer ownership or delete the league.
func (h *LeaguesHandler) leaveLeague(w http.ResponseWriter, r *http.Request, id int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var commissioner int64
	err := h.DB.QueryRow(`SELECT commissioner_id FROM leagues WHERE league_id = ?`, id).Scan(&commissioner)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "league not found")
		return
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] leave lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not leave league")
		return
	}
	if commissioner == user.UserID {
		writeError(w, http.StatusBadRequest, "the owner can't leave; transfer ownership or delete the league")
		return
	}

	tx, err := h.DB.Begin()
	if err == nil {
		defer tx.Rollback()
		if err = deleteTeamsTx(tx, id, user.UserID); err == nil {
			err = tx.Commit()
		}
	}
	if err != nil {
		log.Printf("[LEAGUES ERROR] leave failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not leave league")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}
