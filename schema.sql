-- Disable foreign keys during setup
PRAGMA foreign_keys = OFF;

-- 1. Users Table
CREATE TABLE IF NOT EXISTS users (
    user_id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'User',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 2. Leagues Table
CREATE TABLE IF NOT EXISTS leagues (
    league_id INTEGER PRIMARY KEY AUTOINCREMENT,
    commissioner_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    max_teams INTEGER DEFAULT 8,
    start_date DATE,
    privacy VARCHAR(50) DEFAULT 'Public',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (commissioner_id) REFERENCES users(user_id)
);

-- 3. Fantasy Teams Table
CREATE TABLE IF NOT EXISTS fantasy_teams (
    team_id INTEGER PRIMARY KEY AUTOINCREMENT,
    league_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    team_name VARCHAR(255) NOT NULL,
    wins INTEGER DEFAULT 0,
    losses INTEGER DEFAULT 0,
    total_points REAL DEFAULT 0.0,
    FOREIGN KEY (league_id) REFERENCES leagues(league_id),
    FOREIGN KEY (user_id) REFERENCES users(user_id)
);

-- 4. Pro Players Table
CREATE TABLE IF NOT EXISTS pro_players (
    player_id INTEGER PRIMARY KEY AUTOINCREMENT,
    leaguepedia_id TEXT UNIQUE,            -- Leaguepedia Players.OverviewPage (canonical cross-table key)
    handle VARCHAR(255) UNIQUE NOT NULL,
    role VARCHAR(50) NOT NULL,
    pro_team VARCHAR(255) NOT NULL,
    region VARCHAR(8) NOT NULL,            -- home region code: LCK, LPL, LEC, LCS, LCS.A, PCS, …
    image_url TEXT
);

-- 5. Roster Slots Table
CREATE TABLE IF NOT EXISTS roster_slots (
    roster_id INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id INTEGER NOT NULL,
    player_id INTEGER NOT NULL,
    role VARCHAR(50) NOT NULL,
    FOREIGN KEY (team_id) REFERENCES fantasy_teams(team_id),
    FOREIGN KEY (player_id) REFERENCES pro_players(player_id)
);

-- 6. Pro Match Stats Table
CREATE TABLE IF NOT EXISTS pro_match_stats (
    stat_id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL,
    game_id TEXT,                          -- Leaguepedia GameId for deduplication
    match_date DATE NOT NULL,
    win INTEGER DEFAULT 0,                 -- 1 for Win, 0 for Loss
    kills INTEGER DEFAULT 0,
    deaths INTEGER DEFAULT 0,
    assists INTEGER DEFAULT 0,
    creep_score INTEGER DEFAULT 0,
    fantasy_points REAL DEFAULT 0.0,
    FOREIGN KEY (player_id) REFERENCES pro_players(player_id),
    UNIQUE (player_id, game_id)            -- prevent duplicate rows on re-ingestion
);

-- 7. System Settings Tracker Table
CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 8. Leaguepedia Tournaments Table
--    One row per tournament we have ever ingested from.
--    overview_page is the canonical Leaguepedia wiki-path key (e.g.
--    "LEC/2026 Season/Spring Season") and is used as the join key
--    everywhere on the wiki.  It is also the value stored in
--    ScoreboardPlayers.OverviewPage, ScoreboardGames.OverviewPage, etc.
CREATE TABLE IF NOT EXISTS lp_tournaments (
    lp_tournament_id  INTEGER PRIMARY KEY AUTOINCREMENT,
    overview_page     TEXT UNIQUE NOT NULL,     -- Leaguepedia OverviewPage (stable key)
    name              TEXT NOT NULL,            -- display name, e.g. "LEC 2026 Spring"
    region            VARCHAR(8) NOT NULL,      -- mapped code: LCK, LPL, LEC, LCS, INT, …
    date_start        DATE,                     -- tournament start date from Leaguepedia
    date_end          DATE,                     -- tournament end date from Leaguepedia
    first_ingested_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_synced_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 9. Leaguepedia Games Table
--    One row per individual game (not series/match) that belongs to a
--    tracked tournament.  stats_complete is set to 1 once all 10 player
--    rows for this game_id have been written to pro_match_stats, so
--    subsequent ingestion runs can skip it without re-querying the API.
CREATE TABLE IF NOT EXISTS lp_games (
    lp_game_id       INTEGER PRIMARY KEY AUTOINCREMENT,
    lp_tournament_id INTEGER NOT NULL,
    game_id          TEXT UNIQUE NOT NULL,      -- Leaguepedia GameId
    match_id         TEXT,                      -- Leaguepedia MatchId (series identifier)
    game_date        DATETIME,                  -- UTC datetime of the game
    team_blue        TEXT,                      -- blue-side team name
    team_red         TEXT,                      -- red-side team name
    winner           TEXT,                      -- winning team name
    stats_complete   INTEGER DEFAULT 0,         -- 1 = all 10 player rows imported
    imported_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (lp_tournament_id) REFERENCES lp_tournaments(lp_tournament_id)
);

-- Re-enable foreign key constraints
PRAGMA foreign_keys = ON;