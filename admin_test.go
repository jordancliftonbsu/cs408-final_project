package main

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// SetupTestDB initializes an isolated in-memory SQLite database using schema.sql
func SetupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("failed to read schema.sql: %v", err)
	}

	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("failed to execute schema.sql in test: %v", err)
	}

	return db
}

// TestSchemaValidation verifies tables and FK constraints
func TestSchemaValidation(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	// Clear dependent child table first to satisfy foreign key constraints
	if _, err := db.Exec("DELETE FROM pro_match_stats; DELETE FROM pro_players;"); err != nil {
		t.Fatalf("failed to clear pro_players: %v", err)
	}

	// Insert test player record
	_, err := db.Exec(`
		INSERT INTO pro_players (handle, role, pro_team, region, image_url)
		VALUES (?, ?, ?, ?, ?)
	`, "Faker", "Lee Sang-hyeok", "Mid", "T1", "LCK", "https://example.com/faker.png")

	if err != nil {
		t.Fatalf("failed to insert pro player: %v", err)
	}
}

// TestSystemSettingsTracker verifies storing and retrieving the last_stats_update timestamp
func TestSystemSettingsTracker(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	// Default fallback check
	lastUpdate, err := GetLastStatsUpdate(db)
	if err != nil {
		t.Fatalf("unexpected error reading initial last update: %v", err)
	}
	if lastUpdate == "" {
		t.Errorf("expected non-empty default last update timestamp")
	}

	// Set and verify update
	if err := SetLastStatsUpdate(db); err != nil {
		t.Fatalf("failed to set last stats update: %v", err)
	}

	updatedVal, err := GetLastStatsUpdate(db)
	if err != nil {
		t.Fatalf("failed to read updated last stats update: %v", err)
	}
	if updatedVal == lastUpdate {
		t.Errorf("expected updated timestamp, got unchanged value: %s", updatedVal)
	}
}

// TestAdminUpdateStatsIntegration verifies the HTTP admin route handler
func TestAdminUpdateStatsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in short mode")
	}

	db := SetupTestDB(t)
	defer db.Close()

	adminHandler := &AdminHandler{DB: db}

	payload := []byte(`{}`)
	req, err := http.NewRequest(http.MethodPost, "/admin/stats/update", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(adminHandler.UpdatePlayerStatsHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}
