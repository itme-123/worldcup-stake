package handlers

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestComputeLeaderboardScoresDrawsAndSameOwnerMatches(t *testing.T) {
	database := newTestLeaderboardDB(t)

	entries, err := ComputeLeaderboard(database)
	if err != nil {
		t.Fatalf("compute leaderboard: %v", err)
	}

	byName := map[string]struct{ wins, draws, losses int }{}
	order := []string{}
	for _, entry := range entries {
		byName[entry.Name] = struct{ wins, draws, losses int }{entry.Wins, entry.Draws, entry.Losses}
		order = append(order, entry.Name)
	}

	// Ava (Alpha): draw vs Beta, win vs Gamma
	assertRecord(t, byName, "Ava", 1, 1, 0)
	// Ben (Beta): draw vs Alpha
	assertRecord(t, byName, "Ben", 0, 1, 0)
	// Cam (Gamma + Delta): derby gives a win and a loss; Gamma also lost to Alpha
	assertRecord(t, byName, "Cam", 1, 0, 2)

	// Wins rank first, draws break the tie
	want := []string{"Ava", "Cam", "Ben"}
	for i, name := range want {
		if order[i] != name {
			t.Fatalf("rank %d = %s, want %s (full order %v)", i+1, order[i], name, order)
		}
	}
}

func assertRecord(t *testing.T, byName map[string]struct{ wins, draws, losses int }, name string, wins, draws, losses int) {
	t.Helper()
	got := byName[name]
	if got.wins != wins || got.draws != draws || got.losses != losses {
		t.Fatalf("%s record = %dW %dD %dL, want %dW %dD %dL", name, got.wins, got.draws, got.losses, wins, draws, losses)
	}
}

func newTestLeaderboardDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec(`
		CREATE TABLE users (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE teams (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			code TEXT NOT NULL
		);
		CREATE TABLE user_teams (
			user_id INTEGER REFERENCES users(id),
			team_id INTEGER REFERENCES teams(id),
			PRIMARY KEY (user_id, team_id)
		);
		CREATE TABLE matches (
			id           TEXT PRIMARY KEY,
			home_team_id INTEGER REFERENCES teams(id),
			away_team_id INTEGER REFERENCES teams(id),
			home_score   INTEGER,
			away_score   INTEGER,
			status       TEXT,
			match_date   TEXT,
			stage        TEXT
		);

		INSERT INTO users (id, name) VALUES (1, 'Ava'), (2, 'Ben'), (3, 'Cam');
		INSERT INTO teams (id, name, code) VALUES
			(1, 'Alpha', 'ALP'),
			(2, 'Beta', 'BET'),
			(3, 'Gamma', 'GAM'),
			(4, 'Delta', 'DEL');
		INSERT INTO user_teams (user_id, team_id) VALUES
			(1, 1),
			(2, 2),
			(3, 3),
			(3, 4);

		INSERT INTO matches (id, home_team_id, away_team_id, home_score, away_score, status, match_date, stage) VALUES
			('draw', 1, 2, 1, 1, 'FINISHED', '2026-06-12T00:00:00Z', 'Group'),
			('same-owner', 3, 4, 2, 0, 'FINISHED', '2026-06-13T00:00:00Z', 'Group'),
			('win', 1, 3, 2, 0, 'FINISHED', '2026-06-14T00:00:00Z', 'Group');
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return database
}
