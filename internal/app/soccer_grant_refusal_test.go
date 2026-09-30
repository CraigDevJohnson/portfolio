package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// soccerHTMXRequest sends a form the way an open Soccer page's htmx control
// does, with the browser's cookies.
func soccerHTMXRequest(handler http.Handler, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "https://app.example.com"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func TestOpenSoccerPageExplainsWhyAPrivateActionWasRefused(t *testing.T) {
	invitations := map[string][]string{testSiteEmail: {"soccer"}, otherSiteEmail: {}}
	actions := []struct {
		path string
		form url.Values
	}{
		{"/soccer/import", url.Values{"jwt": {"not-a-jwt"}}},
		{"/soccer/fetch", url.Values{"player_ids": {"1001"}}},
		{"/soccer/google/disconnect", url.Values{}},
	}
	for _, visitor := range []struct {
		name        string
		siteSession func(t *testing.T, app *App) *http.Cookie
		status      int
		explanation string
		signIn      bool
	}{
		{
			name:        "site session ended",
			status:      http.StatusUnauthorized,
			explanation: "Sign in again to use Soccer actions.",
			signIn:      true,
		},
		{
			name: "signed in without the soccer grant",
			siteSession: func(t *testing.T, app *App) *http.Cookie {
				return testSiteSessionCookie(t, app, otherSiteSubject, otherSiteEmail)
			},
			status:      http.StatusForbidden,
			explanation: "Soccer access has not been granted to this account.",
		},
	} {
		t.Run(visitor.name, func(t *testing.T) {
			world := newSoccerGrantWorld(t, invitations)
			var cookies []*http.Cookie
			if visitor.siteSession != nil {
				cookies = append(cookies, visitor.siteSession(t, world.app))
			}
			for _, action := range actions {
				resp := soccerHTMXRequest(world.mux, action.path, action.form, cookies...)
				body := resp.Body.String()
				if resp.Code != visitor.status {
					t.Errorf("%s: status = %d, want %d", action.path, resp.Code, visitor.status)
				}
				// htmx swaps an error response only when it is marked as an
				// intentional fragment, and places it inside the action's
				// own target so connection cards keep their identity.
				if resp.Header().Get("X-Portal-Fragment-Error") != "true" || resp.Header().Get("HX-Reswap") != "innerHTML" {
					t.Errorf("%s: refusal is not a swappable fragment: headers %v", action.path, resp.Header())
				}
				if !strings.HasPrefix(resp.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, `role="alert"`) || !strings.Contains(body, visitor.explanation) {
					t.Errorf("%s: refusal did not explain itself with %q: %q", action.path, visitor.explanation, body)
				}
				if !strings.Contains(body, "Team ID lookup and .ics file downloads are still available.") {
					t.Errorf("%s: refusal did not say the public paths still work: %q", action.path, body)
				}
				if offered := strings.Contains(body, `href="/sign-in?return_to=%2Fsoccer"`); offered != visitor.signIn {
					t.Errorf("%s: refusal offered site sign-in = %t, want %t", action.path, offered, visitor.signIn)
				}
			}

			plain := soccerGrantRequest(world.mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {"not-a-jwt"}}, cookies...)
			if plain.Code != visitor.status || plain.Header().Get("X-Portal-Fragment-Error") != "" || !strings.HasPrefix(plain.Header().Get("Content-Type"), "text/plain") {
				t.Errorf("direct request refusal: status %d, headers %v", plain.Code, plain.Header())
			}
		})
	}

	unconfigured := newTestApp(t)
	mux, _ := buildMux(unconfigured, unconfigured.Logger, false)
	resp := soccerHTMXRequest(mux, "/soccer/import", url.Values{"jwt": {"not-a-jwt"}})
	if body := resp.Body.String(); resp.Code != http.StatusUnauthorized || !strings.Contains(body, "site sign-in is not available here") || strings.Contains(body, "Sign in again") || strings.Contains(body, `href="/sign-in`) {
		t.Errorf("refusal where site sign-in cannot start: status %d, body %q", resp.Code, body)
	}
}
