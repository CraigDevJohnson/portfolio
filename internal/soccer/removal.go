package soccer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/siteidentity"
	"portfolio/internal/soccerarchive"
)

// RemovePlayerHandler erases a confirmed linked player's retained archive data.
func (h *Handler) RemovePlayerHandler(w http.ResponseWriter, r *http.Request) {
	store, enabled := h.ArchiveStore().(soccerarchive.PlayerRemovalStore)
	if !enabled {
		http.Error(w, "Player data removal is unavailable.", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the removal request.", http.StatusBadRequest)
		return
	}
	playerID, err := strconv.Atoi(r.PostForm.Get("player_id"))
	if err != nil || playerID <= 0 {
		http.Error(w, "Enter a valid player ID.", http.StatusBadRequest)
		return
	}
	session, _ := h.LoadSession(w, r)
	if session == nil || session.JWT == "" || !siteidentity.HasGrantForOwner(r.Context(), siteidentity.GrantSoccer, session.OwnerIssuer, session.OwnerSubject) {
		http.Error(w, "A current linked-player import is required for removal.", http.StatusForbidden)
		return
	}
	discovery, err := lps.FetchUserPlayers(r.Context(), h.Config.LPSAPIBaseURL, h.LPSClient, session.JWT)
	if err != nil {
		var fetchErr *lps.FetchError
		if errors.As(err, &fetchErr) && (fetchErr.Kind == lps.ErrorUnauthorized || fetchErr.Kind == lps.ErrorForbidden) {
			http.Error(w, "A current linked-player import is required for removal.", http.StatusForbidden)
			return
		}
		logging.WithContext(h.Logger, r.Context()).Warn("player removal proof unavailable", slog.Any("error", err))
		http.Error(w, "Player access could not be verified. Try again later.", http.StatusBadGateway)
		return
	}
	verified := false
	for _, player := range discovery.Players {
		if player.UPlayerID == playerID {
			verified = true
			break
		}
	}
	if !verified {
		http.Error(w, "Player access could not be verified for removal.", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := store.DeletePlayerEvidence(ctx, playerID); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("player data removal failed", slog.Any("error", err))
		http.Error(w, "Player data could not be removed. Try again later.", http.StatusServiceUnavailable)
		return
	}
	h.clearSession(w, r)
	http.Redirect(w, r, "/soccer?player_removed=1", http.StatusSeeOther)
}
