package types

import "time"

// Experience is a work history entry displayed on the experience page.
type Experience struct {
	ID               int
	Position         string
	Company          string
	Duration         string
	Responsibilities string
	Technologies     []string
	SkillAreas       string
	Side             string
}

// Skill is a technical skill displayed on the skills page.
type Skill struct {
	ID          int
	Name        string
	Icon        string
	IconPath    string
	Link        string
	Proficiency string // "expert", "advanced", "intermediate", "familiar"
	Featured    bool   // Whether to show in featured skills section
	Category    string // Category this skill belongs to (populated for featured skills)
	Description string // Short description for skill detail view
}

// SkillCategory groups related skills under a named heading.
type SkillCategory struct {
	Name   string
	Skills []Skill
}

// Project is a portfolio project displayed on the projects page.
type Project struct {
	ID           int
	Name         string
	Intro        string
	Description  string
	Technologies []string
	Image        string
	GitHubURL    string
	DemoURL      string
	Category     string
}

// Game is a soccer schedule entry from the LPS API.
type Game struct {
	ID               string `json:"id"`
	DateTime         string `json:"datetime"`
	StartAt          string `json:"start_at,omitempty"`
	EndAt            string `json:"end_at,omitempty"`
	Field            string `json:"field"`
	Location         string `json:"location,omitempty"`
	Home             string `json:"home"`
	Away             string `json:"away"`
	Season           string `json:"season"`
	PlayerTeamName   string `json:"player_team_name,omitempty"`
	OpponentTeamName string `json:"opponent_team_name,omitempty"`
	DivisionName     string `json:"division_name,omitempty"`
	FacilityID       int    `json:"facility_id,omitempty"`
	FacilityName     string `json:"facility_name,omitempty"`
	FacilityAddress  string `json:"facility_address,omitempty"`
	FacilityCity     string `json:"facility_city,omitempty"`
	FacilityState    string `json:"facility_state,omitempty"`
	FacilityZIP      string `json:"facility_zip,omitempty"`
	Result           string `json:"result,omitempty"`
}

// LambdaGamesResponse is the envelope for the LPS games API response.
type LambdaGamesResponse struct {
	Games []Game `json:"games"`
}

// LPSPlayer is a player linked to a Let's Play Soccer user account.
type LPSPlayer struct {
	UPlayerID    int    `json:"UPlayerID"`
	FirstName    string `json:"FirstName"`
	LastName     string `json:"LastName"`
	IsMainPlayer bool   `json:"is_main_player"`
}

// SessionData holds the encrypted cookie state for a soccer JWT import session.
type SessionData struct {
	JWT       string      `json:"jwt"`
	UserID    int         `json:"user_id"`
	UserName  string      `json:"user_name"`
	Players   []LPSPlayer `json:"players"`
	ExpiresAt time.Time   `json:"expires_at"`
}

// GoogleCalendarOption is a writable calendar returned by the Google Calendar API.
type GoogleCalendarOption struct {
	ID      string
	Summary string
	Primary bool
}

// Education is an education entry displayed on the education page.
type Education struct {
	ID           int
	School       string
	Degree       string
	FieldOfStudy string
	Duration     string
	Description  string
	Achievements []string
	Credentials  []Credential
}

// Credential is a certification or professional credential.
type Credential struct {
	Name       string
	Issuer     string
	IssueDate  string
	CredlyLink string
}
