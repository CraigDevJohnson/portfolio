package types

// Experience represents a work experience entry
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

// Skill represents a technical skill
type Skill struct {
	ID          int
	Name        string
	Icon        string
	IconPath    string
	Link        string
	Proficiency string // "expert", "advanced", "intermediate", "familiar"
	Featured    bool   // Whether to show in featured skills section
}

// SkillCategory represents a category of skills
type SkillCategory struct {
	Name   string
	Skills []Skill
}

// Project represents a project
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

// Game represents a soccer game
type Game struct {
	ID       string `json:"id"`
	DateTime string `json:"datetime"`
	Field    string `json:"field"`
	Home     string `json:"home"`
	Away     string `json:"away"`
	Season   string `json:"season"`
}

// LambdaGamesResponse represents the response from the games API
type LambdaGamesResponse struct {
	Games []Game `json:"games"`
}

// Education represents an education entry
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

// Credential represents a certification or credential
type Credential struct {
	Name       string
	Issuer     string
	IssueDate  string
	CredlyLink string
}
