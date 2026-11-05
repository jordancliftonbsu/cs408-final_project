package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AuthHandler bundles the DB connection for all authentication endpoints.
type AuthHandler struct {
	DB *sql.DB
}

// sessionCookieName is the name of the HttpOnly cookie that stores the
// opaque session token on the client.
const sessionCookieName = "fl_session"

// sessionDuration controls how long a login session stays valid.
const sessionDuration = 7 * 24 * time.Hour

// usernamePattern allows letters, numbers, underscore, hyphen (3-30 chars).
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,30}$`)

// ---------------------------------------------------------------------------
// Request / response payloads
// ---------------------------------------------------------------------------

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// userResponse is the safe, public view of a user (never includes the hash).
type userResponse struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// validateRegistration checks the submitted fields and returns a
// user-friendly error message if anything is invalid.
func validateRegistration(username, email, password string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("username must be 3-30 characters and contain only letters, numbers, underscores, or hyphens")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return errors.New("please enter a valid email address")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}
	// bcrypt only hashes the first 72 bytes; reject anything longer so the
	// user isn't silently given a weaker-than-expected password.
	if len(password) > 72 {
		return errors.New("password must be 72 characters or fewer")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Session management
// ---------------------------------------------------------------------------

// newSessionToken returns a cryptographically-random, URL-safe token.
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// createSession inserts a new session row and returns the token.
func (h *AuthHandler) createSession(userID int64) (string, time.Time, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(sessionDuration)
	_, err = h.DB.Exec(
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expires,
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// setSessionCookie writes the HttpOnly session cookie to the response.
// Secure is enabled when COOKIE_SECURE=true (set this in production/HTTPS).
func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   os.Getenv("COOKIE_SECURE") == "true",
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie on the client.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   os.Getenv("COOKIE_SECURE") == "true",
		SameSite: http.SameSiteLaxMode,
	})
}

// userFromSession resolves the currently logged-in user from the request's
// session cookie. Returns (nil, nil) when there is no valid/active session.
func (h *AuthHandler) userFromSession(r *http.Request) (*userResponse, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, nil
	}

	var (
		u       userResponse
		expires time.Time
	)
	err = h.DB.QueryRow(`
		SELECT u.user_id, u.username, u.email, u.role, s.expires_at
		FROM sessions s
		JOIN users u ON u.user_id = s.user_id
		WHERE s.token = ?`, c.Value,
	).Scan(&u.UserID, &u.Username, &u.Email, &u.Role, &expires)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Expired session: clean it up and treat as logged out.
	if time.Now().After(expires) {
		_, _ = h.DB.Exec(`DELETE FROM sessions WHERE token = ?`, c.Value)
		return nil, nil
	}

	return &u, nil
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

// RegisterHandler handles POST /api/auth/register.
// Accepts JSON {username, email, password}, validates, hashes the password
// with bcrypt, creates the user, starts a session, and sets the cookie.
func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if err := validateRegistration(req.Username, req.Email, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[AUTH ERROR] failed to hash password: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create account")
		return
	}

	res, err := h.DB.Exec(
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		req.Username, req.Email, string(hash),
	)
	if err != nil {
		// UNIQUE constraint violation -> username or email already taken.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "that username or email is already registered")
			return
		}
		log.Printf("[AUTH ERROR] failed to insert user: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create account")
		return
	}

	userID, _ := res.LastInsertId()

	token, expires, err := h.createSession(userID)
	if err != nil {
		log.Printf("[AUTH ERROR] failed to create session: %v", err)
		writeError(w, http.StatusInternalServerError, "account created but sign-in failed; please log in")
		return
	}
	setSessionCookie(w, token, expires)

	writeJSON(w, http.StatusCreated, userResponse{
		UserID:   userID,
		Username: req.Username,
		Email:    req.Email,
		Role:     "User",
	})
}

// LoginHandler handles POST /api/auth/login.
// Accepts JSON {username, password}, verifies the bcrypt hash, starts a
// session, and sets the cookie.
func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	var (
		userID int64
		email  string
		role   string
		hash   string
	)
	err := h.DB.QueryRow(
		`SELECT user_id, email, role, password_hash FROM users WHERE username = ?`,
		req.Username,
	).Scan(&userID, &email, &role, &hash)

	// Use the same generic error for "no such user" and "wrong password" so
	// we don't leak which usernames exist.
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		log.Printf("[AUTH ERROR] login lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, expires, err := h.createSession(userID)
	if err != nil {
		log.Printf("[AUTH ERROR] failed to create session: %v", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	setSessionCookie(w, token, expires)

	writeJSON(w, http.StatusOK, userResponse{
		UserID:   userID,
		Username: req.Username,
		Email:    email,
		Role:     role,
	})
}

// LogoutHandler handles POST /api/auth/logout.
// Deletes the server-side session and clears the cookie.
func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		if _, err := h.DB.Exec(`DELETE FROM sessions WHERE token = ?`, c.Value); err != nil {
			log.Printf("[AUTH ERROR] failed to delete session: %v", err)
		}
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// MeHandler handles GET /api/auth/me.
// Returns the current user if a valid session exists, or 401 otherwise.
// Useful for the frontend to decide what to render (e.g. the "My Leagues"
// panel vs. a login prompt).
func (h *AuthHandler) MeHandler(w http.ResponseWriter, r *http.Request) {
	u, err := h.userFromSession(r)
	if err != nil {
		log.Printf("[AUTH ERROR] session lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read session")
		return
	}
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
