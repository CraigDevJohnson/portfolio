package soccer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	internalsession "portfolio/internal/session"
	"portfolio/internal/siteidentity"
	"portfolio/internal/soccerarchive"
	"portfolio/types"
)

// ImportHandler validates an imported JWT, discovers linked players, and stores
// the session. When durable collection is wired and the visitor submitted the
// disclosed import, it first records every linked player's team-season
// memberships under the site owner and refuses the import if that fails.
func (h *Handler) ImportHandler(w http.ResponseWriter, r *http.Request) {
	if !h.Config.LoginEnabled() {
		h.RenderLoginFeedback(w, r, "error", "JWT import is unavailable until the session encryption key is configured on the server.")
		return
	}
	if !h.LoginLimiter.Allow(httpx.ClientIP(r)) {
		h.RenderLoginFeedback(w, r, "error", "Too many import attempts. Wait a minute and try again.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		h.RenderLoginFeedback(w, r, "error", "Could not read the import form. Try again.")
		return
	}

	jwt, err := lps.NormalizeImportedJWT(r.FormValue("jwt"))
	if err != nil {
		h.RenderLoginFeedback(w, r, "error", err.Error())
		return
	}

	discovery, err := lps.FetchUserPlayers(r.Context(), h.Config.LPSAPIBaseURL, h.LPSClient, jwt)
	if err != nil {
		var fetchErr *lps.FetchError
		if errors.As(err, &fetchErr) {
			switch fetchErr.Kind {
			case lps.ErrorUnauthorized, lps.ErrorForbidden:
				h.RenderLoginFeedback(w, r, "error", "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and try again.")
				return
			case lps.ErrorUpstream:
				h.RenderLoginFeedback(w, r, "error", "Could not reach Let's Play Soccer to look up your players. Try again in a moment.")
				return
			}
		}
		h.RenderLoginFeedback(w, r, "error", err.Error())
		return
	}
	if len(discovery.Players) == 0 {
		h.RenderLoginFeedback(w, r, "error", "No linked players found for this account.")
		return
	}

	if message, collected := h.collectLinkedPlayerHistory(r, jwt, discovery.Players); !collected {
		h.RenderLoginFeedback(w, r, "error", message)
		return
	}
	now := time.Now()
	sessionID := generateSessionID()
	session := types.SessionData{
		JWT:         jwt,
		UserName:    discovery.UserName,
		Players:     discovery.Players,
		ExpiresAt:   lps.ImportedSessionExpiry(jwt, now),
		SessionID:   sessionID,
		StartedAt:   now,
		ImportGuard: generateSessionID(),
	}
	if principal, ok := siteidentity.PrincipalFromContext(r.Context()); ok {
		session.OwnerIssuer = principal.Issuer
		session.OwnerSubject = principal.Subject
	}
	if err := h.setSession(w, r, &session); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer import session write failed", slog.Any("error", err))
		h.RenderLoginFeedback(w, r, "error", "The import succeeded, but the session cookie could not be saved.")
		return
	}
	h.setImportGuard(w, r, &session)

	persistCtx, cancelPersist := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancelPersist()
	if err := h.persistSessionRecord(persistCtx, sessionID, &session); err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer import session persistence failed", slog.Any("error", err))
	}

	w.Header().Set("HX-Trigger", "soccer-workflow-reset")
	h.setHTMLContentType(w)
	if err := partials.SoccerLoginState(h.LoginStateProps(w, r, &session, true)).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, `<div class="soccer-login-success" data-login-success>Import saved in this browser until its JWT expires, for up to 12 hours. Choose your players below.</div>`)
}

// collectLinkedPlayerHistory records every linked player's teams in the
// durable archive when collection is enabled and the visitor submitted the
// import form that disclosed it. It reports false, with the reason to show,
// when the history the visitor accepted could not be collected in full; the
// import then stops so the visitor can try again.
func (h *Handler) collectLinkedPlayerHistory(r *http.Request, jwt string, players []types.LPSPlayer) (string, bool) {
	membershipStore, enabled := h.ArchiveStore().(soccerarchive.MembershipStore)
	if !enabled || r.FormValue(partials.SoccerHistoryNoticeField) != partials.SoccerHistoryNoticeIndefinite {
		return "", true
	}
	principal, signedIn := siteidentity.PrincipalFromContext(r.Context())
	if !signedIn || !siteidentity.HasGrantForOwner(r.Context(), siteidentity.GrantSoccer, principal.Issuer, principal.Subject) {
		return "Sign in with Soccer access before importing linked players.", false
	}
	teams, memberships, err := h.discoverImportedPlayerTeams(r.Context(), jwt, players)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer player team discovery failed", slog.Any("error", err))
		return "Could not look up every linked player. No player history was saved; try the import again.", false
	}
	persistCtx, cancelPersist := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancelPersist()
	err = membershipStore.SavePlayerDiscovery(persistCtx, &soccerarchive.PlayerDiscovery{
		OwnerIssuer: principal.Issuer, OwnerSubject: principal.Subject,
		Players: players, KnownTeams: teams, Memberships: memberships, ObservedAt: time.Now(),
	})
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer player history write failed", slog.Any("error", err))
		return historyImportFailureMessage(err), false
	}
	return "", true
}

// historyImportFailureMessage tells the visitor why linked-player history
// was not saved: a full admission budget is not a temporary failure to retry.
func historyImportFailureMessage(err error) string {
	if errors.Is(err, soccerarchive.ErrAdmissionFull) {
		return "Linked-player history collection is full. Your import was not saved; try again after capacity is reviewed."
	}
	return "Linked-player history could not be saved. Try the import again."
}

// discoverImportedPlayerTeams looks up every linked player's teams. A player
// LPS rejects as invalid (a 400 or 404) has no team-seasons to observe, so it
// keeps its identity and owner link without memberships and the lookup goes
// on. Any other failure, such as an upstream error, timeout, or rejected JWT,
// stops the discovery.
func (h *Handler) discoverImportedPlayerTeams(ctx context.Context, jwt string, players []types.LPSPlayer) ([]lps.TeamSummary, []soccerarchive.PlayerMembership, error) {
	resolver := lps.NewScheduleResolver(h.Config.LPSAPIBaseURL, h.LPSClient, jwt)
	knownTeams := make(map[int]lps.TeamSummary)
	memberships := make([]soccerarchive.PlayerMembership, 0)
	for _, player := range players {
		teams, err := resolver.FetchPlayerTeams(ctx, player.UPlayerID)
		var fetchErr *lps.FetchError
		if errors.As(err, &fetchErr) && fetchErr.Kind == lps.ErrorInvalidPlayer {
			logging.WithContext(h.Logger, ctx).Warn("soccer linked player has no LPS teams to observe", slog.Int("player_id", player.UPlayerID), slog.Any("error", err))
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		for _, team := range teams {
			if team.UTeamID <= 0 {
				continue
			}
			knownTeams[team.UTeamID] = team
			if team.Season <= 0 {
				continue
			}
			memberships = append(memberships, soccerarchive.PlayerMembership{PlayerID: player.UPlayerID, Team: team})
		}
	}
	result := make([]lps.TeamSummary, 0, len(knownTeams))
	for _, team := range knownTeams {
		result = append(result, team)
	}
	return result, memberships, nil
}

// LogoutHandler clears the imported soccer session.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	h.RenderWorkflowReset(w, r, nil)
}

// ClearImportedAccess removes retained imported LPS access from the browser.
// Explicit site sign-out calls it; a site-session timeout does not, so the
// same owner can use a still-valid import after signing in again.
func (h *Handler) ClearImportedAccess(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
}

func (h *Handler) getSession(r *http.Request) (*types.SessionData, error) {
	cookie, err := r.Cookie(config.LPSSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session types.SessionData
	err = internalsession.DecryptJSONValue(h.Config.SessionKey, cookie.Value, &session)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if session.JWT != "" && (session.ExpiresAt.IsZero() || !now.Before(lps.JWTExpiry(session.JWT))) {
		return nil, ErrSessionExpired
	}
	if !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	if (session.JWT != "" || len(session.Players) > 0) && !importGuardMatches(r, session.ImportGuard) {
		return nil, errImportGuardMismatch
	}
	hasPrivateState := session.JWT != "" || len(session.Players) > 0 || session.Workflow.Source == "imported"
	if hasPrivateState && !siteidentity.SoccerOwnerAllowed(r.Context(), session.OwnerIssuer, session.OwnerSubject) {
		if siteidentity.ForeignOwner(r.Context(), session.OwnerIssuer, session.OwnerSubject) {
			return nil, ErrSessionOwnerMismatch
		}
		return nil, errSessionWithheld
	}
	session.Workflow = normalizeWorkflowState(&session.Workflow, session.Players)
	return &session, nil
}

// LoadSession loads the imported soccer session and reports whether it was cleared.
func (h *Handler) LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool) {
	session, err := h.getSession(r)
	if errors.Is(err, errSessionWithheld) {
		return nil, false
	}
	if errors.Is(err, ErrSessionExpired) || errors.Is(err, errImportGuardMismatch) {
		h.clearSession(w, r)
		return nil, true
	}
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer session read failed", slog.Any("error", err))
		h.clearSession(w, r)
		return nil, true
	}
	return session, false
}

func (h *Handler) setSession(w http.ResponseWriter, r *http.Request, session *types.SessionData) error {
	maxAge := 0
	if session.JWT != "" {
		maxAge = int(time.Until(session.ExpiresAt).Seconds())
		if maxAge < 1 {
			return ErrSessionExpired
		}
	}
	encrypted, err := internalsession.EncryptJSONValue(h.Config.SessionKey, session)
	if err != nil {
		return err
	}
	cookie := httpx.NewSecureCookie(r, config.LPSSessionCookieName, encrypted, config.SoccerCookiePath, maxAge, http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
	if maxAge > 0 {
		cookie.Expires = session.ExpiresAt
	}
	http.SetCookie(w, cookie)
	return nil
}

// setImportGuard writes the import's guard cookie, which lasts as long as the
// import. Soccer responses rewrite lps_session with the whole payload, but
// only an import writes this cookie, so a response still in flight when
// sign-out clears both cannot bring back usable imported access.
func (h *Handler) setImportGuard(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	cookie := httpx.NewSecureCookie(r, config.LPSImportGuardCookieName, session.ImportGuard, config.SoccerCookiePath, int(time.Until(session.ExpiresAt).Seconds()), http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
	cookie.Expires = session.ExpiresAt
	http.SetCookie(w, cookie)
}

// importGuardMatches reports whether the browser still holds the guard cookie
// the import wrote with this payload.
func importGuardMatches(r *http.Request, guard string) bool {
	cookie, err := r.Cookie(config.LPSImportGuardCookieName)
	return err == nil && guard != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(guard)) == 1
}

// clearSession removes the imported session and its guard from the browser.
func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{config.LPSSessionCookieName, config.LPSImportGuardCookieName} {
		cookie := httpx.NewSecureCookie(r, name, "", config.SoccerCookiePath, -1, http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
		cookie.Expires = time.Unix(0, 0)
		http.SetCookie(w, cookie)
	}
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) persistSessionRecord(ctx context.Context, sessionID string, session *types.SessionData) error {
	playersJSON, err := marshalPlayersJSON(session.Players)
	if err != nil {
		return err
	}
	record := &SoccerSessionRecord{
		SessionID:    sessionID,
		OwnerIssuer:  session.OwnerIssuer,
		OwnerSubject: session.OwnerSubject,
		UserName:     session.UserName,
		PlayersJSON:  playersJSON,
		StartedAt:    session.StartedAt,
		ExpiresAt:    session.ExpiresAt,
		TTL:          session.ExpiresAt.Unix(),
	}
	return h.Store().Put(ctx, record)
}
