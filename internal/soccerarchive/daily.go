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

// Validate rejects missing or internally inconsistent limits. A run's request
// ceiling must allow at least one attempt for every team the enrollment
// ceiling admits, so a full archive still gives each enrolled team a daily
// attempt.
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
	QueryDueTeams(ctx context.Context, cutoff time.Time, maxTeams int) (teamIDs []int, more bool, err error)
}

// The structured log messages a scheduled run reports, and is alerted on.
const (
	DailyCompletedLog  = "soccer_history_daily_completed"
	DailyIncompleteLog = "soccer_history_daily_incomplete"
)

// DailyReport distinguishes a complete daily pass from budget-limited work.
type DailyReport struct {
	Complete       bool            `json:"complete"`
	PendingDueWork bool            `json:"pending_due_work"`
	Requests       int             `json:"requests"`
	Results        []RefreshResult `json:"results"`
}

// dailyDueLeeway lets a run attempt a ready team that becomes due shortly
// after the run starts. A refresh makes a team due a day after its fetch,
// which is a little after that day's run began; without the leeway the next
// day's run would find it not yet due and skip it for a day. It is longer
// than any run and far shorter than a day, so a repeated delivery of the same
// day's event finds the teams it refreshed not due.
const dailyDueLeeway = time.Hour

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
func (w *DailyWorker) Run(ctx context.Context) (DailyReport, error) {
	start := w.clock.Now().UTC()
	client := *w.client
	// The client's timeout bounds each LPS request from when it is sent, so
	// the transport applies it after the pacing wait instead of letting that
	// wait spend it.
	transport := &pacedTransport{base: client.Transport, clock: w.clock, maxRequests: w.limits.MaxRequestsPerRun, interval: w.limits.MinRequestInterval, timeout: client.Timeout}
	client.Transport, client.Timeout = transport, 0
	source := &retryingTeamSource{source: lps.NewScheduleResolver(w.baseURL, &client, ""), retries: w.limits.MaxRetriesPerTeam, clock: w.clock}
	refresh := NewRefreshWorker(w.store, source, w.clock.Now)
	report := DailyReport{Complete: true, Results: make([]RefreshResult, 0)}
	dueIDs, more, err := w.store.QueryDueTeams(ctx, start.Add(dailyDueLeeway), w.limits.MaxEnrolledTeams)
	if err != nil {
		report.Complete = false
		return report, fmt.Errorf("select due teams: %w", err)
	}
	if more {
		report.Complete, report.PendingDueWork = false, true
	}
	for _, teamID := range dueIDs {
		state, err := w.store.ReadRefreshState(ctx, teamID)
		if err != nil {
			report.Complete = false
			report.Requests = transport.Used()
			return report, fmt.Errorf("check due team %d: %w", teamID, err)
		}
		if !dueInRun(&state, start) {
			// Another delivery or the on-demand worker refreshed it already.
			continue
		}
		if transport.Used() >= w.limits.MaxRequestsPerRun {
			report.Complete, report.PendingDueWork = false, true
			report.Requests = transport.Used()
			return report, nil
		}
		result := refresh.refreshTeam(ctx, teamID)
		report.Results = append(report.Results, result)
		if result.Outcome != RefreshSucceeded {
			report.Complete = false
		}
		if result.Outcome == RefreshBudgetExhausted {
			report.PendingDueWork = true
			report.Requests = transport.Used()
			return report, nil
		}
	}
	report.Requests = transport.Used()
	return report, nil
}

// dueInRun reports whether an enrolled team is due in a run that started at
// start: a ready team within the run's leeway, a team that failed temporarily
// only once its backoff has passed, and an invalid team never.
func dueInRun(state *RefreshState, start time.Time) bool {
	switch {
	case state.NextDueAt.IsZero() || state.Status == RefreshInvalid:
		return false
	case state.Status == RefreshRetryable:
		return !state.NextDueAt.After(start)
	default:
		return !state.NextDueAt.After(start.Add(dailyDueLeeway))
	}
}

// retryingTeamSource retries a team whose fetch failed temporarily, backing
// off exponentially, within the per-team retry budget.
type retryingTeamSource struct {
	source  TeamSource
	retries int
	clock   DailyClock
}

func (source *retryingTeamSource) FetchTeamSource(ctx context.Context, teamID int) (lps.TeamScheduleSource, error) {
	for retry := 0; ; retry++ {
		result, err := source.source.FetchTeamSource(ctx, teamID)
		if err == nil || retry >= source.retries || !retryableSourceFailure(ctx, err) {
			return result, err
		}
		if err := source.clock.Sleep(ctx, time.Second<<retry); err != nil {
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
