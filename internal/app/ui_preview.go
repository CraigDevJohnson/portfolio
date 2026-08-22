package app

import (
	"io"
	"net/http"
	"slices"
	"strings"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/internal/schedule"
	"portfolio/types"
)

var soccerPreviewFixtureNames = []string{
	"manual", "import", "token-invalid", "token-expired", "token-rejected", "token-upstream-error",
	"players", "no-players", "team-selection", "no-games", "upcoming", "past", "combined",
	"google-disconnected", "google-connected", "google-add-success", "google-add-error",
	"google-sync-success", "google-sync-error", "expired-session-reset", "loading",
}

type soccerPreviewPage struct {
	Page          pages.SoccerProps
	Results       *partials.SoccerTableFragmentProps
	TeamSelection *partials.SoccerTeamSelectProps
	Feedback      *partials.SoccerLoginFeedbackProps
	Loading       bool
}

func soccerPreviewFixture(name string) (soccerPreviewPage, bool) {
	page := soccerPreviewBasePage()
	players := soccerPreviewPlayers()
	upcoming := soccerPreviewUpcomingGames()
	past := soccerPreviewPastGames()

	switch name {
	case "manual":
		return page, true
	case "import":
		page.Page.AuthState.LoginAvailable = true
		return page, true
	case "token-invalid":
		page.Page.AuthState.LoginAvailable = true
		page.Page.ModalOpen = true
		page.Page.ModalFeedback = soccerPreviewFeedback("error", "Token format is invalid", "The imported value must be a JWT with three dot-separated sections.")
		return page, true
	case "token-expired":
		page.Page.AuthState.LoginAvailable = true
		page.Page.ModalOpen = true
		page.Page.ModalFeedback = soccerPreviewFeedback("error", "Token expired", "This JWT has expired. Copy a fresh bearer token from letsplaysoccer.com and import it again.")
		return page, true
	case "token-rejected":
		page.Page.AuthState.LoginAvailable = true
		page.Page.ModalOpen = true
		page.Page.ModalFeedback = soccerPreviewFeedback("rejected", "Token rejected", "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and try again.")
		return page, true
	case "token-upstream-error":
		page.Page.AuthState.LoginAvailable = true
		page.Page.ModalOpen = true
		page.Page.ModalFeedback = soccerPreviewFeedback("upstream", "Player lookup unavailable", "Could not reach Let's Play Soccer to look up your players. Try again in a moment.")
		return page, true
	case "players":
		page.Page.AuthState = soccerPreviewAuthenticatedState(players, false)
		return page, true
	case "no-players":
		page.Page.AuthState = soccerPreviewAuthenticatedState(nil, false)
		return page, true
	case "team-selection":
		page.Page.AuthState = soccerPreviewAuthenticatedState(players, false)
		page.TeamSelection = soccerPreviewTeamSelection(players)
		return page, true
	case "no-games":
		page.Page.AuthState.LoginAvailable = true
		page.Results = &partials.SoccerTableFragmentProps{
			Preview:         true,
			ImportAvailable: true,
			Message:         "No games found for the provided request.",
			Hint:            "Check your team IDs or choose at least one linked player.",
		}
		return page, true
	case "upcoming":
		page.Results = soccerPreviewResults(upcoming, nil, false, false)
		return page, true
	case "past":
		page.Results = soccerPreviewResults(nil, past, false, false)
		return page, true
	case "combined":
		page.Results = soccerPreviewResults(upcoming, past, false, false)
		return page, true
	case "google-disconnected":
		page.Page.AuthState = soccerPreviewGoogleState(false)
		page.Results = soccerPreviewResults(upcoming, past, true, false)
		return page, true
	case "google-connected":
		page.Page.AuthState = soccerPreviewGoogleState(true)
		page.Results = soccerPreviewResults(upcoming, past, true, true)
		return page, true
	case "google-add-success":
		page.Page.AuthState = soccerPreviewGoogleState(true)
		page.Results = soccerPreviewResults(upcoming, past, true, true)
		page.Results.GoogleFeedback = soccerPreviewFeedback("success", "Selected games added", "Added 2 selected game(s) to Google Calendar.")
		return page, true
	case "google-add-error":
		page.Page.AuthState = soccerPreviewGoogleState(true)
		page.Results = soccerPreviewResults(upcoming, past, true, true)
		page.Results.GoogleFeedback = soccerPreviewFeedback("google-error", "Selected games were not added", "Could not add the selected games to Google Calendar. Try again.")
		return page, true
	case "google-sync-success":
		page.Page.AuthState = soccerPreviewGoogleState(true)
		page.Results = soccerPreviewResults(nil, past, true, true)
		page.Results.GoogleFeedback = soccerPreviewFeedback("success", "Selected results synced", "2 game result(s) updated in Google Calendar.")
		return page, true
	case "google-sync-error":
		page.Page.AuthState = soccerPreviewGoogleState(true)
		page.Results = soccerPreviewResults(nil, past, true, true)
		page.Results.GoogleFeedback = soccerPreviewFeedback("google-error", "Selected results were not synced", "Could not sync past game results to Google Calendar. Try again.")
		return page, true
	case "expired-session-reset":
		page.Page.AuthState.LoginAvailable = true
		page.Feedback = soccerPreviewFeedback("error", "Imported session expired", "Your imported Let's Play Soccer token expired. Copy a fresh bearer JWT from letsplaysoccer.com and import it again.")
		return page, true
	case "loading":
		page.Page.AuthState.LoginAvailable = true
		page.Loading = true
		return page, true
	default:
		return soccerPreviewPage{}, false
	}
}

func soccerPreviewBasePage() soccerPreviewPage {
	return soccerPreviewPage{Page: pages.SoccerProps{
		Preview: true,
		AuthState: partials.SoccerLoginStateProps{
			Preview: true,
		},
	}}
}

func soccerPreviewAuthenticatedState(players []types.LPSPlayer, google bool) partials.SoccerLoginStateProps {
	state := partials.SoccerLoginStateProps{
		Authenticated:  true,
		LoginAvailable: true,
		Players:        slices.Clone(players),
		Preview:        true,
	}
	if google {
		state.GoogleAvailable = true
	}
	return state
}

func soccerPreviewGoogleState(connected bool) partials.SoccerLoginStateProps {
	state := soccerPreviewAuthenticatedState(soccerPreviewPlayers(), true)
	state.GoogleConnected = connected
	if connected {
		state.GoogleCalendarSummary = "Matchdays and travel notes"
		state.SelectedGoogleCalendarID = "preview-matchdays"
		state.GoogleCalendars = []types.GoogleCalendarOption{
			{ID: "preview-matchdays", Summary: "Matchdays and travel notes", Primary: true},
			{ID: "preview-shared", Summary: "Shared household calendar with an intentionally long wrapping label"},
		}
	}
	return state
}

func soccerPreviewPlayers() []types.LPSPlayer {
	return []types.LPSPlayer{
		{UPlayerID: 1669080, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		{UPlayerID: 1669081, FirstName: "Taylor Alexandra", LastName: "Johnson-Summit", IsMainPlayer: false},
	}
}

func soccerPreviewTeamSelection(players []types.LPSPlayer) *partials.SoccerTeamSelectProps {
	return &partials.SoccerTeamSelectProps{
		Preview:   true,
		PlayerIDs: []int{1669080, 1669081},
		PlayerGroups: []types.PlayerTeamGroup{
			{Player: players[0], Teams: []types.LPSTeam{
				{TeamID: 479691, TeamName: "Pond Mint United", Season: 169, PlayerID: 1669080},
				{TeamID: 479692, TeamName: "Treasure Valley After-Work Cooperative Football Club", Season: 169, PlayerID: 1669080},
			}},
			{Player: players[1], Teams: []types.LPSTeam{
				{TeamID: 479147, TeamName: "Campfire Rovers", Season: 170, PlayerID: 1669081},
			}},
		},
	}
}

func soccerPreviewUpcomingGames() []types.Game {
	return []types.Game{
		{
			ID: "preview-upcoming-1", DateTime: "Fri, Sep 4 at 7:15 PM", StartAt: "2026-09-04T19:15:00-06:00", EndAt: "2026-09-04T20:30:00-06:00",
			Field: "Field 2", Home: "Pond Mint United", Away: "Campfire Rovers", Season: "Fall 2026", PlayerTeamName: "Pond Mint United", OpponentTeamName: "Campfire Rovers", DivisionName: "Coed Premier",
			Facility: &types.Facility{Name: "Treasure Valley Indoor Sports and Community Fieldhouse", Address: "11448 W President Drive", City: "Boise", State: "ID", ZIP: "83713"},
		},
		{
			ID: "preview-upcoming-2", DateTime: "Fri, Sep 11 at 8:30 PM", StartAt: "2026-09-11T20:30:00-06:00", EndAt: "2026-09-11T21:45:00-06:00",
			Field: "Championship Field with the Extra-Long Sideline Name", Home: "Treasure Valley After-Work Cooperative Football Club", Away: "Rosehip Athletic", Season: "Fall 2026", PlayerTeamName: "Treasure Valley After-Work Cooperative Football Club", OpponentTeamName: "Rosehip Athletic", DivisionName: "Coed Premier",
			Facility: &types.Facility{Name: "West Boise Indoor Soccer and Community Recreation Complex", Address: "11448 W President Drive", City: "Boise", State: "ID", ZIP: "83713"},
		},
	}
}

func soccerPreviewPastGames() []types.Game {
	return []types.Game{
		{
			ID: "preview-past-2", DateTime: "Fri, Aug 28 at 8:30 PM", StartAt: "2026-08-28T20:30:00-06:00", EndAt: "2026-08-28T21:45:00-06:00",
			Field: "Field 1", Home: "Pond Mint United", Away: "Night Mulberry FC", Season: "Summer 2026", PlayerTeamName: "Pond Mint United", OpponentTeamName: "Night Mulberry FC", DivisionName: "Coed Premier", Result: "4 - 2",
			Facility: &types.Facility{Name: "Treasure Valley Indoor Sports and Community Fieldhouse", Address: "11448 W President Drive", City: "Boise", State: "ID", ZIP: "83713"},
		},
		{
			ID: "preview-past-1", DateTime: "Fri, Aug 21 at 7:15 PM", StartAt: "2026-08-21T19:15:00-06:00", EndAt: "2026-08-21T20:30:00-06:00",
			Field: "Field 3", Home: "Candle Oat Wanderers", Away: "Pond Mint United", Season: "Summer 2026", PlayerTeamName: "Pond Mint United", OpponentTeamName: "Candle Oat Wanderers", DivisionName: "Coed Premier", Result: "3 - 3",
			Facility: &types.Facility{Name: "Treasure Valley Indoor Sports and Community Fieldhouse", Address: "11448 W President Drive", City: "Boise", State: "ID", ZIP: "83713"},
		},
	}
}

func soccerPreviewResults(upcoming, past []types.Game, googleAvailable, googleConnected bool) *partials.SoccerTableFragmentProps {
	return &partials.SoccerTableFragmentProps{
		UpcomingGames:   slices.Clone(upcoming),
		PastGames:       slices.Clone(past),
		TeamCodes:       "479691,479147",
		PlayerIDs:       []int{1669080, 1669081},
		GoogleAvailable: googleAvailable,
		GoogleConnected: googleConnected,
		Preview:         true,
	}
}

func soccerPreviewFeedback(kind, title, message string) *partials.SoccerLoginFeedbackProps {
	return &partials.SoccerLoginFeedbackProps{Kind: kind, Title: title, Message: message}
}

func soccerPreviewPageHandler(w http.ResponseWriter, r *http.Request) {
	fixture, ok := soccerPreviewFixture(r.PathValue("fixture"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	page := fixture.Page
	page.InitialResults = fixture.Results
	page.InitialTeamSelection = fixture.TeamSelection
	page.InitialFeedback = fixture.Feedback
	page.Loading = fixture.Loading
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Soccer(page).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerPreviewDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	selected := r.Form["selected"]
	if len(selected) == 0 {
		http.Error(w, "select at least one preview game", http.StatusBadRequest)
		return
	}

	available := make(map[string]types.Game)
	previewGames := append(soccerPreviewUpcomingGames(), soccerPreviewPastGames()...)
	for index := range previewGames {
		available[previewGames[index].ID] = previewGames[index]
	}
	games := make([]types.Game, 0, len(selected))
	seen := make(map[string]struct{})
	for _, rawID := range selected {
		id := strings.TrimSpace(rawID)
		game, ok := available[id]
		if !ok || id == "" {
			http.Error(w, "one or more selected preview games were invalid", http.StatusBadRequest)
			return
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		games = append(games, game)
	}
	if len(games) == 0 {
		http.Error(w, "select at least one preview game", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/calendar")
	w.Header().Set("Content-Disposition", "attachment; filename=soccer_schedule.ics")
	_, _ = io.WriteString(w, schedule.BuildICS(games))
}
