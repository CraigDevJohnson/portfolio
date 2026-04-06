package soccer

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func (h *Handler) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil,
		GoogleAvailable: h.Config.GoogleEnabled(),
		LoginAvailable:  h.Config.LoginEnabled(),
		SwapOOB:         swapOOB,
	}
	if session != nil {
		props.Players = session.Players
	}
	if h.googleHooks != nil {
		h.googleHooks.PopulateLoginState(r.Context(), w, r, &props)
	}
	return props
}

func (h *Handler) RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := h.LoginStateProps(w, r, session, false)
	if err := partials.SoccerLoginState(props).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func RenderLoginFeedback(w http.ResponseWriter, r *http.Request, kind, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := partials.SoccerLoginFeedbackProps{Kind: kind, Message: message}
	if err := partials.SoccerLoginFeedback(props).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) RequestedScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string) ([]types.Game, error) {
	teamIDs := parseTeamIDs(teamCodes)
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

func googleAddScheduleErrorMessage(err error) string {
	message, _, ok := scheduleSelectionFeedback(err)
	if ok {
		return message
	}
	return err.Error()
}

func (h *Handler) ResolveGoogleAddSelection(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool) {
	selectedIDs := parseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		return nil, nil, "Select at least one game to add to Google Calendar.", false
	}

	input := parseScheduleFormInput(r.Form)
	if hasInvalidPlayerInput(input.RawPlayerIDs, input.PlayerIDs) {
		return nil, nil, "One or more selected players were invalid. Clear the imported players and import again to refresh the discovered list.", false
	}

	session, _ := h.LoadSession(w, r)
	games, err := h.RequestedScheduleGames(r.Context(), session, input.PlayerIDs, input.TeamCodes)
	if err != nil {
		return nil, nil, googleAddScheduleErrorMessage(err), false
	}

	filteredGames := selectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		return nil, nil, "No selected games were found to add.", false
	}

	return session, filteredGames, "", true
}

func applyScheduleSelectionError(props *partials.SoccerTableFragmentProps, err error) bool {
	message, hint, ok := scheduleSelectionFeedback(err)
	if !ok {
		return false
	}
	props.Message = message
	props.Hint = hint
	return true
}

func scheduleSelectionFeedback(err error) (message, hint string, ok bool) {
	switch {
	case errors.Is(err, ErrPlayerSessionRequired):
		return "Import a bearer JWT again to fetch schedules for your discovered players.",
			"Your previous session is no longer available.", true
	case errors.Is(err, ErrInvalidTeamSelection):
		return "One or more team IDs were invalid.",
			"Enter numeric Let's Play Soccer team IDs separated by commas.", true
	case errors.Is(err, ErrScheduleSelection):
		return "Enter team IDs or choose at least one discovered player.",
			"Manual team ID entry still works if you do not want to import a token.", true
	default:
		return "", "", false
	}
}

func selectedScheduleGames(games []types.Game, selectedIDs map[string]struct{}) []types.Game {
	filteredGames := make([]types.Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	return filteredGames
}
