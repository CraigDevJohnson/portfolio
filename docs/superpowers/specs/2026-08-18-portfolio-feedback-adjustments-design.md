# Portfolio Feedback Adjustments Design Addendum

**Status:** Proposed for user review

**Date:** 2026-08-18
**Amends:** `docs/superpowers/specs/2026-08-12-emberglass-field-station-2-design.md`

## Purpose

This addendum converts the first hands-on review of Emberglass Field Station 2.0
into a focused refinement pass. The redesign's identity, page-specific layouts,
photo framing, palette, and typography are successful and remain the baseline.
The work below removes elements that feel unfinished or overly ornamental,
simplifies the Skills information architecture, and makes the Soccer tool
resilient enough for real use.

The older specification remains authoritative except where this addendum
explicitly changes Home, the shared signal trail, Experience, Skills, Contact,
or Soccer.

## Design decisions

### Preserve the successful Emberglass system

No new theme is introduced. The implementation continues to derive from:

| Role | Token or typeface | Value |
| --- | --- | --- |
| Canvas | Night Mulberry | `#17121B` |
| Raised surfaces | Cocoa Cedar | `#2E2130` |
| Primary copy | Candle Oat | `#FFF0D8` |
| Warm action | Campfire Apricot | `#FFA677` |
| Emphasis | Rosehip | `#FF7FA8` |
| Success and connected state | Pond Mint | `#78E3C3` |
| Display type | Bricolage Grotesque | Existing site loading and scale |
| Body type | Atkinson Hyperlegible | Existing site loading and scale |
| Utility/data type | IBM Plex Mono | Existing site loading and scale |

The memorable visual remains the field-station composition: dark plum surfaces,
warm pastel operational signals, large editorial headings, and photographic
portals. This pass spends its boldness on one new structural move: the Soccer
schedule becomes a full-width match board below the setup workflow, giving the
data the width it needs instead of trapping it beside the planner rail.

### Home

Remove the entire “Proof of Work / Follow the systems behind the résumé” card
section. It repeats destinations already available in the primary navigation
and hero actions, and its large asymmetric cards make small amounts of copy feel
unfinished. The approved hero, statistics, and “Reliable systems, clear
handoffs” capability topology remain unchanged except for any spacing cleanup
left by the removed section.

### Shared signal trail and About

Remove the three SVG node circles from the shared signal trail on every route.
Remove the dash-offset and breathing animations and render one continuous,
static gradient line with its restrained blur shadow. This keeps the trail as a
structural route marker while eliminating both the unexplained dots and the
visible animation reset. Forced-colors behavior remains a simple system-color
rule; reduced motion requires no special visual substitute because the default
trail is already static.

### Experience

Keep the derived list of eight recurring technologies, but replace the offset
spreadsheet-like strip with one full-width, raised “field kit” panel. Its label,
title, and short explanation align with the rest of the page. Technologies
render as compact, wrapping chips inside the same panel rather than as a
detached two-row grid. The three eras and jump navigation do not change.

### Skills

Keep the Core Competencies mosaic, Concepts & Practices band, current primary
catalog categories, and category/proficiency filters. A skill's existing
category remains its canonical display location when no filter is active; this
pass does not move GitHub or reorganize the unfiltered catalog.

Add a controlled list of secondary tags to each skill that genuinely spans
multiple disciplines. Category filtering matches either the canonical category
or one of those secondary tags. Search matches name, description, canonical
category, and tags. For example, GitHub remains under Development Tools but is
tagged Collaboration Tools, so it appears when that filter is selected. A
filtered result continues to render beneath its canonical category heading,
which prevents duplicate ownership or the impression that the base catalog was
reorganized.

The filter presentation becomes quieter rather than disappearing: keep search
as the primary control, use compact wrapping category/proficiency controls,
remove decorative dots, swipe instructions, fades, and horizontal filter
rails, and expose secondary tags only in the selected skill's detail area. The
normal GET and HTMX fragment behavior remain progressively enhanced and
URL-backed.

Concepts & Practices remains its separate supporting band and is not part of
the searchable vendor/tool catalog or secondary-tag vocabulary.

All Core Competencies vendor logos use one optically consistent logo plate:
fixed visual bounds, a quiet Candle Oat inset surface, no global brightness
filter, and per-asset `object-fit: contain`. This makes dark AWS/Ansible-style
marks and colorful vendor marks equally legible without modifying the SVG
artwork itself.

### Contact

Replace `@`, `IN`, `GH`, and the four expertise glyphs with local inline SVGs.
Mail, LinkedIn, and GitHub use recognizable brand/communication marks;
architecture, automation, security, and observability use a consistent
24-by-24 stroke family. A typed shared icon component supplies the SVGs, while
existing card tones control their color. Icons are decorative inside already
named links/cards and remain `aria-hidden`; accessible names continue to come
from the surrounding content.

### Soccer workflow state

The imported LPS session remains the security boundary. Add a compact workflow
snapshot to its encrypted session data:

- selected player IDs;
- the teams discovered for those players;
- confirmed team IDs;
- whether the active source is imported players or manual team IDs.

Discovering teams persists the player IDs and available teams. A successful
schedule fetch persists the final team IDs and source. A later full-page load,
including the return from Google OAuth, reconstructs the selected players,
confirmed teams, and schedule from that snapshot. A new JWT import or explicit
“Clear import” intentionally clears the snapshot. Connecting, reconnecting,
disconnecting, or changing a Google calendar never clears it.

Per-game checkbox choices are transient browser UI state. Store their checked
IDs in `sessionStorage` under a key derived from the current confirmed-team
fingerprint. Restore those checks after an OAuth round trip or HTMX schedule
replacement, and discard them when the confirmed teams change, a new import is
saved, or the import is cleared. Do not store the JWT, player names, or Google
credentials in browser storage.

### Soccer information architecture

Move connection readiness above the numbered workflow. A compact “Connections”
panel presents two cards:

1. Let’s Play Soccer access: “Not imported” or “Imported for this session.”
2. Google Calendar: “Not connected” or “Connected to [calendar].”

Connected cards use a Pond Mint border, tinted surface, check icon, and explicit
text; color is never the only signal. This makes imported JWT state immediately
obvious and lets users connect Google Calendar before beginning the schedule
workflow.

The numbered workflow becomes:

1. Source — use imported players or manual team IDs.
2. Players — choose discovered players.
3. Teams — confirm teams.
4. Review & output — select games, download, add, or sync.

Calendar output is no longer a fifth stage because its setup is available before
stage 1 and its actions live beside the games they affect.

### Soccer player and team selection

Highlight the complete primary-player row with a mint border, a quiet tinted
surface, and “Primary player” supporting text. Remove the detached `Primary`
pill.

Remove sub-team classification end to end. Import no longer performs the
baseline team fetch used only to classify later teams. `KnownTeams`,
`IsSubTeam`, sub copy, sub badges, and different default-checkbox behavior are
removed. Every currently discovered team is selected by default; the user can
deselect any team before fetching.

### Soccer schedule presentation

Move Review & Output out of the narrow planner workspace and place it full width
below stages 1–3. Replace the eight-column minimum-width table with a responsive
match list:

- desktop rows: select, date/time, matchup, location, season, and result;
- “matchup” combines home and away teams;
- “location” combines field and venue;
- upcoming rows omit an empty result area;
- narrow layouts turn the same semantic row into a labeled card without
  duplicate content.

The schedule must not require horizontal scrolling at any supported viewport.
All “scroll horizontally” hints and the local horizontal scrollbar are removed.

The section title and action toolbar are not sticky. Each Upcoming and Past
section owns a feedback region directly below its actions. While an Add or Sync
request runs, the clicked button is disabled, shows a spinner, and changes its
label to include the selected count. On completion, success or failure replaces
the adjacent feedback region and is announced through its live region. The
single feedback block below both schedules is removed.

## Unchanged areas

- Projects content and composition remain unchanged; adding new projects is a
  separate content task.
- Education remains unchanged.
- The management portal remains unchanged beyond shared signal-trail rendering
  and regression verification.
- Home hero, Home capability topology, About content, Experience eras, Skills
  Core Competencies content, and Contact copy remain unchanged except where
  explicitly described above.

## Accessibility and responsive requirements

- Keep one visible `h1`, sequential headings, 44-by-44 pointer targets, visible
  focus, forced-color support, and keyboard-operable forms.
- Connection status and primary-player meaning use text as well as color.
- Search remains usable without HTMX.
- Restored Soccer state is announced without moving pointer users' focus.
- Keyboard-triggered HTMX actions may move focus to the adjacent feedback
  region after the response.
- Verify 390, 768, 1119, 1121, and 1440 pixel viewport widths.
- No page-level horizontal overflow is permitted. Soccer schedules also have no
  intentional local horizontal overflow.

## Acceptance criteria

1. The Home proof-card section is absent and the approved upper Home composition
   is visually unchanged.
2. Every signal trail is a static continuous line with no node circles or
   visible loop reset.
3. Experience recurring tools align with the section and visually belong to the
   era page.
4. Skills preserves Core Competencies, Concepts & Practices, its primary
   categories, and category/proficiency filtering. Secondary tags participate
   in filtering and search without changing canonical ownership; GitHub remains
   under Development Tools and matches the Collaboration Tools filter.
5. Core Competencies logos have consistent optical size and contrast.
6. Contact uses recognizable SVG icons instead of text abbreviations or glyphs.
7. Returning from Google OAuth restores imported player, team, schedule, and
   compatible per-game selection state.
8. Imported LPS and connected Google states are obvious before stage 1.
9. Primary-player emphasis applies to the full row, and no sub-team concept is
   rendered or computed.
10. Upcoming and Past schedules require no horizontal scroll, have no sticky
    overlay, and show pending/result feedback beside the initiating action.
11. Projects, Education, management functionality, native ICS download, Google
    add/sync, keyboard use, reduced motion, and forced colors do not regress.
