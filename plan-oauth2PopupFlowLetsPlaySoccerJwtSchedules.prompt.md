## Plan: LPS Login Flow for Soccer Page

Add a server-proxied login flow so users can authenticate with their Let's Play Soccer credentials, auto-discover their players, and fetch real schedules — replacing mock data. The Go backend proxies login to lps-api-prod.lps-test.com, stores the JWT in an encrypted HttpOnly cookie, and uses it for schedule API calls. The UX is a modal overlay (not a browser popup window), since LPS has no OAuth2 redirect support.

Key constraint: LPS uses a REST API login (POST /users/sign_in) with reCAPTCHA, not standard OAuth2. Same-origin policy blocks reading their JWT from a cross-origin popup. So the backend proxies all LPS API calls directly.

### Steps

#### Phase 1: Backend Infrastructure (blocking — all later phases depend on this)

1. Add new types in types/types.go — LPSLoginRequest, LPSUser, LPSPlayer, SessionData
2. Add session management in main.go — AES-GCM encrypted cookie functions (encryptSession, decryptSession, getSession, setSession, clearSession); encryption key from LPS_SESSION_KEY env var
3. Add LPS API client in main.go — lpsLogin(email, password, captchaToken) calling POST /users/sign_in; lpsFetchUpcomingGames(jwt, playerID) calling GET /players/{id}/upcoming_games
4. Add new routes — POST /soccer/login, POST /soccer/logout, GET /soccer/session
5. Modify existing fetchSchedulesHandler — if session exists with JWT, call real LPS API; otherwise fall back to team-code mock

#### Phase 2: Frontend Login Modal (parallel with Phase 1 after types defined)

6. Create components/partials/soccer_login_modal.templ — modal dialog with email/password form, ARIA dialog role, focus trap, HTMX post to /soccer/login
7. Create components/partials/soccer_player_select.templ — post-login player list with checkboxes (UPlayerID values), "Fetch My Schedules" button
8. Create components/partials/soccer_login_state.templ — login button (logged out) or user greeting + logout link (logged in); loaded on page load via hx-get="/soccer/session" hx-trigger="load"
9. Update components/pages/soccer.templ — add login modal include, login state container in unified-header, keep team-code form as fallback

#### Phase 3: reCAPTCHA Integration (parallel with Phase 2)

10. Add reCAPTCHA v3 script to components/layouts/base.templ — conditionally for soccer page
11. Add reCAPTCHA execution in static/js/main.js — call grecaptcha.execute() before login submit, inject token into request

#### Phase 4: Styling & JavaScript (parallel with Phase 2)

12. Add modal CSS in static/css/soccer.css — overlay backdrop, centered modal, dark theme matching existing cards, responsive full-width on mobile
13. Add modal JS in static/js/main.js — open/close modal, focus trap, Escape key, HTMX afterSwap handler (close modal on success, show error on failure)

#### Phase 5: Polish & Security (depends on Phases 1-4)

14. JWT expiry handling — check exp claim before API calls, prompt re-login if expired
15. Rate limiting — in-memory rate limiter on /soccer/login (5 attempts/IP/min)
16. Input validation — validate email format + password length server-side
17. Error states — graceful HTMX fragment responses for wrong credentials, network errors, captcha failures

### Relevant files

- main.go — new handlers (login/logout/session), LPS API client, session encryption, route registration
- types/types.go — LPSLoginRequest, LPSUser, LPSPlayer, SessionData structs
- components/pages/soccer.templ — add login modal, login state container, player-select integration
- components/layouts/base.templ — conditionally load reCAPTCHA v3 script
- static/css/soccer.css — modal styles
- static/js/main.js — modal logic, reCAPTCHA, login HTMX handlers
- New: components/partials/soccer_login_modal.templ, soccer_player_select.templ, soccer_login_state.templ

### Verification

1. just generate && just build — compiles without errors
2. Unit test session encrypt/decrypt roundtrip
3. Unit test LPS client functions with httptest mock server
4. Manual test: open /soccer → click login → enter credentials → modal closes → player list appears → fetch schedules → real game data shows → logout → login button returns → page reload preserves session
5. Verify JWT never appears in HTML source, JS console, or browser network responses
6. Accessibility: tab through modal, focus trap works, Escape closes, screen reader announces dialog
7. Responsive: modal works on mobile viewports

### Decisions

- Server-proxied login (not cross-origin popup) — LPS has no OAuth2 redirect flow
- HttpOnly cookie for JWT — most secure; server manages token lifecycle
- AES-GCM encrypted cookies using Go stdlib (crypto/aes, crypto/cipher) — no external session library
- Keep team-code fallback — manual team code flow remains for non-logged-in users
- No persistent user accounts or database — session lives entirely in the encrypted cookie

### Further Considerations

1. reCAPTCHA domain risk: LPS's reCAPTCHA key is tied to letsplaysoccer.com. Tokens generated on our domain may fail. Recommendation: Test API without captcha first; if required, try LPS's site key from our domain; if that fails, consider "paste token" fallback UX.
2. Undocumented API risk: LPS endpoints could change without notice. Add defensive error handling and clear error messages.
3. Unknown game response format: The /players/{id}/upcoming_games response isn't in the HAR. The Game type may need adjustments after testing the endpoint. Recommendation: Test the endpoint manually with the JWT first.
