import { createResource, For, onCleanup, Show } from 'solid-js'
import type { LeaderboardEntry } from '../types'
import { getFlagClass } from '../flags'

async function fetchLeaderboard(): Promise<LeaderboardEntry[]> {
  const res = await fetch('/api/leaderboard')
  if (!res.ok) throw new Error('Failed to fetch leaderboard')
  return res.json()
}

const rankLabel = (rank: number) => {
  if (rank === 1) return '1st'
  if (rank === 2) return '2nd'
  if (rank === 3) return '3rd'
  return `${rank}th`
}

const rankIcon = (rank: number) => {
  if (rank === 1) return '🏆'
  if (rank === 2) return '🥈'
  if (rank === 3) return '🥉'
  return `#${rank}`
}

export default function Leaderboard() {
  const [entries, { refetch }] = createResource(fetchLeaderboard)

  const refreshTimer = window.setInterval(() => {
    refetch()
  }, 30_000)

  onCleanup(() => window.clearInterval(refreshTimer))

  return (
    <div>
      <Show when={entries.loading && !entries()}>
        <p class="loading">Loading leaderboard…</p>
      </Show>
      <Show when={entries.error}>
        <p class="error">Failed to load leaderboard. Is the backend running?</p>
      </Show>
      <Show when={entries()}>
        <div class="leaderboard-table">
          <For each={entries()}>
            {(entry) => (
              <div class={`leaderboard-row rank-${entry.rank}`}>
                <div class="rank-badge">
                  <span class="rank-icon">{rankIcon(entry.rank)}</span>
                  <span class="rank-num">{rankLabel(entry.rank)}</span>
                </div>
                <div class="user-info">
                  <div class="user-name">{entry.name}</div>
                  <div class="user-teams">
                    <For each={entry.teams}>
                      {(team) => {
                        const flagClass = getFlagClass(team)
                        return (
                          <span class="team-chip">
                            <Show when={flagClass}>
                              <span class={`${flagClass} chip-flag`} />
                            </Show>
                            {team}
                          </span>
                        )
                      }}
                    </For>
                  </div>
                </div>
                <div class="points-display">
                  <span class="points-value">{entry.points}</span>
                  <span class="points-label">Wins</span>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  )
}
