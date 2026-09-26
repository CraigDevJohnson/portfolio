package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/internal/testutil"
	"portfolio/types"
)

func soccerGrantRequest(handler http.Handler, method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, "https://app.example.com"+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func TestSoccerRoutesKeepTeamSchedulesPublicAndGatePrivateActions(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/4101" {
			t.Errorf("unexpected LPS path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":7001,"SchedGameDateTime":%q,"field_name":"Field 3","home_team":{"team_name":"Craig FC"},"visitor_team":{"team_name":"Rivals"},"Season":169}]}`, future)
	}))
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL
	mux, _ := buildMux(application, application.Logger, false)

	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Import access") || strings.Contains(page.Body.String(), "Connect Google Calendar") {
		t.Fatalf("anonymous Soccer page exposed private controls: status %d", page.Code)
	}
	fetch := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}})
	if fetch.Code != http.StatusOK || !strings.Contains(fetch.Body.String(), "Craig FC") {
		t.Fatalf("public Team ID fetch failed: status %d, body %q", fetch.Code, fetch.Body.String())
	}
	ics := soccerGrantRequest(mux, http.MethodPost, "/soccer/download", url.Values{"team_codes": {"4101"}, "selected": {"7001"}})
	if ics.Code != http.StatusOK || ics.Header().Get("Content-Type") != "text/calendar" || !strings.Contains(ics.Body.String(), "BEGIN:VCALENDAR") {
		t.Fatalf("public ICS download failed: status %d, body %q", ics.Code, ics.Body.String())
	}
	for _, action := range []struct {
		method string
		path   string
		form   url.Values
	}{
		{http.MethodPost, "/soccer/import", url.Values{"jwt": {"token"}}},
		{http.MethodPost, "/soccer/discover-teams", url.Values{"player_ids": {"1001"}}},
		{http.MethodPost, "/soccer/fetch", url.Values{"player_ids": {"1001"}}},
		{http.MethodPost, "/soccer/fetch", url.Values{"selection_mode": {"teams"}, "team_ids": {"4101"}}},
		{http.MethodPost, "/soccer/download", url.Values{"player_ids": {"1001"}, "selected": {"7001"}}},
		{http.MethodGet, "/soccer/google/connect", nil},
		{http.MethodPost, "/soccer/google/add", url.Values{"selected": {"7001"}}},
	} {
		resp := soccerGrantRequest(mux, action.method, action.path, action.form)
		if resp.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s = %d, want 401", action.method, action.path, resp.Code)
		}
	}

	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	accountCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	application.Config.SiteInvitations["owner@example.com"] = []string{}
	deniedPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, accountCookie)
	if deniedPage.Code != http.StatusOK || !strings.Contains(deniedPage.Body.String(), "has not been granted") || strings.Contains(deniedPage.Body.String(), "Import access") {
		t.Fatal("revoked grant remained visible on the Soccer page")
	}
	for _, path := range []string{"/soccer/import", "/soccer/google/add"} {
		resp := soccerGrantRequest(mux, http.MethodPost, path, url.Values{}, accountCookie)
		if resp.Code != http.StatusForbidden {
			t.Errorf("revoked grant %s = %d, want 403", path, resp.Code)
		}
	}
	if resp := soccerGrantRequest(mux, http.MethodGet, "/soccer/google/connect", nil, accountCookie); resp.Code != http.StatusForbidden {
		t.Errorf("revoked grant Google connect = %d, want 403", resp.Code)
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}
	if resp := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {"bad"}}, accountCookie); resp.Code != http.StatusOK {
		t.Errorf("restored grant did not admit import handler: %d", resp.Code)
	}
}

type soccerGrantTestStore struct {
	record *internalsoccer.SoccerSessionRecord
}

func (store *soccerGrantTestStore) Put(_ context.Context, record *internalsoccer.SoccerSessionRecord) error {
	clone := *record
	store.record = &clone
	return nil
}

func TestSoccerImportBindsPrivateSessionToVerifiedSubject(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/check" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected LPS import request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true}],"user_players":[{"player_id":1001,"deleted":false}]}`))
	}))
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL
	mux, soccerHandler := buildMux(application, application.Logger, false)
	store := &soccerGrantTestStore{}
	soccerHandler.SetStore(store)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	imported := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), "Choose your players") {
		t.Fatalf("granted import failed: status %d, body %q", imported.Code, imported.Body.String())
	}
	lpsCookie := findSessionCookie(t, imported.Result())
	if lpsCookie == nil || store.record == nil {
		t.Fatal("import did not persist the private session and baseline")
	}
	session := decryptTestSession(t, application, lpsCookie.Value)
	if session.OwnerIssuer != fixture.issuer || session.OwnerSubject != "stable-subject" || store.record.OwnerIssuer != fixture.issuer || store.record.OwnerSubject != "stable-subject" {
		t.Fatalf("import ownership was not the validated issuer and subject: session %#v, record %#v", session, store.record)
	}
	ownerPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie, lpsCookie)
	if ownerPage.Code != http.StatusOK || !strings.Contains(ownerPage.Body.String(), "Imported for this session") {
		t.Fatalf("owner could not restore linked players: status %d", ownerPage.Code)
	}

	legacy := types.SessionData{JWT: token, Players: []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Legacy", LastName: "Player"}}, ExpiresAt: time.Now().Add(time.Hour)}
	legacyCookie := &http.Cookie{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, application, &legacy)}
	legacyPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie, legacyCookie)
	clearedLegacy := findSessionCookie(t, legacyPage.Result())
	if strings.Contains(legacyPage.Body.String(), "Legacy Player") || clearedLegacy == nil || clearedLegacy.Value != "" || clearedLegacy.MaxAge >= 0 {
		t.Fatal("ownerless legacy import was inherited instead of denied and cleared")
	}

	fixture.subject = "different-subject"
	stateCookie, state = beginSiteSignIn(t, mux, "/soccer")
	otherCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	otherPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, otherCookie, lpsCookie)
	clearedOther := findSessionCookie(t, otherPage.Result())
	if strings.Contains(otherPage.Body.String(), "Imported for this session") || clearedOther == nil || clearedOther.Value != "" || clearedOther.MaxAge >= 0 {
		t.Fatal("a different Cognito subject inherited an imported session")
	}

	signedOut := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, lpsCookie)
	if signedOut.Code != http.StatusOK || strings.Contains(signedOut.Body.String(), "Imported for this session") {
		t.Fatal("a browser without a site session used imported LPS access")
	}
}

func TestSoccerGoogleRejectsOwnerlessConnectionAndPendingState(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	application.Config.GoogleClientID = "google-client"
	application.Config.GoogleClientSecret = "google-secret"
	application.Config.GoogleConnectionTableName = "connections"
	store := &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{
		"legacy": {ConnectionID: "legacy", TokenCiphertext: "old-token"},
	}}
	application.GoogleHandler.SetStore(store)
	var tokenCalls atomic.Int32
	googleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600}`))
		case "/calendar/v3/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true}]}`))
		default:
			t.Errorf("unexpected Google request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(googleAPI.Close)
	application.GoogleHandler.OAuthAuthURL = googleAPI.URL + "/oauth/authorize"
	application.GoogleHandler.OAuthTokenURL = googleAPI.URL + "/oauth/token"
	application.GoogleHandler.CalendarAPIBaseURL = googleAPI.URL + "/calendar/v3"
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	legacyCookie := &http.Cookie{Name: config.GoogleConnectionCookieName, Value: "legacy"}
	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie, legacyCookie)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Calendar ready") || tokenCalls.Load() != 0 {
		t.Fatal("ownerless Google connection was presented or used")
	}

	disconnect := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/disconnect", nil, ownerCookie, legacyCookie)
	if disconnect.Code != http.StatusOK {
		t.Fatalf("disconnect status = %d", disconnect.Code)
	}
	if _, exists := store.records["legacy"]; !exists {
		t.Fatal("a new site owner deleted the ownerless Google connection")
	}
	connect := soccerGrantRequest(mux, http.MethodGet, "/soccer/google/connect", nil, ownerCookie, legacyCookie)
	if connect.Code != http.StatusSeeOther {
		t.Fatalf("Google connect status = %d", connect.Code)
	}
	var googleStateCookie *http.Cookie
	for _, cookie := range connect.Result().Cookies() {
		if cookie.Name == config.GoogleOAuthStateCookieName {
			googleStateCookie = cookie
		}
	}
	if googleStateCookie == nil {
		t.Fatal("Google connect did not set a pending OAuth state")
	}
	readState := httptest.NewRequest(http.MethodGet, "https://app.example.com/soccer", nil)
	readState.AddCookie(googleStateCookie)
	pending, err := application.GoogleHandler.GetOAuthStateCookie(readState)
	if err != nil || pending == nil || pending.ConnectionID == "legacy" || pending.OwnerIssuer != fixture.issuer || pending.OwnerSubject != "stable-subject" {
		t.Fatalf("Google connect reused another owner's connection or failed to bind pending state: %#v, %v", pending, err)
	}
	connectURL, err := url.Parse(connect.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callbackPath := "/soccer?code=auth-code&state=" + url.QueryEscape(connectURL.Query().Get("state"))
	callback := soccerGrantRequest(mux, http.MethodGet, callbackPath, nil, ownerCookie, googleStateCookie)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/soccer?google=connected" || tokenCalls.Load() != 1 {
		t.Fatalf("granted Google callback did not connect: status %d, redirect %q, exchanges %d", callback.Code, callback.Header().Get("Location"), tokenCalls.Load())
	}
	connected := store.records[pending.ConnectionID]
	if connected.OwnerIssuer != fixture.issuer || connected.OwnerSubject != "stable-subject" {
		t.Fatalf("Google connection lacked verified owner: %#v", connected)
	}

	fixture.subject = "different-subject"
	stateCookie, state = beginSiteSignIn(t, mux, "/soccer")
	otherCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	otherCallback := soccerGrantRequest(mux, http.MethodGet, callbackPath, nil, otherCookie, googleStateCookie)
	if otherCallback.Code != http.StatusSeeOther || otherCallback.Header().Get("Location") != "/soccer?google=failed" || tokenCalls.Load() != 1 {
		t.Fatalf("another owner reused pending Google consent: status %d, redirect %q, exchanges %d", otherCallback.Code, otherCallback.Header().Get("Location"), tokenCalls.Load())
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{}
	revokedCallback := soccerGrantRequest(mux, http.MethodGet, callbackPath, nil, ownerCookie, googleStateCookie)
	if revokedCallback.Code != http.StatusForbidden || tokenCalls.Load() != 1 {
		t.Fatalf("revoked grant reached Google callback: status %d, exchanges %d", revokedCallback.Code, tokenCalls.Load())
	}
	signedOutCallback := soccerGrantRequest(mux, http.MethodGet, callbackPath, nil, googleStateCookie)
	if signedOutCallback.Code != http.StatusUnauthorized || tokenCalls.Load() != 1 {
		t.Fatalf("signed-out browser reached Google callback: status %d, exchanges %d", signedOutCallback.Code, tokenCalls.Load())
	}
}
