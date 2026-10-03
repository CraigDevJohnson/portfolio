# Minecraft Server Page Design Specification

**Status:** Proposed design, awaiting Craig's review
**Drafted:** 2026-10-03
**Scope:** Front end design and behavior of a new `/minecraft` route. No
implementation, backend, AWS permission, or infrastructure change is part of
this document.
**Prototype:** [`docs/design/minecraft/prototype.html`](../../design/minecraft/prototype.html)
(open it directly in a browser; the dashed bar at the bottom switches viewer
role, server state, and server count)

## Purpose

The page is the window onto Craig's family Minecraft world. It answers three
questions for three kinds of visitor, in this order:

1. **Is the world awake?** Anyone landing on the page sees the selected
   server's state, how many of its six places are taken, and whether it is
   healthy, within one glance.
2. **How do I play?** A signed-in family member sees who is on, copies the
   join address, and can wake a sleeping world.
3. **How is it run?** Craig starts, restarts, stops, and reads the server log
   with guardrails that match the operations already built in the
   `CraigDevJohnson/minecraft` repository. A portfolio visitor sees a live
   system whose host only runs while someone plays.

The design follows Craig's reference image (attached to the request on
2026-10-03): site dock, a server switcher strip, a split hero with the server
name and controls on the left and an illustrated world on the right, a band of
health readings, and a pair of Players online and Recent activity panels.

## Product truth the page must carry

These facts come from the minecraft repository as of 2026-10-03 and constrain
every label, state, and number on the page.

| Fact | Source | Consequence for the page |
| --- | --- | --- |
| One server instance today, `survival`; more are allowed | `CONTEXT.md`, ADR 0001 | The switcher works for 1 to 6 servers and collapses gracefully at 1 |
| Up to six players, Java and Bedrock | `README.md` | Capacity reads "of 6", not "of 20" |
| Paper 26.2 with Geyser, Floodgate, ViaVersion | `docs/operations/runtime-content.md` | Java 26.2 and 26.3; Bedrock 26.30 to 26.52; consoles still under test |
| The host is stopped by default and billed only while running | ADR 0003, `idle-stop.md` | Stopped is the normal resting state, not an error |
| Idle stop after 30 quiet minutes (opt-in) | `docs/operations/idle-stop.md` | Online copy says when the world will go back to sleep |
| Backups are cold: a verified recovery point is published on every clean stop | `docs/operations/backup-recovery.md` | No "backup every N minutes" claim; show the last verified recovery point |
| Stop is a drain: save, stop Paper, back up, verify, stop host, park DNS | `idle-stop.md`, host commands | Stop and restart show staged progress and can take several minutes |
| The address is a private family hostname that moves with each host start | ADR 0002, `runtime-content.md` | The join address is never shown to signed-out visitors |
| Allowlist only; a Bedrock UUID can be used without publishing a gamertag | `runtime-content.md` | Gamertags are family-private by default |

## Access model

**Decision pending.** Craig was asked in the project thread; the design and
prototype use the recommended default below. The other two options change
which states render for a signed-out visitor, not the layout.

| Viewer | Sees | Can do |
| --- | --- | --- |
| Visitor (signed out, or signed in without the grant) | State, player count as filled places, health readings, anonymized activity, "How this server runs" | Family sign in |
| Family (a new `minecraft` grant) | Everything above plus player names, editions, full activity, the join address | Start a stopped world |
| Craig (`management` grant) | Everything | Start, restart safely, stop, open console |

Grants follow the existing site sign-in model (`SITE_INVITATIONS_JSON`, grants
read on every request). Adding `minecraft` to the grant vocabulary is an
implementation decision for a later change. Production currently has no EC2
grants (D22), so any control on this page depends on a separate, reviewed
permission path into the minecraft account; until it exists, the controls
render for Craig in preview only.

## Route and navigation

- Route: `GET /minecraft` for the default server, `GET /minecraft/{id}` for a
  specific server instance. The `{id}` is the instance ID (`survival`), not the
  display name.
- Add `{Href: "/minecraft", Label: "Minecraft", Page: "minecraft", FooterGroup: "Tools"}`
  to `navItems()` after Soccer. The header now carries nine items; verify the
  desktop dock still fits above the 70rem navigation breakpoint and falls back
  to the menu below it.
- Title: `Minecraft - Craig Johnson`. Description: "Live status of Craig
  Johnson's family Minecraft world, a Java and Bedrock server that runs on AWS
  only while someone is playing."

## Page anatomy

Regions in reading order. Names in `code` are proposed CSS classes; the
prototype uses the same names with an `mc-` prefix.

### 1. Site dock

Unchanged shared header (`partials.Header`) with Minecraft marked
`aria-current="page"` as the active pill.

### 2. Server switcher (`mc-switcher`)

One full-width panel directly under the dock.

- Left: a server-stack icon, the mono label **Servers**, and a derived count,
  "4 total · 1 online".
- Middle: one link per server. Each shows the server's isometric block icon,
  its display name, a status dot, and "Mode · Runtime" in mono. The selected
  server gets `aria-current="page"`, an apricot-tinted fill, and a 2px apricot
  bottom edge. The list scrolls horizontally with scroll snap when it
  overflows; it never wraps into a second row.
- Right: **All servers**, linking to a future list view. Hidden when there is
  one server.
- Below 70rem the summary column is hidden. Below 30rem All servers shows its
  icon only, with its text kept as the accessible name.

Links, not tabs: each server has its own URL, works without JavaScript, and
htmx may later swap only the hero and panels (`hx-get`, `hx-select`,
`hx-push-url`).

### 3. Hero (`mc-hero`)

Two columns at 5:7. The world scene bleeds from about 30% of the container to
the right edge of the viewport, masked into the canvas on its left and bottom
edges, so the copy sits on night mulberry and the scene never sits behind
text.

Copy column, top to bottom:

1. Eyebrow **Minecraft server** (`page-kit-eyebrow`).
2. `h1`: the server's block icon at 0.78em, then the display name, for example
   **Pinewatch**. Same scale and tracking as `page-hero-title`, on one line
   where it fits.
3. Runtime line in mono: "Survival · Java and Bedrock · Paper 26.2".
4. Status pill, `role="status"` and `aria-live="polite"`: a dot and the state
   word in mono semibold, then a divider and "3 of 6 players".
5. One sentence for the state (copy deck below).
6. Progress steps, only while starting or stopping.
7. Actions for the viewer's role and the server's state (matrix below).
8. Backup note in mono with a shield icon: "Backs up on every stop · last
   Oct 2, 9:14 PM, verified".

Scene column: the living world scene (section 8) with a status chip in its
lower right: a small bar glyph, **World status**, and a mono line such as "All
systems normal". The chip's tone follows the server state.

Below 56rem the hero stacks: copy first, then the scene as a 15 to 26rem tall
band that runs edge to edge, faded at top and bottom. This follows the site
convention of text before imagery on small screens.

### 4. Health band (`mc-vitals`)

One panel with four readings divided by hairlines (not four cards): **CPU**,
**Memory**, **Awake for**, **Tick rate**. Each has a stroke icon, a mono
uppercase label, a large tabular value with a small unit, and a 32-point
sparkline of the current session with an area fill and an endpoint dot. A
one-line mono caption under the band explains anything that is not green.

Colour encodes health only: pond mint is normal, campfire apricot is near a
limit, and rosehip is a fault. The online caption reads "This session, last 60
minutes. Amber marks a reading near its limit. Readings refresh every minute."

When the world is stopped or starting there is no live data, so the band
collapses to one line with a moon icon instead of four empty readings: "Last
session: 2h 14m awake, 4 players at the busiest point. Memory peaked at 6.8 of
8 GB. The backup verified on Oct 2 at 9:14 PM." While starting, the line leads
with "Live readings appear once Paper is running." Not responding keeps all
four readings, because a tick rate of 0 is the point. The reference's latency tile is replaced
by tick rate, the measure a Minecraft server is actually judged by;
per-player connection quality moves to the Players panel. "Uptime" becomes
**Awake for**, because the host lives for one play session, not for days.

Two columns below 70rem, one column below 30rem.

### 5. Players online (`mc-players`)

Panel heading in the mint mono style with "Up to 6" at the right.

- Family and Craig: one row per player with an 8x8 pixel head, the name, the
  role (**Operator** in mint, **Member** muted), an edition chip (Java or
  Bedrock), minutes in this session, and four connection bars with an
  accessible label ("Connection 4 of 4").
- Visitor: six places drawn as tiles; filled tiles carry an anonymous grey
  head. One sentence: "3 of 6 places are taken. Player names stay private to
  family; they appear here after you sign in."
- Empty: "Nobody is online" with the last player and time, family only.

### 6. Recent activity (`mc-activity`)

Panel heading with "Last 24 hours". Rows have a round icon (tone by event),
a title, a mono detail line, and a right-aligned relative time. Event types:
joined, left, world woke up (who started it and how long it took), start or
stop requested, world went to sleep (idle stop), backup verified (size), and
server stopped answering. For visitors every name becomes "A player" with the
detail "Name private to family", and "Started by CraigJ" becomes "Started by a
family member".

### 7. Join band or How this server runs (`mc-band`)

- Family and Craig: **Join Pinewatch**, a short line about the address
  following the host, then two columns. **Java Edition**: the address in a
  read-only mono field with a Copy button, versions 26.2 and 26.3, default
  port. **Bedrock Edition**: the same field, port 19132, versions 26.30 to
  26.52 on phones, tablets, Windows and Xbox, and a note that PlayStation and
  Switch are still being tested. While the world sleeps the intro says the
  address works only while it is awake.
- Visitor: **How this server runs**, the portfolio proof. A one-line summary
  ("The host only runs while someone is playing; the saved world is what
  stays.") then a numbered five-step lifecycle, each step drawn as a small
  crop of the same world scene in that step's light: **Someone asks**, **The
  world wakes**, **Family plays**, **Thirty quiet minutes**, **Backed up,
  asleep**. Numbers are justified here because the order is the content. On
  phones the steps stack with the thumbnail beside the text. A link to the
  infrastructure write-up belongs here once a Projects entry exists.

### 8. Console drawer (Craig only)

A right-side drawer, 36rem wide or full width on phones, over a scrim. It
holds a read-only, monospace tail of the server log with timestamps and level
colours (WARN apricot, ERROR rosehip, joins mint), and keeps the last session
after a stop. Escape and the scrim close it; focus moves to its close button
on open, stays trapped inside, and returns to the button that opened it.
Sending commands is out of scope for the first version (open question 3).

### 9. Stop and restart confirmation

A native `<dialog>`, justified because the action disconnects players and
cannot be cancelled once the drain starts. Title "Stop Pinewatch?" or
"Restart Pinewatch?". The body states how many players will be disconnected
and that the world saves and backs up before the host stops, which can take
several minutes. Buttons: **Keep running** (focused by default) and **Stop and
back up** or **Restart safely** (danger style).

## States and actions

States come from the host and instance status the operator tooling already
reports. The page polls or streams them (see Data contract).

Chip text by state: All systems normal · Asleep, host stopped · Waking up ·
Saving and backing up · Needs attention. The chip is hidden below 56rem, where
the pill directly above already states the same thing.

| State | Pill | Scene | Status sentence |
| --- | --- | --- | --- |
| Online | mint dot, **Online**, "3 of 6 players" | Dusk, every light lit, torches flicker | The world is awake. It goes back to sleep after 30 minutes with nobody online. |
| Stopped | muted dot, **Stopped**, "0 of 6 players" | Night, stars and moon, lights out | The world is asleep and no server is running. Starting it takes a few minutes. (Visitors: Family members wake it when they want to play.) |
| Starting | pulsing apricot dot, **Starting** | Lights come on one at a time as steps finish | Waking the world. The address works as soon as the last step finishes. |
| Stopping | pulsing apricot dot, **Stopping** | Lights go out one at a time | Everyone has been warned. The world saves and backs up before the host shuts down. |
| Not responding | rosehip dot, **Not responding**, "Players can't join" | Desaturated, lights out | Craig: The host is running but Minecraft stopped answering 4 minutes ago. Open the console to see why, or restart safely. Others: The server stopped answering a few minutes ago. Craig has been alerted. |

Starting steps: Starting the host · Attaching the world disk · Launching Paper
and Geyser · Pointing the address at the new host · Ready for players.
Stopping steps: Warning players and saving the world · Stopping Paper ·
Backing up the world · Verifying the backup · Stopping the host. Each step is
a list item with done, in progress, and waiting styles; the in-progress item
carries "(in progress)" for screen readers.

Actions by role and state:

| | Online | Stopped | Starting / Stopping | Not responding |
| --- | --- | --- | --- | --- |
| Visitor | Family sign in · See how it runs | same | same | same |
| Family | Get the join address | **Start server** | Starting… (disabled) | none |
| Craig | **Open console** · Restart safely · Stop server | **Start server** · Last session log | **Open console** | **Open console** · Restart safely · Stop server |

The first action is the primary (apricot) style, the middle ones secondary,
and Stop uses the danger style: rosehip outline and text with a filled square
glyph. All controls keep the 44 by 44 CSS pixel minimum.

## Living world scene

The scene is the page's signature and the one place Minecraft's own material
enters an Emberglass page.

- Drawn on a `<canvas>` at 320 by 180 and scaled with
  `image-rendering: pixelated`, `object-fit: cover`. No image download.
- Seeded per server, so each world has its own ridge line, pine stand, and
  tower position. Layers: dithered sky, stars and moon, clouds, far ridge, far
  pines, water with sky reflection, terraced hill with a stone wall band,
  fence path, stone and timber watchtower, near pines, then additive light
  with a small warm spill under windows and torches, and broken reflection
  rows on the lake.
- One light level drives the whole palette: Emberglass dusk (mulberry into
  rosehip into apricot) at 1, a near-black plum night at 0. Lights switch on
  as a fraction of start progress in a set order (tower windows, tower
  torches, the far ridge, then the lanterns stepping down the path to the
  water), which is how "torches light one by one" while the world wakes.
  Stopping runs the same order in reverse.
- Ambient motion is limited to torch flicker and star twinkle at about 12
  frames per second, paused when the canvas is off screen or the tab is
  hidden. With `prefers-reduced-motion: reduce` the scene draws once per state
  change and nothing loops.
- `role="img"` with a state-aware `aria-label`, for example "Pixel-art view of
  Pinewatch at dusk, torches lit. The server is online."
- Optional later: a real in-game screenshot per world, dimmed and tinted by
  state with CSS. The generated scene stays as the fallback.

## Pixel material

- **Server block icons:** isometric 8x8-texel blocks drawn on canvas: grass for
  survival, cobblestone for creative, magma for a nether-flavoured world,
  planks for modded. A server's icon is part of its configuration.
- **Player heads:** 8x8 faces. In production, Java heads come from the
  player's skin face layer, fetched server side by UUID and cached by the Go
  app so no third-party image host sees visitors. Bedrock players through
  Floodgate have no Java skin; they get a generated head seeded by UUID.
  Visitors only ever see the anonymous grey head.

Pixel material is always rendered crisp and never softened with smoothing,
shadows, or rounding beyond 3px.

## Visual system

Everything inherits Emberglass (`cmd/web/tailwind/theme.css`) and the
interface conventions (`docs/design/interface-conventions.md`). No new
palette tokens are needed. New semantic aliases for this page:

| Alias | Value | Use |
| --- | --- | --- |
| `--mc-tone-ok` | `--color-success-500` (pond mint) | Online, healthy readings, verified backups |
| `--mc-tone-busy` | `--color-warning-500` (campfire apricot) | Starting, stopping, readings near a limit |
| `--mc-tone-fault` | `--color-danger-500` (rosehip) | Not responding, stop action |
| `--mc-tone-idle` | `--plum-400` | Stopped, no live data |

Type: IBM Plex Sans for names, headings, sentences, and controls, including
activity details, the backup note, and progress steps; IBM Plex Mono only for
labels, counts, versions, ports, times, addresses, and the log. The `h1` uses
-0.045em tracking, a little looser than `page-hero-title`, because a single
server name at this size crowds at the hero default. Uppercase mono labels
keep the existing eyebrow tracking.

Panels use the existing `page-kit-panel` treatment. The page adds one new
radius use only: 16px for the switcher's tabs and the address fields.

## Motion

- The one authored moment is the scene's light change on a state transition:
  an exponential ease toward the new light level over about 1.5 seconds.
- Status dots pulse only while starting or stopping.
- Buttons lift 1px on hover, as elsewhere on the site.
- The console slides in over 320ms with the site's ease-out curve.
- Everything respects `prefers-reduced-motion`.

## Accessibility

- The status pill is a polite live region, so a start or stop announces
  "Starting", each finished step, and "Online" without moving focus.
- State is never colour alone: every dot sits beside a word, and connection
  bars carry text.
- Server links expose state as hidden text ("Stopped, Creative · Paper").
- Heading order: `h1` server name, `h2` for Players online, Recent activity,
  and the Join or How it runs band.
- The address fields are selectable text; Copy falls back to selecting the
  text when the clipboard is refused.
- Forced colours follow the site's existing `forced-colors` rules; the scene is
  decorative in that mode and its label still carries the state.

## Data contract

The page needs one read model per server. A sketch for the eventual Go view
model, in `cmd/web/partials`:

```go
type MinecraftState string

const (
	MinecraftOnline        MinecraftState = "online"
	MinecraftStopped       MinecraftState = "stopped"
	MinecraftStarting      MinecraftState = "starting"
	MinecraftStopping      MinecraftState = "stopping"
	MinecraftNotResponding MinecraftState = "not-responding"
)

type MinecraftViewer int

const (
	ViewerVisitor MinecraftViewer = iota
	ViewerFamily
	ViewerOperator
)

type MinecraftServerSummary struct {
	ID, Name, Mode, Runtime, Block string
	State                          MinecraftState
}

type MinecraftPageProps struct {
	Viewer      MinecraftViewer
	Servers     []MinecraftServerSummary
	Selected    MinecraftServerSummary
	Capacity    int
	OnlineCount int
	Players     []MinecraftPlayer // empty for visitors
	Step        int               // index into the start or stop steps while busy
	Vitals      MinecraftVitals
	Activity    []MinecraftEvent // names already redacted for visitors
	LastBackup  time.Time
	JoinHost    string // empty unless Viewer >= ViewerFamily
}
```

Redaction happens in the Go handler before rendering: a visitor's props never
contain a gamertag or the hostname, so no template or client script can leak
them.

Likely sources, to be confirmed in the implementation design: EC2 instance
state; the instance status and player list from the Java Server List Ping
(the same check idle stop uses); CloudWatch for CPU and memory (memory needs
the CloudWatch agent); Paper's tick rate through a small status endpoint or
log metric; the backup receipts for the last verified recovery point. State
refreshes with htmx polling every 15 seconds while starting or stopping and
every 60 seconds otherwise; Server-Sent Events can replace polling later.

## Copy deck

| Element | Text |
| --- | --- |
| Eyebrow | Minecraft server |
| Visitor primary | Family sign in |
| Visitor secondary | See how it runs |
| Family primary (online) | Get the join address |
| Start | Start server |
| Restart | Restart safely |
| Stop | Stop server |
| Console | Open console / Last session log |
| Confirm, safe choice | Keep running |
| Confirm, stop | Stop and back up |
| Backup note (online) | Backs up on every stop · last {time}, verified |
| Backup note (stopped) | Last backup {time}, verified when the world last stopped |
| Visitor players note | {n} of 6 places are taken. Player names stay private to family; they appear here after you sign in. |
| Join intro (online) | Add the server once and it stays in your list. The address follows the host each time the world wakes. |
| Join intro (stopped) | The address works while the world is awake. Start it first, or ask Craig. |

## Where this departs from the reference

- **Heading.** The reference repeats "Minecraft Server" as the page heading
  and then shows the server name below it. The site convention says the
  heading explains the content instead of repeating the section, so the
  section name is the eyebrow and the server name is the `h1`.
- **Identity line.** The reference shows "Cloud systems & automation" under
  Craig's name; the shared identity stays "Cloud Engineer Principal".
- **Numbers.** "of 20 players" becomes "of 6", "Uptime 4d 12h" becomes
  "Awake for", latency becomes tick rate, and "Automatic backups · last 28m
  ago" becomes the last verified recovery point, because backups happen on
  stop.
- **Stopped is first-class.** The reference only shows Online. Stopped is the
  normal resting state of an on-demand host, so it gets the full design: night
  scene, Start server as the primary action, and last-session readings.
- **Join band and visitor privacy** are additions the reference does not show.

## Open questions

1. **Access model.** Public status (recommended), signed-in only, or fully
   public. Asked in the project thread.
2. **Who can start and stop.** The default lets family start a sleeping world
   and keeps restart, stop, and console with Craig. Idle stop already puts the
   world back to sleep.
3. **Console scope.** Read-only log tail in the first version, or guarded
   commands (say, whitelist and broadcast) later.
4. **Server display names.** The `survival` instance needs a world name. The
   prototype uses the reference's sample names.
5. **Permission path.** The portfolio Lambda has no EC2 control today. A
   reviewed cross-account role, or a small API owned by the minecraft
   repository, is a separate design.
6. **Public write-up.** Whether the visitor band links to a Projects entry or
   to the minecraft repository.

## Verification for the eventual build

Run `task ci`. Capture the page at compact, tablet, and desktop widths in each
state and viewer role, including both sides of the 70rem navigation
breakpoint; check keyboard order, console focus trap and return, the dialog's
default focus, live-region announcements during a start, and that a visitor's
HTML contains no gamertag or hostname.
