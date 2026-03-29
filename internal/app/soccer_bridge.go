package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	internalsession "portfolio/internal/session"
	internalsoccer "portfolio/internal/soccer"
)

type loginAttempt = internalsession.LoginAttempt

type loginRateLimiter = internalsession.LoginRateLimiter

func newLoginRateLimiter(maxAttempts int, window time.Duration) *loginRateLimiter {
	return internalsession.NewLoginRateLimiter(maxAttempts, window, config.RateLimiterMaxKeys)
}

func (app *App) newSoccerHandler() *internalsoccer.Handler {
	return internalsoccer.NewHandler(&app.Config, app.LPSClient, app.LoginLimiter, app.MountainTZ, soccerGoogleHooks{app: app})
}

type soccerGoogleHooks struct {
	app *App
}

func (hooks soccerGoogleHooks) HandlePageCallback(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Query().Get("code") == "" && r.URL.Query().Get("error") == "" && r.URL.Query().Get("state") == "" {
		return false
	}
	hooks.app.soccerGoogleCallbackHandler(w, r)
	return true
}

func (hooks soccerGoogleHooks) GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if !hooks.app.Config.GoogleEnabled() {
		return false
	}
	record, err := hooks.app.loadGoogleConnectionRecord(ctx, r)
	if err != nil {
		log.Printf("google connection read failed: %v", err)
		clearGoogleConnectionCookie(w, r)
		return false
	}
	return record != nil
}

func (hooks soccerGoogleHooks) PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps) {
	if !props.GoogleAvailable {
		return
	}
	record, err := hooks.app.loadGoogleConnectionRecord(ctx, r)
	if err != nil {
		log.Printf("google connection read failed: %v", err)
		clearGoogleConnectionCookie(w, r)
		return
	}
	if record == nil {
		return
	}
	calendars, err := hooks.app.googleListCalendars(ctx, r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		var apiErr *googleAPIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			hooks.app.deleteGoogleConnection(ctx, w, r)
		}
		return
	}
	props.GoogleConnected = true
	props.GoogleCalendars = calendars
	props.SelectedGoogleCalendarID, props.GoogleCalendarSummary = hooks.app.syncGoogleCalendarSelection(ctx, record, calendars)
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
