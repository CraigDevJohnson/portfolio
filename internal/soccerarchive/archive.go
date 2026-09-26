// Package soccerarchive holds durable source facts for known LPS teams.
package soccerarchive

import (
	"context"
	"errors"
	"time"

	"portfolio/internal/lps"
)

// Snapshot contains the source facts returned for one accepted Team ID lookup.
// It deliberately contains no imported LPS credential or player membership.
type Snapshot struct {
	TeamID     int
	Team       lps.TeamSummary
	Games      []lps.TeamScheduleGame
	Facilities []lps.FacilityResponse
	FetchedAt  time.Time
}

// Store persists an accepted team response and its source coverage.
type Store interface {
	SaveTeamSnapshot(ctx context.Context, snapshot *Snapshot) error
}

// ErrNoArchive means no accepted snapshot has been stored for this team.
var ErrNoArchive = errors.New("team has no archived LPS response")

// Coverage describes only what the latest team response returned for a season.
// Retained games may outlive a later response that omits them.
type Coverage struct {
	Status            string
	FetchedAt         time.Time
	ReturnedGameCount int
}

// TeamSeason is a read-back view of one team's stored source facts.
type TeamSeason struct {
	Team       lps.TeamSummary
	Coverage   Coverage
	Games      []lps.TeamScheduleGame
	Facilities []lps.FacilityResponse
}
