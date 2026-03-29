package app

import (
	"context"
	"net/http"

	"portfolio/cmd/web/partials"
	internalsession "portfolio/internal/session"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/types"
)

type Game = types.Game

type LPSPlayer = types.LPSPlayer

type SessionData = types.SessionData

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
