package lps

import "portfolio/types"

// UserPlayerDiscovery is the normalized result of the LPS user check lookup.
type UserPlayerDiscovery struct {
	UserName string
	Players  []types.LPSPlayer
}

// UserCheckResponse is the raw LPS /users/check response payload.
type UserCheckResponse struct {
	AuthFailure bool              `json:"authFailure"`
	Error       string            `json:"error"`
	FirstName   string            `json:"first_name"`
	LastName    string            `json:"last_name"`
	Players     []types.LPSPlayer `json:"players"`
	UserPlayers []struct {
		PlayerID int  `json:"player_id"`
		Deleted  bool `json:"deleted"`
	} `json:"user_players"`
}

// TeamSummary is the subset of team metadata returned by the LPS team endpoints.
type TeamSummary struct {
	UTeamID      int    `json:"UTeamID"`
	TeamName     string `json:"team_name"`
	DivisionName string `json:"division_name"`
	FacilityID   int    `json:"FacilityID"`
	FacilityName string `json:"facility_name"`
	Season       int    `json:"Season"`
}

// TeamScheduleGame is a raw game record from the LPS team schedule response.
type TeamScheduleGame struct {
	UGameID           int         `json:"UGameID"`
	FieldName         string      `json:"field_name"`
	SchedGameDateTime string      `json:"SchedGameDateTime"`
	SchedGameEndTime  *string     `json:"schedGameEndTime"`
	FacilityName      string      `json:"facilityName"`
	Result            string      `json:"result"`
	Field             int         `json:"Field"`
	Season            int         `json:"Season"`
	FacilityID        int         `json:"FacilityID"`
	UTeam1            int         `json:"UTeam1"`
	UTeam2            int         `json:"UTeam2"`
	TeamIDSelected    *int        `json:"team_id_selected"`
	HomeTeam          TeamSummary `json:"home_team"`
	VisitorTeam       TeamSummary `json:"visitor_team"`
}

// TeamScheduleResponse is the raw LPS /teams/{id} response payload.
type TeamScheduleResponse struct {
	Games []TeamScheduleGame `json:"games"`
	Team  TeamSummary        `json:"team"`
}

// Facility is the raw LPS facility payload.
type Facility struct {
	FacilityID   int    `json:"FacilityID"`
	FacilityName string `json:"FacilityName"`
	Address      string `json:"Address"`
	City         string `json:"City"`
	State        string `json:"State"`
	ZIP          string `json:"ZIP"`
}
