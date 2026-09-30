<!-- markdownlint-disable MD013 -->
# Soccer schedule

Soccer schedule asks a visitor to choose a calendar output first: an .ics file or Google Calendar. It then reveals the steps that apply: enter team IDs or import temporary Let's Play Soccer access, choose players and teams, review upcoming matches (and past results in Google mode), select games, and download an .ics file or add and sync through Google Calendar.

## Sub-features

- `soccer-entry` offers the calendar output choice first on `/soccer`; Connections and the planner stages appear once an output is chosen.
- `soccer-manual` accepts numeric team IDs and fetches schedules through the LPS boundary. The loopback preview answers from its in-process fake LPS at `/__preview/lps/teams/{id}`; production uses live LPS.
- `soccer-planner-journey` is the choice-first public journey: .ics output, Team IDs, soonest-first rows, deselection memory across refetch and output switching, live count and select-all, and the downloaded .ics content.
- `soccer-import` imports a temporary JWT and discovers linked players and teams.
- `soccer-selection` updates selected counts and action availability for upcoming and past groups.
- `soccer-restoration` restores saved player/team workflow on full-page loads and match selection after reload.
- `soccer-ics` downloads only selected upcoming games as `soccer_schedule.ics`.
- `soccer-google` presents disconnected, connected, add, and result-sync states.
- `soccer-access` distinguishes public Team ID lookup and ICS download from private LPS import and Google Calendar, which need the current `soccer` page grant; the page and every private route make the same decision.

## How to get to it (user POV)

- Choose `Soccer` from the shared navigation or footer Tools group.
- On `/soccer`, choose `Download an .ics file` or, where Google is configured, `Google Calendar`.
- Use `Import access` when enabled or enter numeric IDs in `Team IDs` and choose `Fetch schedules`.
- With imported access, choose linked players and confirm teams; then use `Review & output` to select games.
- Choose `Download selected (.ics)` or, when connected, the Google Calendar action.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes under the loopback preview launch.
- No real JWT, OAuth consent, AWS credential, or live LPS mutation is in scope.
- Treat `/__preview/soccer/*` as an isolated external-boundary fixture, not a production entry point.

- **Production entry.** From `/`, click `"nav[aria-label='Main navigation'] a[data-nav-page='soccer']"` and snapshot. The URL path is `/soccer`; `Soccer Schedule Download` and the `Choose your calendar output` step are visible, with nothing chosen. With the empty verification environment the Google option is disabled (`Not enabled on this server`). After choosing `Download an .ics file`, `Connections` shows `Player discovery` and Google Calendar as unavailable while manual Team IDs remains available.
- **Choice-first journey.** On a fresh launch, run `.cursor/skills/verify-portfolio/scripts/prove-soccer-planner`. It chooses .ics with the keyboard and requires only the public steps (legend `Choose output`, `Choose source`, `Review & output`). It fetches `479691, 479147` from the fake LPS and requires the `Fetching schedules...` status and review stage while the response is held. It then requires rows `7003, 7001, 7002` soonest first, all selected. It deselects `7001` with Space and requires `2 games selected` and an indeterminate select-all. A refetch as `479147 479691` must keep `7001` deselected and show newly published `7004` selected. Switching to Google and back must keep that selection. A refetch with a stale `lps_session` cookie must reset the workflow (`HX-Trigger: soccer-workflow-reset`) and still keep the deselection. The downloaded `soccer_schedule.ics` must hold exactly `7003`, `7002`, and `7004`. A failed first fetch must show an alert in the review stage, and the planner must fit 390px without horizontal scroll. Evidence lands under `evidence/soccer-planner/`.
- **Workflow fixtures.** Open `import`, `players`, and `team-selection` preview fixtures in sequence. Open the import dialog and require its submit control to be disabled and its retention note to say the JWT stays in this browser until it expires, for up to 12 hours, usable only by this signed-in site account with Soccer access, and removed by site sign-out or Clear import. Then require the linked-player and confirmed-team states with their external-boundary controls disabled, and the LPS card status `Imported in this browser` with `Clear import`.
- **Fixture results.** Clear Soccer selection storage, run `goto "$VERIFY_URL/__preview/soccer/combined"`, and snapshot. With results present and no saved choice, the page chooses `Download an .ics file`. Require `Upcoming games`, two upcoming games, a checked `Select all upcoming games` control, a visible `#download-button`, and no visible `Past results`; past results belong to Google mode.
- **Selection and restoration.** Uncheck `Select all upcoming games` and require `0 games selected`. Check only `Select Pond Mint United versus Campfire Rovers`; require `1 game selected` and enabled `#download-button`. Reload and require that exact selection to persist.
- **ICS side effect.** Before clicking download, set Playwright download handling through `run-code` to wait for the download while clicking `#download-button`, then save it under `evidence/soccer-schedule/soccer_schedule.ics`. Inspect the file with `rg -n 'BEGIN:VCALENDAR|BEGIN:VEVENT|SUMMARY:|DTSTART|DTEND'`; exactly the selected fixture game must be present.
- **Google fixtures.** Clear selection storage, open `google-disconnected`, then `google-connected`, and choose `Google Calendar` in the output step; these fixtures render Google as available. Require the distinct connection copy, `Calendar ready`, the selected destination `Matchdays and travel notes (Primary)`, `Past results` with its selection controls, a hidden `#download-button`, and disabled preview-only Google mutation controls.
- **Feedback fixtures.** Open `/__preview/soccer/google-add-success`, `/__preview/soccer/google-add-error`, `/__preview/soccer/google-sync-success`, and `/__preview/soccer/google-sync-error`; capture each distinct success/error status without calling Google.
- **Grant states.** Run `.cursor/skills/verify-portfolio/scripts/prove-soccer-access`; evidence lands under `evidence/soccer-access/`. It opens `/__preview/account/soccer-signed-out`, `/__preview/account/soccer-ungranted`, and `/__preview/account/soccer-granted` in sequence and chooses the .ics output on each; they render the inert Soccer page as if LPS import and Google Calendar were configured. Signed out, it requires the `Private Soccer access` notice with the `Sign in for Soccer access` link (`/sign-in?return_to=%2Fsoccer`) and both connection cards reading `Needs Soccer access`. Ungranted, require `invited.visitor@example.com` with `Sign out` in the navigation, the notice that the account has not been granted access, no Soccer sign-in link, and the same cards. Granted, require no notice, `Import access`, and a disabled `Connect Google Calendar`. `Team IDs` stays available in all three. Then, from the production `/soccer` entry, it proves that hiding controls is not the boundary: a same-origin `fetch` of each private route (`POST /soccer/import`, `/soccer/discover-teams`, `/soccer/fetch` and `/soccer/download` with `player_ids`, `/soccer/google/add`, and `GET /soccer/google/connect`) must return an uncacheable `401`, while `POST /soccer/fetch` with `team_codes` returns the public schedule. The six `401` responses appear as expected browser console errors.
- **Proof.** Retain the production entry snapshot, fixture-labelled selection before/result evidence, the downloaded ICS plus content assertions, browser request list, and server log. State explicitly that live LPS and Google were not tested.

## Gotchas

- The fake LPS publishes game `7004` for team `479691` after that team's first schedule request, and keeps the count until the server stops. Run `prove-soccer-planner` against a fresh launch; it stops with a relaunch message if `7004` is already present.
- The preview has no site sign-in, LPS key, or Google configuration, so the production `/soccer` entry shows no `Private Soccer access` notice and every private route answers `401`. The `soccer-*` account fixtures supply the grant states instead; they prove rendering around the Cognito boundary, not a live grant decision.
- Retaining an import across a browser restart, withholding it during a site-session timeout, restoring it on the same owner's next sign-in, and clearing it on site sign-out, expiry, or another owner's visit all need site sign-in, which the preview disables. Route tests with fake Cognito and LPS in `internal/app/soccer_import_retention_test.go` prove that behavior; the preview proves only its copy.
- The preview has no Google configuration, so the Google output is disabled. The journey enables that radio in the page only to drive the client-side output switch; it proves selection handling, not Google availability or a Google write.

- Never paste or record a real JWT in proof artifacts.
- The preview buttons that would mutate LPS or Google are intentionally disabled. Their disabled state is the expected safety behavior, not a live integration result.
- Preview cannot prove production Google add/result-sync enabling; record that as an external harness boundary.
- An ICS response is a real local file side effect even in preview. Preserve it as evidence and inspect its contents; a download toast alone is insufficient.
- Selection state uses browser `sessionStorage` keyed by the loaded team fingerprint. Use the run-scoped browser session and clear it when comparing unrelated fixtures.
- Live Google OAuth requires user-driven consent and configured external state. Do not claim it from local route behavior.
