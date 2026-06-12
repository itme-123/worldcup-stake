import { A } from '@solidjs/router'

export default function Nav() {
  return (
    <nav>
      <A href="/" class="nav-logo" activeClass="">
        <span>⚽</span>
        <span>Marko's <span class="logo-accent">Footy Sweepstakes</span></span>
        <span class="logo-badge">LIVE</span>
      </A>
      <A href="/" activeClass="active" end>Matches</A>
      <A href="/leaderboard" activeClass="active">Win Counter Leaderboard</A>
    </nav>
  )
}
