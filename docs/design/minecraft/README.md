# Minecraft server page design

Design work for the planned `/minecraft` route. Nothing here is wired into the site yet.

- [Design specification](../../superpowers/specs/2026-10-03-minecraft-server-page-design.md): states, roles, layout, copy, data contract and open questions.
- [`prototype.html`](prototype.html): a standalone, interactive prototype. Open it in a browser. The dashed bar at the bottom switches the viewer (visitor, family, Craig), the server state, and the number of servers. Start, stop, restart and the console are simulated. All names, players, metrics and times are sample data.
- [`screens/`](screens): captures of the prototype at 1440 and 390 pixels wide.

The prototype reuses the Emberglass tokens from `cmd/web/tailwind/theme.css` and mirrors the shared header. It is a design reference, not production markup; the build should use the existing templ partials and Tailwind layers.
