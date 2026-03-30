package soccer

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	"portfolio/cmd/web/partials"
	"portfolio/types"
)

// LoginStateProps returns the soccer auth panel props for the current request state.
func (h *Handler) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil,
		GoogleAvailable: h.Config.GoogleEnabled(),
		LoginAvailable:  h.Config.LoginEnabled(),
		SwapOOB:         swapOOB,
	}
	if session != nil {
		props.UserName = session.UserName
		props.Players = session.Players
	}
	if h.googleHooks != nil {
		h.googleHooks.PopulateLoginState(r.Context(), w, r, &props)
	}
	return props
}

// RenderLoginState renders the soccer auth panel for the given session.
func (h *Handler) RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := h.LoginStateProps(w, r, session, false)
	if err := partials.SoccerLoginState(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// RenderLoginFeedback renders an escaped status or error message for soccer auth and Google actions.
func RenderLoginFeedback(w http.ResponseWriter, kind, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	role := "status"
	if kind == "error" {
		role = "alert"
	}
	_, _ = io.WriteString(w, fmt.Sprintf(`<div class="soccer-login-message soccer-login-message-%s" role="%s">%s</div>`, kind, role, html.EscapeString(message)))
}

// RequestedScheduleGames resolves the game list implied by the current player or team selection.
func (h *Handler) RequestedScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string) ([]types.Game, error) {
	teamIDs := ParseTeamIDs(teamCodes)
	switch {
	case session != nil && len(playerIDs) > 0:
		return h.resolveScheduleGames(ctx, session, playerIDs, nil)
	case len(playerIDs) > 0:
		return nil, ErrPlayerSessionRequired
	case strings.TrimSpace(teamCodes) != "" && len(teamIDs) == 0:
		return nil, ErrInvalidTeamSelection
	case len(teamIDs) > 0:
		return h.resolveScheduleGames(ctx, session, nil, teamIDs)
	default:
		return nil, ErrScheduleSelection
	}
}

// GoogleAddScheduleErrorMessage returns the user-facing Google add message for soccer selection errors.
func GoogleAddScheduleErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrPlayerSessionRequired):
		return "Import a bearer JWT again before adding schedules for discovered players to Google Calendar."
	case errors.Is(err, ErrInvalidTeamSelection):
		return "Enter one or more numeric Let's Play Soccer team IDs separated by commas."
	case errors.Is(err, ErrScheduleSelection):
		return "Enter team IDs or choose at least one discovered player."
	default:
		return err.Error()
	}
}

// SelectedScheduleGames filters games by the currently selected IDs.
func SelectedScheduleGames(games []types.Game, selectedIDs map[string]struct{}) []types.Game {
	filteredGames := make([]types.Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	return filteredGames
}
