package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Init() (*sql.DB, error) {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return nil, err
	}

	database, err := sql.Open("sqlite", Path())
	if err != nil {
		return nil, err
	}
	if err := migrate(database); err != nil {
		return nil, err
	}
	return database, nil
}

func Path() string {
	return filepath.Join(DataDir(), "stake.db")
}

func DataDir() string {
	path := os.Getenv("DATA_DIR")
	if path == "" {
		return "data"
	}
	return path
}

func migrate(database *sql.DB) error {
	_, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS teams (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			code TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS user_teams (
			user_id INTEGER REFERENCES users(id),
			team_id INTEGER REFERENCES teams(id),
			PRIMARY KEY (user_id, team_id)
		);
		CREATE TABLE IF NOT EXISTS matches (
			id               TEXT PRIMARY KEY,
			home_team_id     INTEGER REFERENCES teams(id),
			away_team_id     INTEGER REFERENCES teams(id),
			home_score       INTEGER,
			away_score       INTEGER,
			status           TEXT,
			match_date       TEXT,
			stage            TEXT,
			home_red_cards   INTEGER NOT NULL DEFAULT 0,
			away_red_cards   INTEGER NOT NULL DEFAULT 0,
			red_cards_synced INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS match_sources (
			match_id             TEXT NOT NULL REFERENCES matches(id),
			source               TEXT NOT NULL,
			source_match_id      TEXT NOT NULL,
			source_stage_id      TEXT,
			source_home_team_id  TEXT,
			source_away_team_id  TEXT,
			PRIMARY KEY (match_id, source),
			UNIQUE(source, source_match_id)
		);
		CREATE TABLE IF NOT EXISTS push_subscriptions (
			id                 INTEGER PRIMARY KEY,
			user_id            INTEGER REFERENCES users(id),
			endpoint           TEXT NOT NULL UNIQUE,
			p256dh             TEXT NOT NULL,
			auth               TEXT NOT NULL,
			notify_leaderboard BOOLEAN NOT NULL DEFAULT 1,
			notify_match_start BOOLEAN NOT NULL DEFAULT 1,
			created_at         TEXT NOT NULL,
			updated_at         TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS notification_deliveries (
			subscription_id INTEGER REFERENCES push_subscriptions(id),
			event_key       TEXT NOT NULL,
			sent_at         TEXT NOT NULL,
			PRIMARY KEY (subscription_id, event_key)
		);
		CREATE TABLE IF NOT EXISTS leaderboard_state (
			user_id    INTEGER PRIMARY KEY REFERENCES users(id),
			rank       INTEGER NOT NULL,
			points     REAL NOT NULL,
			updated_at TEXT NOT NULL
		);
		UPDATE matches
		SET status = CASE UPPER(TRIM(status))
			WHEN 'SCHEDULED' THEN 'UPCOMING'
			WHEN 'TIMED' THEN 'UPCOMING'
			WHEN 'IN_PLAY' THEN 'LIVE'
			WHEN 'PAUSED' THEN 'LIVE'
			WHEN 'AWARDED' THEN 'FINISHED'
			WHEN 'CANCELLED' THEN 'FINISHED'
			WHEN 'POSTPONED' THEN 'FINISHED'
			WHEN 'SUSPENDED' THEN 'FINISHED'
			ELSE UPPER(TRIM(status))
		END
		WHERE status IS NOT NULL
			AND UPPER(TRIM(status)) NOT IN ('UPCOMING', 'LIVE', 'FINISHED');
	`)
	if err != nil {
		return err
	}

	// Resilience for any pre-existing database (e.g. if disk ever persists across
	// a deploy): add the red-card columns when an older schema is missing them.
	ensure := []struct{ table, column, def string }{
		{"matches", "home_red_cards", "INTEGER NOT NULL DEFAULT 0"},
		{"matches", "away_red_cards", "INTEGER NOT NULL DEFAULT 0"},
		{"matches", "red_cards_synced", "INTEGER NOT NULL DEFAULT 0"},
		{"match_sources", "source_stage_id", "TEXT"},
		{"match_sources", "source_home_team_id", "TEXT"},
		{"match_sources", "source_away_team_id", "TEXT"},
	}
	for _, c := range ensure {
		has, err := hasColumn(database, c.table, c.column)
		if err != nil {
			return err
		}
		if !has {
			if _, err := database.Exec("ALTER TABLE " + c.table + " ADD COLUMN " + c.column + " " + c.def); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasColumn(database *sql.DB, table, column string) (bool, error) {
	rows, err := database.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
