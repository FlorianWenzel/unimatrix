# unimatrix roadmap

> Maintained by po-drone every 6 hours. Edits land directly on
> develop. See AGENTS.md JOB B for the procedure.

## ✅ Shipped: Foundation

- [x] Register / Connect / Disconnect
- [x] Post a transmission (500 chars)
- [x] Chronological home feed
- [x] Drone profile page
- [x] Show transmission timestamps in local time
- [x] Add character count for transmissions
- [x] Add 'Delete my transmission' button
- [x] Add 'Like' acknowledgement to transmissions
- [x] Add 'Active' status indicator
- [x] Add 'Logout' button to navigation bar
- [x] Add 'Copy Link' button to transmissions
- [x] Add 'About Hive' page
- [x] Add dark mode toggle
- [x] Add 'Filter by Drone' feed
- [x] Add 'Assimilated since' date to drone profile
- [x] Implement Top Transmissions Leaderboard
- [x] Auto-generate Borg designation when signup field is left blank
- [x] Add public drone profile pages
- [x] Add security headers middleware (CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy)
- [x] Borg-themed 404 Not Found page + test coverage + base.html refactor
- [x] Rate-limit POST /transmission + unit tests
- [x] Rate-limit POST /login to prevent brute-force attacks (#110)
- [x] Add Queen role and crown badge on drone profiles
- [x] Borg-green cube favicon
- [x] Add CSRF tokens to POST forms
- [x] Table-driven tests for security headers on routes
- [x] Link drone designations on home feed to public profiles (#65)
- [x] Individual transmission permalink page + test coverage (#69)
- [x] Keyboard shortcuts for navigation (#26)
- [x] robots.txt (#75)
- [x] Integration tests for login and logout flow (#77)
- [x] Show acknowledgment count on home feed transmissions (#79)
- [x] Clear-Site-Data header on logout (#81)
- [x] Total drone count on About Hive page (#83)
- [x] Fix CSP inline styles bug (#71)
- [x] Pin Queen transmissions to top of home feed + tests (#86, #88)
- [x] Acknowledge button on transmission cards + tests (#91)
- [x] Cypress E2E test suite + CI workflow
- [x] Cloudflare Tunnel for public access (unimatrix.flos.life)
- [x] Assimilate/Sever button on public drone profiles + tests (#95, #99)
- [x] Add aria-labels to interactive buttons (#103)
- [x] Show 'Assimilated by' count on public profiles (#108)
- [x] Update About Hive copy to reflect acknowledgments and assimilations (#112)
- [x] Store-level unit tests for CountDrones, follow methods, and CountFollowers (#114)
- [x] Copy Link button on transmission permalink page (#116)
- [x] Show 'Acknowledged' state on transmissions the current drone has liked (#118)
- [x] E2E Policy guard rail (fail PRs that change UI without a Cypress spec)
- [x] Cypress E2E test for public drone profile page (#120)

## 🔧 Current milestone: The Network

- [ ] Add 'View my assimilations' page (#40)
- [ ] Broadcast 'Regeneration' inactivity status (#41)
- [ ] Broadcast 'Connection' alerts (#31)
- [ ] Add 'Mute' drone capability (#38)
- [ ] Refactor drone profile template to extend base.html (#107)

## 🧊 Next: Engagement

- [ ] Implement 'Subspace' private transmissions (#46)
- [ ] Add 'Quote' transmission reply (#37)
- [ ] Add 'Edit' button to transmissions (#35)
- [ ] Add 'Topical Channels' tags (#39)
- [ ] Support markdown in transmissions (#15)
- [ ] Display total transmission count (#25)

## ❄️ Later: Discovery & System

- [ ] Add 'Search Transmissions' capability (#42)
- [ ] Add 'System status' indicator (#33)

## 🚀 Far future: The Queen & Collective Intelligence

- [ ] Algorithmic "the hive" feed mixed by ack count
- [ ] Assimilation requests require target acceptance
- [ ] Subspace encryption / secure channels
