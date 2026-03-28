// Google OAuth connect/callback/disconnect handlers, token refresh, and DynamoDB connection store.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"golang.org/x/oauth2"

	"portfolio/cmd/web/partials"
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
