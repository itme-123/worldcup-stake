package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Syncer struct {
	db        *sql.DB
	providers []MatchProvider
	push      *PushService
}

func NewSyncer(database *sql.DB, providers []MatchProvider, push *PushService) *Syncer {
	return &Syncer{db: database, providers: providers, push: push}
}

type MatchProvider interface {
	Name() string
	FetchMatches() ([]ProviderMatch, error)
}

type ProviderMatch struct {
	Source       string
	SourceID     string
	HomeTeamCode string
	AwayTeamCode string
	HomeScore    *int
	AwayScore    *int
	Status       string
	MatchDate    string
	Stage        string
	// FIFA-only identifiers used to fetch the per-match event timeline for red
	// cards. Empty for providers that don't expose them (e.g. football-data).
	SourceStageID    string
	SourceHomeTeamID string
	SourceAwayTeamID string
}

type FootballDataProvider struct {
	apiKey string
	client *http.Client
}

func NewFootballDataProvider(apiKey string) *FootballDataProvider {
	return &FootballDataProvider{apiKey: apiKey, client: http.DefaultClient}
}

func (p *FootballDataProvider) Name() string {
	return "football-data"
}

func (p *FootballDataProvider) FetchMatches() ([]ProviderMatch, error) {
	req, err := http.NewRequest("GET", "https://api.football-data.org/v4/competitions/WC/matches", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Auth-Token", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result footballDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	matches := make([]ProviderMatch, 0, len(result.Matches))
	for _, m := range result.Matches {
		matches = append(matches, ProviderMatch{
			Source:       p.Name(),
			SourceID:     fmt.Sprintf("%d", m.ID),
			HomeTeamCode: m.HomeTeam.TLA,
			AwayTeamCode: m.AwayTeam.TLA,
			HomeScore:    m.Score.FullTime.Home,
			AwayScore:    m.Score.FullTime.Away,
			Status:       m.Status,
			MatchDate:    m.UtcDate,
			Stage:        m.Stage,
		})
	}
	return matches, nil
}

type FIFAProvider struct {
	client *http.Client
}

func NewFIFAProvider() *FIFAProvider {
	return &FIFAProvider{client: http.DefaultClient}
}

func (p *FIFAProvider) Name() string {
	return "fifa"
}

func (p *FIFAProvider) FetchMatches() ([]ProviderMatch, error) {
	req, err := http.NewRequest("GET", "https://api.fifa.com/api/v3/calendar/matches?language=en&count=500&idCompetition=17&idSeason=285023", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result fifaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	matches := make([]ProviderMatch, 0, len(result.Results))
	for _, m := range result.Results {
		matches = append(matches, ProviderMatch{
			Source:           p.Name(),
			SourceID:         m.ID,
			HomeTeamCode:     m.Home.Abbreviation,
			AwayTeamCode:     m.Away.Abbreviation,
			HomeScore:        m.HomeTeamScore,
			AwayScore:        m.AwayTeamScore,
			Status:           fifaMatchStatus(m.MatchStatus),
			MatchDate:        m.Date,
			Stage:            fifaStage(m),
			SourceStageID:    m.IdStage,
			SourceHomeTeamID: m.Home.IdTeam,
			SourceAwayTeamID: m.Away.IdTeam,
		})
	}
	return matches, nil
}

type teamInfo struct {
	ID   int64
	Name string
}

type footballDataResponse struct {
	Matches []footballDataMatch `json:"matches"`
}

type footballDataMatch struct {
	ID       int      `json:"id"`
	UtcDate  string   `json:"utcDate"`
	Status   string   `json:"status"`
	Stage    string   `json:"stage"`
	HomeTeam apiTeam  `json:"homeTeam"`
	AwayTeam apiTeam  `json:"awayTeam"`
	Score    apiScore `json:"score"`
}

type apiTeam struct {
	TLA string `json:"tla"`
}

type apiScore struct {
	FullTime apiScorePair `json:"fullTime"`
}

type apiScorePair struct {
	Home *int `json:"home"`
	Away *int `json:"away"`
}

type fifaResponse struct {
	Results []fifaMatch `json:"Results"`
}

type fifaMatch struct {
	ID            string           `json:"IdMatch"`
	IdStage       string           `json:"IdStage"`
	Date          string           `json:"Date"`
	MatchStatus   int              `json:"MatchStatus"`
	Home          fifaTeam         `json:"Home"`
	Away          fifaTeam         `json:"Away"`
	HomeTeamScore *int             `json:"HomeTeamScore"`
	AwayTeamScore *int             `json:"AwayTeamScore"`
	StageName     []localizedValue `json:"StageName"`
	GroupName     []localizedValue `json:"GroupName"`
}

type fifaTeam struct {
	IdTeam       string `json:"IdTeam"`
	Abbreviation string `json:"Abbreviation"`
}

type localizedValue struct {
	Locale      string `json:"Locale"`
	Description string `json:"Description"`
}

func (s *Syncer) Sync() {
	log.Println("Syncing match data...")

	for _, provider := range s.providers {
		matches, err := provider.FetchMatches()
		if err != nil {
			log.Printf("Sync: provider %s failed: %v", provider.Name(), err)
			continue
		}
		if len(matches) == 0 {
			log.Printf("Sync: provider %s returned no matches", provider.Name())
			continue
		}
		if err := s.syncMatches(provider.Name(), matches); err != nil {
			log.Printf("Sync: provider %s sync failed: %v", provider.Name(), err)
			continue
		}
		s.syncRedCards()
		return
	}

	log.Println("Sync: all providers failed")
}

func (s *Syncer) syncMatches(providerName string, matches []ProviderMatch) error {
	teamMap, err := s.buildTeamMap()
	if err != nil {
		return fmt.Errorf("build team map: %w", err)
	}

	updated := 0
	for _, m := range matches {
		source := m.Source
		if source == "" {
			source = providerName
		}
		homeCode := normalizeTeamCode(m.HomeTeamCode)
		awayCode := normalizeTeamCode(m.AwayTeamCode)
		if homeCode == "" || awayCode == "" || m.MatchDate == "" || m.SourceID == "" {
			continue
		}
		home, homeOK := teamMap[homeCode]
		away, awayOK := teamMap[awayCode]
		if !homeOK || !awayOK {
			log.Printf("Sync: unknown team code home=%q away=%q, skipping %s match %s", homeCode, awayCode, source, m.SourceID)
			continue
		}
		matchID := internalMatchID(m.MatchDate, homeCode, awayCode)
		previousStatus, err := s.previousMatchStatus(matchID)
		if err != nil {
			log.Printf("Sync: failed to fetch previous match %s status: %v", matchID, err)
			continue
		}
		nextStatus := nextMatchStatus(previousStatus, m.Status)

		_, err = s.db.Exec(`
			INSERT INTO matches (id, home_team_id, away_team_id, home_score, away_score, status, match_date, stage)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				home_score = COALESCE(excluded.home_score, matches.home_score),
				away_score = COALESCE(excluded.away_score, matches.away_score),
				status     = excluded.status,
				match_date = excluded.match_date,
				stage      = excluded.stage
		`, matchID, home.ID, away.ID,
			m.HomeScore, m.AwayScore,
			nextStatus, m.MatchDate, m.Stage)
		if err != nil {
			log.Printf("Sync: failed to upsert match %s: %v", matchID, err)
			continue
		}
		_, err = s.db.Exec(`
			INSERT INTO match_sources (match_id, source, source_match_id, source_stage_id, source_home_team_id, source_away_team_id)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(match_id, source) DO UPDATE SET
				source_match_id     = excluded.source_match_id,
				source_stage_id     = excluded.source_stage_id,
				source_home_team_id = excluded.source_home_team_id,
				source_away_team_id = excluded.source_away_team_id
		`, matchID, source, m.SourceID, m.SourceStageID, m.SourceHomeTeamID, m.SourceAwayTeamID)
		if err != nil {
			log.Printf("Sync: failed to upsert source %s match %s: %v", source, m.SourceID, err)
			continue
		}
		if s.push != nil && isUpcomingStatus(previousStatus) && isLiveStatus(nextStatus) {
			s.push.NotifyMatchStart(matchID, home.ID, away.ID, home.Name, away.Name)
		}
		updated++
	}
	if s.push != nil {
		if err := s.notifyLeaderboardChanges(); err != nil {
			log.Printf("Sync: failed to process leaderboard notifications: %v", err)
		}
	}

	log.Printf("Sync: %d/%d matches upserted from %s", updated, len(matches), providerName)
	return nil
}

const (
	fifaEventYellowCard = 2
	fifaEventRedCard    = 3
)

type fifaTimelineResponse struct {
	Event []fifaTimelineEvent `json:"Event"`
}

type fifaTimelineEvent struct {
	Type   int    `json:"Type"`
	IdTeam string `json:"IdTeam"`
}

// syncRedCards fetches the FIFA event timeline for finished matches that haven't
// had their cards counted yet, tallies red (Type 3) and yellow (Type 2) card
// events per team, and stores the totals. Best-effort: any failure is logged and
// skipped so it can never disrupt the main score sync. Capped per cycle to stay
// gentle on the FIFA API; remaining matches are picked up on subsequent syncs.
func (s *Syncer) syncRedCards() {
	rows, err := s.db.Query(`
		SELECT m.id, ms.source_match_id, ms.source_stage_id, ms.source_home_team_id, ms.source_away_team_id
		FROM matches m
		JOIN match_sources ms ON ms.match_id = m.id AND ms.source = 'fifa'
		WHERE m.status = 'FINISHED' AND m.red_cards_synced = 0
			AND ms.source_stage_id IS NOT NULL AND ms.source_stage_id <> ''
			AND ms.source_home_team_id IS NOT NULL AND ms.source_away_team_id IS NOT NULL
		LIMIT 30
	`)
	if err != nil {
		log.Printf("RedCards: query failed: %v", err)
		return
	}
	type job struct{ id, matchID, stageID, homeTeamID, awayTeamID string }
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.matchID, &j.stageID, &j.homeTeamID, &j.awayTeamID); err != nil {
			log.Printf("RedCards: scan failed: %v", err)
			continue
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if len(jobs) == 0 {
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	processed := 0
	for _, j := range jobs {
		reds, yellows, err := fetchFifaCardCounts(client, j.stageID, j.matchID)
		if err != nil {
			log.Printf("Cards: fetch failed for match %s: %v", j.id, err)
			continue
		}
		if _, err := s.db.Exec(
			`UPDATE matches SET
				home_red_cards = ?, away_red_cards = ?,
				home_yellow_cards = ?, away_yellow_cards = ?,
				red_cards_synced = 1
			WHERE id = ?`,
			reds[j.homeTeamID], reds[j.awayTeamID],
			yellows[j.homeTeamID], yellows[j.awayTeamID],
			j.id,
		); err != nil {
			log.Printf("Cards: update failed for match %s: %v", j.id, err)
			continue
		}
		processed++
	}
	log.Printf("Cards: counted cards for %d/%d finished match(es)", processed, len(jobs))
}

func fetchFifaCardCounts(client *http.Client, stageID, matchID string) (reds, yellows map[string]int, err error) {
	url := fmt.Sprintf("https://api.fifa.com/api/v3/timelines/17/285023/%s/%s?language=en", stageID, matchID)
	resp, err := client.Get(url)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result fifaTimelineResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode response: %w", err)
	}

	reds = map[string]int{}
	yellows = map[string]int{}
	for _, e := range result.Event {
		if e.IdTeam == "" {
			continue
		}
		switch e.Type {
		case fifaEventRedCard:
			reds[e.IdTeam]++
		case fifaEventYellowCard:
			yellows[e.IdTeam]++
		}
	}
	return reds, yellows, nil
}

func fifaMatchStatus(status int) string {
	switch status {
	case 0:
		return "FINISHED"
	case 3:
		return "LIVE"
	case 1:
		return "UPCOMING"
	default:
		return "UPCOMING"
	}
}

func fifaStage(m fifaMatch) string {
	if group := localizedDescription(m.GroupName); group != "" {
		return group
	}
	return localizedDescription(m.StageName)
}

func localizedDescription(values []localizedValue) string {
	for _, value := range values {
		if value.Locale == "en-GB" && value.Description != "" {
			return value.Description
		}
	}
	for _, value := range values {
		if value.Description != "" {
			return value.Description
		}
	}
	return ""
}

var nonMatchIDChars = regexp.MustCompile(`[^A-Z0-9]+`)

var teamCodeAliases = map[string]string{
	"URU": "URY",
}

func normalizeTeamCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if alias, ok := teamCodeAliases[code]; ok {
		return alias
	}
	return code
}

func internalMatchID(matchDate, homeCode, awayCode string) string {
	t, err := time.Parse(time.RFC3339, matchDate)
	if err != nil {
		return fmt.Sprintf("%s_%s_%s", cleanMatchIDPart(matchDate), cleanMatchIDPart(homeCode), cleanMatchIDPart(awayCode))
	}
	return fmt.Sprintf("%s_%s_%s", t.UTC().Format("200601021504"), cleanMatchIDPart(homeCode), cleanMatchIDPart(awayCode))
}

func cleanMatchIDPart(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = nonMatchIDChars.ReplaceAllString(value, "")
	return value
}

func (s *Syncer) buildTeamMap() (map[string]teamInfo, error) {
	rows, err := s.db.Query("SELECT id, code, name FROM teams")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := map[string]teamInfo{}
	for rows.Next() {
		var id int64
		var code, name string
		if err := rows.Scan(&id, &code, &name); err != nil {
			return nil, err
		}
		m[normalizeTeamCode(code)] = teamInfo{ID: id, Name: name}
	}
	return m, nil
}

func (s *Syncer) previousMatchStatus(matchID string) (string, error) {
	var status string
	err := s.db.QueryRow("SELECT status FROM matches WHERE id = ?", matchID).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return status, err
}

func isLiveStatus(status string) bool {
	return normalizeMatchStatus(status) == "LIVE"
}

func isUpcomingStatus(status string) bool {
	return normalizeMatchStatus(status) == "UPCOMING"
}

func isFinishedStatus(status string) bool {
	return normalizeMatchStatus(status) == "FINISHED"
}

func normalizeStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func normalizeMatchStatus(status string) string {
	switch normalizeStatus(status) {
	case "UPCOMING", "SCHEDULED", "TIMED":
		return "UPCOMING"
	case "LIVE", "IN_PLAY", "PAUSED":
		return "LIVE"
	case "FINISHED", "AWARDED", "CANCELLED", "POSTPONED", "SUSPENDED":
		return "FINISHED"
	default:
		return "UPCOMING"
	}
}

func nextMatchStatus(previousStatus, incomingStatus string) string {
	incoming := normalizeMatchStatus(incomingStatus)
	if normalizeStatus(previousStatus) == "" {
		return incoming
	}
	previous := normalizeMatchStatus(previousStatus)
	if isFinishedStatus(previous) {
		return previous
	}
	if isLiveStatus(previous) {
		if isFinishedStatus(incoming) {
			return incoming
		}
		return previous
	}
	if isUpcomingStatus(previous) {
		if isUpcomingStatus(incoming) || isLiveStatus(incoming) || isFinishedStatus(incoming) {
			return incoming
		}
	}
	return previous
}

func (s *Syncer) notifyLeaderboardChanges() error {
	entries, err := ComputeLeaderboard(s.db)
	if err != nil {
		return err
	}

	var existingCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM leaderboard_state").Scan(&existingCount); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	initializing := existingCount == 0

	for _, entry := range entries {
		var oldRank int
		var oldPoints float64
		err := s.db.QueryRow("SELECT rank, points FROM leaderboard_state WHERE user_id = ?", entry.UserID).Scan(&oldRank, &oldPoints)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if !initializing && err == nil && oldRank != entry.Rank {
			s.push.NotifyLeaderboardChange(entry.UserID, oldRank, entry.Rank, entry.Points)
		}
		_, err = s.db.Exec(`
			INSERT INTO leaderboard_state (user_id, rank, points, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET
				rank = excluded.rank,
				points = excluded.points,
				updated_at = excluded.updated_at
		`, entry.UserID, entry.Rank, entry.Points, now)
		if err != nil {
			return err
		}
	}

	return nil
}
