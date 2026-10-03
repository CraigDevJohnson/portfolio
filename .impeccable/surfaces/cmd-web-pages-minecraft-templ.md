---
version: 1
slug: "cmd-web-pages-minecraft-templ"
primary_target: "cmd/web/pages/minecraft.templ"
related_targets: []
---

# Minecraft server page (/minecraft)

Scope: new route inside the established Emberglass world. Mode: Operate for family and Craig, with a Persuade layer for portfolio visitors (proof of on-demand AWS engineering). Design and front end spec only; no implementation yet.

Audience: family players checking "is the world up, who's on, how do I join"; Craig operating start, stop, restart and console; portfolio visitors seeing a live system he built.
Truth to carry: one `survival` instance today (multi-instance allowed by ADR 0001), up to 6 players, Paper 26.2 + Geyser/Floodgate crossplay, host stopped by default, idle stop after 30 min, cold backup on every clean stop, private family hostname, allowlist only.
Constraints: Go + templ + htmx + Tailwind v4; reuse SiteIdentity, nav, ActionButton, Feedback; kids' gamertags and the join address never reach signed-out visitors.
Open: access model (decision card posted), who may start/stop, console scope, metrics source.

## Direction contract

THESIS: The page is the world's window. Its signature is a living pixel-art scene of the selected world whose light follows real server state: dusk with lit torches when online, night with lights out when stopped, torches lighting one by one while it starts. It refuses the generic admin table (the existing /mgmt look) and the stock "status dashboard" of cards and gauges.
OWN-WORLD: Emberglass tokens unchanged: night mulberry canvas, cocoa cedar panels, candle oat copy, apricot primary action, rosehip for stop and faults, pond mint for healthy signal. IBM Plex Sans/Mono. Minecraft enters only as pixel material: isometric block icons per server, 8x8 player heads, the scene, rendered crisp with pixelated scaling. No Minecraft font, no green-on-black terminal costume.
STORY: A visitor understands in one glance whether the world is awake, how many of six beds are taken, and how it is kept safe. Family sees who is on and copies the join address. Craig starts, restarts, stops or opens the console with state-aware guardrails.
FIRST VIEWPORT: site dock with Minecraft active; full-width server switcher strip; left 5/12 copy column (eyebrow, server name at display scale with its block, runtime line, status pill with player count, one-line state sentence, role-aware actions, backup line); right 7/12 the scene bleeding to the viewport edge with a status chip in its lower right. Primary action sits under the status sentence.
FORM: Pinned by the user, so no concept roll ran. Craig's request: "Attached is a reference for how I'd like it to roughly look. This is just to build the specs and design for the front end." Code-led build (no image generation available): static, interactive HTML prototype in docs/design/minecraft/.
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
