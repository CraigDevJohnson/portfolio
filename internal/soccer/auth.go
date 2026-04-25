package soccer

import (
	"context"
	"crypto/rand"
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
	"portfolio/types"
)

// SessionHandler renders the current soccer login state fragment.
func (h *Handler) SessionHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := h.LoadSession(w, r)
	h.RenderLoginState(w, r, session)
}

// ImportHandler validates an imported JWT, discovers linked players, and stores the session.
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

	now := time.Now()
	sessionID := generateSessionID()
	resolver := lps.NewScheduleResolver(h.Config.LPSAPIBaseURL, h.LPSClient, jwt)
	knownTeams := fetchKnownTeams(r.Context(), resolver, discovery.Players)

	session := types.SessionData{
		JWT:        jwt,
		UserName:   discovery.UserName,
		Players:    discovery.Players,
		ExpiresAt:  lps.ImportedSessionExpiry(jwt),
		SessionID:  sessionID,
		KnownTeams: knownTeams,
		StartedAt:  now,
	}
	if err := h.setSession(w, r, &session); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer import session write failed", slog.Any("error", err))
		h.RenderLoginFeedback(w, r, "error", "The import succeeded, but the session cookie could not be saved.")
		return
	}

	go h.persistSessionRecord(sessionID, &session)

	h.setHTMLContentType(w)
	if err := partials.SoccerLoginState(h.LoginStateProps(w, r, &session, true)).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, `<div class="soccer-login-success" data-login-success>Import saved for this browser session. Choose your players below.</div>`)
}

// LogoutHandler clears the imported soccer session.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	h.RenderLoginState(w, r, nil)
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
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return &session, nil
}

// LoadSession loads the imported soccer session and reports whether it was cleared.
func (h *Handler) LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool) {
	session, err := h.getSession(r)
	if errors.Is(err, ErrSessionExpired) {
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
	encrypted, err := internalsession.EncryptJSONValue(h.Config.SessionKey, session)
	if err != nil {
		return err
	}
	http.SetCookie(w, httpx.NewSecureCookie(r, config.LPSSessionCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteStrictMode))
	return nil
}

func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	cookie := httpx.NewSecureCookie(r, config.LPSSessionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// fetchKnownTeams calls FetchPlayerTeams for each player and returns the collected teams.
// Errors from individual players are logged and skipped to avoid blocking import.
func fetchKnownTeams(ctx context.Context, resolver *lps.ScheduleResolver, players []types.LPSPlayer) []types.LPSTeam {
	var known []types.LPSTeam
	for _, player := range players {
		teams, err := resolver.FetchPlayerTeams(ctx, player.UPlayerID)
		if err != nil {
			continue
		}
		for _, t := range teams {
			if t.UTeamID <= 0 {
				continue
			}
			known = append(known, types.LPSTeam{
				TeamID:   t.UTeamID,
				TeamName: t.TeamName,
				Season:   t.Season,
				PlayerID: player.UPlayerID,
			})
		}
	}
	return known
}

func (h *Handler) persistSessionRecord(sessionID string, session *types.SessionData) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	playersJSON, err := marshalPlayersJSON(session.Players)
	if err != nil {
		h.Logger.Warn("soccer session persist: failed to marshal players", slog.Any("error", err))
		return
	}
	teamsJSON, err := marshalTeamsJSON(session.KnownTeams)
	if err != nil {
		h.Logger.Warn("soccer session persist: failed to marshal teams", slog.Any("error", err))
		return
	}

	record := &SoccerSessionRecord{
		SessionID:   sessionID,
		UserName:    session.UserName,
		PlayersJSON: playersJSON,
		TeamsJSON:   teamsJSON,
		StartedAt:   session.StartedAt,
		ExpiresAt:   session.ExpiresAt,
		TTL:         session.ExpiresAt.Unix(),
	}
	if err := h.Store.Put(ctx, record); err != nil {
		h.Logger.Warn("soccer session persist: DynamoDB write failed", slog.Any("error", err))
	}
}
