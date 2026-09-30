package soccerarchive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"portfolio/internal/lps"
)

// Limits are the reviewed numeric ceilings for durable history: how many
// teams may enroll, how many of those slots only player-linked teams may
// take, and how many LPS requests, retries per team, and what pacing one
// scheduled run may use. The zero value is invalid: without every reviewed
// limit there is no archive store and no scheduled worker, so durable
// enrollment and daily polling stay off rather than running under an
// invented ceiling.
type Limits struct {
	MaxEnrolledTeams    int
	ReservedPlayerSlots int
	MaxRequestsPerRun   int
	MaxRetriesPerTeam   int
	MinRequestInterval  time.Duration
}

// maxRetriesPerTeam caps the reviewed retry budget for one team's attempt.
const maxRetriesPerTeam = 5

// Validate rejects missing or internally inconsistent limits. The request
// ceiling must be at least the enrollment ceiling, which guarantees only one
// request per enrolled team. A team really costs its team request, one lookup
// per facility its games use, and any retries, so the #104 activation review
// must size MaxRequestsPerRun to at least MaxEnrolledTeams times the measured
// per-team request cost. Below that, a full archive cannot reach every
// enrolled team each day, and each run reports the teams it left.
func (limits Limits) Validate() error {
	if limits.MaxEnrolledTeams <= 0 || limits.ReservedPlayerSlots < 0 || limits.ReservedPlayerSlots >= limits.MaxEnrolledTeams ||
		limits.MaxRequestsPerRun < limits.MaxEnrolledTeams || limits.MaxRetriesPerTeam < 0 || limits.MaxRetriesPerTeam > maxRetriesPerTeam ||
		limits.MinRequestInterval <= 0 {
		return errors.New("reviewed soccer history enrollment, reservation, request, retry, and pacing limits are required")
	}
	return nil
}

// The environment variables that carry the reviewed limits to a runtime.
const (
	EnvMaxEnrolledTeams      = "SOCCER_HISTORY_MAX_TEAMS"
	EnvReservedPlayerSlots   = "SOCCER_HISTORY_PLAYER_RESERVED"
	EnvMaxRequestsPerRun     = "SOCCER_HISTORY_MAX_REQUESTS"
	EnvMaxRetriesPerTeam     = "SOCCER_HISTORY_MAX_RETRIES"
	EnvMinRequestIntervalMS  = "SOCCER_HISTORY_MIN_INTERVAL_MS"
	EnvArchiveTableName      = "SOCCER_ARCHIVE_TABLE_NAME"
	requiredLimitDescription = "is required; durable history stays off until every reviewed limit is set"
)

// LimitsFromEnvironment reads the reviewed limits. Any missing or invalid
// value is an error, so the caller constructs no store or worker.
func LimitsFromEnvironment(lookup func(string) string) (Limits, error) {
	readInt := func(key string) (int, error) {
		raw := strings.TrimSpace(lookup(key))
		if raw == "" {
			return 0, fmt.Errorf("%s %s", key, requiredLimitDescription)
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", key, err)
		}
		return value, nil
	}
	var limits Limits
	for _, field := range []struct {
		key    string
		target *int
	}{
		{EnvMaxEnrolledTeams, &limits.MaxEnrolledTeams},
		{EnvReservedPlayerSlots, &limits.ReservedPlayerSlots},
		{EnvMaxRequestsPerRun, &limits.MaxRequestsPerRun},
		{EnvMaxRetriesPerTeam, &limits.MaxRetriesPerTeam},
	} {
		value, err := readInt(field.key)
		if err != nil {
			return Limits{}, err
		}
		*field.target = value
	}
	intervalMS, err := readInt(EnvMinRequestIntervalMS)
	if err != nil {
		return Limits{}, err
	}
	limits.MinRequestInterval = time.Duration(intervalMS) * time.Millisecond
	if err := limits.Validate(); err != nil {
		return Limits{}, err
	}
	return limits, nil
}

// DailyClock controls request pacing and retry delays.
type DailyClock interface {
	Now() time.Time
	Sleep(ctx context.Context, delay time.Duration) error
}

type systemDailyClock struct{}

func (systemDailyClock) Now() time.Time { return time.Now() }

func (systemDailyClock) Sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// DailyStore reads the due-teams index and checkpoints each team through the
// same refresh operations the on-demand worker uses.
type DailyStore interface {
	RefreshStore
	QueryDueTeams(ctx context.Context, cutoff time.Time) ([]DueTeam, error)
}

// DueTeam is one entry of the due-teams index: an enrolled team, when it is
// next due, and its refresh status when the index was read.
type DueTeam struct {
	TeamID    int
	NextDueAt time.Time
	Status    RefreshStatus
}

// The structured log messages a scheduled run reports, and is alerted on.
const (
	DailyCompletedLog  = "soccer_history_daily_completed"
	DailyIncompleteLog = "soccer_history_daily_incomplete"
)

// DailyReport distinguishes a complete daily pass from partial work. Results
// name every team the run attempted and every due team it left for a later
// run, with the reason. UnselectedDueTeams counts due teams past the run's
// team ceiling, which the run did not read.
type DailyReport struct {
	Complete           bool            `json:"complete"`
	PendingDueWork     bool            `json:"pending_due_work"`
	Requests           int             `json:"requests"`
	Results            []RefreshResult `json:"results"`
	UnselectedDueTeams int             `json:"unselected_due_teams"`
}

// leaveDue names each team in teams that the due index shows due in the run
// that started at start, as left for a later run with outcome.
func (report *DailyReport) leaveDue(teams []DueTeam, start time.Time, outcome RefreshOutcome) {
	report.Complete, report.PendingDueWork = false, true
	for _, team := range teams {
		if indexedDue(team, start) {
			report.Results = append(report.Results, RefreshResult{TeamID: team.TeamID, Outcome: outcome})
		}
	}
}

// indexedDue reports whether a due-index entry, as read, is due in the run
// that started at start.
func indexedDue(team DueTeam, start time.Time) bool {
	return dueInRun(&RefreshState{Status: team.Status, NextDueAt: team.NextDueAt}, start, start) != notDue
}

// refreshedTeamGuard is how long a successful fetch keeps a team from being
// due again. A team fetched at any time of day, whether by a scheduled run,
// a visitor's lookup, an entered Team ID, or an on-demand refresh, is due at
// the first scheduled run that starts at least this long after the fetch.
// It is longer than the span in which one day's schedule event can be
// delivered again (an hour of Scheduler retries, an hour in Lambda's async
// queue, and the run itself), so a repeated delivery finds the teams that
// day's run refreshed not due. It is far shorter than a day less a run, so
// a run's own fetches are due at the next day's run.
const refreshedTeamGuard = 4 * time.Hour

// dailyWrapUpAllowance is the end of a run's time it keeps for storing the
// last team's work and reporting, after it stops fetching.
const dailyWrapUpAllowance = 10 * time.Second

// defaultDailyRequestAllowance is how long a run expects one LPS request can
// take when its client sets no timeout.
const defaultDailyRequestAllowance = 15 * time.Second

// DailyWorker runs the indexed daily pass without an HTTP request runtime.
type DailyWorker struct {
	store   DailyStore
	baseURL string
	client  *http.Client
	limits  Limits
	clock   DailyClock
}

// NewDailyWorker requires explicit reviewed limits before any source request.
func NewDailyWorker(store DailyStore, baseURL string, client *http.Client, limits Limits, clock DailyClock) (*DailyWorker, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if store == nil || baseURL == "" {
		return nil, errors.New("daily history worker needs a store and LPS base URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	if clock == nil {
		clock = systemDailyClock{}
	}
	return &DailyWorker{store: store, baseURL: baseURL, client: client, limits: limits, clock: clock}, nil
}

// Run invokes each due team once and leaves unattempted teams due for the next
// delivery. Successful and failed team writes are the durable checkpoints.
// When ctx has a deadline, as a Lambda invocation does, the run stops
// fetching dailyWrapUpAllowance before it and starts a team only while the
// longest that team's fetch can take still fits, so it can report the teams
// it did and did not refresh before the invocation ends.
func (w *DailyWorker) Run(ctx context.Context) (DailyReport, error) {
	start := w.clock.Now().UTC()
	client := *w.client
	// The client's timeout bounds each LPS request from when it is sent, so
	// the transport applies it after the pacing wait instead of letting that
	// wait spend it.
	transport := &pacedTransport{base: client.Transport, clock: w.clock, maxRequests: w.limits.MaxRequestsPerRun, interval: w.limits.MinRequestInterval, timeout: client.Timeout}
	teamAllowance := w.limits.teamFetchAllowance(client.Timeout)
	client.Transport, client.Timeout = transport, 0
	var stopFetchingAt time.Time
	if deadline, ok := ctx.Deadline(); ok {
		stopFetchingAt = deadline.Add(-dailyWrapUpAllowance)
	}
	source := &retryingTeamSource{source: lps.NewScheduleResolver(w.baseURL, &client, ""), retries: w.limits.MaxRetriesPerTeam, clock: w.clock, stopAt: stopFetchingAt}
	refresh := NewRefreshWorker(w.store, source, w.clock.Now)
	report := DailyReport{Complete: true, Results: make([]RefreshResult, 0)}
	// The query also finds teams whose backoff after a temporary failure ends
	// during the run, so a team still backing off at its turn is reported.
	due, err := w.store.QueryDueTeams(ctx, start.Add(retryableFailureDelay))
	if err != nil {
		report.Complete = false
		return report, fmt.Errorf("select due teams: %w", err)
	}
	if len(due) > w.limits.MaxEnrolledTeams {
		for _, team := range due[w.limits.MaxEnrolledTeams:] {
			if indexedDue(team, start) {
				report.UnselectedDueTeams++
			}
		}
		due = due[:w.limits.MaxEnrolledTeams]
		if report.UnselectedDueTeams > 0 {
			report.Complete, report.PendingDueWork = false, true
		}
	}
	for i, team := range due {
		state, err := w.store.ReadRefreshState(ctx, team.TeamID)
		if err != nil {
			report.Complete = false
			report.Requests = transport.Used()
			return report, fmt.Errorf("check due team %d: %w", team.TeamID, err)
		}
		switch dueInRun(&state, start, w.clock.Now()) {
		case notDue:
			// This day's work for it is done: it was refreshed shortly before
			// or during this run, or it is invalid.
			continue
		case backingOff:
			report.Results = append(report.Results, RefreshResult{TeamID: team.TeamID, Outcome: RefreshBackingOff})
			report.Complete, report.PendingDueWork = false, true
			continue
		}
		if transport.Used() >= w.limits.MaxRequestsPerRun {
			report.leaveDue(due[i:], start, RefreshBudgetExhausted)
			report.Requests = transport.Used()
			return report, nil
		}
		if !stopFetchingAt.IsZero() && w.clock.Now().Add(teamAllowance).After(stopFetchingAt) {
			report.leaveDue(due[i:], start, RefreshRunTimeExhausted)
			report.Requests = transport.Used()
			return report, nil
		}
		result := refresh.refreshTeam(ctx, team.TeamID)
		report.Results = append(report.Results, result)
		if result.Outcome != RefreshSucceeded {
			report.Complete = false
		}
		if result.Outcome == RefreshBudgetExhausted || result.Outcome == RefreshRunTimeExhausted {
			report.leaveDue(due[i+1:], start, result.Outcome)
			report.Requests = transport.Used()
			return report, nil
		}
	}
	report.Requests = transport.Used()
	return report, nil
}

// runDecision is what a scheduled run does with a team the due query found.
type runDecision int

const (
	attemptNow runDecision = iota
	notDue
	backingOff
)

// dueInRun decides a team's turn in a run that started at start. A ready
// team is due once its refresh guard ended by the start; a team that failed
// temporarily is due once its backoff has ended, and is backing off if that
// happens later in the run; an invalid team is never due.
func dueInRun(state *RefreshState, start, now time.Time) runDecision {
	switch {
	case state.NextDueAt.IsZero() || state.Status == RefreshInvalid:
		return notDue
	case state.Status == RefreshRetryable && !state.NextDueAt.After(now):
		return attemptNow
	case state.Status == RefreshRetryable && !state.NextDueAt.After(start.Add(retryableFailureDelay)):
		return backingOff
	case state.Status != RefreshRetryable && !state.NextDueAt.After(start):
		return attemptNow
	default:
		return notDue
	}
}

// teamFetchAllowance is the longest one team's fetch can take when every
// allowed attempt waits out its pacing interval and then its whole request
// timeout, with the backoffs between attempts. Facility lookups add to it;
// the run's fetch deadline, not this allowance, bounds those.
func (limits Limits) teamFetchAllowance(requestTimeout time.Duration) time.Duration {
	if requestTimeout <= 0 {
		requestTimeout = defaultDailyRequestAllowance
	}
	allowance := time.Duration(1+limits.MaxRetriesPerTeam) * (requestTimeout + limits.MinRequestInterval)
	for retry := range limits.MaxRetriesPerTeam {
		allowance += retryBackoff(retry)
	}
	return allowance
}

// retryBackoff is the wait before a team's retry after its retry-th attempt.
func retryBackoff(retry int) time.Duration {
	return time.Second << retry
}

// ErrRunTimeExhausted means a scheduled run stopped a team's fetch because
// the run's time for fetching ended. It is not an LPS failure.
var ErrRunTimeExhausted = errors.New("daily run time for LPS requests exhausted")

// retryingTeamSource retries a team whose fetch failed temporarily, backing
// off exponentially, within the per-team retry budget. It stops the fetch at
// stopAt, when set.
type retryingTeamSource struct {
	source  TeamSource
	retries int
	clock   DailyClock
	stopAt  time.Time
}

func (source *retryingTeamSource) FetchTeamSource(ctx context.Context, teamID int) (lps.TeamScheduleSource, error) {
	fetchCtx := ctx
	if !source.stopAt.IsZero() {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithDeadline(ctx, source.stopAt)
		defer cancel()
	}
	// outOfTime reports a fetch the run's fetch deadline, not the caller,
	// stopped.
	outOfTime := func() bool { return fetchCtx.Err() != nil && ctx.Err() == nil }
	for retry := 0; ; retry++ {
		result, err := source.source.FetchTeamSource(fetchCtx, teamID)
		if err != nil && outOfTime() {
			return lps.TeamScheduleSource{}, fmt.Errorf("%w: %w", ErrRunTimeExhausted, err)
		}
		if err == nil || retry >= source.retries || !retryableSourceFailure(fetchCtx, err) {
			return result, err
		}
		if err := source.clock.Sleep(fetchCtx, retryBackoff(retry)); err != nil {
			if outOfTime() {
				return lps.TeamScheduleSource{}, fmt.Errorf("%w: %w", ErrRunTimeExhausted, err)
			}
			return lps.TeamScheduleSource{}, err
		}
	}
}

// retryableSourceFailure reports a rate limit, server error, or network
// failure, which a later attempt may not repeat.
func retryableSourceFailure(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, ErrRequestBudget) {
		return false
	}
	var fetchErr *lps.FetchError
	return errors.As(err, &fetchErr) && fetchErr.Kind == lps.ErrorUpstream &&
		(fetchErr.StatusCode == http.StatusTooManyRequests || fetchErr.StatusCode >= http.StatusInternalServerError)
}

// ErrRequestBudget means a scheduled run stopped a source call before its next
// HTTP request because the run's request budget was used.
var ErrRequestBudget = errors.New("daily LPS request budget exhausted")

// pacedTransport counts every LPS request of a run, team and facility alike,
// refuses requests past the budget, and spaces requests by the interval. A
// request's timeout starts once its pacing wait is over.
type pacedTransport struct {
	mu          sync.Mutex
	base        http.RoundTripper
	clock       DailyClock
	maxRequests int
	interval    time.Duration
	timeout     time.Duration
	used        int
	lastRequest time.Time
}

func (transport *pacedTransport) Used() int {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.used
}

func (transport *pacedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := transport.admit(request.Context()); err != nil {
		return nil, err
	}
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	if transport.timeout <= 0 {
		return base.RoundTrip(request)
	}
	ctx, cancel := context.WithTimeout(request.Context(), transport.timeout)
	response, err := base.RoundTrip(request.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	// The timeout also bounds reading the body, so it ends when the body is
	// closed.
	response.Body = &cancelOnClose{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

// admit waits out the pacing interval and counts the request, or refuses it
// once the run's request budget is used.
func (transport *pacedTransport) admit(ctx context.Context) error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.used >= transport.maxRequests {
		return ErrRequestBudget
	}
	if !transport.lastRequest.IsZero() {
		if delay := transport.lastRequest.Add(transport.interval).Sub(transport.clock.Now()); delay > 0 {
			if err := transport.clock.Sleep(ctx, delay); err != nil {
				return err
			}
		}
	}
	transport.used++
	transport.lastRequest = transport.clock.Now()
	return nil
}

// cancelOnClose releases a request's timeout once its response body closes.
type cancelOnClose struct {
	io.ReadCloser

	cancel context.CancelFunc
}

func (body *cancelOnClose) Close() error {
	err := body.ReadCloser.Close()
	body.cancel()
	return err
}
