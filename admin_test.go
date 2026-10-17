package main

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

// SetupTestDB initializes an isolated in-memory SQLite database for testing
func SetupTestDB(t *testing.T) *sql.DB {
	// Use :memory: or shared cache in-memory string to prevent disk locking
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create tables required for testing
	query := `
	CREATE TABLE IF NOT EXISTS stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		player_name TEXT,
		points REAL
	);`
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	return db
}

func TestSQLiteCRUDIntegration(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close() // ALWAYS close DB handle to avoid hanging on test cleanup

	// 1. CREATE (Insert)
	res, err := db.Exec("INSERT INTO stats (player_name, points) VALUES (?, ?)", "Faker", 43.0)
	if err != nil {
		t.Fatalf("failed to insert stat: %v", err)
	}

	id, _ := res.LastInsertId()

	// 2. READ
	var name string
	var points float64
	err = db.QueryRow("SELECT player_name, points FROM stats WHERE id = ?", id).Scan(&name, &points)
	if err != nil {
		t.Fatalf("failed to read stat: %v", err)
	}
	if name != "Faker" || points != 43.0 {
		t.Errorf("expected Faker/43.0, got %s/%.1f", name, points)
	}

	// 3. UPDATE
	_, err = db.Exec("UPDATE stats SET points = ? WHERE id = ?", 48.0, id)
	if err != nil {
		t.Fatalf("failed to update stat: %v", err)
	}

	// 4. DELETE
	_, err = db.Exec("DELETE FROM stats WHERE id = ?", id)
	if err != nil {
		t.Fatalf("failed to delete stat: %v", err)
	}
}

func TestAdminUpdateStatsIntegration(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()

	adminHandler := &AdminHandler{DB: db}

	payload := []byte(`{"player_name": "Faker"}`)
	req, err := http.NewRequest(http.MethodPost, "/admin/stats/update", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(adminHandler.UpdatePlayerStatsHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}
