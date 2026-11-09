-- =============================================================================
-- seed_demo_leagues.sql — Demo/filler data for the Leagues search page.
--
-- Loaded manually by seed-demo.sh (NOT on server startup). Safe to re-run:
-- leagues are matched by name and users by username, so nothing is duplicated.
-- Note: re-running re-adds any demo members who were kicked or left.
--
-- Leagues are inserted oldest first (by start date), so the oldest leagues get
-- the lowest IDs. Start/end dates are relative to the day the script runs.
-- Five leagues are already concluded (hidden from search by default).
-- Demo users: demo01 .. demo16, all with password "demopass1".
-- Private demo leagues also use the password "demopass1".
-- =============================================================================

INSERT OR IGNORE INTO users (username, email, password_hash) VALUES
    ('demo01', 'demo01@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo02', 'demo02@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo03', 'demo03@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo04', 'demo04@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo05', 'demo05@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo06', 'demo06@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo07', 'demo07@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo08', 'demo08@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo09', 'demo09@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo10', 'demo10@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo11', 'demo11@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo12', 'demo12@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo13', 'demo13@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo14', 'demo14@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo15', 'demo15@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy'),
    ('demo16', 'demo16@example.com', '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy');

-- seq (insert order, oldest first), name, owner, max teams,
-- start offset, end offset, private (1/0)
WITH v(seq, name, owner, max_teams, start_off, end_off, pw) AS (VALUES
    ( 1, 'Spring Kickoff Classic',   'demo03',  8, '-120 days', '-60 days',  0),  -- concluded
    ( 2, 'Iron Jungle Invitational', 'demo05', 10,  '-90 days', '-30 days',  1),  -- concluded
    ( 3, 'Season Opener Cup',        'demo01',  6,  '-75 days', '-45 days',  0),  -- concluded
    ( 4, 'Bronze League Brawl',      'demo02',  8,  '-45 days', '-15 days',  0),  -- concluded
    ( 5, 'Old Guard',                'demo05', 10,  '-30 days', '+20 days',  1),
    ( 6, 'Last Split Standing',      'demo04', 12,  '-20 days',  '-1 days',  0),  -- ended yesterday
    ( 7, 'Ends Today Tourney',       'demo03',  6,  '-14 days',  '+0 days',  0),  -- last day is today
    ( 8, 'Quarterfinal Crew',        'demo04',  4,  '-10 days', '+10 days',  0),
    ( 9, 'Playoffs Bound',           'demo01',  6,   '-5 days', '+25 days',  0),
    (10, 'Spring Split Survivors',   'demo03',  8,   '-2 days', '+40 days',  0),
    (11, 'Day One Drafters',         'demo02',  8,   '+0 days', '+30 days',  0),
    (12, 'Jungle Gap',               'demo01',  4,   '+1 days', '+15 days',  1),
    (13, 'Mid Or Feed',              'demo02',  8,   '+2 days', '+20 days',  0),
    (14, 'Worlds Warriors',          'demo01',  8,   '+3 days', '+33 days',  0),
    (15, 'T1 Believers',             'demo02',  5,   '+4 days', '+24 days',  0),
    (16, 'Baron Nashors',            'demo02', 10,   '+5 days', '+35 days',  0),
    (17, 'Faker Fan Club',           'demo03',  6,   '+7 days', '+37 days',  1),
    (18, 'Summoners Rift Elite',     'demo04', 12,  '+10 days', '+50 days',  0),
    (19, 'Pentakill Party',          'demo05', 16,  '+14 days', '+44 days',  0),
    (20, 'LCK Legends',              'demo03',  8,  '+20 days', '+60 days',  0),
    (21, 'Rift Rivals',              'demo04', 14,  '+30 days', '+90 days',  1),
    (22, 'Dragon Soul Society',      'demo05', 10,  '+45 days', '+105 days', 0),
    (23, 'Nexus Defenders',          'demo01',  6,  '+60 days', '+120 days', 1)
)
INSERT INTO leagues (commissioner_id, name, max_teams, start_date, end_date, privacy, password_hash, created_at)
SELECT u.user_id, v.name, v.max_teams,
       date('now', 'localtime', v.start_off),
       date('now', 'localtime', v.end_off),
       CASE WHEN v.pw = 1 THEN 'Private' ELSE 'Public' END,
       CASE WHEN v.pw = 1 THEN '$2a$10$WaZEsvInFrX35F.VYIz9O.hDIKgTyzN9FM9qpoPlFc8Vo6n2.Bspy' END,
       -- created a week before it starts (or now, if that would be in the future)
       MIN(datetime('now', v.start_off, '-7 days'), datetime('now'))
FROM v
JOIN users u ON u.username = v.owner
WHERE NOT EXISTS (SELECT 1 FROM leagues l WHERE l.name = v.name)
ORDER BY v.seq;

-- Fill each league with demo01 .. demoNN (k = number of teams). Leagues where
-- k equals max teams are full.
WITH f(name, k) AS (VALUES
    ('Spring Kickoff Classic',   8),  -- full, concluded
    ('Iron Jungle Invitational', 6),
    ('Season Opener Cup',        5),
    ('Bronze League Brawl',      7),
    ('Old Guard',                9),
    ('Last Split Standing',     10),
    ('Ends Today Tourney',       4),
    ('Quarterfinal Crew',        4),  -- full, already started
    ('Playoffs Bound',           3),
    ('Spring Split Survivors',   5),
    ('Day One Drafters',         6),
    ('Jungle Gap',               4),  -- full
    ('Mid Or Feed',              8),  -- full
    ('Worlds Warriors',          3),
    ('T1 Believers',             3),
    ('Baron Nashors',           10),  -- full
    ('Faker Fan Club',           2),
    ('Summoners Rift Elite',     5),
    ('Pentakill Party',          1),
    ('LCK Legends',              0),
    ('Rift Rivals',              7),
    ('Dragon Soul Society',      2),
    ('Nexus Defenders',          0)
)
INSERT INTO fantasy_teams (league_id, user_id, team_name)
SELECT l.league_id, u.user_id, u.username || '''s Team'
FROM f
JOIN leagues l ON l.name = f.name
JOIN users u ON u.username LIKE 'demo__'
             AND CAST(substr(u.username, 5) AS INTEGER) BETWEEN 1 AND f.k
WHERE NOT EXISTS (
    SELECT 1 FROM fantasy_teams t WHERE t.league_id = l.league_id AND t.user_id = u.user_id
);
