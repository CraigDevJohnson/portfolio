package app

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"portfolio/cmd/web/partials"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/internal/testutil"
)

// candidateHistoryLimits is the #104 readiness packet's candidate tuple
// (docs/deployment/2026-09-26-lps-history-activation-readiness.md). It is a
// proposal for review, not a configured or approved limit.
var candidateHistoryLimits = soccerarchive.Limits{
	MaxEnrolledTeams:    40,
	ReservedPlayerSlots: 30,
	MaxRequestsPerRun:   120,
	MaxRetriesPerTeam:   1,
	MinRequestInterval:  time.Second,
}

// archiveUsage is what the archive asked of DynamoDB, sized as on-demand
// capacity bills it: 1 KB write units (two for a transactional write), 4 KB
// strongly consistent read units, and half that for an index query. A write
// to an item on the due-teams index also writes the index.
type archiveUsage struct {
	puts, transactItems, gets, queries, deletes int
	writeUnits, indexWriteUnits, readUnits      float64
}

func (usage archiveUsage) String() string {
	return fmt.Sprintf("%d puts + %d transactional puts (%.0f WRU, %.0f due-index WRU), %d gets + %d queries (%.1f RRU), %d deletes",
		usage.puts, usage.transactItems, usage.writeUnits, usage.indexWriteUnits, usage.gets, usage.queries, usage.readUnits, usage.deletes)
}

func (usage archiveUsage) minus(earlier archiveUsage) archiveUsage {
	return archiveUsage{
		puts: usage.puts - earlier.puts, transactItems: usage.transactItems - earlier.transactItems,
		gets: usage.gets - earlier.gets, queries: usage.queries - earlier.queries, deletes: usage.deletes - earlier.deletes,
		writeUnits: usage.writeUnits - earlier.writeUnits, indexWriteUnits: usage.indexWriteUnits - earlier.indexWriteUnits,
		readUnits: usage.readUnits - earlier.readUnits,
	}
}

// meteredArchiveTable is the in-memory archive table with a meter on every
// DynamoDB call the durable archive makes.
type meteredArchiveTable struct {
	*archivetest.Table

	mu    sync.Mutex
	usage archiveUsage
	// dueKeys holds the due-teams sort key of each stored item that has one.
	dueKeys map[string]string
}

func newMeteredArchiveTable() *meteredArchiveTable {
	return &meteredArchiveTable{Table: archivetest.NewTable(), dueKeys: map[string]string{}}
}

func (table *meteredArchiveTable) Usage() archiveUsage {
	table.mu.Lock()
	defer table.mu.Unlock()
	return table.usage
}

func (table *meteredArchiveTable) PutItem(ctx context.Context, input *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	output, err := table.Table.PutItem(ctx, input, optFns...)
	table.mu.Lock()
	defer table.mu.Unlock()
	table.usage.puts++
	table.recordWrite(input.Item, 1, err == nil)
	return output, err
}

func (table *meteredArchiveTable) TransactWriteItems(ctx context.Context, input *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	output, err := table.Table.TransactWriteItems(ctx, input, optFns...)
	table.mu.Lock()
	defer table.mu.Unlock()
	for _, write := range input.TransactItems {
		if write.Put != nil {
			table.usage.transactItems++
			table.recordWrite(write.Put.Item, 2, err == nil)
		}
	}
	return output, err
}

func (table *meteredArchiveTable) GetItem(ctx context.Context, input *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	output, err := table.Table.GetItem(ctx, input, optFns...)
	table.mu.Lock()
	defer table.mu.Unlock()
	table.usage.gets++
	if err == nil {
		// A strongly consistent read of a missing item still costs one unit.
		table.usage.readUnits += math.Max(1, math.Ceil(float64(dynamoItemBytes(output.Item))/4096))
	}
	return output, err
}

func (table *meteredArchiveTable) Query(ctx context.Context, input *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	output, err := table.Table.Query(ctx, input, optFns...)
	table.mu.Lock()
	defer table.mu.Unlock()
	table.usage.queries++
	if err == nil {
		bytes := 0
		for _, item := range output.Items {
			bytes += dynamoItemBytes(item)
		}
		units := math.Max(1, math.Ceil(float64(bytes)/4096))
		if input.IndexName != nil {
			// Index queries are eventually consistent: half a unit per 4 KB.
			units /= 2
		}
		table.usage.readUnits += units
	}
	return output, err
}

func (table *meteredArchiveTable) DeleteItem(ctx context.Context, input *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	output, err := table.Table.DeleteItem(ctx, input, optFns...)
	table.mu.Lock()
	defer table.mu.Unlock()
	table.usage.deletes++
	table.usage.writeUnits++
	return output, err
}

// recordWrite bills one attempted write of item, which DynamoDB bills
// whether or not its condition holds, and its due-teams index write once it
// is stored.
func (table *meteredArchiveTable) recordWrite(item map[string]types.AttributeValue, unitsPerKB float64, stored bool) {
	units := unitsPerKB * math.Max(1, math.Ceil(float64(dynamoItemBytes(item))/1024))
	table.usage.writeUnits += units
	if !stored {
		return
	}
	key := itemKeyOf(item)
	previous, hadDue := table.dueKeys[key]
	current, hasDue := item["due_sk"].(*types.AttributeValueMemberS)
	switch {
	case hasDue && hadDue && previous != current.Value:
		// A changed index key deletes the old index entry and writes the new.
		table.usage.indexWriteUnits += 1 + units/unitsPerKB
	case hasDue:
		table.usage.indexWriteUnits += units / unitsPerKB
	case hadDue:
		table.usage.indexWriteUnits++
	}
	if hasDue {
		table.dueKeys[key] = current.Value
	} else {
		delete(table.dueKeys, key)
	}
}

// retainedBytes sums the stored items' DynamoDB sizes by the kind of
// partition that holds them.
func (table *meteredArchiveTable) retainedBytes(t *testing.T) map[string]int {
	t.Helper()
	items, err := table.Items()
	if err != nil {
		t.Fatal(err)
	}
	byPartition := map[string]int{}
	for key, decoded := range items {
		item, err := attributevalue.MarshalMap(decoded)
		if err != nil {
			t.Fatal(err)
		}
		partition, _, _ := strings.Cut(key, "#")
		byPartition[partition] += dynamoItemBytes(item)
	}
	return byPartition
}

func itemKeyOf(item map[string]types.AttributeValue) string {
	pk, _ := item["pk"].(*types.AttributeValueMemberS)
	sk, _ := item["sk"].(*types.AttributeValueMemberS)
	if pk == nil || sk == nil {
		return ""
	}
	return pk.Value + "/" + sk.Value
}

// dynamoItemBytes approximates an item's DynamoDB size: each attribute name
// plus its value, where a number takes one byte per two significant digits
// plus one.
func dynamoItemBytes(item map[string]types.AttributeValue) int {
	size := 0
	for name, value := range item {
		size += len(name) + attributeBytes(value)
	}
	return size
}

func attributeBytes(value types.AttributeValue) int {
	switch v := value.(type) {
	case *types.AttributeValueMemberS:
		return len(v.Value)
	case *types.AttributeValueMemberN:
		digits := strings.Trim(strings.TrimLeft(v.Value, "-"), "0")
		return (len(strings.ReplaceAll(digits, ".", ""))+1)/2 + 1
	case *types.AttributeValueMemberB:
		return len(v.Value)
	case *types.AttributeValueMemberBOOL, *types.AttributeValueMemberNULL:
		return 1
	case *types.AttributeValueMemberL:
		size := 3
		for _, element := range v.Value {
			size += 1 + attributeBytes(element)
		}
		return size
	case *types.AttributeValueMemberM:
		size := 3
		for name, element := range v.Value {
			size += 1 + len(name) + attributeBytes(element)
		}
		return size
	}
	return 0
}

// readinessLPS is a fake LPS that counts every request and response byte
// by endpoint. A team's response can be changed, failed once, or failed for
// good between daily runs.
type readinessLPS struct {
	t   *testing.T
	jwt string

	mu         sync.Mutex
	requests   map[string]int
	bytes      map[string]int
	teams      map[int]string
	failOnce   map[int]int
	failAlways map[int]int
}

// endpoint names the LPS endpoint a request path addresses.
func endpoint(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) == 3 && parts[0] == "players" && parts[2] == "my_teams":
		return "players/{id}/my_teams"
	case len(parts) == 2 && (parts[0] == "teams" || parts[0] == "facilities"):
		return parts[0] + "/{id}"
	}
	return strings.Join(parts, "/")
}

func (fake *readinessLPS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.requests[endpoint(r.URL.Path)]++
	write := func(body string) {
		fake.bytes[endpoint(r.URL.Path)] += len(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}
	authorized := r.Header.Get("Authorization") == "Bearer "+fake.jwt
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	id := 0
	if len(parts) > 1 {
		id, _ = strconv.Atoi(parts[1])
	}
	switch endpoint(r.URL.Path) {
	case "users/check":
		if !authorized {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		write(playerHistoryAccount)
	case "players/{id}/my_teams":
		if !authorized {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch id {
		case 1001:
			write(`[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77,"FacilityID":5}]`)
		case 1002:
			write(`[{"UTeamID":4202,"team_name":"Taylor FC","division_name":"Coed B","Season":79,"FacilityID":5}]`)
		default:
			http.Error(w, "player not found", http.StatusNotFound)
		}
	case "teams/{id}":
		if status := fake.failAlways[id]; status != 0 {
			http.Error(w, "team lookup failed", status)
			return
		}
		if status := fake.failOnce[id]; status != 0 {
			delete(fake.failOnce, id)
			http.Error(w, "team lookup failed", status)
			return
		}
		schedule, found := fake.teams[id]
		if !found {
			fake.t.Errorf("unexpected LPS team lookup %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		write(schedule)
	case "facilities/{id}":
		write(fmt.Sprintf(`{"FacilityID":%d,"FacilityName":"Arena %d","Address":"%d Main St","City":"Boise","State":"ID","ZIP":"83702"}`, id, id, id*100))
	default:
		fake.t.Errorf("unexpected LPS request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

func (fake *readinessLPS) setTeam(teamID int, schedule string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.teams[teamID] = schedule
}

func (fake *readinessLPS) failTeamOnce(teamID, status int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.failOnce[teamID] = status
}

func (fake *readinessLPS) failTeam(teamID, status int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.failAlways[teamID] = status
}

// counts returns the requests and response bytes by endpoint so far.
func (fake *readinessLPS) counts() (requests, bytes map[string]int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	requests, bytes = map[string]int{}, map[string]int{}
	for name, count := range fake.requests {
		requests[name] = count
	}
	for name, count := range fake.bytes {
		bytes[name] = count
	}
	return requests, bytes
}

// since returns the requests by endpoint made after the earlier counts.
func since(now, earlier map[string]int) map[string]int {
	delta := map[string]int{}
	for name, count := range now {
		if count != earlier[name] {
			delta[name] = count - earlier[name]
		}
	}
	return delta
}

func total(counts map[string]int) int {
	sum := 0
	for _, count := range counts {
		sum += count
	}
	return sum
}

// readinessClock is the daily worker's clock: pacing and retry waits
// advance it instead of sleeping.
type readinessClock struct {
	mu    sync.Mutex
	now   time.Time
	slept time.Duration
}

func (clock *readinessClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *readinessClock) Sleep(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(delay)
	clock.slept += delay
	return nil
}

func (clock *readinessClock) advance(delay time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(delay)
}

// takeSlept returns the pacing and retry time waited since the last take.
func (clock *readinessClock) takeSlept() time.Duration {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	slept := clock.slept
	clock.slept = 0
	return slept
}

// Craig FC (4101) plays at arena 5 and travels to arena 6; Taylor FC
// (4202) plays at arena 5; Visitor FC (4301) is a Team ID a visitor entered
// and plays at arena 7.
const (
	readinessCraigFC = `{"team":{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77,"FacilityID":5},"games":[
{"UGameID":7001,"Season":77,"UTeam1":4101,"UTeam2":5001,"FacilityID":5,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"3 - 1"},
{"UGameID":7002,"Season":77,"UTeam1":5002,"UTeam2":4101,"FacilityID":6,"SchedGameDateTime":"2026-01-12T19:00:00Z","result":"3 - 0"},
{"UGameID":7003,"Season":77,"UTeam1":4101,"UTeam2":5003,"FacilityID":5,"SchedGameDateTime":"2099-01-19T19:00:00Z","result":""}]}`
	// The next day LPS corrects game 7001's score and no longer lists the
	// away game 7002.
	readinessCraigFCCorrected = `{"team":{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77,"FacilityID":5},"games":[
{"UGameID":7001,"Season":77,"UTeam1":4101,"UTeam2":5001,"FacilityID":5,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"3 - 2"},
{"UGameID":7003,"Season":77,"UTeam1":4101,"UTeam2":5003,"FacilityID":5,"SchedGameDateTime":"2099-01-19T19:00:00Z","result":""}]}`
	readinessTaylorFC = `{"team":{"UTeamID":4202,"team_name":"Taylor FC","division_name":"Coed B","Season":79,"FacilityID":5},"games":[
{"UGameID":7101,"Season":79,"UTeam1":4202,"UTeam2":5101,"FacilityID":5,"SchedGameDateTime":"2026-02-02T19:00:00Z","result":"0 - 0"}]}`
	readinessVisitorFC = `{"team":{"UTeamID":4301,"team_name":"Visitor FC","division_name":"Open C","Season":80,"FacilityID":7},"games":[
{"UGameID":7201,"Season":80,"UTeam1":4301,"UTeam2":5201,"FacilityID":7,"SchedGameDateTime":"2026-03-02T19:00:00Z","result":"2 - 2"}]}`
)

// TestLPSHistoryReadinessJourneyFromEnrollmentThroughRemoval is the #104
// dry run: the real route assembly with fake Cognito sign-in, a fake LPS, and
// the durable archive over a metered in-memory DynamoDB table, plus the real
// daily worker over that table, under the candidate limits. It proves the
// journey and logs the request, byte, and capacity figures the readiness
// packet quotes; the fixture's sizes are not LPS production sizes.
func TestLPSHistoryReadinessJourneyFromEnrollmentThroughRemoval(t *testing.T) {
	cognito := newFakeSiteCognito(t)
	application := cognito.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	jwt := testutil.TestJWT(t, time.Now().Add(time.Hour))
	source := &readinessLPS{
		t: t, jwt: jwt, requests: map[string]int{}, bytes: map[string]int{},
		teams:    map[int]string{4101: readinessCraigFC, 4202: readinessTaylorFC, 4301: readinessVisitorFC},
		failOnce: map[int]int{}, failAlways: map[int]int{},
	}
	lpsServer := httptest.NewServer(source)
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, handler := buildMux(application, application.Logger, false)
	table := newMeteredArchiveTable()
	store, err := soccerarchive.NewDynamoStoreWithAPI(table, "portfolio-lambda-dev-soccer-history", candidateHistoryLimits)
	if err != nil {
		t.Fatalf("the candidate limits do not build an archive: %v", err)
	}
	handler.SetArchiveStore(store)
	type phaseStart struct {
		requests, bytes map[string]int
		usage           archiveUsage
	}
	start := func() phaseStart {
		requests, bytes := source.counts()
		return phaseStart{requests: requests, bytes: bytes, usage: table.Usage()}
	}
	measure := func(phase string, from phaseStart) {
		requests, bytes := source.counts()
		t.Logf("%s: LPS %v (%d requests, %d response bytes); DynamoDB %s", phase, since(requests, from.requests), total(since(requests, from.requests)),
			total(since(bytes, from.bytes)), table.Usage().minus(from.usage))
	}

	// 1. An anonymous visitor's Team ID lookup enrolls Visitor FC. It proves
	// no membership.
	phase := start()
	visitor := newSiteBrowser(t, mux)
	if body := visitor.postForm("/soccer/fetch", url.Values{"team_codes": {"4301"}}).Body.String(); !strings.Contains(body, "Team 4301 added to history collection.") {
		t.Fatalf("anonymous Team ID lookup did not enroll team 4301: %q", body)
	}
	measure("anonymous Team ID enrollment", phase)

	// 2. The owner's disclosed import captures owner-bound membership for
	// every linked player with one team lookup per player, and fetches no
	// team schedule.
	phase = start()
	owner := newSiteBrowser(t, mux)
	if landing := owner.signIn("/soccer"); landing.Code != http.StatusSeeOther {
		t.Fatalf("site sign-in status = %d", landing.Code)
	}
	imported := owner.postForm("/soccer/import", url.Values{
		"jwt": {jwt}, partials.SoccerHistoryNoticeField: {partials.SoccerHistoryNoticeIndefinite},
	})
	if imported.Code != http.StatusOK || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("disclosed import: status %d, body %q", imported.Code, imported.Body.String())
	}
	now, _ := source.counts()
	if got := since(now, phase.requests); fmt.Sprint(got) != fmt.Sprint(map[string]int{"players/{id}/my_teams": 2, "users/check": 1}) {
		t.Errorf("import LPS requests = %v, want one account check and one team lookup per linked player", got)
	}
	measure("granted disclosed import (2 linked players)", phase)
	for _, proof := range []struct {
		subject              string
		player, team, season int
		want                 bool
		why                  string
	}{
		{cognito.subject, 1001, 4101, 77, true, "Craig's team season through the importing owner"},
		{cognito.subject, 1002, 4202, 79, true, "Taylor's team season through the importing owner"},
		{"another-subject", 1001, 4101, 77, false, "Craig's team season through another owner"},
		{cognito.subject, 1001, 4301, 80, false, "the entered Team ID's season"},
	} {
		if proven, err := store.HasPlayerMembership(t.Context(), cognito.issuer, proof.subject, proof.player, proof.team, proof.season); err != nil || proven != proof.want {
			t.Errorf("membership proof for %s = %v, %v; want %v", proof.why, proven, err, proof.want)
		}
	}

	// 3. The first daily run refreshes the two teams the import enrolled,
	// each with its team lookup and one lookup per arena its games use.
	// Visitor FC was fetched at enrollment and is not due yet.
	clock := &readinessClock{now: time.Now().UTC().Add(time.Minute)}
	worker, err := soccerarchive.NewDailyWorker(store, lpsServer.URL, &http.Client{Timeout: 15 * time.Second}, candidateHistoryLimits, clock)
	if err != nil {
		t.Fatalf("the candidate limits do not build the daily worker: %v", err)
	}
	firstRunAt := clock.Now()
	phase = start()
	first, err := worker.Run(t.Context())
	if err != nil || !first.Complete || first.Requests != 5 || fmt.Sprint(first.Results) != fmt.Sprint([]soccerarchive.RefreshResult{
		{TeamID: 4101, Outcome: soccerarchive.RefreshSucceeded}, {TeamID: 4202, Outcome: soccerarchive.RefreshSucceeded},
	}) {
		t.Fatalf("first daily run = %+v, %v; want Craig FC (team + arenas 5 and 6) and Taylor FC (team + arena 5) refreshed in 5 requests", first, err)
	}
	firstRunSpan := clock.takeSlept()
	if firstRunSpan < 4*time.Second {
		t.Errorf("five requests were paced %s apart in total, want at least 4s at one request a second", firstRunSpan)
	}
	measure("daily run 1 (2 due teams)", phase)

	// 4. A repeated delivery of the same day's event finds nothing due.
	phase = start()
	repeated, err := worker.Run(t.Context())
	if err != nil || !repeated.Complete || repeated.Requests != 0 || len(repeated.Results) != 0 {
		t.Fatalf("repeated delivery = %+v, %v; want a complete run with no LPS requests", repeated, err)
	}
	measure("repeated delivery of run 1", phase)

	// 5. The owner reads Craig FC's proven season. The stored owner-bound
	// observation proves it without another LPS lookup.
	phase = start()
	history := readHistory(t, owner, 1001, 4101, 77)
	if history.Coverage.Status != "fetched" || !during(history.Coverage.FetchedAt, firstRunAt, firstRunSpan) || history.Coverage.ReturnedGameCount != 3 ||
		history.Record.Wins != 1 || history.Record.Losses != 1 || history.Record.Draws != 0 || fmt.Sprint(history.classifications()) != fmt.Sprint(map[int]string{7001: "win", 7002: "loss"}) {
		t.Errorf("authorized read after run 1 = %+v", history)
	}
	if now, _ := source.counts(); total(since(now, phase.requests)) != 0 {
		t.Errorf("the read of a stored proven season asked LPS %v", since(now, phase.requests))
	}
	// An entered Team ID never proves membership, so its season stays private.
	if unproven := owner.get(historyPath(1001, 4301, 80)); unproven.Code != http.StatusForbidden {
		t.Errorf("read of the entered Team ID's season = %d, want 403", unproven.Code)
	}

	// 6. A day later LPS corrects a score and omits the away game. The next
	// run refreshes all three teams; the read follows the correction and
	// keeps the omitted game.
	clock.advance(24 * time.Hour)
	source.setTeam(4101, readinessCraigFCCorrected)
	secondRunAt := clock.Now()
	phase = start()
	second, err := worker.Run(t.Context())
	if err != nil || !second.Complete || second.Requests != 6 || len(second.Results) != 3 {
		t.Fatalf("second daily run = %+v, %v; want three teams refreshed with one arena each in 6 requests", second, err)
	}
	secondRunSpan := clock.takeSlept()
	measure("daily run 2 (3 due teams, 1 corrected score, 1 omitted game)", phase)
	corrected := readHistory(t, owner, 1001, 4101, 77)
	results := map[int]string{}
	for _, game := range corrected.Games {
		results[game.Game.UGameID] = game.Game.Result
	}
	if !during(corrected.Coverage.FetchedAt, secondRunAt, secondRunSpan) || corrected.Coverage.ReturnedGameCount != 2 ||
		fmt.Sprint(results) != fmt.Sprint(map[int]string{7001: "3 - 2", 7002: "3 - 0"}) || corrected.Record.Wins != 1 || corrected.Record.Losses != 1 {
		t.Errorf("read after the correction = coverage %+v, results %v, record %+v; want 7001 corrected to 3 - 2 and omitted 7002 kept", corrected.Coverage, results, corrected.Record)
	}

	// 7. On the third day Taylor FC fails once with a 503 and is retried
	// within its budget, and LPS rejects Visitor FC's ID. The rejected team
	// stops polling and keeps its facts and its admission slot.
	clock.advance(24 * time.Hour)
	source.failTeamOnce(4202, http.StatusServiceUnavailable)
	source.failTeam(4301, http.StatusNotFound)
	phase = start()
	third, err := worker.Run(t.Context())
	if err != nil || third.Complete || third.Requests != 6 || fmt.Sprint(outcomes(third.Results)) != fmt.Sprint(map[int]soccerarchive.RefreshOutcome{
		4101: soccerarchive.RefreshSucceeded, 4202: soccerarchive.RefreshSucceeded, 4301: soccerarchive.RefreshInvalidTeam,
	}) {
		t.Fatalf("third daily run = %+v, %v; want 4101 refreshed, 4202 refreshed after one retry, and 4301 invalid in 6 requests", third, err)
	}
	clock.takeSlept()
	measure("daily run 3 (1 retried 503, 1 rejected Team ID)", phase)
	if enrolled := enrolledCount(t, table); enrolled != 3 {
		t.Errorf("admission counter after LPS rejected a team = %d, want 3: slots are never released", enrolled)
	}
	clock.advance(24 * time.Hour)
	phase = start()
	fourth, err := worker.Run(t.Context())
	if err != nil || !fourth.Complete || fourth.Requests != 4 || len(fourth.Results) != 2 {
		t.Fatalf("fourth daily run = %+v, %v; want only the two valid teams refreshed in 4 requests", fourth, err)
	}
	clock.takeSlept()
	measure("daily run 4 (2 due teams, nothing changed)", phase)
	if kept, err := store.ReadTeamSeason(t.Context(), 4301, 80); err != nil || len(kept.Games) != 1 || kept.Games[0].Result != "2 - 2" {
		t.Errorf("rejected Team ID history = %+v, %v; want its game kept", kept, err)
	}

	// 8. Verified removal erases Craig's identity and proofs for every
	// owner, keeps team and game facts, and ends the import, so the owner's
	// read is refused until a new import.
	phase = start()
	removal := browserForm(siteOrigin, "/soccer/players/remove", url.Values{"player_id": {"1001"}})
	removal.Header.Set("HX-Request", "true")
	if removed := owner.do(removal); removed.Code != http.StatusOK {
		t.Fatalf("verified removal status = %d, body %q", removed.Code, removed.Body.String())
	}
	if now, _ := source.counts(); fmt.Sprint(since(now, phase.requests)) != fmt.Sprint(map[string]int{"users/check": 1}) {
		t.Errorf("removal LPS requests = %v, want one fresh account check", since(now, phase.requests))
	}
	measure("verified removal of player 1001", phase)
	items, err := table.Items()
	if err != nil {
		t.Fatal(err)
	}
	for key := range items {
		if strings.HasPrefix(key, "PLAYER#1001/") {
			t.Errorf("removal kept Craig's record %s", key)
		}
	}
	for _, kept := range []string{"PLAYER#1002/META", "TEAM#4101/META", "GAME#7001/META", "GAME#7002/META", "FACILITY#6/META"} {
		if items[kept] == nil {
			t.Errorf("removal deleted %s", kept)
		}
	}
	if denied := owner.get(historyPath(1001, 4101, 77)); denied.Code != http.StatusUnauthorized {
		t.Errorf("read after removal = %d, want 401 until a new import", denied.Code)
	}

	allRequests, allBytes := source.counts()
	retained := table.retainedBytes(t)
	partitions := make([]string, 0, len(retained))
	for partition := range retained {
		partitions = append(partitions, partition)
	}
	sort.Strings(partitions)
	summary := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		summary = append(summary, fmt.Sprintf("%s %d B", partition, retained[partition]))
	}
	t.Logf("journey totals: LPS requests %v, response bytes %v; DynamoDB %s; retained %d items: %s",
		allRequests, allBytes, table.Usage(), len(items), strings.Join(summary, ", "))
}

// during reports whether at falls within the span of a daily run that
// started at start; the worker stamps each team when its paced fetch ends.
func during(at *time.Time, start time.Time, span time.Duration) bool {
	return at != nil && !at.Before(start) && !at.After(start.Add(span))
}

// outcomes maps each result's team to its outcome.
func outcomes(results []soccerarchive.RefreshResult) map[int]soccerarchive.RefreshOutcome {
	byTeam := make(map[int]soccerarchive.RefreshOutcome, len(results))
	for _, result := range results {
		byTeam[result.TeamID] = result.Outcome
	}
	return byTeam
}

// enrolledCount reads the archive's admission counter.
func enrolledCount(t *testing.T, table *meteredArchiveTable) int {
	t.Helper()
	items, err := table.Items()
	if err != nil {
		t.Fatal(err)
	}
	counter := items["CAPACITY#SOCCER_HISTORY/ENROLLED"]
	if counter == nil {
		return 0
	}
	count, _ := counter["enrolled_count"].(float64)
	return int(count)
}
