# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

~15 adults at the Garage Trip one-week house retreat, plus ~5 children (not assignable, but they add to the workload). Primary users are the adults themselves, picking up chores from personal phones as tasks appear throughout the day. Organisers (admins) also use the app to post, assign, and monitor from the same interface — there is no separate organiser-only tool.

Secondary display audience: everyone in the house via a shared TV dashboard running continuously during the trip.

## Product Purpose

Garage Trip Chores distributes household labour fairly across retreat attendees so no single person ends up carrying a disproportionate share of the work. It makes chores visible in real time, prompts the least-loaded eligible person, and tracks the week's effort so the result speaks for itself on the leaderboard.

Success at the end of the trip means two things, equally: the workload was balanced across attendees, and no critical/urgent task was missed or significantly delayed.

## Positioning

The app is purpose-built for a single recurring event (Garage Trip) and its specific crew. It is wired directly to the event's upstream chore API (`chores.garage-trip.cz`) over REST + WebSocket, and to Discord — the group's existing social infrastructure — for identity and role-based expertise. It replaces nothing off-the-shelf; it exists because the group already has the event management tooling and just needs the coordination layer.

## Operating Context

- **Duration:** one week, always-on, continuous use from multiple devices simultaneously.
- **Mobile-first claiming:** attendees self-assign from phones while going about their day in the house.
- **TV dashboard:** a single always-on display visible to everyone in the communal space. Serves as a shared scoreboard and ambient task prompt — people glance at it walking past.
- **Discord integration:** identity is Discord OAuth; roles carry skill/expertise signals (e.g. `hookah_master`, `barman`, `parenting`); the trip's paid role gates login.
- **Upstream API:** `chores.garage-trip.cz` is the source of truth for tasks, assignments, and workload stats. Our app reads via REST + WebSocket and layers local data on top (claims, templates, chore metadata, manual work, departures).
- **Children factor:** 5 children are present — not assignable, but they scale head-count chores (dishes, cooking, grilling).
- **Early departures:** some attendees may leave mid-week; they keep their leaderboard history but are excluded from new assignments.

## Capabilities and Constraints

- **Chore types (templates):** floor sweep (dry/wet), dishwasher load/unload, bin, grilling, kitchen sweep, shisha (water pipe) cleaning, cooking. Editable via the template manager.
- **Skill gates:** some chores require expertise (`cooking`, `grilling`, `hookah_master`). Skill comes from Discord roles + local profile overrides.
- **Workload balancing:** suggestions rank eligible people by lowest effective workload (upstream stats + local manual work + in-progress claims). Top 3 are highlighted/chimed.
- **Chore urgency:** urgent chores use a distinct colour and sound cue. A chore near its deadline auto-escalates to urgent.
- **Assignment:** attendees self-claim; organisers can auto-assign the best fit or manually assign from the suggestion list; either can remove someone from a chore.
- **Out-of-scope work:** attendees can log work not on the board (sauna, childcare) so it counts toward their load.
- **Single Fly.io instance:** the app holds an in-memory cache and a persistent WebSocket to the upstream; it must run as one always-on machine (cannot autoscale).
- **SQLite persistence (Phase 2):** local data (claims, metadata, templates, departures, profiles) lives in SQLite on a Fly volume. Phase 3 will migrate to a dedicated persistence API.
- **Auth:** Discord OAuth (paid-role gate); shared tablet password for the TV.
- **`AUTH_REQUIRED`:** when on, all pages/API/WS require a session.

## Brand Commitments

- **Name:** Garage Trip Chores (app); Garage Trip (event/brand).
- **Logo/wordmark:** "Garage Trip" typeset in JetBrains Mono 800 weight — drawn directly from garage-trip.cz.
- **Favicon:** the official garage-trip.cz favicon.ico.
- **Background:** garage-trip.cz dark palette (#1b1e22 / #22282f) with the site's SVG pattern overlay.
- **Accent colour:** brand purple #8430ce; Discord blue #5865f2 for login affordances.
- **Language:** English (the group's shared language for the event).

## Evidence on Hand

- Live upstream API at `chores.garage-trip.cz` (OpenAPI + AsyncAPI documented).
- Reference implementation for Discord OAuth + role-gating: `garage-trip-digitalizace-carek` (private repo, same org).
- garage-trip.cz for brand assets (favicon, pattern SVG, colour palette, JetBrains Mono wordmark).
- No marketing copy, testimonials, or press assets exist or should be fabricated.

## Product Principles

1. **Visible fairness.** Every design decision — from the suggestion algorithm to the leaderboard — should make the distribution of effort legible and trusted by the group.
2. **Low friction, high ambient awareness.** Claiming a chore must be one tap on a phone. The TV dashboard must communicate the board state without anyone having to look for it.
3. **The event, not the app.** The product recedes; the experience of the week should be memorable, not the interface. Personality lives in the funny ack messages and the chime, not in chrome.
4. **Respect real signals.** Workload, skills, and capacity come from real data (upstream stats, Discord roles, explicit opt-outs). The app never invents eligibility or effort.
5. **Resilience over perfectionism.** The upstream WebSocket will blip; the session may expire; people arrive late and leave early. Degrade gracefully and recover automatically.
