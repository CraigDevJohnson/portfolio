package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEncryptDecryptSessionRoundTrip(t *testing.T) {
	t.Setenv("LPS_SESSION_KEY", "test-session-secret")

	session := &SessionData{
		JWT: buildTestJWT(time.Now().Add(time.Hour)),
		User: LPSUser{
			FirstName: "Casey",
			LastName:  "Example",
			Email:     "casey@example.com",
		},
		Players: []LPSPlayer{
			{UPlayerID: 101, FirstName: "Casey", LastName: "Player", TeamName: "Orange"},
		},
	}

	encrypted, err := encryptSession(session)
	if err != nil {
		t.Fatalf("encryptSession() error = %v", err)
	}

	decrypted, err := decryptSession(encrypted)
	if err != nil {
		t.Fatalf("decryptSession() error = %v", err)
	}

	if decrypted.JWT != session.JWT {
		t.Fatalf("JWT mismatch: got %q want %q", decrypted.JWT, session.JWT)
	}
	if len(decrypted.Players) != 1 || decrypted.Players[0].UPlayerID != 101 {
		t.Fatalf("players mismatch: got %#v", decrypted.Players)
	}
}

func TestLPSLoginAndFetchSchedules(t *testing.T) {
	t.Setenv("LPS_SESSION_KEY", "test-session-secret")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/sign_in":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jwt":%q,"user":{"first_name":"Casey","last_name":"Example","email":"casey@example.com"},"players":[{"uplayer_id":101,"first_name":"Casey","last_name":"Player","team_name":"Orange"}]}`, buildTestJWT(time.Now().Add(time.Hour)))
		case "/players/101/upcoming_games":
			if got := r.Header.Get("Authorization"); got == "" {
				t.Fatalf("missing authorization header")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"games":[{"id":"game-1","datetime":"Sun 01/11/26 02:55 PM","field":"3","home":"Orange","away":"Blue","season":"168"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousBaseURL := lpsAPIBaseURL
	previousClient := lpsHTTPClient
	lpsAPIBaseURL = server.URL
	lpsHTTPClient = server.Client()
	t.Cleanup(func() {
		lpsAPIBaseURL = previousBaseURL
		lpsHTTPClient = previousClient
	})

	session, err := lpsLogin("casey@example.com", "password123", "")
	if err != nil {
		t.Fatalf("lpsLogin() error = %v", err)
	}
	if session.User.Email != "casey@example.com" {
		t.Fatalf("unexpected user: %#v", session.User)
	}

	games, err := fetchSchedulesForPlayers(session.JWT, []int{101})
	if err != nil {
		t.Fatalf("fetchSchedulesForPlayers() error = %v", err)
	}
	if len(games) != 1 || games[0].ID != "game-1" {
		t.Fatalf("unexpected games: %#v", games)
	}
}

func TestSoccerLoginSessionLogoutFlow(t *testing.T) {
	t.Setenv("LPS_SESSION_KEY", "test-session-secret")
	previousLimiter := loginLimiter
	loginLimiter = &attemptLimiter{attempts: map[string][]time.Time{}}
	t.Cleanup(func() {
		loginLimiter = previousLimiter
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/sign_in" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jwt":%q,"user":{"first_name":"Casey","last_name":"Example","email":"casey@example.com"},"players":[{"uplayer_id":101,"first_name":"Casey","last_name":"Player","team_name":"Orange"}]}`, buildTestJWT(time.Now().Add(time.Hour)))
	}))
	defer server.Close()

	previousBaseURL := lpsAPIBaseURL
	previousClient := lpsHTTPClient
	lpsAPIBaseURL = server.URL
	lpsHTTPClient = server.Client()
	t.Cleanup(func() {
		lpsAPIBaseURL = previousBaseURL
		lpsHTTPClient = previousClient
	})

	form := url.Values{
		"email":    {"casey@example.com"},
		"password": {"password123"},
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/soccer/login", strings.NewReader(form.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.RemoteAddr = "127.0.0.1:1234"

	loginRecorder := httptest.NewRecorder()
	soccerLoginHandler(loginRecorder, loginRequest)

	loginResponse := loginRecorder.Result()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginResponse.StatusCode, loginRecorder.Body.String())
	}
	if trigger := loginResponse.Header.Get("HX-Trigger"); trigger != "soccer-login-success" {
		t.Fatalf("unexpected HX-Trigger: %q", trigger)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range loginResponse.Cookies() {
		if cookie.Name == sessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected session cookie, got %#v", loginResponse.Cookies())
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/soccer/session", nil)
	sessionRequest.AddCookie(sessionCookie)
	sessionRecorder := httptest.NewRecorder()
	soccerSessionHandler(sessionRecorder, sessionRequest)

	sessionBody := sessionRecorder.Body.String()
	if !strings.Contains(sessionBody, "Welcome, Casey Example") {
		t.Fatalf("session body missing greeting: %s", sessionBody)
	}
	if !strings.Contains(sessionBody, "Fetch My Schedules") {
		t.Fatalf("session body missing player form: %s", sessionBody)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/soccer/logout", nil)
	logoutRequest.AddCookie(sessionCookie)
	logoutRecorder := httptest.NewRecorder()
	soccerLogoutHandler(logoutRecorder, logoutRequest)

	logoutResponse := logoutRecorder.Result()
	if logoutResponse.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d", logoutResponse.StatusCode)
	}
	if got := logoutResponse.Header.Get("HX-Trigger"); got != "soccer-logout-success" {
		t.Fatalf("unexpected logout trigger: %q", got)
	}
	if len(logoutResponse.Cookies()) == 0 || logoutResponse.Cookies()[0].MaxAge != -1 {
		t.Fatalf("expected cleared cookie, got %#v", logoutResponse.Cookies())
	}
}

func buildTestJWT(expiry time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, expiry.Unix())))
	return header + "." + payload + ".signature"
}
