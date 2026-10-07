---
name: Garage Trip Chores
description: A dark-mode crew coordination tool for shared house chores during the annual Garage Trip retreat.
colors:
  # Brand / primary action
  purple:          "#8430ce"
  purple-hover:    "#6a26a5"
  purple-light:    "#b48cff"   # text/link/focus purple on dark (contrast-safe, 6.5:1)
  discord-blue:    "#5865f2"
  # Page surfaces (darkest → lightest)
  bg-deep:         "#1b1e22"
  bg:              "#22282f"
  panel:           "#2a313a"
  panel-raised:    "#333b45"
  input-well:      "#0e1626"
  white:           "#ffffff"   # on-accent text (buttons, filled badges) + wordmark
  # Borders / hairlines (dark → light)
  border-card:     "#28324c"   # card outline
  border-nav:      "#26304a"   # sticky nav bottom border
  border-line:     "#38425f"   # inputs, ghost/secondary buttons, chips
  border-badge:    "#333f5c"   # badge hairline
  bar-track:       "#1b2436"   # unfilled workload/effort bar + TV list dividers
  # Text
  text:            "#e8edf7"
  muted:           "#9aa6b5"
  # Semantic signals
  ok:              "#35c48a"
  warn:            "#ffc107"
  urgent:          "#ff4d5e"
  # Status-badge tints
  size-small:      "#9fe6c4"
  size-medium:     "#ffd98a"
  size-large:      "#ff9c9c"
  skill:           "#c9b8ff"
  tv-on-it:        "#4ea1ff"
typography:
  display:
    fontFamily:   '"JetBrains Mono", monospace'
    fontSize:     "2.4rem"
    fontWeight:   800
    lineHeight:   1.1
    letterSpacing: "normal"
  headline:
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "2rem"
    fontWeight:   700
    lineHeight:   1.2
  title:
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1.5rem"
    fontWeight:   700
    lineHeight:   1.3
  body:
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1rem"
    fontWeight:   400
    lineHeight:   1.5
  label:
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "0.74rem"
    fontWeight:   700
    letterSpacing: "0.02em"
  # --- secondary steps actually used across the UI (documented so they're not drift) ---
  tv-accent:      # TV clock + "On it" readout, read across a room
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1.6rem"
    fontWeight:   700
  stat-strong:    # leaderboard cell figures (desktop)
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1.25rem"
    fontWeight:   700
  stat:           # leaderboard rows / dashboard list rows
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1.2rem"
    fontWeight:   700
  card-title:     # chore-card h3 and section h2
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1.15rem"
    fontWeight:   700
  brand:          # "Garage Trip Chores" nav wordmark
    fontFamily:   '"JetBrains Mono", monospace'
    fontSize:     "1.1rem"
    fontWeight:   800
  stat-compact:   # leaderboard figures on small screens
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "1.05rem"
    fontWeight:   700
  meta:           # small buttons, claimer line, helper text
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "0.85rem"
    fontWeight:   600
  micro:          # sort arrows, connection status
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "0.8rem"
    fontWeight:   700
  micro-alt:      # sort-arrow glyph
    fontFamily:   'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    fontSize:     "0.9rem"
    fontWeight:   700
rounded:
  pill:    "999px"
  card:    "14px"
  button:  "10px"
  input:   "10px"
spacing:
  xs:          "6px"
  sm:          "12px"
  md:          "16px"
  lg:          "24px"
  xl:          "32px"
  content-max: "760px"
components:
  button-primary:
    backgroundColor: "{colors.purple}"
    textColor:       "{colors.text}"
    rounded:         "{rounded.button}"
    padding:         "10px 16px"
    typography:      "700"
  button-primary-hover:
    backgroundColor: "{colors.purple-hover}"
    textColor:       "{colors.text}"
    rounded:         "{rounded.button}"
    padding:         "10px 16px"
  button-secondary:
    backgroundColor: "{colors.panel-raised}"
    textColor:       "{colors.text}"
    rounded:         "{rounded.button}"
    padding:         "10px 16px"
  button-ghost:
    backgroundColor: "transparent"
    textColor:       "{colors.muted}"
    rounded:         "{rounded.button}"
    padding:         "10px 16px"
  button-danger:
    backgroundColor: "{colors.urgent}"
    textColor:       "#ffffff"
    rounded:         "{rounded.button}"
    padding:         "10px 16px"
  chip-unselected:
    backgroundColor: "{colors.input-well}"
    textColor:       "{colors.muted}"
    rounded:         "{rounded.pill}"
    padding:         "8px 12px"
  chip-selected:
    backgroundColor: "{colors.purple}"
    textColor:       "#ffffff"
    rounded:         "{rounded.pill}"
    padding:         "8px 12px"
  card:
    backgroundColor: "{colors.panel}"
    rounded:         "{rounded.card}"
    padding:         "16px"
---

# Design System: Garage Trip Chores

## Overview

**Creative North Star: "The Garage Dashboard"**

This is a tool built for a crew that already trusts each other — not a polished consumer product, but a working instrument. The aesthetic is technical and direct: deep near-black surfaces, a single high-saturation purple that cuts through the dark, a monospace wordmark that signals this is something built by hackers for hackers. Density is medium; every screen earns its space but doesn't waste it. The mood is a control room at the start of a day-shift: purposeful, readable from a distance, always-on.

The visual world is derived directly from garage-trip.cz — the brand's own palette, its SVG pattern overlay, and the JetBrains Mono wordmark are exact lifts, not interpretations. The app is a tool surface within an established event identity, not a standalone brand. This keeps the design constraint tight and the result unmistakably *Garage Trip*.

Animation is used sparingly and always semantically: the pulsing glow on a suggested-for-you chore, the spring toast that pops from the bottom, the subtle inset glow on urgent cards. Everything else is static.

**Key Characteristics:**
- Always dark; no light-mode surfaces anywhere
- Bold, high-contrast typography — system font for body, JetBrains Mono for brand moments only
- Single accent color (brand purple `#8430ce`) as the primary call to action and selection state
- Status expressed through color, not shape — urgency is red, success is green, suggestion is purple
- Cards with a decisive left-border accent line as the primary structural motif for chore items
- Pills (999px radius) for all tags, badges, chips, and toasts; moderate radius (14px) for structural containers


## Colors

A near-black stage lit by a single purple beam, with semantic signal colors that read instantly across the room — from a phone screen or a TV ten feet away.

### Primary
- **Garage Purple** (`#8430ce`): The single brand accent. Used on primary buttons, active nav underlines, chip selection, the suggestion-pulse glow, and the progress gradient. Its rarity on any given screen is its power.
- **Purple Hover** (`#6a26a5`): Pressed/hover state for purple surfaces. Darker, no hue shift.
- **Light Purple** (`#b48cff`): The *text* form of the brand purple — links, and the keyboard focus ring. The fill purple (`#8430ce`) fails contrast as small text on dark (2.06:1); this lightened purple reads at 6.5:1 while staying the same brand hue. Fills use `#8430ce`; text/links/focus use `#b48cff`.
- **Discord Blue** (`#5865f2`): Reserved exclusively for the "Log in with Discord" affordance. Never used for general UI actions.

### Neutral
- **Deepest Surface** (`#1b1e22`): Page background, body `background-color`. The floor everything sits on. Also the **bar track** (`#1b2436`, a hair bluer) for the unfilled portion of workload/effort bars and TV list dividers.
- **White** (`#ffffff`): On-accent text — white on filled buttons and badges, and the JetBrains Mono wordmark. Never a background.
- **Border hairlines:** **Card** (`#28324c`, card outlines), **Nav** (`#26304a`, sticky-nav underline), **Line** (`#38425f`, inputs / ghost & secondary buttons / chips), **Badge** (`#333f5c`). All near-invisible — they define an edge without adding noise.
- **Stage** (`#22282f`): Used in body gradient layers and the pattern overlay. One step above the floor.
- **Panel** (`#2a313a`): Card backgrounds — the default surface for all content containers.
- **Raised Panel** (`#333b45`): Secondary buttons, chip unselected backgrounds, table row hover. One elevation above Panel.
- **Input Well** (`#0e1626`): Text inputs, selects, unselected chip backgrounds. Recessed below the page surface to signal interaction affordance.
- **Text** (`#e8edf7`): Primary readable text. Cool-white with a slight blue cast that reads cleanly on dark surfaces.
- **Muted** (`#9aa6b5`): Secondary labels, helper text, nav link defaults, disabled states. Never used for anything the user needs to action.

### Semantic Signals
- **Task Green** (`#35c48a`): Claimed/done states; success toasts; workload-bar fill start. Visually calm — things are handled.
- **Warning Amber** (`#ffc107`): Reserved for future warning states (not yet actively used in UI).
- **Urgent Red** (`#ff4d5e`): Urgent chore borders, inset glows, danger buttons, connection-offline indicator, and the assignee remove button hover. The alarm signal. Used on ≤5% of any screen unless the board is genuinely on fire.

### Status Tints (badge text only)
- **Mint** (`#9fe6c4`): Small chore size label.
- **Amber** (`#ffd98a`): Medium chore size label.
- **Coral** (`#ff9c9c`): Large chore size label.
- **Lavender** (`#c9b8ff`): Skill-requirement badge text.
- **TV Blue** (`#4ea1ff`): "On it:" assignee readout on the TV dashboard only — bright enough to read at room distance.

### Named Rules
**The One Accent Rule.** Garage Purple (`#8430ce`) is the only non-semantic saturated hue in the interface. Every other use of color carries a status meaning (urgent = red, ok = green, suggestion = purple glow). Introducing a second decorative accent is prohibited.

**The Night Rule.** Every background value must be darker than `#22282f`. No surface, modal, panel, or card may use a light background value, regardless of context.


## Typography

**Display Font:** JetBrains Mono (weight 800, optical sizing on) — used exclusively for the "Garage Trip" wordmark wherever it appears.
**Body Font:** system-ui stack (`system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`) — all body copy, headings, labels, and UI text.

**Character:** The pairing is deliberate and minimal. JetBrains Mono at 800 weight carries the brand identity in a single word; the system stack everywhere else keeps render performance high and cognitive weight low. This is a tool, not a publication.

### Hierarchy
- **Display** (JetBrains Mono 800, 2.4rem, lh 1.1): TV dashboard `h1` and the brand wordmark. The only place JetBrains Mono appears. Never used for body or label text.
- **Headline** (system 700, 2rem, lh 1.2): Leaderboard page title. Rare, high-information-density contexts.
- **Title** (system 700, 1.5rem, lh 1.3): Page `h1` headings within the 760px content wrap. First label on any screen.
- **Body** (system 400, 1rem, lh 1.5): All prose, card descriptions, list items, nav links. Max readable line length is naturally constrained by the 760px container.
- **Label** (system 700, 0.74rem, lh 1, ls 0.02em): Badge text, `th` in tables, connection status indicator. Uppercase tracking applies to table headers only.

### Named Rules
**The Mono Gatekeeping Rule.** JetBrains Mono is the brand's single typographic signal. It appears in the wordmark (`.logo`) and the TV heading. Adding it to any other context — labels, badges, code snippets, timestamps — dilutes the signal to zero.


## Layout

Single-column content, constrained to 760px at desktop, edge-to-edge on mobile. The `<main class="wrap">` container handles this with `max-width: 760px; margin: 0 auto; padding: 18px`. The TV dashboard breaks out of this container into a full-width `tv-wrap` with 24px/32px padding.

**Breakpoints observed:**
- `520px`: 2-column form grids collapse to single column.
- `900px`: TV dashboard 2-column grid (chores 2fr + leaderboard 1fr) collapses to single column; chore grid collapses from 2 columns to 1.

**Sticky navigation:** 12px 18px padding, `rgba(15,20,32,.8)` background with `backdrop-filter: blur(8px)`. Behaves as a frosted glass layer — always visible, never blocking.

**Rhythm:** Vertical spacing between cards is 14px (hardcoded `margin-bottom: 14px` on `.card`). Internal card padding is 16px. The grid between form fields uses a 12px gap. The TV chore grid uses a 16px gap; the outer TV grid uses 24px.


## Elevation & Depth

Depth is expressed through a four-layer tonal stack (input-well → bg-deep → panel → panel-raised) combined with a single shadow vocabulary. There is no flat-only philosophy; structural containers use a meaningful shadow to lift content off the page.

### Shadow Vocabulary
- **Standard lift** (`0 6px 24px rgba(0,0,0,.45)`): Applied to all `.card` elements. Moderate vertical offset, wide spread, high opacity — reads on the very dark background.
- **Urgent inset** (`0 0 0 1px var(--urgent) inset, [standard lift]`): Chore cards in urgent state. The inset ring is additive; the base shadow persists underneath.
- **Suggestion pulse** (keyframe `0 0 0 1px #8430ce inset` → `0 0 0 3px #8430ce inset, 0 0 18px rgba(124,92,255,.5)`): Animated glow on chore cards suggested for the current user. The only animated shadow in the system.

### Named Rules
**The Tonal Ladder Rule.** Surfaces progress from deepest (input-well `#0e1626`) to bg-deep → panel → panel-raised. Never reverse this order — a container must never be lighter than its parent context. Violations flatten the depth model.


## Shapes

Corners are soft and fast-reading. The system uses exactly two radii for structural elements — 14px for containers, 10px for controls — plus a universal 999px pill for all tags, indicators, and decorative chips.

**Card / container corners:** Gently rounded (14px, `--radius`). Distinct enough to feel considered; conservative enough to not distract.

**Button / input corners:** Slightly tighter (10px). Controls feel firm and clickable. Buttons and inputs share the same radius so inline form groups read as a cohesive row.

**Pills (999px):** Badges, chips, skill tags, connection-status indicators, toast notifications, the progress bar. The full-round silhouette creates a visual "bubble" language for metadata and status — clearly distinct from interactive buttons (10px) and structural cards (14px).

**Chore card left-border:** A 6px left accent line is the primary structural signature. Color encodes state: accent purple (default), urgent red (urgent chore), suggest purple with glow (user-suggested). This line is the fastest reading signal on the board — visible before the text renders.

**Named Rules**
**The Three-Radius Rule.** The system uses exactly three border-radius values: 14px (containers), 10px (controls), 999px (pills). Introducing intermediate values for one-off components fragments the form language.


## Components

### Buttons
Bold and decisive — heavy font weight (700), no border on primary, high-contrast fills. Actions feel like pressing a physical button.
- **Shape:** 10px radius; padding 10px 16px (small variant: 6px 10px)
- **Primary:** Garage Purple fill (`#8430ce`), white text. Hover darkens to `#6a26a5`. The only button that should draw the eye.
- **Secondary:** Raised Panel fill (`#333b45`), primary text, 1px `#38425f` border. Confirmatory/supporting actions.
- **Ghost:** Transparent fill, muted text, 1px `#38425f` border. Tertiary actions and cancel affordances.
- **Danger:** Urgent Red fill (`#ff4d5e`), white text. Destructive actions only — delete, revoke.
- **Disabled:** 50% opacity on any variant. Cursor `not-allowed`.

### Chips
Toggle-behavior selection controls — skills, template filters, tab switchers.
- **Unselected:** Input-well background (`#0e1626`), muted text, 1px `#38425f` border, 999px radius, 8px 12px padding.
- **Selected (on):** Garage Purple fill, white text, no border. The same accent rule as buttons — selection is an action.

### Cards / Containers
- **Corner Style:** 14px radius
- **Background:** Panel (`#2a313a`)
- **Shadow:** Standard lift (`0 6px 24px rgba(0,0,0,.45)`)
- **Border:** 1px `#28324c` — just barely visible; defines the card edge without adding visual noise
- **Internal Padding:** 16px

### Chore Cards (signature component)
The primary list item — used on feed, manage, and TV dashboard.
- **Left accent border:** 6px solid; purple (default), red (urgent), purple-glow (suggested)
- **Urgent state:** Red left border + `0 0 0 1px #ff4d5e inset` box-shadow
- **Suggested state:** Purple left border + infinite `pulse` keyframe (inset ring → outer glow)
- **Done state:** 50% opacity, no left-border treatment
- **Internal structure:** h3 title → `.badges` row → claimer/assignee line → action row

### Badges
Metadata tags within chore cards — size, urgency, skill requirement, claim count.
- **Base:** Pill (999px), 0.74rem/700, `--panel-2` background, muted text, 1px `#333f5c` border
- **Urgent override:** Urgent Red fill, white text, no border
- **Suggest override:** Garage Purple fill, white text, no border
- **Size tints:** Text-color-only variants (no fill change) using mint/amber/coral on the base badge

### Inputs / Fields
- **Style:** 10px radius, input-well background (`#0e1626`), 1px `#38425f` border, primary text
- **Focus:** A global `:focus-visible` ring — `2px solid var(--accent-light)` with `2px` offset — applies to every focusable element (browser default is invisible on dark). Inputs also shift their border toward the accent.
- **Range slider (`<input type="range">`):** Native browser control; no custom styling
- **Full width:** All inputs are `width: 100%` within their container

### Navigation
- **Style:** Sticky top, 12px 18px padding, frosted-glass `rgba(15,20,32,.8)` background, `backdrop-filter: blur(8px)`, 1px `#26304a` bottom border
- **Brand:** "Garage Trip" in JetBrains Mono 800 + " Chores" in the body font
- **Links:** Muted text by default, 600 weight. Active (`.on`): primary text + 2px Garage Purple underline. No hover state beyond the active treatment.
- **Mobile:** Same single-row layout; no hamburger or drawer. Nav links shrink with the viewport.

### Toast
Ephemeral acknowledgement — appears on claim, save, assignment, and errors. Carries `role="status"` + `aria-live="polite"` so screen readers announce it.
- **Position:** Fixed, horizontally centered, `max(24px, env(safe-area-inset-bottom))` from bottom so it clears the phone home indicator
- **Success style:** Task Green fill (`#35c48a`), very dark text (`#04231a`), 800 weight, 999px radius, 14px 22px padding, standard lift shadow
- **Error variant (`.toast.error`):** Urgent Red fill, white text — failures never appear in success-green
- **Entry animation:** `translateY(120%)` → `translateY(0)` via `cubic-bezier(.2,1.4,.5,1)` — a spring overshoot; reduced-motion swaps it for a plain opacity fade


## Do's and Don'ts

### Do:
- **Do** use `#8430ce` (Garage Purple) as the single action and selection color. Every primary button, active nav link, and selected chip uses this value.
- **Do** express chore state through the left-border color of `.chore` cards (purple default, red urgent, purple + glow suggested). This is the primary at-a-glance signal.
- **Do** use 999px radius for all badges, tags, chips, and status indicators. Use 14px for cards and 10px for buttons/inputs. Never mix these across component types.
- **Do** keep the JetBrains Mono wordmark (`font-weight: 800`) for "Garage Trip" text only. No other use.
- **Do** let semantic colors carry full meaning: Task Green = done/ok, Urgent Red = danger/alarm, Garage Purple = action/selection.
- **Do** size TV dashboard text generously — `h3` at 1.5rem, `on-it` label at 1.6rem 700. The dashboard is read from across a room.
- **Do** use the *light* purple (`#b48cff`, `--accent-light`) for text-purple — links and the focus ring — and reserve the *fill* purple (`#8430ce`) for backgrounds. Purple text on the fill purple would fail contrast.
- **Do** floor interactive targets at 44px under `@media (pointer: coarse)` while keeping the compact density for mouse users, and give every focusable element the visible `:focus-visible` ring.

### Don't:
- **Don't** use any background lighter than `#22282f`. No white cards, no light-mode panels, no high-key surfaces — ever.
- **Don't** let motion run unconditionally — honor `prefers-reduced-motion` by dropping the pulse/spring while preserving the state via a static inset ring.
- **Don't** add a second decorative accent color. Discord Blue (`#5865f2`) is reserved for the login button only; it is not a general secondary palette.
- **Don't** use JetBrains Mono outside the `.logo` class. It is a brand signal, not a code or data typography choice.
- **Don't** animate anything other than status-driven state changes (urgent glow pulse, suggested-chore pulse, toast spring). Decorative motion is prohibited.
- **Don't** apply shadows inside inputs or form fields. The recessed input-well (`#0e1626`) communicates depth without a shadow.
- **Don't** introduce a light-mode toggle or any color-scheme media query. This is a permanently dark interface.
