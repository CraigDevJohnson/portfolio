package soccer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive"
	"portfolio/types"
)

// RemovePlayerHandler erases one linked player's retained history for every
// site owner. The short-lived import records and table backups are outside
// its reach and keep the player until they expire. The route already requires the current site session and soccer
// grant; the handler also requires this owner's unexpired import and a fresh
// LPS lookup through it that still links the requested player ID. Team,
// season, game, and facility facts stay. A completed removal clears the
// import, so only a later, deliberate import can collect the player again.
// The response replaces the LPS card and reports only this owner's outcome.
func (h *Handler) RemovePlayerHandler(w http.ResponseWriter, r *http.Request) {
	store, enabled := h.ArchiveStore().(soccerarchive.PlayerRemovalStore)
	if !enabled {
		http.Error(w, "Player data removal is unavailable because this server keeps no player history.", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the removal request.", http.StatusBadRequest)
		return
	}
	playerID, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("player_id")))
	if err != nil || playerID <= 0 {
		http.Error(w, "Choose a linked player to remove.", http.StatusBadRequest)
		return
	}

	session, _ := h.LoadSession(w, r)
	if session == nil || strings.TrimSpace(session.JWT) == "" {
		h.renderRemovalOutcome(w, r, nil, http.StatusForbidden, removalRefused("Import your LPS access first. Removal needs it to confirm the player, so nothing was removed."))
		return
	}

	discovery, err := lps.FetchUserPlayers(r.Context(), h.Config.LPSAPIBaseURL, h.LPSClient, session.JWT)
	if err != nil {
		var fetchErr *lps.FetchError
		if errors.As(err, &fetchErr) && (fetchErr.Kind == lps.ErrorUnauthorized || fetchErr.Kind == lps.ErrorForbidden || fetchErr.Kind == lps.ErrorMalformedToken) {
			// LPS ended this import, so the browser stops offering it.
			h.clearSession(w, r)
			w.Header().Set("HX-Trigger", "soccer-logout")
			h.renderRemovalOutcome(w, r, nil, http.StatusForbidden, &partials.FeedbackProps{
				Kind: partials.FeedbackError, Title: "Imported LPS access ended",
				Message: "Let's Play Soccer rejected this import, so nothing was removed. Import fresh access, then request removal again.",
			})
			return
		}
		logging.WithContext(h.Logger, r.Context()).Warn("soccer player removal proof unavailable", slog.Any("error", err))
		h.renderRemovalOutcome(w, r, session, http.StatusBadGateway, removalRefused("Could not reach Let's Play Soccer to confirm the player, so nothing was removed. Try again in a moment."))
		return
	}
	player, linked := linkedPlayer(discovery.Players, playerID)
	if !linked {
		h.renderRemovalOutcome(w, r, session, http.StatusForbidden, removalRefused(fmt.Sprintf("Your current LPS access does not link player %d, so nothing was removed.", playerID)))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := store.DeletePlayerEvidence(ctx, playerID); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer player removal failed", slog.Int("player_id", playerID), slog.Any("error", err))
		h.renderRemovalOutcome(w, r, session, http.StatusServiceUnavailable, removalRefused("The player's kept data could not be fully removed. Try again in a moment; your import is kept for the retry."))
		return
	}

	h.clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	h.renderRemovalOutcome(w, r, nil, http.StatusOK, &partials.FeedbackProps{
		Kind: partials.FeedbackSuccess, Title: "Player data removed",
		Message: fmt.Sprintf("Removed the identity, site account links, and team-season links of %s (LPS ID %d) from the kept history for every site account. Team and game history stays. Each import's short-lived record keeps the player until it expires, within 12 hours of that import, and table backups keep removed history until they expire. This browser's import was cleared, and importing again collects the player again.", playerDisplayName(player), playerID),
	})
}

// playerRemovalEnabled reports whether this server keeps player history that
// a verified request can remove.
func (h *Handler) playerRemovalEnabled() bool {
	_, enabled := h.ArchiveStore().(soccerarchive.PlayerRemovalStore)
	return enabled
}

func removalRefused(message string) *partials.FeedbackProps {
	return &partials.FeedbackProps{Kind: partials.FeedbackError, Title: "Player data not removed", Message: message}
}

func linkedPlayer(players []types.LPSPlayer, playerID int) (types.LPSPlayer, bool) {
	for _, player := range players {
		if player.UPlayerID == playerID {
			return player, true
		}
	}
	return types.LPSPlayer{}, false
}

func playerDisplayName(player types.LPSPlayer) string {
	if name := strings.TrimSpace(player.FirstName + " " + player.LastName); name != "" {
		return name
	}
	return "Player " + strconv.Itoa(player.UPlayerID)
}

// renderRemovalOutcome replaces the LPS card with the removal outcome. With
// no session the import has ended, so the planner's private stages reset
// too. An error keeps its status and is marked for htmx to swap.
func (h *Handler) renderRemovalOutcome(w http.ResponseWriter, r *http.Request, session *types.SessionData, status int, notice *partials.FeedbackProps) {
	props := h.LoginStateProps(w, r, session, false)
	props.ImportNotice = notice
	props.ResetWorkflow = session == nil
	h.setHTMLContentType(w)
	if status != http.StatusOK {
		w.Header().Set("X-Portal-Fragment-Error", "true")
	}
	w.WriteHeader(status)
	if err := partials.SoccerLoginState(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer player removal render failed", slog.Any("error", err))
	}
}
