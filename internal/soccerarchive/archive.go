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

// ErrNotEnrolled means no valid source response has enrolled this Team ID.
var ErrNotEnrolled = errors.New("team is not enrolled for LPS history refresh")

// RefreshStatus identifies whether an enrolled team can be polled again.
type RefreshStatus string

const (
	RefreshReady     RefreshStatus = "ready"
	RefreshRetryable RefreshStatus = "retryable_failure"
	RefreshInvalid   RefreshStatus = "invalid_team"
)

// RefreshState is the durable polling state for an enrolled team.
type RefreshState struct {
	TeamID              int
	Status              RefreshStatus
	LastAttemptAt       time.Time
	NextDueAt           time.Time
	LastErrorKind       lps.ErrorKind
	LastErrorStatusCode int
}

// RefreshFailure records an upstream attempt without changing retained facts.
type RefreshFailure struct {
	TeamID         int
	AttemptedAt    time.Time
	Status         RefreshStatus
	NextDueAt      time.Time
	ErrorKind      lps.ErrorKind
	HTTPStatusCode int
}

// CoverageStatus records whether an LPS team-season response was fetched.
type CoverageStatus string

const (
	CoverageFetched    CoverageStatus = "fetched"
	CoverageNotFetched CoverageStatus = "not_fetched"
)

// Coverage describes only what the latest team response returned for a season.
// Retained games may outlive a later response that omits them.
type Coverage struct {
	Status            CoverageStatus
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
