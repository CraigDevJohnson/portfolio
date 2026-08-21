<!-- markdownlint-disable MD013 -->
# Soccer schedule

Soccer schedule lets a visitor import temporary Let's Play Soccer access or enter team IDs, choose players and teams, review upcoming matches and past results, select games, download ICS, and optionally add or sync through Google Calendar.

## Sub-features

- `soccer-entry` explains Connections and the four-stage planner on `/soccer`.
- `soccer-manual` accepts numeric team IDs and fetches schedules through the live LPS boundary.
- `soccer-import` imports a temporary JWT and discovers linked players and teams.
- `soccer-selection` updates selected counts and action availability for upcoming and past groups.
- `soccer-ics` downloads only selected upcoming games as `soccer_schedule.ics`.
- `soccer-google` presents disconnected, connected, add, and result-sync states.

## How to get to it (user POV)

- Choose `Soccer` from the shared navigation or footer Tools group.
- On `/soccer`, use `Import access` when enabled or enter numeric IDs in `Team IDs` and choose `Fetch Schedules`.
- Choose linked players, confirm teams, then use `Review & output` to select games.
- Choose `Download Selected (.ics)` or, when connected, the Google Calendar action.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes under the loopback preview launch.
- No real JWT, OAuth consent, AWS credential, or live LPS mutation is in scope.
- Treat `/__preview/soccer/*` as an isolated external-boundary fixture, not a production entry point.

- **Production entry.** From `/`, click `"nav[aria-label='Main navigation'] a[data-nav-page='soccer']"` and snapshot. The URL path is `/soccer`; `Soccer Schedule Download`, `Connections`, and stages 1 through 4 are visible. With the empty verification environment, `Player discovery` and Google Calendar report unavailable while manual Team IDs remains available.
- **Fixture results.** Run `goto "$VERIFY_URL/__preview/soccer/combined"` and snapshot. Require `Upcoming games`, `Past results`, two upcoming games, two past results, and a checked `Select all` control for both groups.
- **Selection.** Run `uncheck "input[aria-label='Select all upcoming games']"`, snapshot, then `check "input[aria-label='Select Pond Mint United versus Campfire Rovers']"`. The selected-count status moves from `0 selected` to `1 selected` and `#download-button` becomes enabled.
- **ICS side effect.** Before clicking download, set Playwright download handling through `run-code` to wait for the download while clicking `#download-button`, then save it under `evidence/soccer-schedule/soccer_schedule.ics`. Inspect the file with `rg -n 'BEGIN:VCALENDAR|BEGIN:VEVENT|SUMMARY:|DTSTART|DTEND'`; exactly the selected fixture game must be present.
- **Connected fixture.** Run `goto "$VERIFY_URL/__preview/soccer/google-connected"`. Require `Calendar ready`, the selected destination `Matchdays and travel notes (Primary)`, and disabled preview-only Google mutation controls.
- **Feedback fixtures.** Open `/__preview/soccer/google-add-success`, `/__preview/soccer/google-add-error`, `/__preview/soccer/google-sync-success`, and `/__preview/soccer/google-sync-error`; capture each distinct success/error status without calling Google.
- **Proof.** Retain the production entry snapshot, fixture-labelled selection before/result evidence, the downloaded ICS plus content assertions, browser request list, and server log. State explicitly that live LPS and Google were not tested.

## Gotchas

- Never paste or record a real JWT in proof artifacts.
- The preview buttons that would mutate LPS or Google are intentionally disabled. Their disabled state is the expected safety behavior, not a live integration result.
- An ICS response is a real local file side effect even in preview. Preserve it as evidence and inspect its contents; a download toast alone is insufficient.
- Selection state uses browser `sessionStorage` keyed by the loaded team fingerprint. Use the run-scoped browser session and clear it when comparing unrelated fixtures.
- Live Google OAuth requires user-driven consent and configured external state. Do not claim it from local route behavior.
