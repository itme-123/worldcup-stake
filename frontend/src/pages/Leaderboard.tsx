import { createMemo, createResource, createSignal, For, onCleanup, Show } from 'solid-js'
import type { LeaderboardEntry, Match, MatchesResponse } from '../types'
import { getFlagClass } from '../flags'

async function fetchLeaderboard(): Promise<LeaderboardEntry[]> {
  const res = await fetch('/api/leaderboard')
  if (!res.ok) throw new Error('Failed to fetch leaderboard')
  return res.json()
}

async function fetchMatches(): Promise<MatchesResponse> {
  const res = await fetch('/api/matches')
  if (!res.ok) throw new Error('Failed to fetch matches')
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

const isGroupStage = (m: Match) => m.stage.startsWith('Group')

const earliest = (ms: Match[]) => Math.min(...ms.map((m) => Date.parse(m.matchDate)))

// A team is out of the running when it loses a decided knockout match, or when
// the next knockout round is fully known and the team isn't in it. Teams in the
// third-place play-off are already eliminated from winning the cup.
function computeEliminated(matches: Match[], allCodes: string[]): Set<string> {
  const eliminated = new Set<string>()
  const knockouts = matches.filter((m) => !isGroupStage(m))
  if (knockouts.length === 0) return eliminated

  const byStage = new Map<string, Match[]>()
  for (const m of knockouts) {
    byStage.set(m.stage, [...(byStage.get(m.stage) ?? []), m])
  }
  let rounds = [...byStage.values()].sort((a, b) => earliest(a) - earliest(b))
  // Drop the third-place play-off: a single-match round that isn't the final
  rounds = rounds.filter((ms, i) => !(ms.length === 1 && i < rounds.length - 1))

  const participants = (ms: Match[]) =>
    new Set(ms.flatMap((m) => [m.homeTeamCode, m.awayTeamCode]).filter(Boolean))
  const fullyKnown = (ms: Match[]) => ms.every((m) => m.homeTeamCode && m.awayTeamCode)

  let prev = new Set(allCodes)
  for (const ms of rounds) {
    if (!fullyKnown(ms)) break
    const current = participants(ms)
    for (const code of prev) if (!current.has(code)) eliminated.add(code)
    prev = current
  }

  for (const ms of rounds) {
    for (const m of ms) {
      if (m.status !== 'FINISHED' || m.homeScore == null || m.awayScore == null) continue
      if (m.homeScore > m.awayScore) eliminated.add(m.awayTeamCode)
      else if (m.awayScore > m.homeScore) eliminated.add(m.homeTeamCode)
    }
  }
  return eliminated
}

interface PlayerStats {
  goalsFor: number
  goalsAgainst: number
  matchGoals: number
  redCards: number
  yellowCards: number
  cleanSheets: number
  streak: number
}

function computeStats(matches: Match[], owners: Record<string, string>) {
  const stats = new Map<string, PlayerStats>()
  const results = new Map<string, boolean[]>()
  const get = (p: string) => {
    if (!stats.has(p))
      stats.set(p, {
        goalsFor: 0,
        goalsAgainst: 0,
        matchGoals: 0,
        redCards: 0,
        yellowCards: 0,
        cleanSheets: 0,
        streak: 0,
      })
    return stats.get(p)!
  }
  const finished = matches
    .filter((m) => m.status === 'FINISHED' && m.homeScore != null && m.awayScore != null)
    .sort((a, b) => Date.parse(a.matchDate) - Date.parse(b.matchDate))
  for (const m of finished) {
    const sides = [
      { code: m.homeTeamCode, gf: m.homeScore!, ga: m.awayScore!, reds: m.homeRedCards ?? 0, yellows: m.homeYellowCards ?? 0 },
      { code: m.awayTeamCode, gf: m.awayScore!, ga: m.homeScore!, reds: m.awayRedCards ?? 0, yellows: m.awayYellowCards ?? 0 },
    ]
    for (const s of sides) {
      const owner = owners[s.code]
      if (!owner) continue
      const st = get(owner)
      st.goalsFor += s.gf
      st.goalsAgainst += s.ga
      st.matchGoals += s.gf + s.ga
      st.redCards += s.reds
      st.yellowCards += s.yellows
      if (s.ga === 0) st.cleanSheets += 1
      results.set(owner, [...(results.get(owner) ?? []), s.gf > s.ga])
    }
  }
  for (const [p, wins] of results) {
    let streak = 0
    for (let i = wins.length - 1; i >= 0 && wins[i]; i--) streak++
    get(p).streak = streak
  }
  return stats
}

interface Badge {
  icon: string
  label: string
  detail: string
}

const SOLE_LEADER_HINT = 'Needs a sole leader — nobody is outright ahead yet.'

const BADGE_TYPES = [
  { icon: '🥇', label: 'Golden Boots', desc: 'Most goals scored by their teams.', unclaimedHint: SOLE_LEADER_HINT },
  { icon: '🧱', label: 'The Wall', desc: 'Most clean sheets (matches their teams conceded zero).', unclaimedHint: SOLE_LEADER_HINT },
  { icon: '🍿', label: 'The Entertainer', desc: "Most goals seen across their teams' matches, win or lose.", unclaimedHint: SOLE_LEADER_HINT },
  { icon: '🔥', label: 'On Fire', desc: 'Three or more wins in a row across their teams.', unclaimedHint: 'No one is on a 3-win streak yet.' },
  { icon: '🛡️', label: 'Unbeaten', desc: "None of their teams has lost yet (and they've played).", unclaimedHint: 'Everyone has tasted defeat — nobody is unbeaten.' },
  { icon: '🕳️', label: 'The Sieve', desc: 'Most goals conceded by their teams.', unclaimedHint: SOLE_LEADER_HINT },
  { icon: '🟨', label: 'Card Magnet', desc: 'Most yellow cards collected by their teams.', unclaimedHint: SOLE_LEADER_HINT },
  { icon: '🟥', label: 'Hot Heads', desc: 'Most red cards collected by their teams.', unclaimedHint: SOLE_LEADER_HINT },
  { icon: '🥄', label: 'Wooden Spoon', desc: 'Outright last on the Win Counter.', unclaimedHint: "Nobody is outright last — it's still tied at the bottom." },
]

function computeBadges(entries: LeaderboardEntry[], stats: Map<string, PlayerStats>) {
  const badges = new Map<string, Badge[]>()
  const add = (player: string, badge: Badge) => {
    badges.set(player, [...(badges.get(player) ?? []), badge])
  }
  // Only awarded to a sole leader — no badge while tied
  const soleLeader = (metric: (st: PlayerStats) => number) => {
    let best: string | null = null
    let bestVal = 0
    let tied = false
    for (const [p, st] of stats) {
      const v = metric(st)
      if (v > bestVal) {
        best = p
        bestVal = v
        tied = false
      } else if (v === bestVal && v > 0) {
        tied = true
      }
    }
    return best && bestVal > 0 && !tied ? { player: best, value: bestVal } : null
  }

  const boots = soleLeader((st) => st.goalsFor)
  if (boots)
    add(boots.player, {
      icon: '🥇',
      label: 'Golden Boots',
      detail: `Their teams have scored the most goals (${boots.value})`,
    })

  const sieve = soleLeader((st) => st.goalsAgainst)
  if (sieve)
    add(sieve.player, {
      icon: '🕳️',
      label: 'The Sieve',
      detail: `Their teams have conceded the most goals (${sieve.value})`,
    })

  const entertainer = soleLeader((st) => st.matchGoals)
  if (entertainer)
    add(entertainer.player, {
      icon: '🍿',
      label: 'The Entertainer',
      detail: `Most goals seen across their teams' matches (${entertainer.value})`,
    })

  for (const [p, st] of stats) {
    if (st.streak >= 3)
      add(p, { icon: '🔥', label: 'On Fire', detail: `${st.streak} wins in a row` })
  }

  const hotHeads = soleLeader((st) => st.redCards)
  if (hotHeads)
    add(hotHeads.player, {
      icon: '🟥',
      label: 'Hot Heads',
      detail: `Their teams have collected the most red cards (${hotHeads.value})`,
    })

  const cardMagnet = soleLeader((st) => st.yellowCards)
  if (cardMagnet)
    add(cardMagnet.player, {
      icon: '🟨',
      label: 'Card Magnet',
      detail: `Their teams have collected the most yellow cards (${cardMagnet.value})`,
    })

  const wall = soleLeader((st) => st.cleanSheets)
  if (wall)
    add(wall.player, {
      icon: '🧱',
      label: 'The Wall',
      detail: `Most clean sheets across their teams (${wall.value})`,
    })

  // Status badge — awarded to everyone who qualifies, not just a sole leader.
  for (const e of entries) {
    if (e.losses === 0 && e.wins + e.draws > 0)
      add(e.name, {
        icon: '🛡️',
        label: 'Unbeaten',
        detail: `None of their teams has lost yet (${e.wins}W ${e.draws}D)`,
      })
  }

  if (entries.length >= 2) {
    const last = entries[entries.length - 1]
    const secondLast = entries[entries.length - 2]
    const soloLast =
      last.wins < secondLast.wins ||
      (last.wins === secondLast.wins && last.draws < secondLast.draws)
    if (soloLast)
      add(last.name, { icon: '🥄', label: 'Wooden Spoon', detail: 'Dead last. Someone has to be.' })
  }

  return badges
}

export default function Leaderboard() {
  const [entries, { refetch }] = createResource(fetchLeaderboard)
  const [matchData, { refetch: refetchMatches }] = createResource(fetchMatches)
  const [selectedBadge, setSelectedBadge] = createSignal<string | null>(null)

  const refreshTimer = window.setInterval(() => {
    refetch()
    refetchMatches()
  }, 30_000)

  onCleanup(() => window.clearInterval(refreshTimer))

  const nameToCode = createMemo(() => {
    const map: Record<string, string> = {}
    for (const m of matchData()?.matches ?? []) {
      if (m.homeTeamCode) map[m.homeTeam] = m.homeTeamCode
      if (m.awayTeamCode) map[m.awayTeam] = m.awayTeamCode
    }
    return map
  })

  const eliminated = createMemo(() => {
    const data = matchData()
    if (!data) return new Set<string>()
    return computeEliminated(data.matches, Object.keys(data.teamOwners))
  })

  const badges = createMemo(() => {
    const data = matchData()
    const list = entries()
    if (!data || !list) return new Map<string, Badge[]>()
    return computeBadges(list, computeStats(data.matches, data.teamOwners))
  })

  const ownedCodes = createMemo(() => {
    const map = new Map<string, string[]>()
    const owners = matchData()?.teamOwners ?? {}
    for (const [code, owner] of Object.entries(owners)) {
      map.set(owner, [...(map.get(owner) ?? []), code])
    }
    return map
  })

  const aliveCount = (player: string) => {
    const owned = ownedCodes().get(player)
    if (!owned) return null
    const out = eliminated()
    return { alive: owned.filter((c) => !out.has(c)).length, total: owned.length }
  }

  return (
    <div>
      <Show when={entries.loading && !entries()}>
        <p class="loading">Loading leaderboard…</p>
      </Show>
      <Show when={entries.error}>
        <p class="error">Failed to load leaderboard. Is the backend running?</p>
      </Show>
      <Show when={entries()}>
        <div class="badge-intro">
          <span class="badge-intro-title">🏅 Honours &amp; Badges</span>
          <span class="badge-intro-sub">Tap a badge to see who holds it</span>
        </div>
        <div class="badge-strip">
          <For each={BADGE_TYPES}>
            {(bt) => {
              const holders = () =>
                [...badges().entries()]
                  .filter(([, list]) => list.some((b) => b.label === bt.label))
                  .map(([player, list]) => ({
                    name: player,
                    detail: list.find((b) => b.label === bt.label)!.detail,
                  }))
              const hoverText = () =>
                `${bt.desc} ${holders().length ? 'Held by ' + holders().map((h) => h.name).join(', ') + '.' : 'Unclaimed right now — ' + bt.unclaimedHint}`
              return (
                <button
                  class={`badge-strip-chip ${selectedBadge() === bt.label ? 'active' : ''}`}
                  title={hoverText()}
                  onClick={() =>
                    setSelectedBadge(selectedBadge() === bt.label ? null : bt.label)
                  }
                >
                  {bt.icon} {bt.label}
                </button>
              )
            }}
          </For>
        </div>
        <Show when={BADGE_TYPES.find((bt) => bt.label === selectedBadge())}>
          {(bt) => {
            const holders = () =>
              [...badges().entries()]
                .filter(([, list]) => list.some((b) => b.label === bt().label))
                .map(([player, list]) => ({
                  name: player,
                  detail: list.find((b) => b.label === bt().label)!.detail,
                }))
            return (
              <div class="badge-info-panel">
                <div class="badge-info-title">{bt().icon} {bt().label}</div>
                <div class="badge-info-desc">{bt().desc}</div>
                <div class="badge-info-holder">
                  <Show
                    when={holders().length > 0}
                    fallback={<span>Unclaimed right now — {bt().unclaimedHint}</span>}
                  >
                    <For each={holders()}>
                      {(h) => (
                        <span class="badge-info-holder-name">
                          👑 {h.name} — {h.detail}
                        </span>
                      )}
                    </For>
                  </Show>
                </div>
              </div>
            )
          }}
        </Show>
        <div class="leaderboard-table">
          <For each={entries()}>
            {(entry) => {
              const alive = () => aliveCount(entry.name)
              const isHighlighted = () =>
                selectedBadge() != null &&
                (badges().get(entry.name) ?? []).some((b) => b.label === selectedBadge())
              return (
                <div
                  class={`leaderboard-row rank-${entry.rank} ${isHighlighted() ? 'badge-holder' : ''}`}
                >
                  <div class="rank-badge">
                    <span class="rank-icon">{rankIcon(entry.rank)}</span>
                    <span class="rank-num">{rankLabel(entry.rank)}</span>
                  </div>
                  <div class="user-info">
                    <div class="user-name">
                      {entry.name}
                      <For each={badges().get(entry.name) ?? []}>
                        {(badge) => (
                          <span class="badge-chip" title={badge.detail}>
                            {badge.icon} {badge.label}
                          </span>
                        )}
                      </For>
                    </div>
                    <Show when={alive()}>
                      <div class={`alive-line ${alive()!.alive === 0 ? 'dead' : ''}`}>
                        {alive()!.alive === 0
                          ? '💀 No teams left'
                          : `${alive()!.alive}/${alive()!.total} teams still alive`}
                      </div>
                    </Show>
                    <div class="user-teams">
                      <For each={entry.teams}>
                        {(team) => {
                          const flagClass = getFlagClass(team)
                          const code = () => nameToCode()[team]
                          const isOut = () => {
                            const c = code()
                            return c ? eliminated().has(c) : false
                          }
                          return (
                            <span class={`team-chip ${isOut() ? 'eliminated' : ''}`}>
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
                    <span class="points-value">{entry.wins}</span>
                    <span class="points-label">Wins</span>
                    <span class="points-record">{entry.draws}D · {entry.losses}L</span>
                  </div>
                </div>
              )
            }}
          </For>
        </div>
      </Show>
    </div>
  )
}
