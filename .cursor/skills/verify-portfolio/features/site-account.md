<!-- markdownlint-disable MD013 -->
# Site account

Site account covers what a visitor sees of invite-only site sign-in: public pages that need no account, the shared navigation's Sign in entry and the page it returns to, the signed-in account with Sign out, and the difference between public and granted Soccer actions. It is the one browser flow for the site identity spec (#83), but it only partly meets that spec's browser requirement: the preview never initializes site Cognito, so the browser never completes sign-in. It does not prove sign-in return or account state from a real session; cite the route tests below for those, not this flow's evidence.

## Sub-features

- `account-public-pages` follows every desktop navigation destination without an account; with site sign-in unconfigured, navigation shows neither an account nor a Sign in entry.
- `account-sign-in-entry` shows the preview fixture's Sign in entry in both navigation regions carrying the current page as `return_to`, and the sign-in landing URL keeping that destination.
- `account-signed-in` shows the preview fixture's active account and the shared `Sign out` control in both navigation regions.
- `account-soccer-grant` compares the signed-out, ungranted, and granted Soccer page, then requires `401` for direct private Soccer requests while public Team ID lookup still answers.

## How to get to it (user POV)

- Open `/` and follow the header links; no sign-in is needed for any of them.
- Where site sign-in is configured, choose `Sign in` in the header; the landing's `Sign in with Google` starts Cognito sign-in, and the callback returns to the page you came from.
- Signed in, the header shows your account email and `Sign out`.
- On `/soccer`, Team ID lookup and .ics file downloads stay public; linked players and Google Calendar need the `soccer` grant.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes and `VERIFY_URL`, `PORTFOLIO_PWCLI`, and `PORTFOLIO_PW_SESSION` are set as documented in `SKILL.md`.
- The server was launched by `control-portfolio`, so preview identities and the fake LPS are available and site sign-in is not configured.

- **Scripted proof.** Run `.cursor/skills/verify-portfolio/scripts/prove-site-account`; evidence lands under `evidence/site-account/`. Its steps live in `scripts/site-account/`, and it reuses the grant-state and direct-request steps in `scripts/soccer-access/`.
- **Public pages.** From `/`, click each `nav[aria-label='Main navigation'] a.nav-link` destination and back to Home. Require a `200` document, a visible H1, exactly one `aria-current="page"` link matching the path, and no `.site-account-email` or `a[href^='/sign-in']` in the navigation.
- **Sign in entry and its return destination.** Open `/__preview/account/signed-out` (About with sign-in available). Require `a.site-account-link` with `href="/sign-in?return_to=%2Fabout"` in both `Main navigation` and `Mobile navigation`, then click the desktop one. Require `/sign-in?return_to=%2Fabout`, the H1 `Sign in`, and `Cache-Control: no-store`. The preview has no Cognito, so the landing answers `503` with `Site sign-in is unavailable right now.`, offers no `Sign in with Google` form, and navigation stops advertising sign-in.
- **Signed-in account.** Open `/__preview/account/signed-in`. Require `invited.visitor@example.com` in `.site-account-email` and a `Sign out` button in `form[action='/sign-out']` in both navigation regions, and no Sign in entry. At `390x844`, require no horizontal overflow.
- **Soccer grant states.** Open `/__preview/account/soccer-signed-out`, `/__preview/account/soccer-ungranted`, and `/__preview/account/soccer-granted` as described in [Soccer schedule](./soccer-schedule.md), then fetch each private Soccer route from the production `/soccer` entry and require `401`, while `POST /soccer/fetch` with `team_codes` returns the public schedule.
- **Proof.** Retain the before and after snapshots and screenshots, `assertions.txt`, the command transcript, browser requests, console warnings, and `manifest.sha256`.

## Gotchas

- The spec's browser requirement for sign-in return is not met. The preview never initializes site Cognito, so the landing is a `503` without the `Sign in with Google` form, and the browser cannot complete sign-in or observe the callback's return. With sign-in unconfigured, no real page shows the Sign in entry either; only the `/__preview/account/signed-out` fixture does. `TestInvitedSiteJourneyUsesCurrentGrantsAndSignsOut` and `TestSiteSignInDeniesUninvitedAndUnsafeReturn` in `internal/app` prove the return to the starting page, the account shown from the session that sign-in produced, and the rejection of external destinations, with fake Cognito. Closing the gap needs a loopback fake Cognito behind a dedicated proof launch; the HTTPS-only domain, issuer and logout URL checks and the preview's refusal to initialize site Cognito stand in the way today.
- The `/__preview/account/*` fixtures render account state around the Cognito boundary; they do not prove a live grant decision. Route tests with fake Cognito prove grant checks on every request.
- Desktop and mobile navigation both hold account controls. Scope selectors to the named navigation landmark; the mobile one is hidden at desktop width.
- The direct private Soccer requests' `401` responses appear as expected browser console errors while that page is open.
