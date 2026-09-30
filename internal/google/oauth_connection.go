package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
	"portfolio/internal/logging"
	internalsession "portfolio/internal/session"
	"portfolio/internal/siteidentity"
	"portfolio/types"
)

// RenderDisconnectFeedback removes the Google connection and renders status UI.
func (h *Handler) RenderDisconnectFeedback(w http.ResponseWriter, r *http.Request, session *types.SessionData, message string) {
	h.DeleteConnection(r.Context(), w, r)
	h.Soccer.RenderLoginStateOOB(w, r, session)
	h.Soccer.RenderLoginFeedback(w, r, "error", message)
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
	record, err := h.Store().Get(ctx, connectionID)
	if err != nil || record == nil {
		return nil, err
	}
	if !siteidentity.SoccerOwnerAllowed(r.Context(), record.OwnerIssuer, record.OwnerSubject) {
		return nil, nil
	}
	if !record.accountVerified() {
		return nil, nil
	}
	return record, nil
}

// DeleteConnection removes the current site owner's Google connection and
// clears its cookie. A cookie naming another site owner's connection, or one
// whose removal failed, stays so that connection is never stranded with its
// token.
func (h *Handler) DeleteConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if connectionID := GetConnectionID(r); connectionID != "" && !h.releaseConnection(ctx, r, connectionID) {
		return
	}
	ClearConnectionCookie(w, r)
}

// releaseBrowserWideConnection releases the connection named by the
// browser-wide cookie that site owners shared before each had their own, and
// clears that cookie once the connection is gone. Another owner's connection
// stays with the cookie for that owner to release.
func (h *Handler) releaseBrowserWideConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if connectionID := browserWideConnectionID(r); connectionID != "" && h.releaseConnection(ctx, r, connectionID) {
		clearSoccerCookie(w, r, config.GoogleConnectionCookieName)
	}
}

// releaseConnection deletes the stored connection when this request may let
// it go: the current granted owner's connection, or a legacy connection saved
// before connections had owners and presented by a granted visitor. Holding
// the legacy cookie was the authority to delete it, and deleting grants no
// access, so its stored token is not stranded. Another owner's connection
// stays. It reports whether the connection is gone.
func (h *Handler) releaseConnection(ctx context.Context, r *http.Request, connectionID string) bool {
	logger := logging.WithContext(h.Logger, ctx)
	record, err := h.Store().Get(ctx, connectionID)
	if err != nil {
		logger.Error("google connection read before delete failed", slog.Any("error", err))
		return false
	}
	if record == nil {
		return true
	}
	legacy := record.OwnerIssuer == "" || record.OwnerSubject == ""
	if legacy && !siteidentity.SoccerPrivateAllowed(r.Context()) {
		return false
	}
	if !legacy && !siteidentity.SoccerOwnerAllowed(r.Context(), record.OwnerIssuer, record.OwnerSubject) {
		return false
	}
	if err := h.Store().Delete(ctx, connectionID); err != nil {
		logger.Error("google connection delete failed", slog.String("connection_id", connectionID), slog.Any("error", err))
		return false
	}
	if legacy {
		logger.Info("legacy ownerless google connection deleted", slog.String("connection_id", connectionID))
	}
	return true
}

// errStoredTokenUnreadable reports a stored OAuth token the site cannot
// decrypt, such as one sealed under a previous session key. Unlike a failed
// renewal, retrying never helps.
var errStoredTokenUnreadable = errors.New("stored google token unreadable")

// CurrentToken retrieves and refreshes the stored OAuth token.
func (h *Handler) CurrentToken(ctx context.Context, r *http.Request, record *ConnectionRecord) (*oauth2.Token, error) {
	storedToken, err := h.DecryptToken(record.TokenCiphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errStoredTokenUnreadable, err)
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

// GoogleConnected returns true if a valid Google connection exists for the
// request. A failed read reports no connection but keeps the cookie, so a
// transient storage error never strands the owner's connection.
func (h *Handler) GoogleConnected(ctx context.Context, _ http.ResponseWriter, r *http.Request) bool {
	if !h.GoogleAvailable() {
		return false
	}
	record, err := h.LoadConnectionRecord(ctx, r)
	if err != nil {
		logging.WithContext(h.Logger, ctx).Error("google connection read failed", slog.Any("error", err))
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
		// Keep the cookie: the connection may still exist once storage recovers.
		logging.WithContext(h.Logger, ctx).Error("google connection read failed", slog.Any("error", err))
		return
	}
	if record == nil {
		props.GoogleNeedsReconnect = h.ownerHasUnverifiedConnection(ctx, r)
		return
	}
	calendars, err := h.ListCalendars(ctx, r, record)
	if connectionUnusable(err) {
		logging.WithContext(h.Logger, ctx).Warn("google calendar connection unusable", slog.Any("error", err))
		h.DeleteConnection(ctx, w, r)
		return
	}
	props.GoogleConnected = true
	props.GoogleAccountEmail = record.AccountEmail
	if err != nil {
		// The connection stays: Google refused only this check, so the card
		// asks for a retry rather than offering to connect again.
		logging.WithContext(h.Logger, ctx).Error("google calendar list failed", slog.Any("error", err))
		props.GoogleCalendarsUnavailable = true
		return
	}
	props.GoogleCalendars = calendars
	props.SelectedGoogleCalendarID, props.GoogleCalendarSummary = h.SyncCalendarSelection(ctx, record, calendars)
	props.GoogleCalendarNeedsSelection = record.CalendarSelectionRequired || props.GoogleCalendarSummary == ""
}

// ownerHasUnverifiedConnection reports whether the browser-wide cookie names
// the current owner's connection saved before its Google account was
// verified. That connection stays unused until the owner reconnects or
// disconnects it.
func (h *Handler) ownerHasUnverifiedConnection(ctx context.Context, r *http.Request) bool {
	connectionID := browserWideConnectionID(r)
	if connectionID == "" {
		return false
	}
	record, err := h.Store().Get(ctx, connectionID)
	if err != nil {
		logging.WithContext(h.Logger, ctx).Error("google connection read failed", slog.Any("error", err))
		return false
	}
	return record != nil && !record.accountVerified() && siteidentity.SoccerOwnerAllowed(r.Context(), record.OwnerIssuer, record.OwnerSubject)
}

// connectionUnusable reports whether err shows the stored connection can
// never work again: Google rejected its grant, or the site cannot read its
// stored token. Any other failure may pass, so the connection stays.
func connectionUnusable(err error) bool {
	return isGoogleAuthRejected(err) || errors.Is(err, errStoredTokenUnreadable)
}

func isGoogleAuthRejected(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.credentialsRejected()
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
			"openid",
			"email",
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
