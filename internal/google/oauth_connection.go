package google

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"portfolio/cmd/web/partials"
	internalhttpx "portfolio/internal/httpx"
	"portfolio/internal/logging"
	internalsession "portfolio/internal/session"
	"portfolio/types"
)

// RenderDisconnectFeedback removes the Google connection and renders status UI.
func (h *Handler) RenderDisconnectFeedback(w http.ResponseWriter, r *http.Request, session *types.SessionData, message string) {
	h.DeleteConnection(r.Context(), w, r)
	h.Soccer.RenderLoginStateOOB(w, r, session)
	h.Soccer.RenderLoginFeedback(w, r, "error", message)
}

// SyncCalendarSelection ensures the connection record has a valid calendar selection.
func (h *Handler) SyncCalendarSelection(ctx context.Context, record *ConnectionRecord, calendars []types.GoogleCalendarOption) (calendarID, summary string) {
	calendarID = strings.TrimSpace(record.CalendarID)
	if calendarID == "" {
		calendarID, summary = preferredCalendar(calendars)
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := h.Store().Put(ctx, record); err != nil {
			logging.WithContext(h.Logger, ctx).Error("google connection default calendar save failed", slog.Any("error", err))
		}
		return calendarID, summary
	}
	summary = calendarSummary(calendars, calendarID)
	if summary == "" {
		calendarID, summary = preferredCalendar(calendars)
	}
	if summary != "" && (record.CalendarID != calendarID || record.CalendarSummary != summary) {
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := h.Store().Put(ctx, record); err != nil {
			logging.WithContext(h.Logger, ctx).Error("google connection calendar sync failed", slog.Any("error", err))
		}
	}
	return calendarID, summary
}

// EncryptToken encrypts an OAuth token for storage.
func (h *Handler) EncryptToken(token *oauth2.Token) (string, error) {
	return h.encryptJSONValue(token)
}

// DecryptToken decrypts a stored OAuth token ciphertext.
func (h *Handler) DecryptToken(ciphertext string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := h.decryptJSONValue(ciphertext, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// LoadConnectionRecord loads the Google connection for the current request.
func (h *Handler) LoadConnectionRecord(ctx context.Context, r *http.Request) (*ConnectionRecord, error) {
	connectionID := GetConnectionID(r)
	if connectionID == "" {
		return nil, nil
	}
	return h.Store().Get(ctx, connectionID)
}

// DeleteConnection removes the Google connection and clears the cookie.
func (h *Handler) DeleteConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	connectionID := GetConnectionID(r)
	if connectionID != "" {
		if err := h.Store().Delete(ctx, connectionID); err != nil {
			logging.WithContext(h.Logger, ctx).Error(
				"google connection delete failed",
				slog.String("connection_id", connectionID),
				slog.Any("error", err),
			)
		}
	}
	ClearConnectionCookie(w, r)
}

// CurrentToken retrieves and refreshes the stored OAuth token.
func (h *Handler) CurrentToken(ctx context.Context, r *http.Request, record *ConnectionRecord) (*oauth2.Token, error) {
	storedToken, err := h.DecryptToken(record.TokenCiphertext)
	if err != nil {
		return nil, err
	}
	tokenSource := h.oauthConfigForRequest(r).TokenSource(h.httpContext(ctx), storedToken)
	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = storedToken.RefreshToken
	}
	if token.AccessToken != storedToken.AccessToken || token.RefreshToken != storedToken.RefreshToken || !token.Expiry.Equal(storedToken.Expiry) {
		encryptedToken, encryptErr := h.EncryptToken(token)
		if encryptErr != nil {
			return nil, encryptErr
		}
		record.TokenCiphertext = encryptedToken
		record.UpdatedAt = time.Now().UTC()
		if err := h.Store().Put(ctx, record); err != nil {
			return nil, err
		}
	}
	return token, nil
}

// ListCalendars retrieves writable calendars for a connection.
func (h *Handler) ListCalendars(ctx context.Context, r *http.Request, record *ConnectionRecord) ([]types.GoogleCalendarOption, error) {
	token, err := h.CurrentToken(ctx, r, record)
	if err != nil {
		return nil, err
	}
	return h.listCalendarsWithToken(h.httpContext(ctx), token)
}

// GoogleConnected returns true if a valid Google connection exists for the request.
func (h *Handler) GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if !h.GoogleAvailable() {
		return false
	}
	record, err := h.LoadConnectionRecord(ctx, r)
	if err != nil {
		logging.WithContext(h.Logger, ctx).Error("google connection read failed", slog.Any("error", err))
		ClearConnectionCookie(w, r)
		return false
	}
	return record != nil
}

// PopulateLoginState fills Google-related fields on login state props.
func (h *Handler) PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps) {
	if !props.GoogleAvailable {
		return
	}
	record, err := h.LoadConnectionRecord(ctx, r)
	if err != nil {
		logging.WithContext(h.Logger, ctx).Error("google connection read failed", slog.Any("error", err))
		ClearConnectionCookie(w, r)
		return
	}
	if record == nil {
		return
	}
	calendars, err := h.ListCalendars(ctx, r, record)
	if err != nil {
		logger := logging.WithContext(h.Logger, ctx)
		if isGoogleAuthRejected(err) {
			logger.Warn("google calendar connection expired", slog.Any("error", err))
			h.DeleteConnection(ctx, w, r)
		} else {
			logger.Error("google calendar list failed", slog.Any("error", err))
		}
		return
	}
	props.GoogleConnected = true
	props.GoogleCalendars = calendars
	props.SelectedGoogleCalendarID, props.GoogleCalendarSummary = h.SyncCalendarSelection(ctx, record, calendars)
}

func isGoogleAuthRejected(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
	}

	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		if strings.EqualFold(strings.TrimSpace(retrieveErr.ErrorCode), "invalid_grant") {
			return true
		}
		description := strings.ToLower(strings.TrimSpace(retrieveErr.ErrorDescription))
		return strings.Contains(description, "expired") || strings.Contains(description, "revoked")
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid_grant") &&
		(strings.Contains(message, "expired") || strings.Contains(message, "revoked"))
}

func (h *Handler) encryptJSONValue(data any) (string, error) {
	return internalsession.EncryptJSONValue(h.Config.SessionKey, data)
}

func (h *Handler) decryptJSONValue(value string, out any) error {
	return internalsession.DecryptJSONValue(h.Config.SessionKey, value, out)
}

func (h *Handler) oauthConfigForRequest(r *http.Request) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.Config.GoogleClientID,
		ClientSecret: h.Config.GoogleClientSecret,
		RedirectURL:  internalhttpx.RequestBaseURL(r) + "/soccer",
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar.events",
			"https://www.googleapis.com/auth/calendar.calendarlist.readonly",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  h.OAuthAuthURL,
			TokenURL: h.OAuthTokenURL,
		},
	}
}

func (h *Handler) httpContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, h.LPSClient)
}
