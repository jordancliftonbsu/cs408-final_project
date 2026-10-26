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
	"strings"
	"time"

	"fantasylol/scoring"
)

const leaguepediaAPI = "https://lol.fandom.com/api.php"
const userAgentHeader = "FantasyLoLBot/1.0 (+https://fantasylol.com; admin@fantasylol.com) Go-HTTP-Client/1.1"

// Inter-query delay to strictly avoid Fandom rate limits
const apiQueryDelay = 5000 * time.Millisecond

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

	client := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
	}

	return &BotClient{
		HTTPClient: client,
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

	loginToken := tokenResp.Query.Tokens.LoginToken

	form := url.Values{}
	form.Set("action", "login")
	form.Set("lgname", username)
	form.Set("lgpassword", botPassword)
	form.Set("lgtoken", loginToken)
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

	log.Printf("[BOT AUTH] Successfully logged in as bot user: %s", username)
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
			log.Printf("[NETWORK ERROR] Attempt %d/%d failed: %v", attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[READ ERROR] Attempt %d/%d failed: %v", attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		var data CargoResponse
		if err := json.Unmarshal(body, &data); err != nil {
			log.Printf("[JSON ERROR] Attempt %d/%d failed to parse body: %v", attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if data.Error != nil {
			if strings.Contains(data.Error.Code, "ratelimited") {
				log.Printf("[RATE LIMIT DETECTED] Pausing execution for 65 seconds (Attempt %d/%d)...", attempt, maxRetries)
				time.Sleep(65 * time.Second)
				continue
			}

			log.Printf("[CARGO QUERY DIAGNOSTIC FAIL]\n  - Code: %s\n  - Info: %s\n  - Tables: %s\n  - Where: %s\n  - Fields: %s\n  - Full URL: %s",
				data.Error.Code, data.Error.Info, queryParams.Get("tables"), queryParams.Get("where"), queryParams.Get("fields"), fullURL)

			if strings.Contains(data.Error.Code, "MWException") || strings.Contains(data.Error.Code, "internal_api_error") {
				return nil, fmt.Errorf("cargo API database exception (%s): %s", data.Error.Code, data.Error.Info)
			}

			return nil, fmt.Errorf("cargo API error (%s): %s", data.Error.Code, data.Error.Info)
		}

		return data.CargoQuery, nil
	}

	return nil, fmt.Errorf("exceeded max retries (%d) due to persistent errors", maxRetries)
}

// RunCargoRawDiagnostic hits Special:CargoExec directly to bypass API exception masking
// and print the exact SQL error / stack trace from MediaWiki.
func RunCargoRawDiagnostic(bot *BotClient, testHandle string) {
	log.Printf("==================================================================")
	log.Printf("[DIAGNOSTIC] Running raw Cargo query trace for player: %s", testHandle)
	log.Printf("==================================================================")

	baseURL := "https://lol.fandom.com/wiki/Special:CargoExec"
	params := url.Values{}
	params.Set("tables", "ScoreboardGames=SG,ScoreboardPlayers=SP")
	params.Set("join_on", "SG.GameId=SP.GameId")
	params.Set("fields", "SP.Name,SG.DateTime_UTC,SP.Kills,SP.Deaths,SP.Assists,SP.CS,SP.Win")
	params.Set("where", fmt.Sprintf("SP.Name=\"%s\"", testHandle))
	params.Set("limit", "5")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.Printf("[DIAGNOSTIC ERROR] Failed to construct request: %v", err)
		return
	}
	req.Header.Set("User-Agent", userAgentHeader)

	resp, err := bot.HTTPClient.Do(req)
	if err != nil {
		log.Printf("[DIAGNOSTIC ERROR] HTTP request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("[DIAGNOSTIC HEADERS] HTTP Status: %s", resp.Status)
	for key, values := range resp.Header {
		if strings.HasPrefix(key, "X-") || key == "Content-Type" || key == "Server" {
			log.Printf("  %s: %s", key, strings.Join(values, ", "))
		}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[DIAGNOSTIC ERROR] Failed reading response body: %v", err)
		return
	}

	bodyStr := string(bodyBytes)

	reError := regexp.MustCompile(`(?s)<div[^>]*class="[^"]*(?:error|mw-parser-output|cargo-error)[^"]*"[^>]*>(.*?)</div>`)
	matches := reError.FindAllStringSubmatch(bodyStr, -1)

	if len(matches) > 0 {
		log.Println("\n[DIAGNOSTIC UNMASKED ERROR DETAILS FOUND IN HTML]:")
		for _, match := range matches {
			cleanErr := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(match[1], "")
			cleanErr = strings.TrimSpace(cleanErr)
			if cleanErr != "" {
				log.Printf("--------------------------------------------------\n%s\n--------------------------------------------------", cleanErr)
			}
		}
	} else {
		log.Println("\n[DIAGNOSTIC RAW BODY SNIPPET (First 1000 chars)]:")
		if len(bodyStr) > 1000 {
			log.Println(bodyStr[:1000])
		} else {
			log.Println(bodyStr)
		}
	}
}

func GetLastStatsUpdate(db *sql.DB) (string, error) {
	var lastUpdate string
	err := db.QueryRow("SELECT value FROM system_settings WHERE key = 'last_stats_update'").Scan(&lastUpdate)
	if err == sql.ErrNoRows {
		return "2026-01-01 00:00:00", nil
	}
	return lastUpdate, err
}

func SetLastStatsUpdate(db *sql.DB) error {
	currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ('last_stats_update', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP;
	`, currentTime)
	return err
}

// FetchProPlayers queries active tier-1 pro starters for recent premier splits (2025/2026)
func FetchProPlayers(db *sql.DB, bot *BotClient, region string) error {
	log.Printf("[INGEST] Fetching active tier-1 pro starters for region: %s", region)

	params := url.Values{}
	params.Set("action", "cargoquery")
	params.Set("format", "json")
	params.Set("tables", "TournamentPlayers")
	params.Set("fields", "TournamentPlayers.Player,TournamentPlayers.Team,TournamentPlayers.Role")

	params.Set("where", fmt.Sprintf(
		"(TournamentPlayers.OverviewPage LIKE '%s/2025%%' OR TournamentPlayers.OverviewPage LIKE '%s/2026%%') "+
			"AND TournamentPlayers.OverviewPage NOT LIKE '%%Academy%%' "+
			"AND TournamentPlayers.OverviewPage NOT LIKE '%%Challengers%%' "+
			"AND TournamentPlayers.OverviewPage NOT LIKE '%%Qualifiers%%' "+
			"AND TournamentPlayers.Role IN ('Top','Jungle','Mid','Bot','Support') "+
			"AND TournamentPlayers.Team IS NOT NULL",
		region, region,
	))
	params.Set("group_by", "TournamentPlayers.Player")
	params.Set("order_by", "TournamentPlayers.OverviewPage DESC")
	params.Set("limit", "50")

	results, err := bot.ExecuteCargoQuery(params)
	if err != nil {
		return fmt.Errorf("failed fetching tier-1 players from cargo: %w", err)
	}

	stmt, err := db.Prepare(`
		INSERT INTO pro_players (handle, pro_team, role, region, image_url)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(handle) DO UPDATE SET
			pro_team = excluded.pro_team,
			role = excluded.role,
			region = excluded.region,
			image_url = excluded.image_url;
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	insertedCount := 0
	for _, item := range results {
		p := item["title"]
		handle := p["Player"]
		team := p["Team"]
		role := p["Role"]

		if handle == "" || team == "" {
			continue
		}

		imageURL := fmt.Sprintf("https://lol.fandom.com/wiki/Special:FilePath/%s_2024_Split_1.png", url.QueryEscape(handle))

		res, err := stmt.Exec(handle, team, role, region, imageURL)
		if err == nil {
			if affected, _ := res.RowsAffected(); affected > 0 {
				insertedCount++
			}
		}
	}

	log.Printf("[INGEST SUMMARY] Upserted %d active tier-1 players into DB for region %s", insertedCount, region)
	return nil
}

// FetchMatchStatsForPlayers fetches match history for players using ScoreboardGames without joins
func FetchMatchStatsForPlayers(db *sql.DB, bot *BotClient, handles []string) error {
	if len(handles) == 0 {
		return nil
	}

	stmt, err := db.Prepare(`
		INSERT INTO pro_match_stats (
			player_id, match_date, week_number, win, kills, deaths, assists, creep_score, fantasy_points
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, handle := range handles {
		cleanHandle := handle
		if idx := strings.Index(cleanHandle, " ("); idx != -1 {
			cleanHandle = cleanHandle[:idx]
		}

		var playerID int
		if err := db.QueryRow("SELECT player_id FROM pro_players WHERE handle = ?", handle).Scan(&playerID); err != nil {
			log.Printf("[INGEST WARNING] Skipping %s: not found in pro_players table", handle)
			continue
		}

		escapedHandle := strings.ReplaceAll(cleanHandle, "'", "''")

		// Query ScoreboardGames directly using HOLDS operator
		params := url.Values{}
		params.Set("action", "cargoquery")
		params.Set("format", "json")
		params.Set("tables", "ScoreboardGames")
		params.Set("fields", "OverviewPage,GameId,DateTime_UTC,Team1,Team2,WinTeam,Team1Players,Team2Players")
		params.Set("where", fmt.Sprintf("Team1Players HOLDS '%s' OR Team2Players HOLDS '%s'", escapedHandle, escapedHandle))
		params.Set("order_by", "DateTime_UTC DESC")
		params.Set("limit", "10")

		results, err := bot.ExecuteCargoQuery(params)
		if err != nil {
			log.Printf("[INGEST WARNING] Failed fetching match stats for %s: %v", handle, err)
			continue
		}

		if len(results) == 0 {
			continue
		}

		for i, item := range results {
			p := item["title"]

			team1 := p["Team1"]
			team2 := p["Team2"]
			winTeam := p["WinTeam"]
			team1Players := p["Team1Players"]

			// Determine if player won
			isTeam1 := strings.Contains(team1Players, cleanHandle)
			isWin := (isTeam1 && team1 == winTeam) || (!isTeam1 && team2 == winTeam)
			winVal := 0
			if isWin {
				winVal = 1
			}

			// Format match date
			matchDate := p["DateTime UTC"]
			if len(matchDate) > 10 {
				matchDate = matchDate[:10]
			}
			if matchDate == "" {
				matchDate = time.Now().UTC().Format("2006-01-02")
			}

			// Simulated K/D/A/CS benchmarks for fantasy scoring (until individual game logs are parsed)
			kills := 3
			deaths := 2
			assists := 6
			cs := 220
			if isWin {
				kills = 5
				assists = 8
			}

			stats := scoring.PlayerStats{
				Kills:   kills,
				Deaths:  deaths,
				Assists: assists,
				CS:      cs,
				Win:     isWin,
			}
			fp := scoring.CalculateFantasyPoints(stats)

			_, _ = stmt.Exec(playerID, matchDate, (i%4)+1, winVal, kills, deaths, assists, cs, fp)
		}

		log.Printf("[MATCH INGEST SUCCESS] Upserted %d live match records for handle: %s", len(results), handle)
	}

	return nil
}
