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
    handle VARCHAR(255) UNIQUE NOT NULL,
    real_name VARCHAR(255),
    role VARCHAR(50) NOT NULL,
    pro_team VARCHAR(255) NOT NULL,
    region VARCHAR(4) NOT NULL,
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
    match_date DATE NOT NULL,
    week_number INTEGER DEFAULT 1,
    win INTEGER DEFAULT 0, -- 1 for Win, 0 for Loss
    kills INTEGER DEFAULT 0,
    deaths INTEGER DEFAULT 0,
    assists INTEGER DEFAULT 0,
    creep_score INTEGER DEFAULT 0,
    fantasy_points REAL DEFAULT 0.0,
    FOREIGN KEY (player_id) REFERENCES pro_players(player_id)
);

-- 7. System Settings Tracker Table
CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Re-enable foreign key constraints
PRAGMA foreign_keys = ON;

-- Seed Data for Pro Players
INSERT OR IGNORE INTO pro_players (handle, real_name, role, pro_team, region, image_url) VALUES
('Faker', 'Lee Sang-hyeok', 'Mid', 'T1', 'LCK', ''),
('Chovy', 'Jeong Ji-hoon', 'Mid', 'Gen.G', 'LCK', ''),
('Caps', 'Rasmus Winther', 'Mid', 'G2 Esports', 'LEC', ''),
('Blaber', 'Robert Huang', 'Jng', 'Cloud9', 'LCS', ''),
('Knight', 'Zhuo Ding', 'Mid', 'BLG', 'LPL', '');

-- -- Seed Data for Pro Match Stats
-- INSERT OR IGNORE INTO pro_match_stats (stat_id, player_id, match_date, week_number, kills, deaths, assists, creep_score, fantasy_points) VALUES
-- (1, (SELECT player_id FROM pro_players WHERE handle='Faker'), '2026-03-01', 1, 6, 1, 8, 280, 24.50),
-- (2, (SELECT player_id FROM pro_players WHERE handle='Chovy'), '2026-03-01', 1, 8, 0, 5, 310, 28.20),
-- (3, (SELECT player_id FROM pro_players WHERE handle='Caps'), '2026-03-01', 1, 5, 3, 9, 250, 21.00),
-- (4, (SELECT player_id FROM pro_players WHERE handle='Blaber'), '2026-03-01', 1, 4, 2, 11, 190, 22.80),
-- (5, (SELECT player_id FROM pro_players WHERE handle='Knight'), '2026-03-01', 1, 9, 1, 4, 295, 29.10);