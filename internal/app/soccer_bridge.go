package app

import (
	"context"
	"net/http"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	internalsession "portfolio/internal/session"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/types"
)

func newLoginRateLimiter(maxAttempts int, window time.Duration) *internalsession.LoginRateLimiter {
	return internalsession.NewLoginRateLimiter(maxAttempts, window, config.RateLimiterMaxKeys)
}

// soccerGoogleHooks implements soccer.GoogleHooks by delegating to google.Handler.
type soccerGoogleHooks struct {
	google *internalgoogle.Handler
}

func (hooks soccerGoogleHooks) GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	return hooks.google.Connected(ctx, w, r)
}

func (hooks soccerGoogleHooks) PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps) {
	hooks.google.PopulateLoginState(ctx, w, r, props)
}

// googleSoccerBridge implements google.SoccerBridge by delegating to soccer.Handler.
type googleSoccerBridge struct {
	soccer *internalsoccer.Handler
}

func newGoogleSoccerBridge(h *internalsoccer.Handler) *googleSoccerBridge {
	return &googleSoccerBridge{soccer: h}
}

func (b *googleSoccerBridge) LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool) {
	return b.soccer.LoadSession(w, r)
}

func (b *googleSoccerBridge) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	return b.soccer.LoginStateProps(w, r, session, swapOOB)
}

func (b *googleSoccerBridge) RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	b.soccer.RenderLoginState(w, r, session)
}

func (b *googleSoccerBridge) RenderLoginFeedback(w http.ResponseWriter, kind, message string) {
	internalsoccer.RenderLoginFeedback(w, kind, message)
}

func (b *googleSoccerBridge) ResolveGoogleAddSelection(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool) {
	return b.soccer.ResolveGoogleAddSelection(w, r)
}
