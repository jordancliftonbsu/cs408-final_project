package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const leaguepediaAPI = "https://lol.fandom.com/api.php"
const userAgentHeader = "FantasyLoLBot/1.0 (+https://fantasylol.com; admin@fantasylol.com) Go-HTTP-Client/1.1"

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
		return fmt.Errorf("bot login failed: %s - %s", loginResult.Login.Result, loginResult.Login.Reason)
	}

	return nil
}

func (c *BotClient) ExecuteCargoQuery(fullURL string) ([]map[string]map[string]string, error) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentHeader)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		CargoQuery []map[string]map[string]string `json:"cargoquery"`
		Error      *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if data.Error != nil {
		return nil, fmt.Errorf("cargo API error (%s): %s", data.Error.Code, data.Error.Info)
	}

	return data.CargoQuery, nil
}

func main() {
	// Look for .env in root directory
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("../../.env")
	}

	username := os.Getenv("FANDOM_USERNAME")
	botPass := os.Getenv("FANDOM_BOT_PASS")

	if username == "" || botPass == "" {
		log.Fatal("Error: FANDOM_USERNAME or FANDOM_BOT_PASS is missing from .env")
	}

	fmt.Printf("Testing Bot Authentication for user: %s...\n", username)

	bot, err := NewBotClient(leaguepediaAPI)
	if err != nil {
		log.Fatalf("Failed to create bot client: %v", err)
	}

	if err := bot.Login(username, botPass); err != nil {
		log.Fatalf("❌ Authentication Failed: %v", err)
	}
	fmt.Println("✅ Successfully authenticated with Fandom!")

	// Step 2: Test a live Cargo query using valid Leaguepedia Cargo schema fields
	params := url.Values{}
	params.Set("action", "cargoquery")
	params.Set("format", "json")
	params.Set("tables", "Players")
	params.Set("fields", "Players.ID,Players.Player,Players.Team,Players.Role")
	params.Set("where", "Players.Team IS NOT NULL")
	params.Set("limit", "5")

	fullURL := fmt.Sprintf("%s?%s", leaguepediaAPI, params.Encode())

	fmt.Println("Testing Cargo Query fetching players...")
	results, err := bot.ExecuteCargoQuery(fullURL)
	if err != nil {
		log.Fatalf("❌ Query Failed: %v", err)
	}

	fmt.Printf("✅ Ingestion Test Passed! Received %d players:\n", len(results))
	for i, item := range results {
		p := item["title"]
		fmt.Printf("  %d. Handle: %s | Team: %s | Role: %s\n", i+1, p["Player"], p["Team"], p["Role"])
	}
}
