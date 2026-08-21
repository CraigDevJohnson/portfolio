package types

import (
	"strings"
	"time"
)

// InstanceSummary is the display model for a managed EC2 instance.
type InstanceSummary struct {
	ID           string
	Name         string
	State        string
	InstanceType string
	AZ           string
}

// MetricPoint is a CloudWatch CPU utilization data point.
type MetricPoint struct {
	Timestamp  time.Time
	CPUPercent float64
}

// LogEvent is a CloudWatch log event.
type LogEvent struct {
	Timestamp time.Time
	Message   string
}

// Experience describes a single entry in the portfolio experience timeline.
type Experience struct {
	ID               int      `json:"id"`
	Position         string   `json:"position"`
	Company          string   `json:"company"`
	Duration         string   `json:"duration"`
	Responsibilities string   `json:"responsibilities"`
	Technologies     []string `json:"technologies"`
	SkillAreas       string   `json:"skill_areas"`
	Side             string   `json:"side"`
}

// Skill describes one portfolio skill and its presentation metadata.
type Skill struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Icon        string   `json:"icon,omitempty"`
	IconPath    string   `json:"icon_path,omitempty"`
	Link        string   `json:"link,omitempty"`
	Proficiency string   `json:"proficiency"`
	Featured    bool     `json:"featured,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
}

// SkillCategory groups related skills for presentation.
type SkillCategory struct {
	Name   string  `json:"name"`
	Skills []Skill `json:"skills"`
}

// ProjectImageRatio is the closed set of supported project image compositions.
type ProjectImageRatio string

const (
	ProjectImageLandscape ProjectImageRatio = "landscape"
	ProjectImagePortrait  ProjectImageRatio = "portrait"
	ProjectImageSquare    ProjectImageRatio = "square"
)

// Project describes one portfolio project and its explicit dossier metadata.
type Project struct {
	ID           int               `json:"id"`
	Name         string            `json:"name"`
	Featured     bool              `json:"featured,omitempty"`
	ImageRatio   ProjectImageRatio `json:"image_ratio"`
	Problem      string            `json:"problem"`
	Approach     string            `json:"approach"`
	Outcome      string            `json:"outcome"`
	Technologies []string          `json:"technologies"`
	Image        string            `json:"image"`
	GitHubURL    string            `json:"github_url,omitempty"`
	DemoURL      string            `json:"demo_url,omitempty"`
	Category     string            `json:"category"`
}

// Facility describes a game venue as returned by LPS schedule data.
type Facility struct {
	ID      int    `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Address string `json:"address,omitempty"`
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	ZIP     string `json:"zip,omitempty"`
}

// Game describes a normalized soccer game entry.
//
// Core fields such as ID, DateTime, Field, Home, Away, and Season are expected
// on every record. Optional schedule metadata is included when the upstream API
// provides it.
type Game struct {
	ID               string    `json:"id"`
	DateTime         string    `json:"datetime"`
	StartAt          string    `json:"start_at,omitempty"`
	EndAt            string    `json:"end_at,omitempty"`
	Field            string    `json:"field"`
	Location         string    `json:"location,omitempty"`
	Home             string    `json:"home"`
	Away             string    `json:"away"`
	Season           string    `json:"season"`
	PlayerTeamName   string    `json:"player_team_name,omitempty"`
	OpponentTeamName string    `json:"opponent_team_name,omitempty"`
	DivisionName     string    `json:"division_name,omitempty"`
	Facility         *Facility `json:"facility,omitempty"`
	Result           string    `json:"result,omitempty"`
}

// NewFacilityDetails builds and normalizes a Facility from legacy flat fields.
func NewFacilityDetails(id int, name, address, city, state, zip string) *Facility {
	return NormalizeFacility(&Facility{
		ID:      id,
		Name:    name,
		Address: address,
		City:    city,
		State:   state,
		ZIP:     zip,
	})
}

// NormalizeFacility trims string fields and returns nil for an empty facility.
func NormalizeFacility(facility *Facility) *Facility {
	if facility == nil {
		return nil
	}

	normalized := &Facility{
		ID:      facility.ID,
		Name:    strings.TrimSpace(facility.Name),
		Address: strings.TrimSpace(facility.Address),
		City:    strings.TrimSpace(facility.City),
		State:   strings.TrimSpace(facility.State),
		ZIP:     strings.TrimSpace(facility.ZIP),
	}

	if normalized.ID == 0 &&
		normalized.Name == "" &&
		normalized.Address == "" &&
		normalized.City == "" &&
		normalized.State == "" &&
		normalized.ZIP == "" {
		return nil
	}

	return normalized
}

// MergeFacility prefers populated base fields and fills any gaps from incoming.
func MergeFacility(base, incoming *Facility) *Facility {
	base = NormalizeFacility(base)
	incoming = NormalizeFacility(incoming)

	if base == nil {
		return incoming
	}
	if incoming == nil {
		return base
	}

	merged := *base
	if merged.ID == 0 {
		merged.ID = incoming.ID
	}
	if merged.Name == "" {
		merged.Name = incoming.Name
	}
	if merged.Address == "" {
		merged.Address = incoming.Address
	}
	if merged.City == "" {
		merged.City = incoming.City
	}
	if merged.State == "" {
		merged.State = incoming.State
	}
	if merged.ZIP == "" {
		merged.ZIP = incoming.ZIP
	}

	return NormalizeFacility(&merged)
}

// LPSPlayer describes a player discovered from the Let's Play Soccer account.
type LPSPlayer struct {
	UPlayerID    int    `json:"UPlayerID"`
	FirstName    string `json:"FirstName"`
	LastName     string `json:"LastName"`
	IsMainPlayer bool   `json:"is_main_player"`
}

// LPSTeam describes a team discovered for an imported LPS player.
type LPSTeam struct {
	TeamID   int    `json:"team_id"`
	TeamName string `json:"team_name"`
	Season   int    `json:"season"`
	PlayerID int    `json:"player_id"`
}

// PlayerTeamGroup pairs a player with their discovered teams for the team-select step.
type PlayerTeamGroup struct {
	Player LPSPlayer
	Teams  []LPSTeam
}

// SoccerWorkflowState stores the non-secret choices needed to reconstruct the
// Soccer workflow after a full-page navigation such as a Google OAuth return.
type SoccerWorkflowState struct {
	Source            string `json:"source,omitempty"`
	SelectedPlayerIDs []int  `json:"selected_player_ids,omitempty"`
	SelectedTeamIDs   []int  `json:"selected_team_ids,omitempty"`
}

// SessionData stores the encrypted soccer session payload in the auth cookie.
type SessionData struct {
	JWT       string              `json:"jwt"`
	UserName  string              `json:"user_name"`
	Players   []LPSPlayer         `json:"players"`
	ExpiresAt time.Time           `json:"expires_at"`
	SessionID string              `json:"session_id,omitempty"`
	StartedAt time.Time           `json:"started_at,omitempty"`
	Workflow  SoccerWorkflowState `json:"workflow,omitempty"`
}

// GoogleCalendarOption describes a calendar the user can target for event sync.
type GoogleCalendarOption struct {
	ID      string
	Summary string
	Primary bool
}
