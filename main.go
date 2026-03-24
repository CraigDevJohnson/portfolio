package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"golang.org/x/oauth2"

	"portfolio/components/partials"
	"portfolio/types"
)

type googleConnectionRecord struct {
	ConnectionID    string    `dynamodbav:"connection_id"`
	TokenCiphertext string    `dynamodbav:"token_ciphertext"`
	CalendarID      string    `dynamodbav:"calendar_id"`
	CalendarSummary string    `dynamodbav:"calendar_summary"`
	CreatedAt       time.Time `dynamodbav:"created_at"`
	UpdatedAt       time.Time `dynamodbav:"updated_at"`
}

type googleConnectionStore interface {
	Delete(ctx context.Context, connectionID string) error
	Get(ctx context.Context, connectionID string) (*googleConnectionRecord, error)
	Put(ctx context.Context, record *googleConnectionRecord) error
}

type dynamoGoogleConnectionStore struct {
	client    *dynamodb.Client
	tableName string
}

type noopGoogleConnectionStore struct{}

type googleOAuthState struct {
	ConnectionID string    `json:"connection_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	State        string    `json:"state"`
}

type googleCalendarListResponse struct {
	Items []googleCalendar `json:"items"`
}

type googleEventListResponse struct {
	Items []googleEvent `json:"items"`
}

type googleCalendar struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
	Summary string `json:"summary"`
}

type googleEventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

type googleEvent struct {
	Description        string              `json:"description,omitempty"`
	End                googleEventDateTime `json:"end"`
	ExtendedProperties struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties"`
	ID       string              `json:"id,omitempty"`
	Location string              `json:"location,omitempty"`
	Source   *googleEventSource  `json:"source,omitempty"`
	Start    googleEventDateTime `json:"start"`
	Status   string              `json:"status,omitempty"`
	Summary  string              `json:"summary"`
}

type googleEventSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type googleAPIError struct {
	StatusCode int
	Message    string
}

func (err *googleAPIError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return "google api request failed"
}

func lpsAPIEndpoint(pathParts ...string) (string, error) {
	baseURL, err := normalizeLPSAPIBaseURL(configData.LPSAPIBaseURL)
	if err != nil {
		return "", err
	}
	return url.JoinPath(baseURL, pathParts...)
}

func newLPSAPIRequest(ctx context.Context, method, bearerToken string, pathParts ...string) (*http.Request, error) {
	endpoint, err := lpsAPIEndpoint(pathParts...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err := validateLPSAPIRequest(req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return req, nil
}

func validateLPSAPIRequest(req *http.Request) error {
	if req == nil || req.URL == nil {
		return errors.New("LPS API request URL is required")
	}

	if req.URL.RawQuery != "" || req.URL.Fragment != "" {
		return errors.New("LPS API requests cannot include query or fragment")
	}
	return nil
}

func doLPSAPIRequest(req *http.Request) (*http.Response, error) {
	if err := validateLPSAPIRequest(req); err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req) //nolint:gosec // Request URLs are rebuilt from normalizeLPSAPIBaseURL and revalidated here.
}

func newGoogleConnectionStore(ctx context.Context, config *serverConfig) (googleConnectionStore, error) {
	if strings.TrimSpace(config.GoogleConnectionTableName) == "" {
		return noopGoogleConnectionStore{}, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &dynamoGoogleConnectionStore{
		client:    dynamodb.NewFromConfig(awsCfg),
		tableName: config.GoogleConnectionTableName,
	}, nil
}

func currentGoogleConnectionStore() googleConnectionStore {
	googleConnectionsMu.RLock()
	defer googleConnectionsMu.RUnlock()
	return googleConnections
}

func setGoogleConnectionStore(store googleConnectionStore) {
	googleConnectionsMu.Lock()
	googleConnections = store
	googleConnectionsMu.Unlock()
}

func (noopGoogleConnectionStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (noopGoogleConnectionStore) Get(_ context.Context, _ string) (*googleConnectionRecord, error) {
	return nil, nil
}

func (noopGoogleConnectionStore) Put(_ context.Context, _ *googleConnectionRecord) error {
	return nil
}

func (store *dynamoGoogleConnectionStore) Delete(ctx context.Context, connectionID string) error {
	if strings.TrimSpace(connectionID) == "" {
		return nil
	}
	_, err := store.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"connection_id": &dynamodbTypes.AttributeValueMemberS{Value: connectionID},
		},
		TableName: &store.tableName,
	})
	return err
}

func (store *dynamoGoogleConnectionStore) Get(ctx context.Context, connectionID string) (*googleConnectionRecord, error) {
	if strings.TrimSpace(connectionID) == "" {
		return nil, nil
	}
	output, err := store.client.GetItem(ctx, &dynamodb.GetItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"connection_id": &dynamodbTypes.AttributeValueMemberS{Value: connectionID},
		},
		TableName: &store.tableName,
	})
	if err != nil || len(output.Item) == 0 {
		return nil, err
	}
	var record googleConnectionRecord
	if err := attributevalue.UnmarshalMap(output.Item, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (store *dynamoGoogleConnectionStore) Put(ctx context.Context, record *googleConnectionRecord) error {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return err
	}
	_, err = store.client.PutItem(ctx, &dynamodb.PutItemInput{
		Item:      item,
		TableName: &store.tableName,
	})
	return err
}

func main() {
	mimeTypes := map[string]string{
		".css":  "text/css",
		".js":   "application/javascript",
		".ico":  "image/x-icon",
		".svg":  "image/svg+xml",
		".webp": "image/webp",
		".png":  "image/png",
		".jpg":  "image/jpeg",
	}
	for ext, mtype := range mimeTypes {
		if err := mime.AddExtensionType(ext, mtype); err != nil {
			log.Fatalf("Failed to add MIME type for %s: %v", ext, err)
		}
	}

	// routes - pages
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/about", aboutHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/experience/timeline", experienceTimelineHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/skills/grid", skillsGridHandler)
	http.HandleFunc("/skills/filtered", skillsFilteredHandler)
	http.HandleFunc("/skills/detail", skillsDetailHandler)
	http.HandleFunc("/projects", projectsHandler)
	http.HandleFunc("/projects/grid", projectsGridHandler)
	http.HandleFunc("/education", educationHandler)
	http.HandleFunc("/contact", contactHandler)

	// soccer routes
	http.HandleFunc("/soccer", soccerHandler)
	http.HandleFunc("/soccer/session", soccerSessionHandler)
	http.HandleFunc("/soccer/import", soccerImportHandler)
	http.HandleFunc("/soccer/logout", soccerLogoutHandler)
	http.HandleFunc("/soccer/google/add", soccerGoogleAddHandler)
	http.HandleFunc("/soccer/google/calendar", soccerGoogleCalendarHandler)
	http.HandleFunc("/soccer/google/connect", soccerGoogleConnectHandler)
	http.HandleFunc("/soccer/google/disconnect", soccerGoogleDisconnectHandler)
	http.HandleFunc("/soccer/fetch", fetchSchedulesHandler)
	http.HandleFunc("/soccer/download", downloadICSHandler)
	http.HandleFunc("/soccer/subscribe", subscribeHandler)

	// static files
	http.Handle(
		"/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static")),
		),
	)

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/images/favicon.ico")
	})

	// Bind the listener before any slow init so health checks pass immediately.
	listenAddress := serverListenAddress()
	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddress, err)
	}

	server := &http.Server{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Craig Johnson Portfolio running at %s", localServerURL(listenAddress))
		log.Fatal(server.Serve(ln))
	}()

	// Initialize the Google connection store in the background so App Runner
	// health checks never wait on AWS SDK startup or credential resolution.
	if googleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer initCancel()
			store, initErr := newGoogleConnectionStore(initCtx, &configData)
			if initErr != nil {
				log.Printf("google calendar add disabled: could not initialize connection store: %v", initErr)
				return
			}
			setGoogleConnectionStore(store)
		}()
	}

	select {}
}

func redirectSoccerWithGoogleStatus(w http.ResponseWriter, r *http.Request, status string) {
	target := "/soccer"
	if strings.TrimSpace(status) != "" {
		target += "?google=" + url.QueryEscape(status)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func soccerGoogleConnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !googleEnabled() {
		redirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	connectionID := getGoogleConnectionID(r)
	if connectionID == "" {
		var err error
		connectionID, err = newRandomHex(16)
		if err != nil {
			log.Printf("google connection id generation failed: %v", err)
			redirectSoccerWithGoogleStatus(w, r, "failed")
			return
		}
	}
	state, err := newGoogleOAuthState(connectionID)
	if err != nil {
		log.Printf("google oauth state generation failed: %v", err)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	if err := setGoogleOAuthStateCookie(w, r, state); err != nil {
		log.Printf("google oauth state cookie write failed: %v", err)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	authURL := googleOAuthConfigForRequest(r).AuthCodeURL(
		state.State,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

func soccerGoogleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if !googleEnabled() {
		redirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "denied")
		return
	}
	state, err := getGoogleOAuthStateCookie(r)
	if errors.Is(err, errSessionExpired) {
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	if err != nil || state == nil || state.State == "" || state.State != strings.TrimSpace(r.URL.Query().Get("state")) {
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	ctx := googleHTTPContext(r.Context())
	token, err := googleOAuthConfigForRequest(r).Exchange(ctx, strings.TrimSpace(r.URL.Query().Get("code")))
	if err != nil {
		log.Printf("google token exchange failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	calendars, err := googleListCalendarsWithToken(ctx, token)
	if err != nil || len(calendars) == 0 {
		log.Printf("google calendar list after connect failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	selectedCalendarID, selectedCalendarSummary := preferredGoogleCalendar(calendars)
	encryptedToken, err := encryptGoogleToken(token)
	if err != nil {
		log.Printf("google token encryption failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	createdAt := time.Now().UTC()
	if existing, err := currentGoogleConnectionStore().Get(r.Context(), state.ConnectionID); err == nil && existing != nil && !existing.CreatedAt.IsZero() {
		createdAt = existing.CreatedAt
	}
	record := googleConnectionRecord{
		ConnectionID:    state.ConnectionID,
		TokenCiphertext: encryptedToken,
		CalendarID:      selectedCalendarID,
		CalendarSummary: selectedCalendarSummary,
		CreatedAt:       createdAt,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := currentGoogleConnectionStore().Put(r.Context(), &record); err != nil {
		log.Printf("google connection save failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	setGoogleConnectionCookie(w, r, state.ConnectionID)
	clearGoogleOAuthStateCookie(w, r)
	redirectSoccerWithGoogleStatus(w, r, "connected")
}

func soccerGoogleCalendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := loadSoccerSession(w, r)
	if !googleEnabled() {
		renderSoccerLoginState(w, r, session)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginState(w, r, session)
		return
	}
	record, err := loadGoogleConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		renderSoccerLoginState(w, r, session)
		return
	}
	calendars, err := googleListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		renderSoccerLoginState(w, r, session)
		return
	}
	selectedCalendarID := strings.TrimSpace(r.FormValue("calendar_id"))
	selectedCalendarSummary := googleCalendarSummary(calendars, selectedCalendarID)
	if selectedCalendarSummary == "" {
		selectedCalendarID, selectedCalendarSummary = preferredGoogleCalendar(calendars)
	}
	record.CalendarID = selectedCalendarID
	record.CalendarSummary = selectedCalendarSummary
	record.UpdatedAt = time.Now().UTC()
	if err := currentGoogleConnectionStore().Put(r.Context(), record); err != nil {
		log.Printf("google calendar selection save failed: %v", err)
	}
	renderSoccerLoginState(w, r, session)
}

func soccerGoogleDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := loadSoccerSession(w, r)
	deleteGoogleConnection(r.Context(), w, r)
	renderSoccerLoginState(w, r, session)
}

func renderGoogleDisconnectFeedback(w http.ResponseWriter, r *http.Request, session *SessionData, message string) {
	deleteGoogleConnection(r.Context(), w, r)
	if session != nil {
		if err := partials.SoccerLoginState(soccerLoginStateProps(w, r, session, true)).Render(context.Background(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	renderSoccerLoginFeedback(w, "error", message)
}

func googleAPIResponseError(resp *http.Response) (bool, error) {
	apiErr := readGoogleAPIError(resp)
	log.Printf("google event insert rejected: %v", apiErr)
	var googleErr *googleAPIError
	return errors.As(apiErr, &googleErr) && (googleErr.StatusCode == http.StatusUnauthorized || googleErr.StatusCode == http.StatusForbidden), apiErr
}

func soccerGoogleAddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !googleEnabled() {
		renderSoccerLoginFeedback(w, "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginFeedback(w, "error", "Could not read the selected games. Try again.")
		return
	}
	record, err := loadGoogleConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		renderSoccerLoginFeedback(w, "error", "Connect Google Calendar before adding selected games.")
		return
	}
	selectedIDs := parseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		renderSoccerLoginFeedback(w, "error", "Select at least one game to add to Google Calendar.")
		return
	}
	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(rawPlayerIDs)
	if len(nonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		renderSoccerLoginFeedback(w, "error", "One or more selected players were invalid. Clear the imported players and import again to refresh the discovered list.")
		return
	}
	session, _ := loadSoccerSession(w, r)
	games, err := requestedScheduleGames(r.Context(), session, playerIDs, teamCodes)
	if err != nil {
		renderSoccerLoginFeedback(w, "error", googleAddScheduleErrorMessage(err))
		return
	}
	filteredGames := selectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		renderSoccerLoginFeedback(w, "error", "No selected games were found to add.")
		return
	}
	token, err := currentGoogleToken(r.Context(), r, record)
	if err != nil {
		log.Printf("google token refresh failed: %v", err)
		renderGoogleDisconnectFeedback(w, r, session, "Your Google Calendar connection has expired. Connect again and retry.")
		return
	}
	added, updated, skipped, authRejected, err := insertGoogleCalendarEvents(r, record, token, filteredGames)
	if err != nil {
		log.Printf("google event insert failed: %v", err)
		renderSoccerLoginFeedback(w, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if authRejected {
		renderGoogleDisconnectFeedback(w, r, session, "Your Google Calendar connection is no longer valid. Connect again and retry.")
		return
	}
	message := fmt.Sprintf("Added %d selected game(s) to Google Calendar.", added)
	if updated > 0 {
		message += fmt.Sprintf(" Updated/restored %d matching game(s).", updated)
	}
	if skipped > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", skipped)
	}
	renderSoccerLoginFeedback(w, "success", message)
}

type googleCalendarEventAction int

const (
	googleCalendarEventSkipped googleCalendarEventAction = iota
	googleCalendarEventInserted
	googleCalendarEventUpdated
)

func insertGoogleCalendarEvents(r *http.Request, record *googleConnectionRecord, token *oauth2.Token, games []Game) (int, int, int, bool, error) {
	added := 0
	updated := 0
	skipped := 0
	for i := range games {
		event, ok := googleEventPayload(r, &games[i])
		if !ok {
			continue
		}
		action, authRejected, err := syncGoogleCalendarEvent(googleHTTPContext(r.Context()), record.CalendarID, token, &event)
		if err != nil {
			return 0, 0, 0, false, err
		}
		if authRejected {
			return 0, 0, 0, true, nil
		}
		switch action {
		case googleCalendarEventInserted:
			added++
		case googleCalendarEventUpdated:
			updated++
		case googleCalendarEventSkipped:
			skipped++
		}
	}
	return added, updated, skipped, false, nil
}

func syncGoogleCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *googleEvent) (googleCalendarEventAction, bool, error) {
	existingEvent, found, authRejected, err := googleFindCalendarEventByGameID(ctx, calendarID, token, event.ID)
	if err != nil {
		return googleCalendarEventSkipped, false, err
	}
	if authRejected {
		return googleCalendarEventSkipped, true, nil
	}
	if found {
		return refreshGoogleCalendarEvent(ctx, calendarID, token, existingEvent, event)
	}

	resp, err := googleInsertCalendarEvent(ctx, calendarID, token, event)
	if err != nil {
		return googleCalendarEventSkipped, false, err
	}
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		resp.Body.Close()
		return googleCalendarEventInserted, false, nil
	case http.StatusConflict:
		resp.Body.Close()
		existingEvent, found, authRejected, err = googleFindCalendarEventByGameID(ctx, calendarID, token, event.ID)
		if err != nil {
			return googleCalendarEventSkipped, false, err
		}
		if authRejected {
			return googleCalendarEventSkipped, true, nil
		}
		if !found {
			return googleCalendarEventSkipped, false, nil
		}
		return refreshGoogleCalendarEvent(ctx, calendarID, token, existingEvent, event)
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return googleCalendarEventSkipped, true, nil
		}
		return googleCalendarEventSkipped, false, apiErr
	}
}

func refreshGoogleCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, existingEvent, event *googleEvent) (googleCalendarEventAction, bool, error) {
	refreshedEvent := *event
	if existingEvent != nil {
		if existingID := strings.TrimSpace(existingEvent.ID); existingID != "" {
			refreshedEvent.ID = existingID
		}
	}

	resp, err := googleUpdateCalendarEvent(ctx, calendarID, refreshedEvent.ID, token, &refreshedEvent)
	if err != nil {
		return googleCalendarEventSkipped, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		resp.Body.Close()
		return googleCalendarEventUpdated, false, nil
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return googleCalendarEventSkipped, false, nil
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return googleCalendarEventSkipped, true, nil
		}
		return googleCalendarEventSkipped, false, apiErr
	}
}

func googleFindCalendarEventByGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*googleEvent, bool, bool, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return nil, false, false, nil
	}

	resp, err := googleGetCalendarEvent(ctx, calendarID, gameID, token)
	if err != nil {
		return nil, false, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		existingEvent, decodeErr := decodeGoogleEvent(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		if googleEventMatchesGameID(existingEvent, gameID) {
			return existingEvent, true, false, nil
		}
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}

	resp, err = googleListCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID)
	if err != nil {
		return nil, false, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		events, decodeErr := decodeGoogleEventList(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		for i := range events {
			if googleEventMatchesGameID(&events[i], gameID) {
				return &events[i], true, false, nil
			}
		}
		return nil, false, false, nil
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}
}

func googleEventMatchesGameID(event *googleEvent, gameID string) bool {
	if event == nil {
		return false
	}
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return false
	}
	if strings.TrimSpace(event.ID) == gameID {
		return true
	}
	return strings.TrimSpace(event.ExtendedProperties.Private["game_id"]) == gameID
}

func decodeGoogleEvent(resp *http.Response) (*googleEvent, error) {
	defer resp.Body.Close()
	var event googleEvent
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRequestBodySize)).Decode(&event); err != nil {
		return nil, err
	}
	return &event, nil
}

func decodeGoogleEventList(resp *http.Response) ([]googleEvent, error) {
	defer resp.Body.Close()
	var response googleEventListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRequestBodySize)).Decode(&response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func syncGoogleCalendarSelection(ctx context.Context, record *googleConnectionRecord, calendars []types.GoogleCalendarOption) (calendarID, summary string) {
	calendarID = strings.TrimSpace(record.CalendarID)
	if calendarID == "" {
		calendarID, summary = preferredGoogleCalendar(calendars)
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := currentGoogleConnectionStore().Put(ctx, record); err != nil {
			log.Printf("google connection default calendar save failed: %v", err)
		}
		return calendarID, summary
	}
	summary = googleCalendarSummary(calendars, calendarID)
	if summary == "" {
		calendarID, summary = preferredGoogleCalendar(calendars)
	}
	if summary != "" && (record.CalendarID != calendarID || record.CalendarSummary != summary) {
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := currentGoogleConnectionStore().Put(ctx, record); err != nil {
			log.Printf("google connection calendar sync failed: %v", err)
		}
	}
	return calendarID, summary
}

func newRandomHex(byteLength int) (string, error) {
	random := make([]byte, byteLength)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func newGoogleOAuthState(connectionID string) (googleOAuthState, error) {
	stateValue, err := newRandomHex(16)
	if err != nil {
		return googleOAuthState{}, err
	}
	return googleOAuthState{
		ConnectionID: connectionID,
		ExpiresAt:    time.Now().Add(googleOAuthStateTTL),
		State:        stateValue,
	}, nil
}

func googleOAuthConfigForRequest(r *http.Request) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     configData.GoogleClientID,
		ClientSecret: configData.GoogleClientSecret,
		RedirectURL:  requestBaseURL(r) + "/soccer",
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar.events",
			"https://www.googleapis.com/auth/calendar.calendarlist.readonly",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  googleOAuthAuthURL,
			TokenURL: googleOAuthTokenURL,
		},
	}
}

func googleHTTPContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, lpsHTTPClient)
}

func encryptGoogleToken(token *oauth2.Token) (string, error) {
	return encryptJSONValue(token)
}

func decryptGoogleToken(ciphertext string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := decryptJSONValue(ciphertext, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func loadGoogleConnectionRecord(ctx context.Context, r *http.Request) (*googleConnectionRecord, error) {
	connectionID := getGoogleConnectionID(r)
	if connectionID == "" {
		return nil, nil
	}
	return currentGoogleConnectionStore().Get(ctx, connectionID)
}

func deleteGoogleConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	connectionID := getGoogleConnectionID(r)
	if connectionID != "" {
		if err := currentGoogleConnectionStore().Delete(ctx, connectionID); err != nil {
			log.Printf("google connection delete failed: %v", err)
		}
	}
	clearGoogleConnectionCookie(w, r)
}

func currentGoogleToken(ctx context.Context, r *http.Request, record *googleConnectionRecord) (*oauth2.Token, error) {
	storedToken, err := decryptGoogleToken(record.TokenCiphertext)
	if err != nil {
		return nil, err
	}
	tokenSource := googleOAuthConfigForRequest(r).TokenSource(googleHTTPContext(ctx), storedToken)
	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = storedToken.RefreshToken
	}
	if token.AccessToken != storedToken.AccessToken || token.RefreshToken != storedToken.RefreshToken || !token.Expiry.Equal(storedToken.Expiry) {
		encryptedToken, encryptErr := encryptGoogleToken(token)
		if encryptErr != nil {
			return nil, encryptErr
		}
		record.TokenCiphertext = encryptedToken
		record.UpdatedAt = time.Now().UTC()
		if err := currentGoogleConnectionStore().Put(ctx, record); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func newGoogleAPIRequest(ctx context.Context, method, requestPath string, query url.Values, token *oauth2.Token, body any) (*http.Request, error) {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(payload)
	}
	endpoint, err := url.JoinPath(googleCalendarAPIBaseURL, requestPath)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody) //nolint:gosec // endpoint is derived from the constant Google Calendar API base URL and fixed paths.
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func googleInsertCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *googleEvent) (*http.Response, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodPost, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func googleGetCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token) (*http.Response, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodGet, "calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), nil, token, nil)
	if err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req)
}

func googleUpdateCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token, event *googleEvent) (*http.Response, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodPut, "calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func googleListCalendarEventsByPrivateGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*http.Response, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodGet, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{
		"maxResults":              {"10"},
		"privateExtendedProperty": {"game_id=" + gameID},
		"showDeleted":             {"true"},
	}, token, nil)
	if err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req)
}

func readGoogleAPIError(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRequestBodySize))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return &googleAPIError{
		StatusCode: resp.StatusCode,
		Message:    message,
	}
}

func googleListCalendarsWithToken(ctx context.Context, token *oauth2.Token) ([]types.GoogleCalendarOption, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodGet, "users/me/calendarList", url.Values{"minAccessRole": {"writer"}}, token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := lpsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readGoogleAPIError(resp)
	}
	defer resp.Body.Close()
	var response googleCalendarListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRequestBodySize)).Decode(&response); err != nil {
		return nil, err
	}
	options := make([]types.GoogleCalendarOption, 0, len(response.Items))
	for _, item := range response.Items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Summary) == "" {
			continue
		}
		options = append(options, types.GoogleCalendarOption{
			ID:      item.ID,
			Primary: item.Primary,
			Summary: item.Summary,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Primary != options[j].Primary {
			return options[i].Primary
		}
		return strings.ToLower(options[i].Summary) < strings.ToLower(options[j].Summary)
	})
	return options, nil
}

func googleListCalendars(ctx context.Context, r *http.Request, record *googleConnectionRecord) ([]types.GoogleCalendarOption, error) {
	token, err := currentGoogleToken(ctx, r, record)
	if err != nil {
		return nil, err
	}
	return googleListCalendarsWithToken(googleHTTPContext(ctx), token)
}

func preferredGoogleCalendar(calendars []types.GoogleCalendarOption) (string, string) {
	for _, calendar := range calendars {
		if calendar.Primary {
			return calendar.ID, calendar.Summary
		}
	}
	if len(calendars) == 0 {
		return "", ""
	}
	return calendars[0].ID, calendars[0].Summary
}

func googleCalendarSummary(calendars []types.GoogleCalendarOption, calendarID string) string {
	for _, calendar := range calendars {
		if calendar.ID == calendarID {
			return calendar.Summary
		}
	}
	return ""
}

func googleEventPayload(r *http.Request, game *Game) (googleEvent, bool) {
	formatted, ok := canonicalGameEvent(game)
	if !ok {
		return googleEvent{}, false
	}
	event := googleEvent{
		Description: formatted.Description,
		End: googleEventDateTime{
			DateTime: formatted.End.Format("2006-01-02T15:04:05"),
			TimeZone: mountainTimeZoneID,
		},
		ID:       formatted.ID,
		Location: formatted.Location,
		Start: googleEventDateTime{
			DateTime: formatted.Start.Format("2006-01-02T15:04:05"),
			TimeZone: mountainTimeZoneID,
		},
		Status:  formatted.Status,
		Summary: formatted.Summary,
	}
	event.ExtendedProperties.Private = map[string]string{
		"game_id": formatted.ID,
	}
	event.Source = &googleEventSource{
		Title: "Soccer Schedule",
		URL:   requestBaseURL(r) + "/soccer",
	}
	return event, true
}

func jwtExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func normalizeImportedJWT(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("Paste the bearer JWT from your Let's Play Soccer browser session.")
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("Paste a single JWT value without extra spaces or line breaks.")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
	}
	for _, segment := range parts[:2] {
		if segment == "" {
			return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
		}
		if _, err := base64.RawURLEncoding.DecodeString(segment); err != nil {
			return "", errors.New("The imported JWT format is not valid base64url data.")
		}
	}

	expiresAt := jwtExpiry(token)
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return "", errors.New("This JWT has expired. Copy a fresh bearer token from letsplaysoccer.com and import it again.")
	}

	return token, nil
}

func importedSessionExpiry(token string) time.Time {
	deadline := time.Now().Add(defaultSessionTTL)
	expiresAt := jwtExpiry(token)
	if expiresAt.IsZero() || expiresAt.After(deadline) {
		return deadline
	}
	return expiresAt
}

type lpsUserPlayerDiscovery struct {
	UserName string
	Players  []LPSPlayer
}

func lpsFetchUserPlayers(ctx context.Context, jwt string) (lpsUserPlayerDiscovery, error) {
	var discovery lpsUserPlayerDiscovery

	normalizedJWT, err := normalizeImportedJWT(jwt)
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, normalizedJWT, "users", "check")
	if err != nil {
		return discovery, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "could not reach Let's Play Soccer while loading players: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "could not read the player lookup response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return discovery, newLPSFetchError(lpsErrorUnauthorized, 0, resp.StatusCode, "Let's Play Soccer rejected the imported token with status 401")
	}
	if resp.StatusCode == http.StatusForbidden {
		return discovery, newLPSFetchError(lpsErrorForbidden, 0, resp.StatusCode, "Let's Play Soccer denied access to the player lookup with status 403")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, resp.StatusCode, "Let's Play Soccer returned status %d while loading players", resp.StatusCode)
	}

	discovery, err = decodeLPSUserPlayers(responseBody)
	if err != nil {
		var fetchErr *lpsFetchError
		if errors.As(err, &fetchErr) {
			return discovery, err
		}
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "%v", err)
	}

	return discovery, nil
}

func lpsFetchGamesForPlayers(ctx context.Context, jwt string, playerIDs []int) ([]Game, error) {
	normalizedJWT, err := normalizeImportedJWT(jwt)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	resolver := newLPSScheduleResolver(normalizedJWT)
	teamByID := make(map[int]lpsTeamSummary)
	for _, playerID := range sortedUniqueIDs(playerIDs) {
		playerTeams, err := resolver.fetchPlayerTeams(ctx, playerID)
		if err != nil {
			return nil, err
		}
		for _, team := range playerTeams {
			if team.UTeamID <= 0 {
				continue
			}
			if _, exists := teamByID[team.UTeamID]; !exists {
				teamByID[team.UTeamID] = team
			}
		}
	}

	teamIDs := make([]int, 0, len(teamByID))
	for teamID := range teamByID {
		teamIDs = append(teamIDs, teamID)
	}
	sort.Ints(teamIDs)

	games := make([]Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range teamIDs {
		team := teamByID[teamID]
		teamGames, err := resolver.fetchTeamGames(ctx, teamID, &team)
		if err != nil {
			return nil, err
		}
		games = mergeScheduleGames(games, teamGames, indexByKey)
	}
	sortScheduleGames(games)
	return games, nil
}

func lpsFetchGamesForTeams(ctx context.Context, teamIDs []int) ([]Game, error) {
	resolver := newLPSScheduleResolver("")
	games := make([]Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range sortedUniqueIDs(teamIDs) {
		teamGames, err := resolver.fetchTeamGames(ctx, teamID, nil)
		if err != nil {
			return nil, err
		}
		games = mergeScheduleGames(games, teamGames, indexByKey)
	}
	sortScheduleGames(games)
	return games, nil
}

type lpsUserCheckResponse struct {
	AuthFailure bool        `json:"authFailure"`
	Error       string      `json:"error"`
	FirstName   string      `json:"first_name"`
	LastName    string      `json:"last_name"`
	Players     []LPSPlayer `json:"players"`
	UserPlayers []struct {
		PlayerID int  `json:"player_id"`
		Deleted  bool `json:"deleted"`
	} `json:"user_players"`
}

type lpsTeamSummary struct {
	UTeamID      int    `json:"UTeamID"`
	TeamName     string `json:"team_name"`
	DivisionName string `json:"division_name"`
	FacilityID   int    `json:"FacilityID"`
	FacilityName string `json:"facility_name"`
	Season       int    `json:"Season"`
}

type lpsTeamScheduleGame struct {
	UGameID           int            `json:"UGameID"`
	FieldName         string         `json:"field_name"`
	SchedGameDateTime string         `json:"SchedGameDateTime"`
	SchedGameEndTime  *string        `json:"schedGameEndTime"`
	FacilityName      string         `json:"facilityName"`
	Result            string         `json:"result"`
	Field             int            `json:"Field"`
	Season            int            `json:"Season"`
	FacilityID        int            `json:"FacilityID"`
	UTeam1            int            `json:"UTeam1"`
	UTeam2            int            `json:"UTeam2"`
	TeamIDSelected    *int           `json:"team_id_selected"`
	HomeTeam          lpsTeamSummary `json:"home_team"`
	VisitorTeam       lpsTeamSummary `json:"visitor_team"`
}

type lpsTeamScheduleResponse struct {
	Games []lpsTeamScheduleGame `json:"games"`
	Team  lpsTeamSummary        `json:"team"`
}

type lpsFacility struct {
	FacilityID   int    `json:"FacilityID"`
	FacilityName string `json:"FacilityName"`
	Address      string `json:"Address"`
	City         string `json:"City"`
	State        string `json:"State"`
	ZIP          string `json:"ZIP"`
}

type lpsScheduleResolver struct {
	jwt           string
	facilityCache map[int]lpsFacility
}

func newLPSScheduleResolver(jwt string) *lpsScheduleResolver {
	return &lpsScheduleResolver{
		jwt:           jwt,
		facilityCache: make(map[int]lpsFacility),
	}
}

func lpsFetchUpcomingGames(ctx context.Context, normalizedJWT string, playerID int) ([]Game, error) {
	if playerID <= 0 {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, http.StatusBadRequest, "player ID %d is invalid", playerID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, normalizedJWT, "players", strconv.Itoa(playerID), "upcoming_games")
	if err != nil {
		return nil, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not read the schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, newLPSFetchError(lpsErrorUnauthorized, playerID, resp.StatusCode, "Let's Play Soccer rejected the imported token for player %d with status 401", playerID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, newLPSFetchError(lpsErrorForbidden, playerID, resp.StatusCode, "Let's Play Soccer denied access to player %d with status 403", playerID)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, resp.StatusCode, "Let's Play Soccer could not find upcoming games for player %d", playerID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, resp.StatusCode, "Let's Play Soccer returned status %d while loading schedules", resp.StatusCode)
	}

	games, err := decodeLPSGames(responseBody)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "%v", err)
	}
	normalizeScheduleGames(games)
	return games, nil
}

func lpsFetchTeamGames(ctx context.Context, teamID int) ([]Game, error) {
	return newLPSScheduleResolver("").fetchTeamGames(ctx, teamID, nil)
}

func (resolver *lpsScheduleResolver) fetchPlayerTeams(ctx context.Context, playerID int) ([]lpsTeamSummary, error) {
	if playerID <= 0 {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, http.StatusBadRequest, "player ID %d is invalid", playerID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, resolver.jwt, "players", strconv.Itoa(playerID), "my_teams")
	if err != nil {
		return nil, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading player teams: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not read the player teams response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, newLPSFetchError(lpsErrorUnauthorized, playerID, resp.StatusCode, "Let's Play Soccer rejected the imported token for player %d with status 401", playerID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, newLPSFetchError(lpsErrorForbidden, playerID, resp.StatusCode, "Let's Play Soccer denied access to player %d with status 403", playerID)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, resp.StatusCode, "Let's Play Soccer could not find teams for player %d", playerID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, resp.StatusCode, "Let's Play Soccer returned status %d while loading player teams", resp.StatusCode)
	}

	var teams []lpsTeamSummary
	if err := json.Unmarshal(responseBody, &teams); err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "The player teams response format was not recognized.")
	}

	sort.Slice(teams, func(i, j int) bool {
		if teams[i].UTeamID != teams[j].UTeamID {
			return teams[i].UTeamID < teams[j].UTeamID
		}
		return teams[i].TeamName < teams[j].TeamName
	})
	return teams, nil
}

func (resolver *lpsScheduleResolver) fetchTeamGames(ctx context.Context, teamID int, selectedTeam *lpsTeamSummary) ([]Game, error) {
	response, err := resolver.fetchTeamSchedule(ctx, teamID)
	if err != nil {
		return nil, err
	}

	games := make([]Game, 0, len(response.Games))
	for i := range response.Games {
		game, err := resolver.mapTeamScheduleGame(ctx, &response.Games[i], response.Team, selectedTeam)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	normalizeScheduleGames(games)
	return upcomingScheduleGames(games), nil
}

func (resolver *lpsScheduleResolver) fetchTeamSchedule(ctx context.Context, teamID int) (lpsTeamScheduleResponse, error) {
	var schedule lpsTeamScheduleResponse
	if teamID <= 0 {
		return schedule, newLPSFetchError(lpsErrorInvalidTeam, teamID, http.StatusBadRequest, "team ID %d is invalid", teamID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, "", "teams", strconv.Itoa(teamID))
	if err != nil {
		return schedule, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading team schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "could not read the team schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return schedule, newLPSFetchError(lpsErrorInvalidTeam, teamID, resp.StatusCode, "Let's Play Soccer could not find team %d", teamID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, resp.StatusCode, "Let's Play Soccer returned status %d while loading team schedules", resp.StatusCode)
	}

	if err := json.Unmarshal(responseBody, &schedule); err != nil {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "The team schedule response format was not recognized.")
	}
	return schedule, nil
}

func (resolver *lpsScheduleResolver) mapTeamScheduleGame(ctx context.Context, rawGame *lpsTeamScheduleGame, responseTeam lpsTeamSummary, selectedTeam *lpsTeamSummary) (Game, error) {
	if rawGame == nil {
		return Game{}, nil
	}

	var selected lpsTeamSummary
	if selectedTeam != nil {
		selected = *selectedTeam
	}

	facilityID := firstPositiveInt(rawGame.FacilityID, selected.FacilityID, responseTeam.FacilityID, rawGame.HomeTeam.FacilityID, rawGame.VisitorTeam.FacilityID)
	facilityName := firstNonEmptyString(rawGame.FacilityName, selected.FacilityName, responseTeam.FacilityName, rawGame.HomeTeam.FacilityName, rawGame.VisitorTeam.FacilityName)
	facility, err := resolver.fetchFacility(ctx, facilityID)
	if err != nil {
		return Game{}, err
	}
	if strings.TrimSpace(facility.FacilityName) != "" {
		facilityName = strings.TrimSpace(facility.FacilityName)
	}

	fieldName := strings.TrimSpace(rawGame.FieldName)
	if fieldName == "" && rawGame.Field > 0 {
		fieldName = fmt.Sprintf("Field %d", rawGame.Field)
	}

	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	playerTeamName, opponentTeamName, divisionName := resolveSelectedTeamMatchup(rawGame, responseTeam, &selected)
	if playerTeamName == "" {
		playerTeamName = homeName
	}
	if opponentTeamName == "" {
		opponentTeamName = visitorName
		if playerTeamName == visitorName {
			opponentTeamName = homeName
		}
	}

	game := Game{
		ID:               intString(rawGame.UGameID),
		DateTime:         formatGameDateTime(normalizeLPSScheduleTime(rawGame.SchedGameDateTime)),
		StartAt:          normalizeLPSScheduleTime(rawGame.SchedGameDateTime),
		EndAt:            normalizeLPSScheduleTime(stringPointerValue(rawGame.SchedGameEndTime)),
		Field:            fieldName,
		Location:         strings.TrimSpace(facilityName),
		Home:             homeName,
		Away:             visitorName,
		Season:           firstNonEmptyString(intString(selected.Season), intString(responseTeam.Season), intString(rawGame.Season), intString(rawGame.HomeTeam.Season), intString(rawGame.VisitorTeam.Season)),
		PlayerTeamName:   playerTeamName,
		OpponentTeamName: opponentTeamName,
		DivisionName:     divisionName,
		FacilityID:       facilityID,
		FacilityName:     strings.TrimSpace(facilityName),
		FacilityAddress:  strings.TrimSpace(facility.Address),
		FacilityCity:     strings.TrimSpace(facility.City),
		FacilityState:    strings.TrimSpace(facility.State),
		FacilityZIP:      strings.TrimSpace(facility.ZIP),
		Result:           strings.TrimSpace(rawGame.Result),
	}

	return game, nil
}

func (resolver *lpsScheduleResolver) fetchFacility(ctx context.Context, facilityID int) (lpsFacility, error) {
	if facilityID <= 0 {
		return lpsFacility{}, nil
	}
	if facility, ok := resolver.facilityCache[facilityID]; ok {
		return facility, nil
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, "", "facilities", strconv.Itoa(facilityID))
	if err != nil {
		return lpsFacility{}, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading facility %d: %w", facilityID, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, http.StatusBadGateway, "could not read the facility response for facility %d: %w", facilityID, err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, resp.StatusCode, "Let's Play Soccer could not find facility %d", facilityID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, resp.StatusCode, "Let's Play Soccer returned status %d while loading facility %d", resp.StatusCode, facilityID)
	}

	var facility lpsFacility
	if err := json.Unmarshal(responseBody, &facility); err != nil {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, http.StatusBadGateway, "The facility response format was not recognized.")
	}
	resolver.facilityCache[facilityID] = facility
	return facility, nil
}

func decodeLPSUserPlayers(payload []byte) (lpsUserPlayerDiscovery, error) {
	var discovery lpsUserPlayerDiscovery

	var envelope lpsUserCheckResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return discovery, errors.New("The player lookup response format was not recognized.")
	}
	if envelope.AuthFailure {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = "Let's Play Soccer rejected the imported token."
		}
		return discovery, newLPSFetchError(lpsErrorUnauthorized, 0, http.StatusUnauthorized, "%s", message)
	}
	discovery.UserName = fullName(strings.TrimSpace(envelope.FirstName), strings.TrimSpace(envelope.LastName))
	if discovery.UserName == "" {
		discovery.UserName = "Let's Play Soccer account"
	}
	if len(envelope.Players) == 0 {
		discovery.Players = []LPSPlayer{}
		return discovery, nil
	}

	deletedPlayerIDs := make(map[int]struct{})
	for _, userPlayer := range envelope.UserPlayers {
		if userPlayer.Deleted {
			deletedPlayerIDs[userPlayer.PlayerID] = struct{}{}
		}
	}
	if len(deletedPlayerIDs) == 0 {
		discovery.Players = envelope.Players
		return discovery, nil
	}

	players := make([]LPSPlayer, 0, len(envelope.Players))
	for _, player := range envelope.Players {
		if _, deleted := deletedPlayerIDs[player.UPlayerID]; deleted {
			continue
		}
		players = append(players, player)
	}

	discovery.Players = players
	return discovery, nil
}

func resolveSelectedTeamMatchup(rawGame *lpsTeamScheduleGame, responseTeam lpsTeamSummary, selectedTeam *lpsTeamSummary) (string, string, string) {
	if rawGame == nil {
		return "", "", ""
	}

	selectedTeamID := responseTeam.UTeamID
	selectedTeamName := strings.TrimSpace(responseTeam.TeamName)
	divisionName := strings.TrimSpace(responseTeam.DivisionName)
	if selectedTeam != nil {
		selectedTeamID = firstPositiveInt(selectedTeam.UTeamID, responseTeam.UTeamID)
		selectedTeamName = firstNonEmptyString(selectedTeam.TeamName, responseTeam.TeamName)
		divisionName = firstNonEmptyString(selectedTeam.DivisionName, responseTeam.DivisionName)
	}
	if selectedTeamID == 0 && rawGame.TeamIDSelected != nil {
		selectedTeamID = *rawGame.TeamIDSelected
	}

	homeID := firstPositiveInt(rawGame.HomeTeam.UTeamID, rawGame.UTeam1)
	visitorID := firstPositiveInt(rawGame.VisitorTeam.UTeamID, rawGame.UTeam2)
	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	homeDivision := strings.TrimSpace(rawGame.HomeTeam.DivisionName)
	visitorDivision := strings.TrimSpace(rawGame.VisitorTeam.DivisionName)

	switch {
	case selectedTeamID > 0 && homeID == selectedTeamID:
		return firstNonEmptyString(selectedTeamName, homeName), visitorName, firstNonEmptyString(divisionName, homeDivision, visitorDivision)
	case selectedTeamID > 0 && visitorID == selectedTeamID:
		return firstNonEmptyString(selectedTeamName, visitorName), homeName, firstNonEmptyString(divisionName, visitorDivision, homeDivision)
	}

	playerTeamName := firstNonEmptyString(selectedTeamName, homeName)
	if playerTeamName == visitorName {
		return playerTeamName, homeName, firstNonEmptyString(divisionName, visitorDivision, homeDivision)
	}
	return playerTeamName, visitorName, firstNonEmptyString(divisionName, homeDivision, visitorDivision)
}

func decodeLPSGames(payload []byte) ([]Game, error) {
	var envelope types.LambdaGamesResponse
	if err := json.Unmarshal(payload, &envelope); err == nil && len(envelope.Games) > 0 {
		return envelope.Games, nil
	}

	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, errors.New("The schedule response format was not recognized.")
	}

	items := extractGameMaps(raw)
	if len(items) == 0 {
		return []Game{}, nil
	}
	games := make([]Game, 0, len(items))
	for _, item := range items {
		game := mapLPSGame(item)
		if game.ID == "" && game.DateTime == "" && game.Home == "" && game.Away == "" {
			continue
		}
		games = append(games, game)
	}
	return games, nil
}

func extractGameMaps(raw any) []map[string]any {
	switch value := raw.(type) {
	case []any:
		games := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if mapped, ok := item.(map[string]any); ok {
				games = append(games, mapped)
			}
		}
		return games
	case map[string]any:
		for _, key := range []string{"games", "upcoming_games", "data", "results", "items"} {
			if nested, ok := value[key]; ok {
				games := extractGameMaps(nested)
				if len(games) > 0 {
					return games
				}
			}
		}
		return []map[string]any{value}
	default:
		return nil
	}
}

func mapLPSGame(raw map[string]any) Game {
	startAt := normalizeLPSScheduleTime(firstString(raw,
		"start_at", "starts_at", "start_datetime", "StartDateTime", "SchedGameDateTime", "schedGameDateTime", "game_datetime", "datetime", "date_time",
	))
	endAt := normalizeLPSScheduleTime(firstString(raw,
		"end_at", "ends_at", "end_datetime", "EndDateTime", "schedGameEndTime", "SchedGameEndTime", "game_end_datetime", "end_time",
	))
	dateTime := normalizeLPSScheduleTime(firstString(raw, "display_datetime", "DisplayDateTime", "DateTime", "datetime", "date_time"))
	if dateTime == "" {
		dateTime = formatGameDateTime(startAt)
	}

	homeTeam := firstString(raw, "home", "Home", "home_team", "HomeTeam", "home_team_name", "TeamName")
	awayTeam := firstString(raw, "away", "Away", "away_team", "visitor_team", "AwayTeam", "away_team_name", "visitor_team_name", "OpponentName", "opponent_name")
	if homeTeam == "" && awayTeam == "" {
		matchup := firstString(raw, "matchup", "Matchup", "title", "Title")
		if matchup != "" {
			parts := strings.Split(matchup, " vs ")
			if len(parts) == 2 {
				homeTeam = strings.TrimSpace(parts[0])
				awayTeam = strings.TrimSpace(parts[1])
			}
		}
	}

	return Game{
		ID:               firstString(raw, "id", "ID", "game_id", "GameID", "UGameID"),
		DateTime:         dateTime,
		StartAt:          startAt,
		EndAt:            endAt,
		Field:            firstString(raw, "field_name", "FieldName", "field", "Field"),
		Location:         firstString(raw, "location", "Location", "venue", "Venue", "facility", "Facility", "facilityName"),
		Home:             homeTeam,
		Away:             awayTeam,
		Season:           firstString(raw, "season", "Season", "season_id", "SeasonID"),
		PlayerTeamName:   homeTeam,
		OpponentTeamName: awayTeam,
		DivisionName:     firstString(raw, "division_name", "DivisionName"),
		FacilityID:       firstInt(raw, "FacilityID", "facility_id"),
		FacilityName:     firstString(raw, "facilityName", "FacilityName", "facility_name"),
		FacilityAddress:  firstString(raw, "Address", "address"),
		FacilityCity:     firstString(raw, "City", "city"),
		FacilityState:    firstString(raw, "State", "state"),
		FacilityZIP:      firstString(raw, "ZIP", "zip"),
		Result:           firstString(raw, "result", "Result"),
	}
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if converted := anyToString(value); converted != "" {
			return converted
		}
	}
	return ""
}

func firstInt(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	case map[string]any:
		for _, nestedKey := range []string{"name", "Name", "title", "Title", "value", "Value", "team_name", "TeamName", "display_name", "DisplayName"} {
			if nestedValue, ok := typed[nestedKey]; ok {
				if converted := anyToString(nestedValue); converted != "" {
					return converted
				}
			}
		}
	}
	return ""
}

func mergeGames(base, incoming *Game) Game {
	merged := *base
	if merged.ID == "" {
		merged.ID = incoming.ID
	}
	if merged.DateTime == "" {
		merged.DateTime = incoming.DateTime
	}
	if merged.StartAt == "" {
		merged.StartAt = incoming.StartAt
	}
	if merged.EndAt == "" {
		merged.EndAt = incoming.EndAt
	}
	if merged.Field == "" {
		merged.Field = incoming.Field
	}
	if merged.Location == "" {
		merged.Location = incoming.Location
	}
	if merged.Home == "" {
		merged.Home = incoming.Home
	}
	if merged.Away == "" {
		merged.Away = incoming.Away
	}
	if merged.Season == "" {
		merged.Season = incoming.Season
	}
	if merged.PlayerTeamName == "" {
		merged.PlayerTeamName = incoming.PlayerTeamName
	}
	if merged.OpponentTeamName == "" {
		merged.OpponentTeamName = incoming.OpponentTeamName
	}
	if merged.DivisionName == "" {
		merged.DivisionName = incoming.DivisionName
	}
	if merged.FacilityID == 0 {
		merged.FacilityID = incoming.FacilityID
	}
	if merged.FacilityName == "" {
		merged.FacilityName = incoming.FacilityName
	}
	if merged.FacilityAddress == "" {
		merged.FacilityAddress = incoming.FacilityAddress
	}
	if merged.FacilityCity == "" {
		merged.FacilityCity = incoming.FacilityCity
	}
	if merged.FacilityState == "" {
		merged.FacilityState = incoming.FacilityState
	}
	if merged.FacilityZIP == "" {
		merged.FacilityZIP = incoming.FacilityZIP
	}
	if merged.Result == "" {
		merged.Result = incoming.Result
	}
	if merged.Location == "" && merged.Field != "" {
		merged.Location = "Field " + merged.Field
	}
	if merged.Field == "" && merged.Location != "" {
		merged.Field = merged.Location
	}
	if merged.DateTime == "" {
		merged.DateTime = formatGameDateTime(merged.StartAt)
	}
	if merged.ID == "" {
		merged.ID = fallbackGameID(&merged)
	}
	return merged
}

func stableGameFields(game *Game) string {
	return strings.Join([]string{game.Home, game.Away, game.StartAt, game.DateTime, game.Location, game.Season}, "|")
}

func fallbackGameID(game *Game) string {
	base := stableGameFields(game)
	if strings.ReplaceAll(base, "|", "") == "" {
		return ""
	}
	hash := md5.Sum([]byte(base))
	return hex.EncodeToString(hash[:])
}

func gameKey(game *Game) string {
	if game.ID != "" {
		return game.ID
	}
	return stableGameFields(game)
}

func gameStartTime(game *Game) (time.Time, bool) {
	if parsed, ok := parseScheduleTime(game.StartAt); ok {
		return parsed, true
	}
	return parseScheduleTime(game.DateTime)
}

func loadMountainTimeLocation() *time.Location {
	location, err := time.LoadLocation(mountainTimeZoneID)
	if err == nil {
		return location
	}

	log.Printf("could not load %s timezone; falling back to MST: %v", mountainTimeZoneID, err)
	return time.FixedZone("MST", -7*60*60)
}

func parseScheduleTime(value string) (time.Time, bool) {
	if parsed, ok := parseMislabelledLPSZuluTime(value); ok {
		return parsed, true
	}
	return parseFlexibleTime(value)
}

func normalizeLPSScheduleTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, ok := parseMislabelledLPSZuluTime(value)
	if !ok {
		return value
	}
	return parsed.Format(time.RFC3339)
}

func parseMislabelledLPSZuluTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}

	trimmed := strings.TrimSuffix(value, "Z")
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		parsed, err := time.ParseInLocation(layout, trimmed, mountainTimeLocation)
		if err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}

func parseFlexibleTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []struct {
		layout   string
		location *time.Location
	}{
		{layout: time.RFC3339Nano},
		{layout: time.RFC3339},
		{layout: "2006-01-02T15:04:05.000Z", location: time.UTC},
		{layout: "2006-01-02T15:04:05Z", location: time.UTC},
		{layout: "2006-01-02T15:04:05", location: time.Local},
		{layout: "2006-01-02 15:04:05", location: time.Local},
		{layout: "2006-01-02 15:04", location: time.Local},
		{layout: "Mon 01/02/06 03:04 PM MST", location: mountainTimeLocation},
		{layout: "Mon 01/02/06 03:04 PM", location: time.Local},
	}
	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.location != nil {
			parsed, err = time.ParseInLocation(candidate.layout, value, candidate.location)
		} else {
			parsed, err = time.Parse(candidate.layout, value)
		}
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func formatGameDateTime(value string) string {
	parsed, ok := parseScheduleTime(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return parsed.In(mountainTimeLocation).Format("Mon 01/02/06 03:04 PM MST")
}

func normalizeScheduleGames(games []Game) {
	for index := range games {
		if games[index].ID == "" {
			games[index].ID = fallbackGameID(&games[index])
		}
		if games[index].DateTime == "" {
			games[index].DateTime = formatGameDateTime(games[index].StartAt)
		}
		if games[index].Location == "" && games[index].Field != "" {
			games[index].Location = "Field " + games[index].Field
		}
		if games[index].Field == "" && games[index].Location != "" {
			games[index].Field = games[index].Location
		}
		if games[index].PlayerTeamName == "" {
			games[index].PlayerTeamName = games[index].Home
		}
		if games[index].OpponentTeamName == "" {
			games[index].OpponentTeamName = games[index].Away
		}
		if games[index].FacilityName == "" {
			games[index].FacilityName = games[index].Location
		}
	}
}

func mergeScheduleGames(games, incoming []Game, indexByKey map[string]int) []Game {
	for i := range incoming {
		game := &incoming[i]
		key := gameKey(game)
		if existingIndex, exists := indexByKey[key]; exists {
			games[existingIndex] = mergeGames(&games[existingIndex], game)
			continue
		}
		indexByKey[key] = len(games)
		games = append(games, *game)
	}
	return games
}

func sortScheduleGames(games []Game) {
	sort.Slice(games, func(i, j int) bool {
		left, leftOK := gameStartTime(&games[i])
		right, rightOK := gameStartTime(&games[j])
		if leftOK && rightOK {
			if !left.Equal(right) {
				return left.Before(right)
			}
			return compareScheduleGames(&games[i], &games[j]) < 0
		}
		if games[i].DateTime != games[j].DateTime {
			return games[i].DateTime < games[j].DateTime
		}
		return compareScheduleGames(&games[i], &games[j]) < 0
	})
}

func compareScheduleGames(left, right *Game) int {
	for _, pair := range [][2]string{
		{left.DateTime, right.DateTime},
		{left.StartAt, right.StartAt},
		{left.Home, right.Home},
		{left.Away, right.Away},
		{left.Location, right.Location},
		{left.Field, right.Field},
		{left.Season, right.Season},
		{left.PlayerTeamName, right.PlayerTeamName},
		{left.OpponentTeamName, right.OpponentTeamName},
		{left.DivisionName, right.DivisionName},
		{left.FacilityName, right.FacilityName},
		{left.Result, right.Result},
		{left.ID, right.ID},
	} {
		if pair[0] == pair[1] {
			continue
		}
		if pair[0] < pair[1] {
			return -1
		}
		return 1
	}
	return 0
}

func upcomingScheduleGames(games []Game) []Game {
	filtered := make([]Game, 0, len(games))
	now := time.Now()
	for i := range games {
		start, ok := gameStartTime(&games[i])
		if ok && start.Before(now) {
			continue
		}
		filtered = append(filtered, games[i])
	}
	return filtered
}

func buildICS(games []Game) string {
	var builder strings.Builder
	writeICSLine(&builder, "BEGIN:VCALENDAR")
	writeICSLine(&builder, "VERSION:2.0")
	writeICSLine(&builder, "PRODID:-//Craig Johnson Portfolio//Soccer Schedule//EN")
	writeICSLine(&builder, "X-WR-TIMEZONE:"+mountainTimeZoneID)
	for i := range games {
		game := &games[i]
		formatted, ok := canonicalGameEvent(game)
		if !ok {
			log.Printf("skipping game: could not parse start time")
			continue
		}
		writeICSLine(&builder, "BEGIN:VEVENT")
		writeICSLine(&builder, "UID:"+escapeICSText(formatted.ID))
		writeICSLine(&builder, "DTSTAMP:"+time.Now().UTC().Format("20060102T150405Z"))
		writeICSLine(&builder, "DTSTART;TZID="+mountainTimeZoneID+":"+formatted.Start.Format("20060102T150405"))
		writeICSLine(&builder, "DTEND;TZID="+mountainTimeZoneID+":"+formatted.End.Format("20060102T150405"))
		writeICSLine(&builder, "SUMMARY:"+escapeICSText(formatted.Summary))
		writeICSLine(&builder, "DESCRIPTION:"+escapeICSText(formatted.Description))
		writeICSLine(&builder, "LOCATION:"+escapeICSText(formatted.Location))
		writeICSLine(&builder, "STATUS:"+strings.ToUpper(formatted.Status))
		writeICSLine(&builder, "END:VEVENT")
	}
	writeICSLine(&builder, "END:VCALENDAR")
	return builder.String()
}

type formattedGameEvent struct {
	Description string
	End         time.Time
	ID          string
	Location    string
	Start       time.Time
	Status      string
	Summary     string
}

func canonicalGameEvent(game *Game) (formattedGameEvent, bool) {
	start, end, ok := scheduleTimes(game)
	if !ok {
		return formattedGameEvent{}, false
	}

	start = start.In(mountainTimeLocation)
	end = end.In(mountainTimeLocation)

	playerTeam := strings.TrimSpace(game.PlayerTeamName)
	if playerTeam == "" {
		playerTeam = strings.TrimSpace(game.Home)
	}

	opponentTeam := strings.TrimSpace(game.OpponentTeamName)
	if opponentTeam == "" {
		opponentTeam = strings.TrimSpace(game.Away)
	}

	fieldName := strings.TrimSpace(game.Field)
	location := canonicalGameLocation(game)
	if location == "" {
		location = strings.TrimSpace(game.Location)
	}

	gameID := strings.TrimSpace(game.ID)
	if gameID == "" {
		gameID = fallbackGameID(game)
	}

	status := canonicalGameStatus(game)

	return formattedGameEvent{
		Description: fmt.Sprintf("%s is playing %s\nDivision: %s\nFacility: %s\nField: %s\nResult: %s",
			playerTeam,
			opponentTeam,
			strings.TrimSpace(game.DivisionName),
			strings.TrimSpace(game.FacilityName),
			fieldName,
			strings.TrimSpace(game.Result),
		),
		End:      end,
		ID:       gameID,
		Location: location,
		Start:    start,
		Status:   status,
		Summary:  fmt.Sprintf("%s vs %s - %s", playerTeam, opponentTeam, fieldName),
	}, true
}

func canonicalGameLocation(game *Game) string {
	parts := []string{
		strings.TrimSpace(game.FacilityAddress),
		strings.TrimSpace(game.FacilityCity),
		strings.TrimSpace(game.FacilityState),
		strings.TrimSpace(game.FacilityZIP),
	}

	locationParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		locationParts = append(locationParts, part)
	}

	return strings.Join(locationParts, ", ")
}

func canonicalGameStatus(game *Game) string {
	if strings.EqualFold(strings.TrimSpace(game.Result), "canceled") {
		return "canceled"
	}
	return "confirmed"
}

func scheduleTimes(game *Game) (time.Time, time.Time, bool) {
	start, ok := gameStartTime(game)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, ok := parseScheduleTime(game.EndAt)
	if !ok || !end.After(start) {
		end = start.Add(defaultGameDuration)
	}
	return start, end, true
}

func escapeICSText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\;`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func writeICSLine(builder *strings.Builder, line string) {
	const maxLineBytes = 75

	firstSegment := true
	for line != "" {
		available := maxLineBytes
		if !firstSegment {
			builder.WriteByte(' ')
			available--
		}

		written := 0
		for index := 0; index < len(line); {
			_, size := utf8.DecodeRuneInString(line[index:])
			if written > 0 && written+size > available {
				break
			}
			written += size
			index += size
		}

		builder.WriteString(line[:written])
		builder.WriteString("\r\n")
		line = line[written:]
		firstSegment = false
	}
}
