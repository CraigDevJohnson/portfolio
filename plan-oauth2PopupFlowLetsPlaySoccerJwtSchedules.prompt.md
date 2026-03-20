## Plan: LPS JWT Import Flow for Soccer Page

Add a JWT import flow so users can paste a bearer JWT from their signed-in Let's Play Soccer browser session, auto-discover their linked players, and fetch real schedules — replacing mock data. The Go backend stores the JWT in an encrypted HttpOnly cookie and uses it for schedule API calls. The UX is a modal overlay with a JWT paste field (not a credentials form).

Users copy a bearer JWT from their authenticated LPS browser session and paste it into the import modal. The backend stores and forwards this token for all subsequent API calls, using AES-GCM encryption for the session cookie.

### Implemented Flow

1. User visits /soccer and sees "Import Access" button when LPS_SESSION_KEY is configured.
2. User clicks the button → modal opens with a JWT paste field.
3. User pastes their bearer JWT from letsplaysoccer.com → POST /soccer/import.
4. Server normalizes the token, calls GET /users/check, discovers linked players (filtering deleted ones), and stores the JWT in an AES-GCM encrypted HttpOnly cookie.
5. UI swaps to show the authenticated state: discovered player list with checkboxes.
6. User selects players → POST /soccer/schedules → server fetches real upcoming games via GET /players/{id}/upcoming_games for each player.
7. User selects games → POST /soccer/download → server generates and returns an ICS file.
8. Logout (POST /soccer/logout) clears the cookie and returns to the unauthenticated state.

### Relevant files

- main.go — soccerImportHandler, soccerLogoutHandler, soccerSessionHandler, fetchSchedulesHandler, downloadICSHandler, LPS API client helpers, session encryption, route registration
- types/types.go — LPSPlayer, SessionData structs
- components/pages/soccer.templ — import modal include, login state container, player-select integration
- components/layouts/base.templ — base layout (JWT paste flow requires no third-party auth scripts)
- static/css/soccer.css — modal and authenticated panel styles
- static/js/main.js — modal open/close, focus trap, Escape key, HTMX event handlers
- New: components/partials/soccer_login_modal.templ (JWT paste form), soccer_player_select.templ, soccer_login_state.templ

### Verification

1. just generate && just build — compiles without errors
2. Unit test session encrypt/decrypt roundtrip
3. Unit test LPS client functions with httptest mock server
4. Manual test: open /soccer → click "Import Access" → paste JWT → modal closes → player list appears → fetch schedules → real game data shows → logout → import button returns → page reload preserves session
5. Verify JWT never appears in HTML source, JS console, or browser network responses
6. Accessibility: tab through modal, focus trap works, Escape closes, screen reader announces dialog
7. Responsive: modal works on mobile viewports

### Decisions

- JWT paste import — server stores the token in an encrypted HttpOnly cookie and forwards it to the LPS API
- HttpOnly cookie for JWT — most secure; server manages token lifecycle
- AES-GCM encrypted cookies using Go stdlib (crypto/aes, crypto/cipher) — no external session library
- Keep team-code fallback — manual team code flow remains for non-logged-in users
- No persistent user accounts or database — session lives entirely in the encrypted cookie
- loginEnabled() requires LPS_SESSION_KEY (64-character hex, decodes to 32 bytes) to be configured

### Further Considerations

1. Token expiry: LPS JWTs have an expiration. The server checks for expired sessions and prompts the user to import a fresh token.
2. Undocumented API risk: LPS endpoints could change without notice. Add defensive error handling and clear error messages.
3. Player deduplication: Games fetched across multiple players are deduplicated using stable game fields (home/away/start/end/location/season) so the same game only appears once.
