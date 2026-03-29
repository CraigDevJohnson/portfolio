package app

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	internalsession "portfolio/internal/session"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/types"
)

type loginAttempt = internalsession.LoginAttempt

type loginRateLimiter = internalsession.LoginRateLimiter

func newLoginRateLimiter(maxAttempts int, window time.Duration) *loginRateLimiter {
	return internalsession.NewLoginRateLimiter(maxAttempts, window, config.RateLimiterMaxKeys)
}

// soccerGoogleHooks implements soccer.GoogleHooks by delegating to google.Handler.
type soccerGoogleHooks struct {
	google *internalgoogle.Handler
}

func (hooks soccerGoogleHooks) HandlePageCallback(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Query().Get("code") == "" && r.URL.Query().Get("error") == "" && r.URL.Query().Get("state") == "" {
		return false
	}
	hooks.google.CallbackHandler(w, r)
	return true
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

func (b *googleSoccerBridge) RequestedScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string) ([]types.Game, error) {
	return b.soccer.RequestedScheduleGames(ctx, session, playerIDs, teamCodes)
}

func (b *googleSoccerBridge) SelectedScheduleGames(games []types.Game, selectedIDs map[string]struct{}) []types.Game {
	return internalsoccer.SelectedScheduleGames(games, selectedIDs)
}

func (b *googleSoccerBridge) GoogleAddScheduleErrorMessage(err error) string {
	return internalsoccer.GoogleAddScheduleErrorMessage(err)
}

func (b *googleSoccerBridge) ParseSelectedIDs(form url.Values) map[string]struct{} {
	return internalsoccer.ParseSelectedIDs(form)
}

func (b *googleSoccerBridge) ParsePlayerIDs(values []string) []int {
	return internalsoccer.ParsePlayerIDs(values)
}

// --- Test compatibility bridges ---
// These forwarding methods allow existing tests in internal/app to call
// Google handler methods through the App struct until Task-009 migrates tests.

func (app *App) soccerGoogleConnectHandler(w http.ResponseWriter, r *http.Request) {
	app.GoogleHandler.ConnectHandler(w, r)
}

func (app *App) soccerGoogleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	app.GoogleHandler.CallbackHandler(w, r)
}

func (app *App) soccerGoogleDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	app.GoogleHandler.DisconnectHandler(w, r)
}

func (app *App) soccerGoogleAddHandler(w http.ResponseWriter, r *http.Request) {
	app.GoogleHandler.AddHandler(w, r)
}

func (app *App) soccerGoogleCalendarHandler(w http.ResponseWriter, r *http.Request) {
	app.GoogleHandler.CalendarHandler(w, r)
}

func (app *App) newSoccerHandler() *internalsoccer.Handler {
	return internalsoccer.NewHandler(&app.Config, app.LPSClient, app.LoginLimiter, app.MountainTZ, soccerGoogleHooks{google: app.GoogleHandler})
}

func (app *App) soccerHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().SoccerPage(w, r)
}

func (app *App) soccerSessionHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().SessionHandler(w, r)
}

func (app *App) soccerImportHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().ImportHandler(w, r)
}

func (app *App) soccerLogoutHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().LogoutHandler(w, r)
}

func (app *App) fetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().FetchSchedulesHandler(w, r)
}

func (app *App) downloadICSHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().DownloadICSHandler(w, r)
}

func (app *App) subscribeHandler(w http.ResponseWriter, r *http.Request) {
	app.newSoccerHandler().SubscribeHandler(w, r)
}

func (app *App) loadSoccerSession(w http.ResponseWriter, r *http.Request) (*SessionData, bool) {
	return app.newSoccerHandler().LoadSession(w, r)
}

func (app *App) soccerLoginStateProps(w http.ResponseWriter, r *http.Request, session *SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	return app.newSoccerHandler().LoginStateProps(w, r, session, swapOOB)
}

func (app *App) renderSoccerLoginState(w http.ResponseWriter, r *http.Request, session *SessionData) {
	app.newSoccerHandler().RenderLoginState(w, r, session)
}

func renderSoccerLoginFeedback(w http.ResponseWriter, kind, message string) {
	internalsoccer.RenderLoginFeedback(w, kind, message)
}

func (app *App) requestedScheduleGames(ctx context.Context, session *SessionData, playerIDs []int, teamCodes string) ([]Game, error) {
	return app.newSoccerHandler().RequestedScheduleGames(ctx, session, playerIDs, teamCodes)
}

func googleAddScheduleErrorMessage(err error) string {
	return internalsoccer.GoogleAddScheduleErrorMessage(err)
}

func selectedScheduleGames(games []Game, selectedIDs map[string]struct{}) []Game {
	return internalsoccer.SelectedScheduleGames(games, selectedIDs)
}

func (app *App) encryptSession(data *SessionData) (string, error) {
	return internalsession.EncryptSession(app.Config.SessionKey, data)
}

func (app *App) decryptSession(value string) (SessionData, error) {
	return internalsession.DecryptSession(app.Config.SessionKey, value)
}
