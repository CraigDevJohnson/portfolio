// Shared cookie builders, encryption helpers, and test-compatibility bridges for internal/google types.
package app

import (
	"net/http"

	"golang.org/x/oauth2"

	internalgoogle "portfolio/internal/google"
	internalhttpx "portfolio/internal/httpx"
	internalsession "portfolio/internal/session"
)

func newSecureCookie(r *http.Request, name, value, path string, maxAge int, sameSite http.SameSite) *http.Cookie {
	return internalhttpx.NewSecureCookie(r, name, value, path, maxAge, sameSite)
}

func (app *App) encryptJSONValue(data any) (string, error) {
	return internalsession.EncryptJSONValue(app.Config.SessionKey, data)
}

func (app *App) decryptJSONValue(value string, out any) error {
	return internalsession.DecryptJSONValue(app.Config.SessionKey, value, out)
}

// Google cookie bridges retained for test compatibility.

func getGoogleConnectionID(r *http.Request) string {
	return internalgoogle.GetConnectionID(r)
}

func setGoogleConnectionCookie(w http.ResponseWriter, r *http.Request, connectionID string) {
	internalgoogle.SetConnectionCookie(w, r, connectionID)
}

func clearGoogleConnectionCookie(w http.ResponseWriter, r *http.Request) {
	internalgoogle.ClearConnectionCookie(w, r)
}

func (app *App) setGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request, state googleOAuthState) error {
	return app.GoogleHandler.SetOAuthStateCookie(w, r, state)
}

func (app *App) getGoogleOAuthStateCookie(r *http.Request) (*googleOAuthState, error) {
	return app.GoogleHandler.GetOAuthStateCookie(r)
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	internalgoogle.ClearOAuthStateCookie(w, r)
}

// Google token bridges retained for test compatibility.

func (app *App) encryptGoogleToken(token any) (string, error) {
	return app.encryptJSONValue(token)
}

func (app *App) decryptGoogleToken(ciphertext string) (*oauth2.Token, error) {
	return app.GoogleHandler.DecryptToken(ciphertext)
}

func (app *App) loadGoogleConnectionRecord(r *http.Request) (*googleConnectionRecord, error) {
	return app.GoogleHandler.LoadConnectionRecord(r.Context(), r)
}

// Google calendar/event bridges retained for test compatibility.

func googleEventPayload(r *http.Request, game *Game) (googleEvent, bool) {
	return internalgoogle.EventPayload(r, game)
}

func googleEventMatchesGameID(event *googleEvent, gameID string) bool {
	return internalgoogle.EventMatchesGameID(event, gameID)
}

func (app *App) setGoogleConnectionStore(store googleConnectionStore) {
	app.GoogleHandler.SetStore(store)
}

func (app *App) currentGoogleConnectionStore() googleConnectionStore {
	return app.GoogleHandler.Store()
}
