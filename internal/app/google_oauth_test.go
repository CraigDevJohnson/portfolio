package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
)

func TestSoccerGoogleConnectHandlerRedirectsToOAuth(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}
	app := newTestAppWithGoogle(t, store, "https://accounts.example.com/o/oauth2/auth", "", "")

	req := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	req.Host = "example.com"
	resp := httptest.NewRecorder()

	app.soccerGoogleConnectHandler(resp, req)

	result := resp.Result()
	if result.StatusCode != http.StatusSeeOther {
		t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusSeeOther)
	}
	location, err := result.Location()
	if err != nil {
		t.Fatalf("result.Location returned error: %v", err)
	}
	if location.Scheme != "https" || location.Host != "accounts.example.com" {
		t.Fatalf("unexpected redirect host: %s", location.String())
	}
	if got := location.Query().Get("redirect_uri"); got != "http://example.com/soccer" {
		t.Fatalf("unexpected redirect_uri: got %q want %q", got, "http://example.com/soccer")
	}
	if location.Query().Get("state") == "" {
		t.Fatal("expected oauth state query parameter")
	}

	var stateCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == config.GoogleOAuthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth state cookie to be set")
	}
	if stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected SameSite for state cookie: %v", stateCookie.SameSite)
	}
}

func TestSoccerGoogleCallbackHandlerPersistsConnection(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("token request ParseForm error: %v", err)
			}
			if got := r.FormValue("code"); got != "auth-code" {
				t.Fatalf("unexpected authorization code: %q", got)
			}
			if got := r.FormValue("redirect_uri"); got != "http://example.com/soccer" {
				t.Fatalf("unexpected token redirect_uri: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600}`))
		case "/calendar/v3/users/me/calendarList":
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true},{"id":"team","summary":"Team Calendar","primary":false}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := newTestAppWithGoogle(t, store, server.URL+"/oauth/auth", server.URL+"/oauth/token", server.URL+"/calendar/v3")

	connectReq := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	connectReq.Host = "example.com"
	connectResp := httptest.NewRecorder()
	app.soccerGoogleConnectHandler(connectResp, connectReq)

	connectResult := connectResp.Result()
	location, err := connectResult.Location()
	if err != nil {
		t.Fatalf("connect redirect location error: %v", err)
	}
	stateValue := location.Query().Get("state")
	if stateValue == "" {
		t.Fatal("expected oauth state query parameter")
	}

	var stateCookie *http.Cookie
	for _, cookie := range connectResult.Cookies() {
		if cookie.Name == config.GoogleOAuthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth state cookie to be set")
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/soccer?code=auth-code&state="+url.QueryEscape(stateValue), nil)
	callbackReq.Host = "example.com"
	callbackReq.AddCookie(stateCookie)
	callbackResp := httptest.NewRecorder()

	app.soccerHandler(callbackResp, callbackReq)

	callbackResult := callbackResp.Result()
	if callbackResult.StatusCode != http.StatusSeeOther {
		t.Fatalf("unexpected callback status code: got %d want %d", callbackResult.StatusCode, http.StatusSeeOther)
	}
	callbackLocation, err := callbackResult.Location()
	if err != nil {
		t.Fatalf("callback redirect location error: %v", err)
	}
	if got := callbackLocation.String(); got != "/soccer?google=connected" {
		t.Fatalf("unexpected callback redirect: got %q want %q", got, "/soccer?google=connected")
	}

	cookieReq := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	cookieReq.AddCookie(stateCookie)
	storedState, err := app.getGoogleOAuthStateCookie(cookieReq)
	if err != nil || storedState == nil {
		t.Fatalf("getGoogleOAuthStateCookie returned %v, %v", storedState, err)
	}
	record, ok := store.records[storedState.ConnectionID]
	if !ok {
		t.Fatal("expected persisted google connection record")
	}
	if record.CalendarID != "primary" || record.CalendarSummary != "Primary Calendar" {
		t.Fatalf("unexpected stored calendar selection: %#v", record)
	}
	token, err := app.decryptGoogleToken(record.TokenCiphertext)
	if err != nil {
		t.Fatalf("decryptGoogleToken returned error: %v", err)
	}
	if token.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected stored refresh token: %#v", token)
	}

	var connectionCookie *http.Cookie
	for _, cookie := range callbackResult.Cookies() {
		if cookie.Name == config.GoogleConnectionCookieName {
			connectionCookie = cookie
			break
		}
	}
	if connectionCookie == nil {
		t.Fatal("expected persistent google connection cookie")
	}
}

func TestSoccerGoogleCalendarHandlerUpdatesSelection(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}
	app := newTestAppWithGoogle(t, store, "", "", "")
	tokenCiphertext, err := app.encryptGoogleToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("encryptGoogleToken returned error: %v", err)
	}
	store.records["connection-1"] = googleConnectionRecord{
		ConnectionID:    "connection-1",
		TokenCiphertext: tokenCiphertext,
		CalendarID:      "primary",
		CalendarSummary: "Primary Calendar",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/v3/users/me/calendarList" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true},{"id":"team","summary":"Team Calendar","primary":false}]}`))
	}))
	defer server.Close()

	app.GoogleCalendarAPIBaseURL = server.URL + "/calendar/v3"

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/calendar", strings.NewReader(url.Values{
		"calendar_id": {"team"},
	}.Encode()))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: config.GoogleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()

	app.soccerGoogleCalendarHandler(resp, req)

	record := store.records["connection-1"]
	if record.CalendarID != "team" || record.CalendarSummary != "Team Calendar" {
		t.Fatalf("unexpected updated calendar selection: %#v", record)
	}
	if !strings.Contains(resp.Body.String(), "Selected calendar: Team Calendar") {
		t.Fatalf("expected selected calendar in response body, got %q", resp.Body.String())
	}
}
