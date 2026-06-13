package models

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Match struct {
	ID           string `json:"id"`
	HomeTeamID   int    `json:"homeTeamId"`
	HomeTeam     string `json:"homeTeam"`
	HomeTeamCode string `json:"homeTeamCode"`
	AwayTeamID   int    `json:"awayTeamId"`
	AwayTeam     string `json:"awayTeam"`
	AwayTeamCode string `json:"awayTeamCode"`
	HomeScore    *int   `json:"homeScore"`
	AwayScore    *int   `json:"awayScore"`
	HomeRedCards    int `json:"homeRedCards"`
	AwayRedCards    int `json:"awayRedCards"`
	HomeYellowCards int `json:"homeYellowCards"`
	AwayYellowCards int `json:"awayYellowCards"`
	Status       string `json:"status"`
	MatchDate    string `json:"matchDate"`
	Stage        string `json:"stage"`
}

type LeaderboardEntry struct {
	Rank   int      `json:"rank"`
	UserID int      `json:"userId"`
	Name   string   `json:"name"`
	Points float64  `json:"points"`
	Wins   int      `json:"wins"`
	Draws  int      `json:"draws"`
	Losses int      `json:"losses"`
	Teams  []string `json:"teams"`
}
