package handlers

import (
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNextMatchStatus(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		incoming string
		want     string
	}{
		{name: "new match stores incoming as app status", incoming: "TIMED", want: "UPCOMING"},
		{name: "scheduled can become timed", previous: "SCHEDULED", incoming: "TIMED", want: "UPCOMING"},
		{name: "timed can become live", previous: "TIMED", incoming: "IN_PLAY", want: "LIVE"},
		{name: "live cannot become timed", previous: "IN_PLAY", incoming: "TIMED", want: "LIVE"},
		{name: "live can become finished", previous: "IN_PLAY", incoming: "FINISHED", want: "FINISHED"},
		{name: "finished cannot become live", previous: "FINISHED", incoming: "IN_PLAY", want: "FINISHED"},
		{name: "finished cannot become timed", previous: "FINISHED", incoming: "TIMED", want: "FINISHED"},
		{name: "cancelled stores as finished", previous: "UPCOMING", incoming: "CANCELLED", want: "FINISHED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextMatchStatus(tt.previous, tt.incoming); got != tt.want {
				t.Fatalf("nextMatchStatus(%q, %q) = %q, want %q", tt.previous, tt.incoming, got, tt.want)
			}
		})
	}
}

func TestFIFAMatchStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{status: 0, want: "FINISHED"},
		{status: 1, want: "UPCOMING"},
		{status: 3, want: "LIVE"},
		{status: 99, want: "UPCOMING"},
	}

	for _, tt := range tests {
		if got := fifaMatchStatus(tt.status); got != tt.want {
			t.Fatalf("fifaMatchStatus(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestInternalMatchID(t *testing.T) {
	got := internalMatchID("2026-06-12T02:00:00Z", "KOR", "CZE")
	want := "202606120200_KOR_CZE"
	if got != want {
		t.Fatalf("internalMatchID() = %q, want %q", got, want)
	}
}

func TestNormalizeTeamCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{code: "kor", want: "KOR"},
		{code: " URU ", want: "URY"},
		{code: "URY", want: "URY"},
	}

	for _, tt := range tests {
		if got := normalizeTeamCode(tt.code); got != tt.want {
			t.Fatalf("normalizeTeamCode(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestSyncFallsBackAndStoresSourceID(t *testing.T) {
	database := newTestSyncDB(t)
	syncer := NewSyncer(database, []MatchProvider{
		fakeProvider{name: "fifa", err: errors.New("unavailable")},
		fakeProvider{name: "football-data", matches: []ProviderMatch{{
			Source:       "football-data",
			SourceID:     "12345",
			HomeTeamCode: "KOR",
			AwayTeamCode: "CZE",
			Status:       "TIMED",
			MatchDate:    "2026-06-12T02:00:00Z",
			Stage:        "Group A",
		}}},
	}, nil)

	syncer.Sync()

	var matchID, sourceMatchID, status string
	err := database.QueryRow(`
		SELECT m.id, ms.source_match_id, m.status
		FROM matches m
		JOIN match_sources ms ON ms.match_id = m.id
		WHERE ms.source = 'football-data'
	`).Scan(&matchID, &sourceMatchID, &status)
	if err != nil {
		t.Fatalf("query synced match: %v", err)
	}
	if matchID != "202606120200_KOR_CZE" {
		t.Fatalf("matchID = %q, want %q", matchID, "202606120200_KOR_CZE")
	}
	if sourceMatchID != "12345" {
		t.Fatalf("sourceMatchID = %q, want %q", sourceMatchID, "12345")
	}
	if status != "UPCOMING" {
		t.Fatalf("status = %q, want %q", status, "UPCOMING")
	}
}

func TestSyncUsesTeamCodeAliases(t *testing.T) {
	database := newTestSyncDB(t)
	syncer := NewSyncer(database, []MatchProvider{
		fakeProvider{name: "fifa", matches: []ProviderMatch{{
			Source:       "fifa",
			SourceID:     "400000001",
			HomeTeamCode: "URU",
			AwayTeamCode: "KOR",
			Status:       "UPCOMING",
			MatchDate:    "2026-06-14T02:00:00Z",
			Stage:        "Group H",
		}}},
	}, nil)

	syncer.Sync()

	var matchID, homeCode string
	err := database.QueryRow(`
		SELECT m.id, ht.code
		FROM matches m
		JOIN teams ht ON ht.id = m.home_team_id
	`).Scan(&matchID, &homeCode)
	if err != nil {
		t.Fatalf("query synced match: %v", err)
	}
	if matchID != "202606140200_URY_KOR" {
		t.Fatalf("matchID = %q, want %q", matchID, "202606140200_URY_KOR")
	}
	if homeCode != "URY" {
		t.Fatalf("homeCode = %q, want %q", homeCode, "URY")
	}
}

type fakeProvider struct {
	name    string
	matches []ProviderMatch
	err     error
}

func (p fakeProvider) Name() string {
	return p.name
}

func (p fakeProvider) FetchMatches() ([]ProviderMatch, error) {
	return p.matches, p.err
}

func newTestSyncDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec(`
		CREATE TABLE teams (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			code TEXT NOT NULL
		);
		CREATE TABLE matches (
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
		CREATE TABLE match_sources (
			match_id             TEXT NOT NULL REFERENCES matches(id),
			source               TEXT NOT NULL,
			source_match_id      TEXT NOT NULL,
			source_stage_id      TEXT,
			source_home_team_id  TEXT,
			source_away_team_id  TEXT,
			PRIMARY KEY (match_id, source),
			UNIQUE(source, source_match_id)
		);
		INSERT INTO teams (id, name, code) VALUES (1, 'Korea Republic', 'KOR');
		INSERT INTO teams (id, name, code) VALUES (2, 'Czechia', 'CZE');
		INSERT INTO teams (id, name, code) VALUES (3, 'Uruguay', 'URY');
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return database
}
