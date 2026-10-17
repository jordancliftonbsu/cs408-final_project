package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database and sets up test tables
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create a sample player_stats table for testing CRUD
	schema := `
	CREATE TABLE IF NOT EXISTS player_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		player_name TEXT NOT NULL,
		kills INTEGER DEFAULT 0,
		deaths INTEGER DEFAULT 0,
		assists INTEGER DEFAULT 0,
		cs INTEGER DEFAULT 0,
		fantasy_points REAL DEFAULT 0.0
	);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create test schema: %v", err)
	}

	return db
}

func TestSQLiteCRUDIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. CREATE: Insert a player stat record
	insertQuery := `INSERT INTO player_stats (player_name, kills, deaths, assists, cs, fantasy_points) VALUES (?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(insertQuery, "Faker", 8, 1, 6, 280, 40.6)
	if err != nil {
		t.Fatalf("failed to CREATE record: %v", err)
	}

	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		t.Fatalf("failed to retrieve last insert ID: %v", err)
	}

	// 2. READ: Query the record back
	var playerName string
	var points float64
	readQuery := `SELECT player_name, fantasy_points FROM player_stats WHERE id = ?`
	err = db.QueryRow(readQuery, id).Scan(&playerName, &points)
	if err != nil {
		t.Fatalf("failed to READ record: %v", err)
	}
	if playerName != "Faker" || points != 40.6 {
		t.Errorf("READ mismatch: expected Faker with 40.6 points, got %s with %.2f", playerName, points)
	}

	// 3. UPDATE: Modify stats and recalculate points
	updateQuery := `UPDATE player_stats SET kills = ?, fantasy_points = ? WHERE id = ?`
	_, err = db.Exec(updateQuery, 10, 46.6, id)
	if err != nil {
		t.Fatalf("failed to UPDATE record: %v", err)
	}

	// Verify Update
	err = db.QueryRow(readQuery, id).Scan(&playerName, &points)
	if err != nil || points != 46.6 {
		t.Errorf("UPDATE verification failed: expected 46.6 points, got %.2f", points)
	}

	// 4. DELETE: Remove the test record
	deleteQuery := `DELETE FROM player_stats WHERE id = ?`
	_, err = db.Exec(deleteQuery, id)
	if err != nil {
		t.Fatalf("failed to DELETE record: %v", err)
	}

	// Verify Deletion
	err = db.QueryRow(readQuery, id).Scan(&playerName, &points)
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after DELETE, got: %v", err)
	}
}

func TestAdminUpdateStatsIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := &AdminHandler{DB: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/stats/update", nil)
	rec := httptest.NewRecorder()

	handler.UpdatePlayerStatsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", rec.Code)
	}
}
