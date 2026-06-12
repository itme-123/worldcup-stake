package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"worldcup-stake/models"
)

func GetLeaderboard(database *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		entries, err := ComputeLeaderboard(database)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, entries)
	}
}

func ComputeLeaderboard(database *sql.DB) ([]models.LeaderboardEntry, error) {
	// Each owned team's result counts separately: owning both sides of a match
	// yields one win and one loss (or two draws). Ranking is wins, then draws
	// as the tiebreaker, then fewest losses.
	rows, err := database.Query(`
		SELECT
			u.id,
			u.name,
			COALESCE(SUM(CASE
				WHEN m.home_team_id = ut.team_id AND m.home_score > m.away_score THEN 1
				WHEN m.away_team_id = ut.team_id AND m.away_score > m.home_score THEN 1
				ELSE 0
			END), 0) AS wins,
			COALESCE(SUM(CASE
				WHEN m.id IS NOT NULL AND m.home_score = m.away_score THEN 1
				ELSE 0
			END), 0) AS draws,
			COALESCE(SUM(CASE
				WHEN m.home_team_id = ut.team_id AND m.home_score < m.away_score THEN 1
				WHEN m.away_team_id = ut.team_id AND m.away_score < m.home_score THEN 1
				ELSE 0
			END), 0) AS losses
		FROM users u
		LEFT JOIN user_teams ut ON ut.user_id = u.id
		LEFT JOIN matches m
			ON (m.home_team_id = ut.team_id OR m.away_team_id = ut.team_id)
			AND m.status = 'FINISHED'
			AND m.home_score IS NOT NULL
			AND m.away_score IS NOT NULL
		GROUP BY u.id, u.name
		ORDER BY wins DESC, draws DESC, losses ASC, u.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	for rows.Next() {
		var e models.LeaderboardEntry
		if err := rows.Scan(&e.UserID, &e.Name, &e.Wins, &e.Draws, &e.Losses); err != nil {
			return nil, err
		}
		e.Points = float64(e.Wins)
		entries = append(entries, e)
	}

	teamRows, err := database.Query(`
		SELECT ut.user_id, t.name
		FROM user_teams ut
		JOIN teams t ON t.id = ut.team_id
		ORDER BY ut.user_id, t.name
	`)
	if err != nil {
		return nil, err
	}
	defer teamRows.Close()

	userTeams := map[int][]string{}
	for teamRows.Next() {
		var userID int
		var teamName string
		if err := teamRows.Scan(&userID, &teamName); err != nil {
			continue
		}
		userTeams[userID] = append(userTeams[userID], teamName)
	}

	for i := range entries {
		entries[i].Rank = i + 1
		entries[i].Teams = userTeams[entries[i].UserID]
		if entries[i].Teams == nil {
			entries[i].Teams = []string{}
		}
	}

	if entries == nil {
		entries = []models.LeaderboardEntry{}
	}

	return entries, nil
}
