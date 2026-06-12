export interface Match {
  id: string
  homeTeam: string
  homeTeamCode: string
  homeTeamId: number
  awayTeam: string
  awayTeamCode: string
  awayTeamId: number
  homeScore: number | null
  awayScore: number | null
  status: string
  matchDate: string
  stage: string
}

export interface MatchesResponse {
  matches: Match[]
  teamOwners: Record<string, string>
}

export interface LeaderboardEntry {
  rank: number
  userId: number
  name: string
  points: number
  wins: number
  draws: number
  losses: number
  teams: string[]
}

export interface User {
  id: number
  name: string
}
