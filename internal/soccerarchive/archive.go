// Package soccerarchive holds durable source facts for known LPS teams.
package soccerarchive

import (
	"context"
	"errors"
	"time"

	"portfolio/internal/lps"
	"portfolio/types"
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

// HistoryStore supplies exact membership proof and team-season facts for a
// private read. A caller must check the current site and imported identities.
type HistoryStore interface {
	Store
	HasPlayerMembership(ctx context.Context, ownerIssuer, ownerSubject string, playerID, teamID, seasonID int) (bool, error)
	ReadTeamSeason(ctx context.Context, teamID, seasonID int) (TeamSeason, error)
	ReadRefreshState(ctx context.Context, teamID int) (RefreshState, error)
}

// PlayerMembership is positive evidence from an authenticated LPS player lookup.
// A team with no LPS season ID is not membership proof.
type PlayerMembership struct {
	PlayerID int
	Team     lps.TeamSummary
}

// PlayerDiscovery is the durable, credential-free result of one granted import.
// KnownTeams includes IDs with incomplete season metadata so they can still be
// enrolled; only Memberships grants exact player-team-season evidence.
type PlayerDiscovery struct {
	OwnerIssuer  string
	OwnerSubject string
	Players      []types.LPSPlayer
	KnownTeams   []lps.TeamSummary
	Memberships  []PlayerMembership
	ObservedAt   time.Time
}

// MembershipStore persists owner-bound player evidence and known-team enrollment.
// It is wired with Store only when durable history collection is enabled,
// which stays off until activation.
type MembershipStore interface {
	SavePlayerDiscovery(ctx context.Context, discovery *PlayerDiscovery) error
}

// PlayerRemovalStore erases all retained evidence under a verified player ID.
type PlayerRemovalStore interface {
	DeletePlayerEvidence(ctx context.Context, playerID int) error
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

// RefreshStoreErrorKind marks a refresh attempt whose fetched LPS response
// could not be stored.
const RefreshStoreErrorKind lps.ErrorKind = "store"
