package app

import (
	"bytes"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/siteidentity"
	internalsoccer "portfolio/internal/soccer"
)

// previewLinkedAccountCookie marks a loopback preview browser that opened the
// linked-player journey at /__preview/account/soccer-linked.
const previewLinkedAccountCookie = "preview_soccer_account"

// previewLinkedLPSBaseURL names the linked journey's fake LPS. The .invalid
// name never resolves, and its client answers in process.
const previewLinkedLPSBaseURL = "http://preview-lps.invalid"

// previewAccountPrincipal is the invited account the preview account fixtures
// show in place of a site Cognito sign-in.
func previewAccountPrincipal() *siteidentity.Principal {
	return &siteidentity.Principal{Issuer: "https://preview.invalid/pool", Subject: "preview-subject", Email: "invited.visitor@example.com"}
}

// previewLinkedSoccer serves the linked-player Soccer journey in the loopback
// preview. A browser holding previewLinkedAccountCookie acts as the preview's
// invited account with the soccer grant on the Soccer page and its LPS
// routes. Those run the real Soccer handlers with their own session key
// against an in-process fake LPS, so the journey replaces only site Cognito
// and LPS and never reaches a live service. Google Calendar stays off.
type previewLinkedSoccer struct {
	routes *http.ServeMux
}

func newPreviewLinkedSoccer(app *App, logger *slog.Logger) *previewLinkedSoccer {
	cfg := app.Config
	cfg.SessionKey = make([]byte, 32)
	_, _ = rand.Read(cfg.SessionKey)
	cfg.LPSAPIBaseURL = previewLinkedLPSBaseURL
	cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleConnectionTableName = "", "", ""
	lpsClient := &http.Client{Transport: inProcessTransport{handler: newSoccerPreviewLPS().linkedRoutes()}, Timeout: lpsClientTimeout}
	handler := internalsoccer.NewHandler(&cfg, lpsClient, app.LoginLimiter, nil, internalsoccer.NoopSoccerStore{}, logger.With(slog.String("component", "soccer_preview_linked")))

	routes := http.NewServeMux()
	routes.HandleFunc("/soccer", handler.SoccerPage)
	registerSoccerLPSRoutes(routes, handler)
	return &previewLinkedSoccer{routes: routes}
}

// enter marks this browser for the linked journey and opens the Soccer page.
func (p *previewLinkedSoccer) enter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, httpx.NewSecureCookie(r, previewLinkedAccountCookie, "linked", config.SoccerCookiePath, 0, http.SameSiteLaxMode))
	http.Redirect(w, r, "/soccer", http.StatusSeeOther)
}

// wrap sends a marked browser's linked-journey requests to the preview
// handlers as the granted preview account; every other request reaches the
// ordinary Soccer routes unchanged.
func (p *previewLinkedSoccer) wrap(soccerRoutes http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(previewLinkedAccountCookie); err == nil && cookie.Value == "linked" {
			if _, pattern := p.routes.Handler(r); pattern != "" {
				w.Header().Set("Cache-Control", "no-store")
				ctx := siteidentity.WithRequestIdentity(r.Context(), previewAccountPrincipal(), []siteidentity.Grant{siteidentity.GrantSoccer}, r.URL.Path)
				p.routes.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		soccerRoutes.ServeHTTP(w, r)
	})
}

// inProcessTransport answers HTTP client requests with an in-process handler.
type inProcessTransport struct {
	handler http.Handler
}

func (t inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response := &bufferedResponse{header: make(http.Header)}
	t.handler.ServeHTTP(response, req.Clone(req.Context()))
	if response.status == 0 {
		response.status = http.StatusOK
	}
	return &http.Response{
		Status:        strconv.Itoa(response.status) + " " + http.StatusText(response.status),
		StatusCode:    response.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        response.header,
		Body:          io.NopCloser(bytes.NewReader(response.body.Bytes())),
		ContentLength: int64(response.body.Len()),
		Request:       req,
	}, nil
}

// bufferedResponse collects one in-process response.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *bufferedResponse) Header() http.Header { return r.header }

func (r *bufferedResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *bufferedResponse) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(p)
}

// previewLinkedTeams lists each preview player's current team for the fake
// LPS: Craig plays for Pond Mint United and Taylor for Campfire Rovers.
var previewLinkedTeams = map[string]string{
	"1669080": `[{"UTeamID":479691,"team_name":"Pond Mint United","Season":169}]`,
	"1669081": `[{"UTeamID":479147,"team_name":"Campfire Rovers","Season":170}]`,
}

// linkedRoutes serves the LPS endpoints the linked-player journey reads:
// the imported account's players, each player's current teams, and the team
// schedules the public preview lookup also uses.
func (fake *soccerPreviewLPS) linkedRoutes() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /teams/{id}", fake.teamScheduleHandler)
	routes.HandleFunc("GET /users/check", func(w http.ResponseWriter, r *http.Request) {
		if !previewBearer(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[` +
			`{"UPlayerID":1669080,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},` +
			`{"UPlayerID":1669081,"FirstName":"Taylor Alexandra","LastName":"Johnson-Summit"}],` +
			`"user_players":[{"player_id":1669080,"deleted":false},{"player_id":1669081,"deleted":false}]}`))
	})
	routes.HandleFunc("GET /players/{id}/my_teams", func(w http.ResponseWriter, r *http.Request) {
		if !previewBearer(w, r) {
			return
		}
		// A token signed "revoked" imports, but its team lookups are then
		// refused, as when LPS withdraws access after an import.
		if strings.HasSuffix(r.Header.Get("Authorization"), ".revoked") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		teams, ok := previewLinkedTeams[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"player not found"}`))
			return
		}
		_, _ = w.Write([]byte(teams))
	})
	return routes
}

// previewBearer refuses a fake LPS account request without a bearer token,
// as LPS does, and prepares a JSON answer otherwise.
func previewBearer(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	if len(r.Header.Get("Authorization")) <= len("Bearer ") {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return false
	}
	return true
}
