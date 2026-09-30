package soccer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	internalsession "portfolio/internal/session"
	"portfolio/internal/siteidentity"
	"portfolio/internal/soccerarchive"
	"portfolio/types"
)

// DefaultHistoryImportLookupBudget is how long an import that collects
// linked-player history may spend on LPS lookups, counted from when the
// request arrives: the /users/check account lookup, then each linked
// player's my_teams lookup, one at a time in the order LPS lists the
// players. A player LPS has not listed by then is skipped for this import.
// At most historyWriteTimeout of archive writes and importRecordTimeout for
// the import record follow.
const DefaultHistoryImportLookupBudget = 11 * time.Second

// DefaultHistoryImportBudget bounds the whole of an import that collects
// linked-player history, counted from when the request arrives. Its
// response's Google connection check (a connection read, a possible token
// refresh, and a calendar list) gets what remains after the lookups and
// writes, so the import's work ends within 24 s, the budget Google add and
// result sync keep, leaving 5 s of API Gateway's 29 s for the Lambda and
// gateway around it. An import that collects no history is bounded only by
// the LPS and Google clients' timeouts, as before.
const DefaultHistoryImportBudget = DefaultHistoryImportLookupBudget + historyWriteTimeout + importRecordTimeout

const (
	// historyWriteTimeout bounds saving an import's linked-player history.
	historyWriteTimeout = 10 * time.Second
	// importRecordTimeout bounds saving the import's session record.
	importRecordTimeout = 3 * time.Second
)

// ImportHandler validates an imported JWT, discovers linked players, and stores
// the session. When durable collection is wired and the visitor submitted the
// disclosed import, it first records every linked player's team-season
// memberships under the site owner and refuses the import if that fails. Its
// LPS lookups then share one deadline (HistoryImportLookupBudget), and its
// response's Google check ends by the import's own (HistoryImportBudget).
func (h *Handler) ImportHandler(w http.ResponseWriter, r *http.Request) {
	arrived := time.Now()
	if !h.Config.LoginEnabled() {
		h.RenderLoginFeedback(w, r, "error", "JWT import is unavailable until the session encryption key is configured on the server.")
		return
	}
	if !h.LoginLimiter.Allow(httpx.ClientIP(r)) {
		h.RenderLoginFeedback(w, r, "error", "Too many import attempts. Wait a minute and try again.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		h.RenderLoginFeedback(w, r, "error", "Could not read the import form. Try again.")
		return
	}

	jwt, err := lps.NormalizeImportedJWT(r.FormValue("jwt"))
	if err != nil {
		h.RenderLoginFeedback(w, r, "error", err.Error())
		return
	}

	membershipStore, collecting := h.historyCollection(r)
	lookupCtx := r.Context()
	if collecting {
		var stopLookups context.CancelFunc
		lookupCtx, stopLookups = context.WithDeadline(r.Context(), arrived.Add(h.historyImportLookupBudget()))
		defer stopLookups()
	}

	discovery, err := lps.FetchUserPlayers(lookupCtx, h.Config.LPSAPIBaseURL, h.LPSClient, jwt)
	if err != nil {
		var fetchErr *lps.FetchError
		if errors.As(err, &fetchErr) {
			switch fetchErr.Kind {
			case lps.ErrorUnauthorized, lps.ErrorForbidden:
				h.RenderLoginFeedback(w, r, "error", "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and try again.")
				return
			case lps.ErrorUpstream:
				h.RenderLoginFeedback(w, r, "error", "Could not reach Let's Play Soccer to look up your players. Try again in a moment.")
				return
			}
		}
		h.RenderLoginFeedback(w, r, "error", err.Error())
		return
	}
	if len(discovery.Players) == 0 {
		h.RenderLoginFeedback(w, r, "error", "No linked players found for this account.")
		return
	}

	var historyNotice *partials.FeedbackProps
	if collecting {
		var historyFailure string
		historyNotice, historyFailure = h.collectLinkedPlayerHistory(lookupCtx, r, membershipStore, jwt, discovery.Players)
		if historyFailure != "" {
			h.RenderLoginFeedback(w, r, "error", historyFailure)
			return
		}
	}
	now := time.Now()
	sessionID := generateSessionID()
	session := types.SessionData{
		JWT:         jwt,
		UserName:    discovery.UserName,
		Players:     discovery.Players,
		ExpiresAt:   lps.ImportedSessionExpiry(jwt, now),
		SessionID:   sessionID,
		StartedAt:   now,
		ImportGuard: generateSessionID(),
	}
	if principal, ok := siteidentity.PrincipalFromContext(r.Context()); ok {
		session.OwnerIssuer = principal.Issuer
		session.OwnerSubject = principal.Subject
	}
	if err := h.setSession(w, r, &session); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer import session write failed", slog.Any("error", err))
		h.RenderLoginFeedback(w, r, "error", "The import succeeded, but the session cookie could not be saved.")
		return
	}
	h.setImportGuard(w, r, &session)

	persistCtx, cancelPersist := context.WithTimeout(r.Context(), importRecordTimeout)
	defer cancelPersist()
	if err := h.persistSessionRecord(persistCtx, sessionID, &session); err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer import session persistence failed", slog.Any("error", err))
	}

	w.Header().Set("HX-Trigger", "soccer-workflow-reset")
	h.setHTMLContentType(w)
	// Only the Google check shares the import's deadline. The fragment
	// renders under the request's own context, which templ would refuse
	// once that deadline had passed.
	loginStateRequest := r
	if collecting {
		checkCtx, stopCheck := context.WithDeadline(r.Context(), arrived.Add(h.historyImportBudget()))
		defer stopCheck()
		loginStateRequest = r.WithContext(checkCtx)
	}
	loginState := h.LoginStateProps(w, loginStateRequest, &session, true)
	loginState.ImportNotice = historyNotice
	if err := partials.SoccerLoginState(loginState).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, `<div class="soccer-login-success" data-login-success>Import saved in this browser until its JWT expires, for up to 12 hours. Choose your players below.</div>`)
}

// historyCollection returns the durable membership store when this import
// collects linked-player history: collection is wired and the visitor
// submitted the import form that disclosed it.
func (h *Handler) historyCollection(r *http.Request) (soccerarchive.MembershipStore, bool) {
	store, enabled := h.ArchiveStore().(soccerarchive.MembershipStore)
	return store, enabled && r.FormValue(partials.SoccerHistoryNoticeField) == partials.SoccerHistoryNoticeIndefinite
}

func (h *Handler) historyImportLookupBudget() time.Duration {
	if h.HistoryImportLookupBudget > 0 {
		return h.HistoryImportLookupBudget
	}
	return DefaultHistoryImportLookupBudget
}

func (h *Handler) historyImportBudget() time.Duration {
	if h.HistoryImportBudget > 0 {
		return h.HistoryImportBudget
	}
	return DefaultHistoryImportBudget
}

// collectLinkedPlayerHistory records every linked player's teams in the
// durable archive, looking them up until lookupCtx's deadline. It returns the
// reason to show, and the import then stops so the visitor can try again,
// when the history the visitor accepted could not be collected. Neither a
// player LPS has not listed by the deadline nor a new team refused because
// the reviewed history capacity is full stops the import: the rest of the
// history is saved, and the returned notice names what was left out.
func (h *Handler) collectLinkedPlayerHistory(lookupCtx context.Context, r *http.Request, membershipStore soccerarchive.MembershipStore, jwt string, players []types.LPSPlayer) (notice *partials.FeedbackProps, failure string) {
	principal, signedIn := siteidentity.PrincipalFromContext(r.Context())
	if !signedIn || !siteidentity.HasGrantForOwner(r.Context(), siteidentity.GrantSoccer, principal.Issuer, principal.Subject) {
		return nil, "Sign in with Soccer access before importing linked players."
	}
	discovered, err := h.discoverImportedPlayerTeams(lookupCtx, jwt, players)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer player team discovery failed", slog.Any("error", err))
		return nil, "Could not look up every linked player. No player history was saved; try the import again."
	}
	persistCtx, cancelPersist := context.WithTimeout(r.Context(), historyWriteTimeout)
	defer cancelPersist()
	err = membershipStore.SavePlayerDiscovery(persistCtx, &soccerarchive.PlayerDiscovery{
		OwnerIssuer: principal.Issuer, OwnerSubject: principal.Subject,
		Players: players, KnownTeams: discovered.teams, Memberships: discovered.memberships, ObservedAt: time.Now(),
	})
	var refused *soccerarchive.AdmissionError
	switch {
	case errors.As(err, &refused):
		h.logAdmissionRejected(r.Context(), refused)
	case err != nil:
		logging.WithContext(h.Logger, r.Context()).Error("soccer player history write failed", slog.Any("error", err))
		return nil, "Linked-player history could not be saved. Try the import again."
	}
	return uncollectedHistoryNotice(discovered.late, refused), ""
}

// uncollectedHistoryNotice names the linked-player history an import saved
// without: players LPS had not listed by the lookup deadline and new teams
// the reviewed capacity refused. It is nil when nothing was left out.
func uncollectedHistoryNotice(late []types.LPSPlayer, refused *soccerarchive.AdmissionError) *partials.FeedbackProps {
	if len(late) == 0 {
		if refused == nil {
			return nil
		}
		return &partials.FeedbackProps{
			Kind:    partials.FeedbackWarning,
			Title:   "History collection is full",
			Message: teamsNotAdded(refused.TeamIDs) + " Your import and your other teams' history were saved.",
		}
	}
	names := make([]string, 0, len(late))
	for _, player := range late {
		names = append(names, playerDisplayName(player))
	}
	message := "Let's Play Soccer did not list teams for " + listWords(names) + " in time, so their history was not collected. A later import may collect it."
	if refused != nil {
		message += " " + teamsNotAdded(refused.TeamIDs)
	}
	return &partials.FeedbackProps{
		Kind:    partials.FeedbackWarning,
		Title:   "Some player history was not collected",
		Message: message + " Your import was saved.",
	}
}

// teamsNotAdded says which teams history collection refused at capacity.
func teamsNotAdded(teamIDs []int) string {
	names := make([]string, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		names = append(names, strconv.Itoa(teamID))
	}
	switch len(names) {
	case 0:
		return "Some teams were not added to history collection because its reviewed capacity is full."
	case 1:
		return "Team " + names[0] + " was not added to history collection because its reviewed capacity is full."
	default:
		return "Teams " + listWords(names) + " were not added to history collection because its reviewed capacity is full."
	}
}

// listWords joins words as a sentence lists them: "a", "a and b", "a, b and c".
func listWords(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// importedPlayerTeams is what an import's lookups found about its linked
// players' teams.
type importedPlayerTeams struct {
	teams       []lps.TeamSummary
	memberships []soccerarchive.PlayerMembership
	// late lists the players LPS had not listed by the lookup deadline.
	late []types.LPSPlayer
}

// discoverImportedPlayerTeams looks up every linked player's teams, one
// player at a time in the order LPS lists them so LPS sees one request at a
// time from an import, until ctx's deadline. A player LPS rejects as invalid
// (a 400 or 404) has no team-seasons to observe, and a player whose lookup
// has not finished by the deadline is skipped for this import and returned
// as late. Either keeps its identity and owner link without memberships, and
// the lookup goes on; once the deadline has passed, every remaining player
// is late. The order is the same on every import, so an LPS that stays slow
// rather than briefly slow can leave the same last players late each time.
// Any other failure, such as an upstream error or a rejected JWT, stops the
// discovery.
func (h *Handler) discoverImportedPlayerTeams(ctx context.Context, jwt string, players []types.LPSPlayer) (importedPlayerTeams, error) {
	resolver := lps.NewScheduleResolver(h.Config.LPSAPIBaseURL, h.LPSClient, jwt)
	knownTeams := make(map[int]lps.TeamSummary)
	discovered := importedPlayerTeams{memberships: make([]soccerarchive.PlayerMembership, 0)}
	for _, player := range players {
		teams, err := resolver.FetchPlayerTeams(ctx, player.UPlayerID)
		var fetchErr *lps.FetchError
		switch {
		case errors.As(err, &fetchErr) && fetchErr.Kind == lps.ErrorInvalidPlayer:
			logging.WithContext(h.Logger, ctx).Warn("soccer linked player has no LPS teams to observe", slog.Int("player_id", player.UPlayerID), slog.Any("error", err))
			continue
		case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
			logging.WithContext(h.Logger, ctx).Warn("soccer linked player team lookup missed the import deadline", slog.Int("player_id", player.UPlayerID), slog.Any("error", err))
			discovered.late = append(discovered.late, player)
			continue
		case err != nil:
			return importedPlayerTeams{}, err
		}
		for _, team := range teams {
			if team.UTeamID <= 0 {
				continue
			}
			knownTeams[team.UTeamID] = team
			if team.Season <= 0 {
				continue
			}
			discovered.memberships = append(discovered.memberships, soccerarchive.PlayerMembership{PlayerID: player.UPlayerID, Team: team})
		}
	}
	discovered.teams = make([]lps.TeamSummary, 0, len(knownTeams))
	for _, team := range knownTeams {
		discovered.teams = append(discovered.teams, team)
	}
	return discovered, nil
}

// LogoutHandler clears the imported soccer session.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	h.RenderWorkflowReset(w, r, nil)
}

// ClearImportedAccess removes retained imported LPS access from the browser.
// Explicit site sign-out calls it; a site-session timeout does not, so the
// same owner can use a still-valid import after signing in again.
func (h *Handler) ClearImportedAccess(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
}

func (h *Handler) getSession(r *http.Request) (*types.SessionData, error) {
	cookie, err := r.Cookie(config.LPSSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session types.SessionData
	err = internalsession.DecryptJSONValue(h.Config.SessionKey, cookie.Value, &session)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if session.JWT != "" && (session.ExpiresAt.IsZero() || !now.Before(lps.JWTExpiry(session.JWT))) {
		return nil, ErrSessionExpired
	}
	if !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	if (session.JWT != "" || len(session.Players) > 0) && !importGuardMatches(r, session.ImportGuard) {
		return nil, errImportGuardMismatch
	}
	hasPrivateState := session.JWT != "" || len(session.Players) > 0 || session.Workflow.Source == "imported"
	if hasPrivateState && !siteidentity.SoccerOwnerAllowed(r.Context(), session.OwnerIssuer, session.OwnerSubject) {
		if siteidentity.ForeignOwner(r.Context(), session.OwnerIssuer, session.OwnerSubject) {
			return nil, ErrSessionOwnerMismatch
		}
		return nil, errSessionWithheld
	}
	session.Workflow = normalizeWorkflowState(&session.Workflow, session.Players)
	return &session, nil
}

// LoadSession loads the imported soccer session and reports whether it was cleared.
func (h *Handler) LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool) {
	session, err := h.getSession(r)
	if errors.Is(err, errSessionWithheld) {
		return nil, false
	}
	if errors.Is(err, ErrSessionExpired) || errors.Is(err, errImportGuardMismatch) {
		h.clearSession(w, r)
		return nil, true
	}
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer session read failed", slog.Any("error", err))
		h.clearSession(w, r)
		return nil, true
	}
	return session, false
}

func (h *Handler) setSession(w http.ResponseWriter, r *http.Request, session *types.SessionData) error {
	maxAge := 0
	if session.JWT != "" {
		maxAge = int(time.Until(session.ExpiresAt).Seconds())
		if maxAge < 1 {
			return ErrSessionExpired
		}
	}
	encrypted, err := internalsession.EncryptJSONValue(h.Config.SessionKey, session)
	if err != nil {
		return err
	}
	cookie := httpx.NewSecureCookie(r, config.LPSSessionCookieName, encrypted, config.SoccerCookiePath, maxAge, http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
	if maxAge > 0 {
		cookie.Expires = session.ExpiresAt
	}
	http.SetCookie(w, cookie)
	return nil
}

// setImportGuard writes the import's guard cookie, which lasts as long as the
// import. Soccer responses rewrite lps_session with the whole payload, but
// only an import writes this cookie, so a response still in flight when
// sign-out clears both cannot bring back usable imported access.
func (h *Handler) setImportGuard(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	cookie := httpx.NewSecureCookie(r, config.LPSImportGuardCookieName, session.ImportGuard, config.SoccerCookiePath, int(time.Until(session.ExpiresAt).Seconds()), http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
	cookie.Expires = session.ExpiresAt
	http.SetCookie(w, cookie)
}

// importGuardMatches reports whether the browser still holds the guard cookie
// the import wrote with this payload.
func importGuardMatches(r *http.Request, guard string) bool {
	cookie, err := r.Cookie(config.LPSImportGuardCookieName)
	return err == nil && guard != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(guard)) == 1
}

// clearSession removes the imported session and its guard from the browser.
func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{config.LPSSessionCookieName, config.LPSImportGuardCookieName} {
		cookie := httpx.NewSecureCookie(r, name, "", config.SoccerCookiePath, -1, http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
		cookie.Expires = time.Unix(0, 0)
		http.SetCookie(w, cookie)
	}
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) persistSessionRecord(ctx context.Context, sessionID string, session *types.SessionData) error {
	playersJSON, err := marshalPlayersJSON(session.Players)
	if err != nil {
		return err
	}
	record := &SoccerSessionRecord{
		SessionID:    sessionID,
		OwnerIssuer:  session.OwnerIssuer,
		OwnerSubject: session.OwnerSubject,
		UserName:     session.UserName,
		PlayersJSON:  playersJSON,
		StartedAt:    session.StartedAt,
		ExpiresAt:    session.ExpiresAt,
		TTL:          session.ExpiresAt.Unix(),
	}
	return h.Store().Put(ctx, record)
}
