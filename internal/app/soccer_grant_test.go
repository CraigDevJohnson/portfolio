package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
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
	if cookie := findSessionCookie(t, fetch.Result()); cookie != nil && (cookie.MaxAge != 0 || !cookie.Expires.IsZero()) {
		t.Fatalf("anonymous Team ID lookup set a persistent import cookie: %#v", cookie)
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
		{http.MethodPost, "/soccer/google/calendar", url.Values{"calendar_id": {"primary"}}},
		{http.MethodPost, "/soccer/google/sync-results", url.Values{"selected": {"7001"}}},
		{http.MethodPost, "/soccer/google/disconnect", nil},
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
	for _, path := range []string{"/soccer/import", "/soccer/google/add", "/soccer/google/calendar", "/soccer/google/sync-results", "/soccer/google/disconnect"} {
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
	tokenExpiry := time.Now().Add(30 * time.Minute)
	token := testutil.TestJWT(t, tokenExpiry)
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/check":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("unexpected LPS import token: %s", r.Header.Get("Authorization"))
				http.Error(w, "unexpected token", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true}],"user_players":[{"player_id":1001,"deleted":false}]}`))
		case "/teams/4101":
			future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":7001,"SchedGameDateTime":%q,"field_name":"Field 3","home_team":{"team_name":"Craig FC"},"visitor_team":{"team_name":"Rivals"},"Season":169}]}`, future)
		default:
			t.Errorf("unexpected LPS path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
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
	guardCookie := findImportGuardCookie(imported.Result())
	if lpsCookie == nil || guardCookie == nil || store.record == nil {
		t.Fatal("import did not persist the private session, its guard, and baseline")
	}
	if lpsCookie.MaxAge <= 0 || lpsCookie.MaxAge > int((30*time.Minute).Seconds()) || lpsCookie.Expires.IsZero() {
		t.Fatalf("import cookie will not survive a browser restart within JWT expiry: max-age %d, expires %v", lpsCookie.MaxAge, lpsCookie.Expires)
	}
	if guardCookie.Value == "" || guardCookie.MaxAge <= 0 || guardCookie.MaxAge > lpsCookie.MaxAge || !guardCookie.Expires.Equal(lpsCookie.Expires) || guardCookie.Path != lpsCookie.Path {
		t.Fatalf("import guard does not last exactly as long as the import: guard %#v, import %#v", guardCookie, lpsCookie)
	}
	session := decryptTestSession(t, application, lpsCookie.Value)
	if session.ExpiresAt.After(tokenExpiry) || session.ExpiresAt.Before(tokenExpiry.Add(-time.Second)) || lpsCookie.Expires.After(session.ExpiresAt) {
		t.Fatalf("import outlived JWT expiry: JWT %v, session %v, cookie %v", tokenExpiry, session.ExpiresAt, lpsCookie.Expires)
	}
	if session.OwnerIssuer != fixture.issuer || session.OwnerSubject != "stable-subject" || store.record.OwnerIssuer != fixture.issuer || store.record.OwnerSubject != "stable-subject" {
		t.Fatalf("import ownership was not the validated issuer and subject: session %#v, record %#v", session, store.record)
	}
	ownerPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie, lpsCookie, guardCookie)
	if ownerPage.Code != http.StatusOK || !strings.Contains(ownerPage.Body.String(), "Imported in this browser") || !strings.Contains(ownerPage.Body.String(), "up to 12 hours") {
		t.Fatalf("owner could not restore linked players: status %d", ownerPage.Code)
	}
	timedOutPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, lpsCookie, guardCookie)
	if timedOutPage.Code != http.StatusOK || strings.Contains(timedOutPage.Body.String(), "Imported in this browser") || findSessionCookie(t, timedOutPage.Result()) != nil {
		t.Fatalf("site timeout exposed or discarded a still-valid imported credential: status %d", timedOutPage.Code)
	}
	publicFetch := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}}, lpsCookie, guardCookie)
	if publicFetch.Code != http.StatusOK || !strings.Contains(publicFetch.Body.String(), "Craig FC") || findSessionCookie(t, publicFetch.Result()) != nil {
		t.Fatalf("public Team ID fetch after site timeout overwrote a retained import: status %d", publicFetch.Code)
	}
	stateCookie, state = beginSiteSignIn(t, mux, "/soccer")
	ownerAgain := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	restoredPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerAgain, lpsCookie, guardCookie)
	if restoredPage.Code != http.StatusOK || !strings.Contains(restoredPage.Body.String(), "Imported in this browser") {
		t.Fatal("same-owner site sign-in did not restore a still-valid imported credential")
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{}
	revokedPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerAgain, lpsCookie, guardCookie)
	if revokedPage.Code != http.StatusOK || strings.Contains(revokedPage.Body.String(), "Imported in this browser") || findSessionCookie(t, revokedPage.Result()) != nil {
		t.Fatal("current Soccer grant was not required or revocation discarded the import")
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}
	expiredJWTSession := session
	expiredJWTSession.JWT = testutil.TestJWT(t, time.Now().Add(-time.Minute))
	expiredJWTCookie := &http.Cookie{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, application, &expiredJWTSession)}
	expiredPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerAgain, expiredJWTCookie)
	if expiredPage.Code != http.StatusOK || strings.Contains(expiredPage.Body.String(), "Imported in this browser") {
		t.Fatal("an expired LPS JWT remained usable from a retained cookie")
	}
	assertClearedSessionCookie(t, expiredPage.Result())

	legacy := types.SessionData{JWT: token, Players: []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Legacy", LastName: "Player"}}, ExpiresAt: time.Now().Add(time.Hour)}
	legacyPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, append([]*http.Cookie{ownerCookie}, importedAccessCookies(t, application, &legacy)...)...)
	clearedLegacy := findSessionCookie(t, legacyPage.Result())
	if strings.Contains(legacyPage.Body.String(), "Legacy Player") || clearedLegacy == nil || clearedLegacy.Value != "" || clearedLegacy.MaxAge >= 0 {
		t.Fatal("ownerless legacy import was inherited instead of denied and cleared")
	}

	fixture.subject = "different-subject"
	stateCookie, state = beginSiteSignIn(t, mux, "/soccer")
	otherCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	otherPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, otherCookie, lpsCookie, guardCookie)
	clearedOther := findSessionCookie(t, otherPage.Result())
	if strings.Contains(otherPage.Body.String(), "Imported in this browser") || clearedOther == nil || clearedOther.Value != "" || clearedOther.MaxAge >= 0 {
		t.Fatal("a different Cognito subject inherited an imported session")
	}

	signedOut := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, lpsCookie, guardCookie)
	if signedOut.Code != http.StatusOK || strings.Contains(signedOut.Body.String(), "Imported in this browser") {
		t.Fatal("a browser without a site session used imported LPS access")
	}
}

func TestSoccerImportRejectsJWTWithoutExpiry(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("JWT without expiry reached LPS: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	response := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{
		"jwt": {"e30.eyJzdWIiOiJub2V4cCJ9.c2ln"},
	}, ownerCookie)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "valid expiry") || findSessionCookie(t, response.Result()) != nil {
		t.Fatalf("JWT without expiry was retained: status %d, body %q", response.Code, response.Body.String())
	}
}

func TestSoccerImportStopsAtTwelveHoursBeforeLongerJWT(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(48*time.Hour))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/check" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected LPS import request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson"}],"user_players":[{"player_id":1001,"deleted":false}]}`))
	}))
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	response := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Choose your players") {
		t.Fatalf("granted import failed: status %d, body %q", response.Code, response.Body.String())
	}
	importCookie := findSessionCookie(t, response.Result())
	if importCookie == nil {
		t.Fatal("import did not create a retained cookie")
	}
	session := decryptTestSession(t, application, importCookie.Value)
	if session.ExpiresAt.After(session.StartedAt.Add(12*time.Hour)) || session.ExpiresAt.Before(session.StartedAt.Add(12*time.Hour-time.Second)) || importCookie.MaxAge > 12*60*60 || importCookie.Expires.After(session.ExpiresAt) {
		t.Fatalf("import exceeded the 12-hour cap: session started %v, expires %v, cookie max-age %d, cookie expires %v", session.StartedAt, session.ExpiresAt, importCookie.MaxAge, importCookie.Expires)
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
	connectedEmail := "calendar-owner@example.net"
	googleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600}`))
		case "/calendar/v3/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true}]}`))
		case "/userinfo":
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Errorf("userinfo authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"sub":"google-subject","email":%q,"email_verified":true}`, connectedEmail)
		default:
			t.Errorf("unexpected Google request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(googleAPI.Close)
	application.GoogleHandler.OAuthAuthURL = googleAPI.URL + "/oauth/authorize"
	application.GoogleHandler.OAuthTokenURL = googleAPI.URL + "/oauth/token"
	application.GoogleHandler.CalendarAPIBaseURL = googleAPI.URL + "/calendar/v3"
	application.GoogleHandler.OAuthUserInfoURL = googleAPI.URL + "/userinfo"
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	legacyCookie := &http.Cookie{Name: config.GoogleConnectionCookieName, Value: "legacy"}
	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie, legacyCookie)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Calendar ready") || tokenCalls.Load() != 0 {
		t.Fatal("ownerless Google connection was presented or used")
	}

	// Holding the cookie was the authority to delete a connection saved
	// before connections had owners. Disconnecting or reconnecting releases
	// it rather than stranding its stored token, and neither grants access.
	legacy := store.records["legacy"]
	disconnect := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/disconnect", nil, ownerCookie, legacyCookie)
	if disconnect.Code != http.StatusOK || strings.Contains(disconnect.Body.String(), "Calendar ready") {
		t.Fatalf("disconnect status = %d", disconnect.Code)
	}
	if _, exists := store.records["legacy"]; exists {
		t.Fatal("disconnect left the ownerless Google connection and its token stored")
	}
	store.records["legacy"] = legacy
	suggestedConnect := soccerGrantRequest(mux, http.MethodGet, "/soccer/google/connect?account=suggested", nil, ownerCookie)
	suggestedURL, err := url.Parse(suggestedConnect.Header().Get("Location"))
	if err != nil || suggestedURL.Query().Get("login_hint") != "owner@example.com" {
		t.Fatalf("suggested Google account hint = %q, error %v", suggestedConnect.Header().Get("Location"), err)
	}
	connect := soccerGrantRequest(mux, http.MethodGet, "/soccer/google/connect", nil, ownerCookie, legacyCookie)
	if connect.Code != http.StatusSeeOther {
		t.Fatalf("Google connect status = %d", connect.Code)
	}
	if _, exists := store.records["legacy"]; exists {
		t.Fatal("reconnecting stranded the ownerless Google connection and its token")
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
	if got := connectURL.Query().Get("login_hint"); got != "" {
		t.Fatalf("alternate account chooser had a forced hint = %q", got)
	}
	if prompt := connectURL.Query().Get("prompt"); !strings.Contains(prompt, "select_account") || !strings.Contains(prompt, "consent") {
		t.Fatalf("Google consent did not permit alternate account choice: %q", prompt)
	}
	if scopes := connectURL.Query().Get("scope"); !strings.Contains(scopes, "openid") || !strings.Contains(scopes, "email") {
		t.Fatalf("Google consent did not request account identity: %q", scopes)
	}
	badState := soccerGrantRequest(mux, http.MethodGet, "/soccer?code=auth-code&state=wrong", nil, ownerCookie, googleStateCookie)
	if badState.Code != http.StatusSeeOther || badState.Header().Get("Location") != "/soccer?google=failed" || tokenCalls.Load() != 0 {
		t.Fatalf("mismatched Google callback state reached token exchange: status %d, redirect %q, exchanges %d", badState.Code, badState.Header().Get("Location"), tokenCalls.Load())
	}
	callbackPath := "/soccer?code=auth-code&state=" + url.QueryEscape(connectURL.Query().Get("state"))
	callback := soccerGrantRequest(mux, http.MethodGet, callbackPath, nil, ownerCookie, googleStateCookie)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/soccer?google=connected" || tokenCalls.Load() != 1 {
		t.Fatalf("granted Google callback did not connect: status %d, redirect %q, exchanges %d", callback.Code, callback.Header().Get("Location"), tokenCalls.Load())
	}
	connected := store.records[pending.ConnectionID]
	if connected.OwnerIssuer != fixture.issuer || connected.OwnerSubject != "stable-subject" || connected.AccountSubject != "google-subject" || connected.AccountEmail != connectedEmail {
		t.Fatalf("Google connection lacked verified owner: %#v", connected)
	}
	var connectionCookie *http.Cookie
	for _, cookie := range callback.Result().Cookies() {
		if cookie.Name == config.GoogleConnectionCookieName {
			connectionCookie = cookie
		}
	}
	if connectionCookie == nil {
		t.Fatal("Google callback did not set connection cookie")
	}
	ownerPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie, connectionCookie)
	if ownerPage.Code != http.StatusOK || !strings.Contains(ownerPage.Body.String(), "Connected Google account") || !strings.Contains(ownerPage.Body.String(), connectedEmail) {
		t.Fatalf("owner page omitted actual Google account: status %d", ownerPage.Code)
	}
	if strings.Contains(ownerPage.Body.String(), "Connected Google account: owner@example.com") {
		t.Fatal("site account suggestion was presented as connected account")
	}
	if !strings.Contains(ownerPage.Body.String(), "owner@example.com") || !strings.Contains(ownerPage.Body.String(), "account=suggested") {
		t.Fatal("owner page did not offer the site Google account as a suggestion")
	}
	publicPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, connectionCookie)
	if publicPage.Code != http.StatusOK || strings.Contains(publicPage.Body.String(), connectedEmail) || strings.Contains(publicPage.Body.String(), "Calendar ready") {
		t.Fatal("signed-out visitor inherited owner Google connection")
	}

	fixture.subject = "different-subject"
	stateCookie, state = beginSiteSignIn(t, mux, "/soccer")
	otherCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	otherPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, otherCookie, connectionCookie)
	if otherPage.Code != http.StatusOK || strings.Contains(otherPage.Body.String(), connectedEmail) || strings.Contains(otherPage.Body.String(), "Calendar ready") {
		t.Fatal("different site owner inherited Google connection")
	}
	otherDisconnect := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/disconnect", nil, otherCookie, connectionCookie)
	if otherDisconnect.Code != http.StatusOK {
		t.Fatalf("different owner disconnect status = %d", otherDisconnect.Code)
	}
	if _, exists := store.records[pending.ConnectionID]; !exists {
		t.Fatal("different site owner deleted the Google connection")
	}
	for _, cookie := range otherDisconnect.Result().Cookies() {
		if cookie.Name == config.GoogleConnectionCookieName && cookie.MaxAge < 0 {
			t.Fatal("different site owner cleared the Google connection cookie")
		}
	}
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
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}
	signOut := soccerGrantRequest(mux, http.MethodPost, "/sign-out", nil, ownerCookie, connectionCookie)
	if signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
	if _, exists := store.records[pending.ConnectionID]; !exists {
		t.Fatal("site sign-out deleted the owner Google connection")
	}
	for _, cookie := range signOut.Result().Cookies() {
		if cookie.Name == config.GoogleConnectionCookieName && cookie.MaxAge < 0 {
			t.Fatal("site sign-out cleared the Google connection cookie")
		}
	}
	fixture.subject = "stable-subject"
	stateCookie, state = beginSiteSignIn(t, mux, "/soccer")
	returningOwnerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	ownerPage = soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, returningOwnerCookie, connectionCookie)
	if !strings.Contains(ownerPage.Body.String(), connectedEmail) {
		t.Fatal("returning site owner did not regain Google connection")
	}
	disconnected := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/disconnect", nil, returningOwnerCookie, connectionCookie)
	if disconnected.Code != http.StatusOK {
		t.Fatalf("Google disconnect status = %d", disconnected.Code)
	}
	if _, exists := store.records[pending.ConnectionID]; exists {
		t.Fatal("Google disconnect retained the owner's connection")
	}
}

// postForm submits a form from the browser, sending the cookies it holds.
func (b *siteBrowser) postForm(path string, form url.Values) *httptest.ResponseRecorder {
	b.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://app.example.com"+path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return b.do(request)
}

func TestGrantedVisitorEntersLinkedPlayerFlowThroughSiteSession(t *testing.T) {
	cognito := newFakeSiteCognito(t)
	application := cognito.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	world := newSoccerGrantWorldFor(t, application)
	browser := newSiteBrowser(t, world.mux)

	if landing := browser.signIn("/soccer"); landing.Code != http.StatusSeeOther || landing.Header().Get("Location") != "/soccer" {
		t.Fatalf("site sign-in did not return to Soccer: %d %q", landing.Code, landing.Header().Get("Location"))
	}
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), "Import access") || strings.Contains(page.Body.String(), "Private Soccer access") {
		t.Fatal("granted Soccer page did not offer LPS import")
	}

	imported := browser.postForm("/soccer/import", url.Values{"jwt": {world.jwt}})
	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), "Import saved in this browser until its JWT expires") || !strings.Contains(imported.Body.String(), `name="player_ids"`) {
		t.Fatalf("granted import did not list linked players: status %d, body %q", imported.Code, imported.Body.String())
	}
	discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}})
	if discovered.Code != http.StatusOK || !strings.Contains(discovered.Body.String(), "Craig FC") || !strings.Contains(discovered.Body.String(), `name="team_ids"`) {
		t.Fatalf("linked-player discovery did not offer the player's teams: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	schedule := browser.postForm("/soccer/fetch", url.Values{"selection_mode": {"teams"}, "player_ids": {"1001"}, "team_ids": {"4101"}})
	if schedule.Code != http.StatusOK || !strings.Contains(schedule.Body.String(), `value="7001"`) {
		t.Fatalf("discovered-team schedule did not list the team's game: status %d, body %q", schedule.Code, schedule.Body.String())
	}
	ics := browser.postForm("/soccer/download", url.Values{"player_ids": {"1001"}, "selected": {"7001"}})
	if ics.Code != http.StatusOK || !strings.Contains(ics.Body.String(), "BEGIN:VEVENT") || !strings.Contains(ics.Body.String(), "Rivals") {
		t.Fatalf("linked-player ICS download failed: status %d, body %q", ics.Code, ics.Body.String())
	}
	if world.lpsCredentialCalls.Load() == 0 {
		t.Fatal("the linked-player flow never used the imported LPS access")
	}
}

func TestDiscoveredTeamSelectionRestoresForTheGrantedOwnerWithoutImportedAccess(t *testing.T) {
	selection := url.Values{"selection_mode": {"teams"}, "team_ids": {"4101"}, "player_ids": {"1001"}}
	for _, start := range []struct {
		name  string
		saved func(t *testing.T, mux http.Handler, site *http.Cookie) []*http.Cookie
	}{
		{
			name:  "no saved workflow",
			saved: func(*testing.T, http.Handler, *http.Cookie) []*http.Cookie { return nil },
		},
		{
			name: "a saved Team ID lookup",
			saved: func(t *testing.T, mux http.Handler, site *http.Cookie) []*http.Cookie {
				lookup := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}}, site)
				if cookie := findSessionCookie(t, lookup.Result()); cookie != nil && cookie.Value != "" {
					return []*http.Cookie{cookie}
				}
				t.Fatal("Team ID lookup did not save its workflow")
				return nil
			},
		},
	} {
		t.Run(start.name, func(t *testing.T) {
			world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
			var logs bytes.Buffer
			mux, _ := buildMux(world.app, slog.New(slog.NewTextHandler(&logs, nil)), false)
			site := testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail)

			fetch := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", selection, append(start.saved(t, mux, site), site)...)
			if fetch.Code != http.StatusOK || !strings.Contains(fetch.Body.String(), `value="7001"`) {
				t.Fatalf("discovered-team schedule: status %d, body %q", fetch.Code, fetch.Body.String())
			}
			saved := findSessionCookie(t, fetch.Result())
			if saved == nil || saved.Value == "" {
				t.Fatal("discovered-team schedule did not save the team selection")
			}

			page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, site, saved)
			if cleared := findSessionCookie(t, page.Result()); cleared != nil {
				t.Error("Soccer page discarded the owner's saved team selection")
			}
			if !strings.Contains(page.Body.String(), `value="7001"`) {
				t.Error("Soccer page did not restore the selected team's schedule")
			}
			if strings.Contains(logs.String(), internalsoccer.ErrSessionOwnerMismatch.Error()) {
				t.Errorf("the owner's own saved selection was logged as an owner mismatch: %s", logs.String())
			}
		})
	}

	t.Run("ungranted visitor", func(t *testing.T) {
		world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}, otherSiteEmail: {}})
		site := testSiteSessionCookie(t, world.app, otherSiteSubject, otherSiteEmail)
		fetch := soccerGrantRequest(world.mux, http.MethodPost, "/soccer/fetch", selection, site)
		if fetch.Code != http.StatusForbidden {
			t.Errorf("ungranted discovered-team schedule status = %d, want 403", fetch.Code)
		}
		if cookie := findSessionCookie(t, fetch.Result()); cookie != nil {
			t.Error("ungranted visitor received a saved discovered-team selection")
		}
	})
}
