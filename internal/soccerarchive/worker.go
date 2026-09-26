package soccerarchive

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"portfolio/internal/lps"
)

// TeamSource fetches one confirmed LPS team response with facility context.
type TeamSource interface {
	FetchTeamSource(ctx context.Context, teamID int) (lps.TeamScheduleSource, error)
}

// RefreshStore provides the durable enrollment and archive operations used by a worker pass.
type RefreshStore interface {
	ReadRefreshState(ctx context.Context, teamID int) (RefreshState, error)
	SaveTeamSnapshot(ctx context.Context, snapshot *Snapshot) error
	RecordRefreshFailure(ctx context.Context, failure *RefreshFailure) error
}

// RefreshOutcome reports what happened for one requested Team ID.
type RefreshOutcome string

const (
	RefreshSucceeded        RefreshOutcome = "succeeded"
	RefreshRetryableFailure RefreshOutcome = "retryable_failure"
	RefreshInvalidTeam      RefreshOutcome = "invalid_team"
	RefreshSkippedInvalid   RefreshOutcome = "skipped_invalid"
	RefreshStoreFailed      RefreshOutcome = "store_failure"
	RefreshNotEnrolled      RefreshOutcome = "not_enrolled"
)

// RefreshResult is one team's outcome in an on-demand worker pass.
type RefreshResult struct {
	TeamID  int
	Outcome RefreshOutcome
	Error   string
}

// RefreshReport is complete only if every requested enrolled team was refreshed.
type RefreshReport struct {
	Complete bool
	Results  []RefreshResult
}

// RefreshWorker refreshes explicitly requested enrolled teams without scheduling.
type RefreshWorker struct {
	store  RefreshStore
	source TeamSource
	now    func() time.Time
}

// NewRefreshWorker creates an on-demand worker with injectable boundaries.
func NewRefreshWorker(store RefreshStore, source TeamSource, now func() time.Time) *RefreshWorker {
	return &RefreshWorker{store: store, source: source, now: now}
}

// Run makes one independent attempt per distinct Team ID.
func (w *RefreshWorker) Run(ctx context.Context, teamIDs []int) RefreshReport {
	ids := make(map[int]struct{}, len(teamIDs))
	for _, id := range teamIDs {
		ids[id] = struct{}{}
	}
	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	report := RefreshReport{Complete: len(ordered) > 0}
	for _, teamID := range ordered {
		result := RefreshResult{TeamID: teamID}
		state, err := w.store.ReadRefreshState(ctx, teamID)
		switch {
		case errors.Is(err, ErrNotEnrolled):
			result.Outcome = RefreshNotEnrolled
		case err != nil:
			result.Outcome, result.Error = RefreshStoreFailed, err.Error()
		case state.Status == RefreshInvalid:
			result.Outcome = RefreshSkippedInvalid
		default:
			source, fetchErr := w.source.FetchTeamSource(ctx, teamID)
			if fetchErr != nil {
				result = w.recordFailure(ctx, teamID, fetchErr)
			} else if source.Response.Team.UTeamID != teamID {
				result = w.recordFailure(ctx, teamID, lps.NewFetchError(lps.ErrorUpstream, teamID, http.StatusBadGateway, "team response did not confirm Team ID %d", teamID))
			} else {
				fetchedAt := w.now().UTC()
				if err := w.store.SaveTeamSnapshot(ctx, &Snapshot{TeamID: teamID, Team: source.Response.Team, Games: source.Response.Games, Facilities: source.Facilities, FetchedAt: fetchedAt}); err != nil {
					result.Outcome, result.Error = RefreshStoreFailed, err.Error()
				} else {
					result.Outcome = RefreshSucceeded
				}
			}
		}
		if result.Outcome != RefreshSucceeded {
			report.Complete = false
		}
		report.Results = append(report.Results, result)
	}
	return report
}

func (w *RefreshWorker) recordFailure(ctx context.Context, teamID int, fetchErr error) RefreshResult {
	result := RefreshResult{TeamID: teamID, Outcome: RefreshRetryableFailure, Error: fetchErr.Error()}
	attemptedAt := w.now().UTC()
	failure := RefreshFailure{TeamID: teamID, AttemptedAt: attemptedAt, Status: RefreshRetryable, NextDueAt: attemptedAt.Add(15 * time.Minute), ErrorKind: lps.ErrorUpstream}
	var upstream *lps.FetchError
	if errors.As(fetchErr, &upstream) {
		failure.ErrorKind = upstream.Kind
		failure.HTTPStatusCode = upstream.StatusCode
		if upstream.Kind == lps.ErrorInvalidTeam && (upstream.StatusCode == http.StatusBadRequest || upstream.StatusCode == http.StatusNotFound) {
			result.Outcome = RefreshInvalidTeam
			failure.Status = RefreshInvalid
			failure.NextDueAt = time.Time{}
		}
	}
	if err := w.store.RecordRefreshFailure(ctx, &failure); err != nil {
		result.Outcome = RefreshStoreFailed
		result.Error += "; recording failure: " + err.Error()
	}
	return result
}
