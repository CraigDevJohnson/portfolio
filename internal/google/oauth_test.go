package google

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	"portfolio/types"
)

func TestConnectHandlerRedirectsToOAuth(t *testing.T) {
	store := &fakeConnectionStore{records: map[string]ConnectionRecord{}}
	h := newTestHandlerWithURLs(t, store, "https://accounts.example.com/o/oauth2/auth", "", "")

	req := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	req.Host = "example.com"
	resp := httptest.NewRecorder()

	h.ConnectHandler(resp, req)

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

func TestGoogleAvailabilityRequiresReadyConnectionStore(t *testing.T) {
	cfg := &config.Config{
		SessionKey:                []byte("0123456789abcdef0123456789abcdef"),
		GoogleClientID:            "google-client-id",
		GoogleClientSecret:        "google-client-secret",
		GoogleConnectionTableName: "google-connections",
	}
	h := NewHandler(cfg, &http.Client{Timeout: time.Second}, nil, &stubSoccerBridge{})

	if h.GoogleAvailable() {
		t.Fatal("configured Google handler reported available before its connection store was ready")
	}

	request := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	response := httptest.NewRecorder()
	h.ConnectHandler(response, request)
	if got := response.Header().Get("Location"); got != "/soccer?google=unavailable" {
		t.Fatalf("unready Google connect redirect = %q, want unavailable status", got)
	}

	h.SetStore(&fakeConnectionStore{records: map[string]ConnectionRecord{}})
	if !h.GoogleAvailable() {
		t.Fatal("configured Google handler remained unavailable after its connection store became ready")
	}
}

func TestCallbackHandlerPersistsConnection(t *testing.T) {
	store := &fakeConnectionStore{records: map[string]ConnectionRecord{}}

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

	h := newTestHandlerWithURLs(t, store, server.URL+"/oauth/auth", server.URL+"/oauth/token", server.URL+"/calendar/v3")

	// Step 1: Initiate connect to get state cookie
	connectReq := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	connectReq.Host = "example.com"
	connectResp := httptest.NewRecorder()
	h.ConnectHandler(connectResp, connectReq)

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

	// Step 2: Simulate the callback
	callbackReq := httptest.NewRequest(http.MethodGet, "/soccer?code=auth-code&state="+url.QueryEscape(stateValue), nil)
	callbackReq.Host = "example.com"
	callbackReq.AddCookie(stateCookie)
	callbackResp := httptest.NewRecorder()

	h.CallbackHandler(callbackResp, callbackReq)

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

	// Verify state cookie was used to derive connection ID
	readReq := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	readReq.AddCookie(stateCookie)
	storedState, err := h.GetOAuthStateCookie(readReq)
	if err != nil || storedState == nil {
		t.Fatalf("GetOAuthStateCookie returned %v, %v", storedState, err)
	}
	record, ok := store.records[storedState.ConnectionID]
	if !ok {
		t.Fatal("expected persisted google connection record")
	}
	if record.CalendarID != "primary" || record.CalendarSummary != "Primary Calendar" {
		t.Fatalf("unexpected stored calendar selection: %#v", record)
	}
	token, err := h.DecryptToken(record.TokenCiphertext)
	if err != nil {
		t.Fatalf("DecryptToken returned error: %v", err)
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
	if connectionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("google connection cookie SameSite = %v, want Lax for the OAuth return", connectionCookie.SameSite)
	}
}

func TestCalendarHandlerUpdatesSelection(t *testing.T) {
	store := &fakeConnectionStore{records: map[string]ConnectionRecord{}}
	h := newTestHandler(t, store)
	workflowSession := &types.SessionData{
		JWT: "still-imported",
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{101},
			SelectedTeamIDs:   []int{202},
		},
	}
	bridge := &stubSoccerBridge{session: workflowSession}
	h.Soccer = bridge
	tokenCiphertext, err := h.EncryptToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("EncryptToken returned error: %v", err)
	}
	store.records["connection-1"] = ConnectionRecord{
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

	h.CalendarAPIBaseURL = server.URL + "/calendar/v3"

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/calendar", strings.NewReader(url.Values{
		"calendar_id": {"team"},
	}.Encode()))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: config.GoogleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()

	h.CalendarHandler(resp, req)

	record := store.records["connection-1"]
	if record.CalendarID != "team" || record.CalendarSummary != "Team Calendar" {
		t.Fatalf("unexpected updated calendar selection: %#v", record)
	}
	if !strings.Contains(resp.Body.String(), "login-state-refresh-rendered") {
		t.Fatalf("expected primary auth plus granular calendar refresh, got %q", resp.Body.String())
	}
	if bridge.lastRefreshSession != workflowSession {
		t.Fatal("calendar selection refresh discarded the loaded soccer workflow session")
	}
}

func TestDisconnectHandlerUsesPrimaryAuthRefresh(t *testing.T) {
	h := newTestHandler(t, &fakeConnectionStore{records: map[string]ConnectionRecord{}})
	workflowSession := &types.SessionData{
		JWT: "still-imported",
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{101},
			SelectedTeamIDs:   []int{202},
		},
	}
	bridge := &stubSoccerBridge{session: workflowSession}
	h.Soccer = bridge
	req := httptest.NewRequest(http.MethodPost, "/soccer/google/disconnect", nil)
	resp := httptest.NewRecorder()

	h.DisconnectHandler(resp, req)

	if !strings.Contains(resp.Body.String(), "login-state-refresh-rendered") {
		t.Fatalf("expected primary auth plus granular calendar refresh, got %q", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "login-state-rendered") {
		t.Fatalf("disconnect used the primary non-OOB fragment: %q", resp.Body.String())
	}
	if bridge.lastRefreshSession != workflowSession {
		t.Fatal("Google disconnect refresh discarded the loaded soccer workflow session")
	}
}
