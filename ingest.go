package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fantasylol/scoring"
)

const leaguepediaAPI = "https://lol.fandom.com/api.php"
const userAgentHeader = "FantasyLoLBot/1.0 (+https://fantasylol.com; admin@fantasylol.com) Go-HTTP-Client/1.1"

// Inter-query delay to respect Fandom rate limits.
const apiQueryDelay = 2000 * time.Millisecond

// leaguepediaRegionMap maps region strings (both long-form from Tournaments
// and short-form from Teams) to the codes stored in pro_players.region.
var leaguepediaRegionMap = map[string]string{
	"Korea":         "LCK",
	"China":         "LPL",
	"EMEA":          "LEC",
	"North America": "LCS",
	"International": "INT",
	"LCK":           "LCK",
	"LPL":           "LPL",
	"LEC":           "LEC",
	"LCS":           "LCS",
	"LCS.A":         "LCS.A",
	"PCS":           "PCS",
	"VCS":           "VCS",
	"CBLOL":         "CBLOL",
	"LJL":           "LJL",
	"LCO":           "LCO",
	"TCL":           "TCL",
}

func mapRegionCode(region string) string {
	if code, ok := leaguepediaRegionMap[region]; ok {
		return code
	}
	if len(region) > 8 {
		return region[:8]
	}
	return region
}

// ─── Cargo API ───────────────────────────────────────────────────────────────

type CargoResponse struct {
	CargoQuery []map[string]map[string]string `json:"cargoquery"`
	Error      *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error,omitempty"`
}

type BotClient struct {
	HTTPClient *http.Client
	BaseURL    string
}

func NewBotClient(baseURL string) (*BotClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &BotClient{
		HTTPClient: &http.Client{Timeout: 30 * time.Second, Jar: jar},
		BaseURL:    baseURL,
	}, nil
}

func (c *BotClient) Login(username, botPassword string) error {
	tokenURL := fmt.Sprintf("%s?action=query&meta=tokens&type=login&format=json", c.BaseURL)
	req, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgentHeader)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		Query struct {
			Tokens struct {
				LoginToken string `json:"logintoken"`
			} `json:"tokens"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse login token: %w", err)
	}

	form := url.Values{}
	form.Set("action", "login")
	form.Set("lgname", username)
	form.Set("lgpassword", botPassword)
	form.Set("lgtoken", tokenResp.Query.Tokens.LoginToken)
	form.Set("format", "json")

	postReq, err := http.NewRequest("POST", c.BaseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("User-Agent", userAgentHeader)

	postResp, err := c.HTTPClient.Do(postReq)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()

	postBody, _ := io.ReadAll(postResp.Body)
	var loginResult struct {
		Login struct {
			Result string `json:"result"`
			Reason string `json:"reason,omitempty"`
		} `json:"login"`
	}
	if err := json.Unmarshal(postBody, &loginResult); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}
	if loginResult.Login.Result != "Success" {
		return fmt.Errorf("bot login failed: %s %s", loginResult.Login.Result, loginResult.Login.Reason)
	}

	log.Printf("[BOT AUTH] Successfully logged in as: %s", username)
	return nil
}

func (c *BotClient) ExecuteCargoQuery(queryParams url.Values) ([]map[string]map[string]string, error) {
	time.Sleep(apiQueryDelay)

	fullURL := fmt.Sprintf("%s?%s", c.BaseURL, queryParams.Encode())
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgentHeader)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			log.Printf("[NETWORK ERROR] Attempt %d/%d: %v", attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[READ ERROR] Attempt %d/%d: %v", attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		var data CargoResponse
		if err := json.Unmarshal(body, &data); err != nil {
			log.Printf("[JSON ERROR] Attempt %d/%d: %v", attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if data.Error != nil {
			if strings.Contains(data.Error.Code, "ratelimited") {
				log.Printf("[RATE LIMIT] Pausing 65 s (attempt %d/%d)...", attempt, maxRetries)
				time.Sleep(65 * time.Second)
				continue
			}
			log.Printf("[CARGO QUERY FAIL] code=%s info=%s tables=%s where=%s url=%s",
				data.Error.Code, data.Error.Info,
				queryParams.Get("tables"), queryParams.Get("where"), fullURL)
			if strings.Contains(data.Error.Code, "MWException") || strings.Contains(data.Error.Code, "internal_api_error") {
				return nil, fmt.Errorf("cargo API database exception (%s): %s", data.Error.Code, data.Error.Info)
			}
			return nil, fmt.Errorf("cargo API error (%s): %s", data.Error.Code, data.Error.Info)
		}

		return data.CargoQuery, nil
	}

	return nil, fmt.Errorf("exceeded max retries (%d)", maxRetries)
}

// ─── Team region lookup ──────────────────────────────────────────────────────

// fetchTeamRegions queries the Teams table for a list of team names and returns
// a map of team name → region code.
//
// The Teams table is small and indexed on _pageName, so this query is safe.
// We avoid the Players table entirely — it is too large for the Fandom DB
// governor to handle from the external API without MWException.
//
// Team names are plain ASCII strings without parentheses, so building a WHERE
// clause with OR equality checks is reliable and avoids IN (...) parser issues.
func fetchTeamRegions(bot *BotClient, teamNames []string, fallback string) map[string]string {
	result := make(map[string]string, len(teamNames))
	if len(teamNames) == 0 {
		return result
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, t := range teamNames {
		if t != "" && !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}

	// Build WHERE using OR equality — safe even for team names with spaces.
	// We use _pageName (the wiki page title) which equals the team's canonical name.
	var parts []string
	for _, t := range unique {
		escaped := strings.ReplaceAll(t, "'", "\\'")
		parts = append(parts, fmt.Sprintf("_pageName='%s'", escaped))
	}

	params := url.Values{}
	params.Set("action", "cargoquery")
	params.Set("format", "json")
	params.Set("tables", "Teams")
	params.Set("fields", "_pageName=Team, Region")
	params.Set("where", strings.Join(parts, " OR "))
	params.Set("limit", strconv.Itoa(len(unique)+5))

	rows, err := bot.ExecuteCargoQuery(params)
	if err != nil {
		log.Printf("[INGEST WARNING] fetchTeamRegions query failed (%v) — using fallback %q for all teams", err, fallback)
		for _, t := range unique {
			result[t] = fallback
		}
		return result
	}

	for _, row := range rows {
		r := row["title"]
		team := strings.TrimSpace(r["Team"])
		rawRegion := strings.TrimSpace(r["Region"])
		if team == "" {
			continue
		}
		if rawRegion != "" {
			result[team] = mapRegionCode(rawRegion)
		} else {
			result[team] = fallback
		}
	}

	// Fill in fallback for any teams the query didn't return
	for _, t := range unique {
		if _, ok := result[t]; !ok {
			result[t] = fallback
		}
	}
	return result
}

// ─── Database helpers ────────────────────────────────────────────────────────

func upsertPlayer(db *sql.DB, leaguepediaID, handle, role, proTeam, regionCode string) (int64, error) {
	_, err := db.Exec(
		`INSERT INTO pro_players (leaguepedia_id, handle, role, pro_team, region)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(handle) DO UPDATE SET
		     leaguepedia_id = COALESCE(excluded.leaguepedia_id, pro_players.leaguepedia_id),
		     pro_team       = excluded.pro_team,
		     role           = excluded.role,
		     region         = CASE
		                          WHEN pro_players.region = 'INT' THEN excluded.region
		                          ELSE pro_players.region
		                      END`,
		nullableString(leaguepediaID), handle, role, proTeam, regionCode,
	)
	if err != nil {
		return 0, fmt.Errorf("upsertPlayer failed for %q: %w", handle, err)
	}

	var playerID int64
	if err := db.QueryRow(`SELECT player_id FROM pro_players WHERE handle = ?`, handle).Scan(&playerID); err != nil {
		return 0, fmt.Errorf("upsertPlayer SELECT failed for %q: %w", handle, err)
	}
	return playerID, nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func insertMatchStat(db *sql.DB, playerID int64, gameID, matchDate string, win, kills, deaths, assists, cs int, fp float64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO pro_match_stats
		 (player_id, game_id, match_date, win, kills, deaths, assists, creep_score, fantasy_points)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		playerID, gameID, matchDate, win, kills, deaths, assists, cs, fp,
	)
	return err
}

func loadKnownGameIDs(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query(`SELECT game_id FROM lp_games WHERE stats_complete = 1`)
	if err != nil {
		return nil, fmt.Errorf("loadKnownGameIDs: %w", err)
	}
	defer rows.Close()

	known := make(map[string]struct{})
	for rows.Next() {
		var gid string
		if err := rows.Scan(&gid); err != nil {
			continue
		}
		known[gid] = struct{}{}
	}
	return known, rows.Err()
}

// upsertTournament inserts or updates a row in lp_tournaments and returns its
// lp_tournament_id.
func upsertTournament(db *sql.DB, overviewPage, name, regionCode, dateStart, dateEnd string) (int64, error) {
	_, err := db.Exec(`
		INSERT INTO lp_tournaments (overview_page, name, region, date_start, date_end, last_synced_at)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), CURRENT_TIMESTAMP)
		ON CONFLICT(overview_page) DO UPDATE SET
		    name           = excluded.name,
		    last_synced_at = CURRENT_TIMESTAMP`,
		overviewPage, name, regionCode, dateStart, dateEnd,
	)
	if err != nil {
		return 0, fmt.Errorf("upsertTournament failed for %q: %w", overviewPage, err)
	}

	var id int64
	if err := db.QueryRow(`SELECT lp_tournament_id FROM lp_tournaments WHERE overview_page = ?`, overviewPage).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsertTournament SELECT failed for %q: %w", overviewPage, err)
	}
	return id, nil
}

// upsertGame inserts a row into lp_games the first time we see a game_id.
// If the row already exists (e.g. a partial previous run) it is left unchanged
// so we don't reset stats_complete.  Returns the lp_game_id.
func upsertGame(db *sql.DB, lpTournamentID int64, gameID, matchID, gameDate, teamBlue, teamRed, winner string) (int64, error) {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO lp_games
		    (lp_tournament_id, game_id, match_id, game_date, team_blue, team_red, winner)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`,
		lpTournamentID, gameID,
		nullableString(matchID), nullableString(gameDate),
		nullableString(teamBlue), nullableString(teamRed), nullableString(winner),
	)
	if err != nil {
		return 0, fmt.Errorf("upsertGame failed for %q: %w", gameID, err)
	}

	var id int64
	if err := db.QueryRow(`SELECT lp_game_id FROM lp_games WHERE game_id = ?`, gameID).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsertGame SELECT failed for %q: %w", gameID, err)
	}
	return id, nil
}

// markGameComplete sets stats_complete = 1 on an lp_games row.
func markGameComplete(db *sql.DB, lpGameID int64) error {
	_, err := db.Exec(`UPDATE lp_games SET stats_complete = 1 WHERE lp_game_id = ?`, lpGameID)
	return err
}

// ─── Core ingestion ──────────────────────────────────────────────────────────

// FetchMatchStatsForRoster is the main ingestion loop for one region.
// It only considers tournaments whose end date (or start date, for ongoing ones)
// falls on or after sinceDate, and only fetches ScoreboardPlayers rows with
// DateTime_UTC > sinceDate — so each run is incremental from the last.
func FetchMatchStatsForRoster(db *sql.DB, bot *BotClient, region, sinceDate string, knownGameIDs map[string]struct{}) error {
	fallbackRegionCode := mapRegionCode(region)
	isIntl := fallbackRegionCode == "INT"

	// ----------------------------------------------------------------
	// Step 1 — Find tournaments that could contain games since sinceDate.
	//
	// We need tournaments that both:
	//   a) Started recently enough to still be running (DateStart within
	//      the last ~3 months covers any current regional split or
	//      international event), AND
	//   b) Have not been obviously finished well before sinceDate.
	//
	// We can't query Tournaments.Date (end date) — it uses a special
	// Cargo [Date] type that throws db_error on >= comparisons from the
	// external API.
	//
	// Strategy: use DateStart >= (sinceDate - 90 days). 90 days covers
	// the full length of any regional split or international tournament.
	// A tournament that started more than 90 days before sinceDate is
	// definitively over and has no new games to fetch.
	//
	// The real incremental gate is DateTime_UTC > sinceDate on
	// ScoreboardPlayers (Step 3) — this just narrows which tournaments
	// we bother querying.
	// ----------------------------------------------------------------

	// Truncate sinceDate to date-only for DateStart comparison
	sinceDateOnly := sinceDate
	if len(sinceDateOnly) > 10 {
		sinceDateOnly = sinceDateOnly[:10]
	}

	sinceTime, parseErr := time.Parse("2006-01-02", sinceDateOnly)
	if parseErr != nil {
		sinceTime, parseErr = time.Parse("2006-01-02 15:04:05", sinceDate)
		if parseErr != nil {
			sinceTime = time.Now().UTC()
		}
	}
	// Look back 90 days from sinceDate — enough for any active split
	windowStart := sinceTime.AddDate(0, 0, -90).Format("2006-01-02")

	whereClause := fmt.Sprintf(
		"TournamentLevel='Primary' AND Region='%s' AND DateStart >= '%s'",
		region, windowStart,
	)

	tournParams := url.Values{}
	tournParams.Set("action", "cargoquery")
	tournParams.Set("format", "json")
	tournParams.Set("tables", "Tournaments")
	tournParams.Set("fields", "Name, OverviewPage, DateStart, Date, Region")
	tournParams.Set("where", whereClause)
	tournParams.Set("order_by", "DateStart DESC")
	tournParams.Set("limit", "10")

	log.Printf("[INGEST] Fetching active/recent tournaments for region %s since %s", region, sinceDate)

	tournResults, err := bot.ExecuteCargoQuery(tournParams)
	if err != nil {
		return fmt.Errorf("failed fetching tournaments for %s: %w", region, err)
	}
	if len(tournResults) == 0 {
		log.Printf("[INGEST] No active tournaments for region %s since %s", region, sinceDate)
		return nil
	}

	totalNew := 0
	const pageSize = 10
	const dateLayout = "2006-01-02 15:04:05"
	const dateOnlyLayout = "2006-01-02"

	for _, tournRow := range tournResults {
		tr := tournRow["title"]
		tournName := tr["Name"]
		overviewPage := tr["OverviewPage"]
		dateStart := tr["DateStart"]
		dateEnd := tr["Date"]

		if overviewPage == "" {
			log.Printf("[INGEST WARNING] Tournament %q has no OverviewPage — skipping", tournName)
			continue
		}

		lpTournamentID, err := upsertTournament(db, overviewPage, tournName, fallbackRegionCode, dateStart, dateEnd)
		if err != nil {
			log.Printf("[INGEST WARNING] upsertTournament failed for %q: %v — continuing anyway", tournName, err)
		}

		log.Printf("[INGEST] Processing tournament: %s (%s)", tournName, overviewPage)

		// ----------------------------------------------------------------
		// Step 2 — For INT tournaments, pre-fetch team→region mapping.
		// ----------------------------------------------------------------
		teamRegions := make(map[string]string)
		if isIntl {
			sgParams := url.Values{}
			sgParams.Set("action", "cargoquery")
			sgParams.Set("format", "json")
			sgParams.Set("tables", "ScoreboardGames")
			sgParams.Set("fields", "Team1, Team2")
			sgParams.Set("where", fmt.Sprintf("OverviewPage='%s'", strings.ReplaceAll(overviewPage, "'", "\\'")))
			sgParams.Set("group_by", "Team1, Team2")
			sgParams.Set("limit", "50")

			sgRows, err := bot.ExecuteCargoQuery(sgParams)
			if err != nil {
				log.Printf("[INGEST WARNING] Could not fetch teams for %q: %v — will use INT fallback", tournName, err)
			} else {
				var teamNames []string
				for _, row := range sgRows {
					if t1 := strings.TrimSpace(row["title"]["Team1"]); t1 != "" {
						teamNames = append(teamNames, t1)
					}
					if t2 := strings.TrimSpace(row["title"]["Team2"]); t2 != "" {
						teamNames = append(teamNames, t2)
					}
				}
				teamRegions = fetchTeamRegions(bot, teamNames, fallbackRegionCode)
				log.Printf("[INGEST] Resolved regions for %d teams in %q", len(teamRegions), tournName)
			}
		}

		// ----------------------------------------------------------------
		// Step 3 — Page through ScoreboardPlayers, only rows after sinceDate.
		//
		// We order ASC here so the oldest new games are imported first.
		// Combined with the DateTime_UTC > sinceDate filter, each page
		// only returns genuinely new rows — so the early-exit on a full
		// page of known game_ids is still valid (once a page is all-known,
		// everything beyond it is older and also known).
		// ----------------------------------------------------------------
		savedThisTournament := 0
		offset := 0

		// Escape sinceDate for the Cargo WHERE clause
		sinceDateEsc := strings.ReplaceAll(sinceDate, "'", "\\'")
		overviewPageEsc := strings.ReplaceAll(overviewPage, "'", "\\'")

		for {
			params := url.Values{}
			params.Set("action", "cargoquery")
			params.Set("format", "json")
			params.Set("tables", "ScoreboardPlayers")
			params.Set("fields", "Name, Link, Team, Role, Kills, Deaths, Assists, CS, PlayerWin, GameId, MatchId, DateTime_UTC")
			params.Set("where", fmt.Sprintf("OverviewPage='%s' AND DateTime_UTC > '%s'", overviewPageEsc, sinceDateEsc))
			params.Set("order_by", "DateTime_UTC ASC")
			params.Set("limit", strconv.Itoa(pageSize))
			params.Set("offset", strconv.Itoa(offset))

			rows, err := bot.ExecuteCargoQuery(params)
			if err != nil {
				log.Printf("[INGEST WARNING] ScoreboardPlayers failed for %q offset %d: %v", overviewPage, offset, err)
				break
			}
			if len(rows) == 0 {
				break
			}

			// Count distinct new game_ids in this page.
			newGamesInPage := 0
			seenInPage := make(map[string]bool)
			for _, item := range rows {
				gid := strings.TrimSpace(item["title"]["GameId"])
				if gid == "" || seenInPage[gid] {
					continue
				}
				seenInPage[gid] = true
				if _, known := knownGameIDs[gid]; !known {
					newGamesInPage++
				}
			}
			if newGamesInPage == 0 {
				log.Printf("[INGEST] %q offset %d — all game_ids already complete, stopping", tournName, offset)
				break
			}
			log.Printf("[INGEST DEBUG] %q offset %d — %d rows, %d new games", overviewPage, offset, len(rows), newGamesInPage)

			// Group rows by game_id for per-game completion tracking.
			type rowData = map[string]string
			gameRows := make(map[string][]rowData)
			for _, item := range rows {
				gid := strings.TrimSpace(item["title"]["GameId"])
				if gid == "" {
					continue
				}
				gameRows[gid] = append(gameRows[gid], item["title"])
			}

			for gameID, playerRows := range gameRows {
				if _, seen := knownGameIDs[gameID]; seen {
					continue
				}

				first := playerRows[0]
				matchID := strings.TrimSpace(first["MatchId"])
				rawDate := strings.TrimSpace(first["DateTime UTC"])

				parsedTime, parseErr := time.ParseInLocation(dateLayout, rawDate, time.UTC)
				if parseErr != nil {
					parsedTime, parseErr = time.ParseInLocation(dateOnlyLayout, rawDate, time.UTC)
					if parseErr != nil {
						log.Printf("[INGEST WARNING] Cannot parse date %q for game %q — skipping", rawDate, gameID)
						continue
					}
				}
				gameDate := parsedTime.Format(dateLayout)

				lpGameID, err := upsertGame(db, lpTournamentID, gameID, matchID, gameDate, "", "", "")
				if err != nil {
					log.Printf("[INGEST WARNING] upsertGame failed for %q: %v", gameID, err)
				}

				playersImported := 0
				for _, p := range playerRows {
					handle := strings.TrimSpace(p["Name"])
					proTeam := strings.TrimSpace(p["Team"])
					role := strings.TrimSpace(p["Role"])
					leaguepediaID := strings.TrimSpace(p["Link"])

					if handle == "" {
						continue
					}

					playerRegion := fallbackRegionCode
					if isIntl {
						if r, ok := teamRegions[proTeam]; ok {
							playerRegion = r
						}
					}

					playerID, err := upsertPlayer(db, leaguepediaID, handle, role, proTeam, playerRegion)
					if err != nil {
						log.Printf("[INGEST WARNING] upsertPlayer failed for %q: %v", handle, err)
						continue
					}

					kills, _ := strconv.Atoi(p["Kills"])
					deaths, _ := strconv.Atoi(p["Deaths"])
					assists, _ := strconv.Atoi(p["Assists"])
					cs, _ := strconv.Atoi(p["CS"])

					rawWin := strings.ToLower(strings.TrimSpace(p["PlayerWin"]))
					isWin := rawWin == "yes" || rawWin == "1" || rawWin == "true"
					winVal := 0
					if isWin {
						winVal = 1
					}

					fp := scoring.CalculateFantasyPoints(scoring.PlayerStats{
						Kills:   kills,
						Deaths:  deaths,
						Assists: assists,
						CS:      cs,
						Win:     isWin,
					})

					if err := insertMatchStat(db, playerID, gameID, gameDate, winVal, kills, deaths, assists, cs, fp); err != nil {
						log.Printf("[INGEST WARNING] insertMatchStat failed for %q game %q: %v", handle, gameID, err)
						continue
					}
					playersImported++
					savedThisTournament++
					totalNew++
				}

				if playersImported == 10 && lpGameID > 0 {
					if err := markGameComplete(db, lpGameID); err != nil {
						log.Printf("[INGEST WARNING] markGameComplete failed for game %q: %v", gameID, err)
					}
					knownGameIDs[gameID] = struct{}{}
				} else if playersImported > 0 {
					log.Printf("[INGEST DEBUG] Game %q: imported %d/10 player rows (partial)", gameID, playersImported)
				}
			}

			if len(rows) < pageSize {
				break
			}
			offset += pageSize
		}

		log.Printf("[INGEST] Tournament %q — saved %d new stat records", tournName, savedThisTournament)
	}

	log.Printf("[INGEST COMPLETE] Region %s — %d total new records this run", region, totalNew)
	return nil
}

// ─── System settings ─────────────────────────────────────────────────────────

// GetLastStatsUpdate returns the last successful ingestion timestamp from
// system_settings. Returns a sentinel of one year ago if no record exists yet,
// so the very first run picks up the current active season rather than
// attempting to ingest the entire historical database.
func GetLastStatsUpdate(db *sql.DB) (string, error) {
	var lastUpdate string
	err := db.QueryRow(`SELECT value FROM system_settings WHERE key = 'last_stats_update'`).Scan(&lastUpdate)
	if err == sql.ErrNoRows {
		// Default: one year ago — catches the current season without
		// pulling years of historical data on a brand-new install.
		oneYearAgo := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01-02 15:04:05")
		return oneYearAgo, nil
	}
	return lastUpdate, err
}

// SetLastStatsUpdate writes the current UTC time as the last successful
// ingestion timestamp, replacing any previous value.
func SetLastStatsUpdate(db *sql.DB) error {
	currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ('last_stats_update', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
		    value      = excluded.value,
		    updated_at = CURRENT_TIMESTAMP`,
		currentTime,
	)
	return err
}

// ─── Entry point ─────────────────────────────────────────────────────────────

// IngestRecentLeagueStats ingests all games in the given region that occurred
// after sinceDate. It is the exported bridge called by admin.go per region.
func IngestRecentLeagueStats(db *sql.DB, bot *BotClient, region, sinceDate string, knownGameIDs map[string]struct{}) error {
	log.Printf("[INGEST] Starting region: %s (games after %s)", region, sinceDate)
	return FetchMatchStatsForRoster(db, bot, region, sinceDate, knownGameIDs)
}

// ─── Diagnostic ──────────────────────────────────────────────────────────────

func RunCargoRawDiagnostic(bot *BotClient, testHandle string) {
	log.Printf("[DIAGNOSTIC] Trace for player: %s", testHandle)

	baseURL := "https://lol.fandom.com/wiki/Special:CargoExec"
	params := url.Values{}
	params.Set("tables", "ScoreboardGames=SG,ScoreboardPlayers=SP")
	params.Set("join_on", "SG.GameId=SP.GameId")
	params.Set("fields", "SP.Name,SG.DateTime_UTC,SP.Kills,SP.Deaths,SP.Assists,SP.CS,SP.Win")
	params.Set("where", fmt.Sprintf("SP.Name=\"%s\"", testHandle))
	params.Set("limit", "5")

	req, err := http.NewRequest("GET", fmt.Sprintf("%s?%s", baseURL, params.Encode()), nil)
	if err != nil {
		log.Printf("[DIAGNOSTIC ERROR] %v", err)
		return
	}
	req.Header.Set("User-Agent", userAgentHeader)

	resp, err := bot.HTTPClient.Do(req)
	if err != nil {
		log.Printf("[DIAGNOSTIC ERROR] %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("[DIAGNOSTIC] Status: %s", resp.Status)

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	reError := regexp.MustCompile(`(?s)<div[^>]*class="[^"]*(?:error|mw-parser-output|cargo-error)[^"]*"[^>]*>(.*?)</div>`)
	if matches := reError.FindAllStringSubmatch(bodyStr, -1); len(matches) > 0 {
		for _, match := range matches {
			clean := strings.TrimSpace(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(match[1], ""))
			if clean != "" {
				log.Printf("[DIAGNOSTIC] %s", clean)
			}
		}
	} else {
		if len(bodyStr) > 1000 {
			bodyStr = bodyStr[:1000]
		}
		log.Printf("[DIAGNOSTIC] Body:\n%s", bodyStr)
	}
}
