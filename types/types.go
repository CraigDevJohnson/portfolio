package types

import (
	"encoding/json"
	"strings"
	"time"
)

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

type Skill struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon,omitempty"`
	IconPath    string `json:"icon_path,omitempty"`
	Link        string `json:"link,omitempty"`
	Proficiency string `json:"proficiency"`
	Featured    bool   `json:"featured,omitempty"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
}

type SkillCategory struct {
	Name   string  `json:"name"`
	Skills []Skill `json:"skills"`
}

type Project struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Intro        string   `json:"intro"`
	Description  string   `json:"description"`
	Technologies []string `json:"technologies"`
	Image        string   `json:"image"`
	GitHubURL    string   `json:"github_url,omitempty"`
	DemoURL      string   `json:"demo_url,omitempty"`
	Category     string   `json:"category"`
}

type Facility struct {
	ID      int    `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Address string `json:"address,omitempty"`
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	ZIP     string `json:"zip,omitempty"`
}

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

type LambdaGamesResponse struct {
	Games []Game `json:"games"`
}

type gameAlias Game

type gameJSON struct {
	gameAlias

	FacilityID      int    `json:"facility_id,omitempty"`
	FacilityName    string `json:"facility_name,omitempty"`
	FacilityAddress string `json:"facility_address,omitempty"`
	FacilityCity    string `json:"facility_city,omitempty"`
	FacilityState   string `json:"facility_state,omitempty"`
	FacilityZIP     string `json:"facility_zip,omitempty"`
}

func (game *Game) UnmarshalJSON(data []byte) error {
	var payload gameJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*game = Game(payload.gameAlias)
	legacyFacility := NewFacilityDetails(
		payload.FacilityID,
		payload.FacilityName,
		payload.FacilityAddress,
		payload.FacilityCity,
		payload.FacilityState,
		payload.FacilityZIP,
	)
	game.Facility = MergeFacility(game.Facility, legacyFacility)
	return nil
}

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

	if normalized.ID == 0 && normalized.Name == "" && normalized.Address == "" && normalized.City == "" && normalized.State == "" && normalized.ZIP == "" {
		return nil
	}

	return normalized
}

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

type LPSPlayer struct {
	UPlayerID    int    `json:"UPlayerID"`
	FirstName    string `json:"FirstName"`
	LastName     string `json:"LastName"`
	IsMainPlayer bool   `json:"is_main_player"`
}

type SessionData struct {
	JWT       string      `json:"jwt"`
	UserName  string      `json:"user_name"`
	Players   []LPSPlayer `json:"players"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type GoogleCalendarOption struct {
	ID      string
	Summary string
	Primary bool
}

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

type Credential struct {
	Name       string
	Issuer     string
	IssueDate  string
	CredlyLink string
}
