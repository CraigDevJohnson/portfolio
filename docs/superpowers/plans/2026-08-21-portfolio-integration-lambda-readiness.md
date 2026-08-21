# Portfolio Integration and Lambda Readiness Implementation Plan

<!-- markdownlint-disable MD013 MD010 -->

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate the audited application tree onto current `main`, make the Go application correct behind API Gateway, and open one replacement PR with reproducible CI and container proof.

**Architecture:** Start from the reviewed `main` SHA and squash the complete `c5419d40` candidate tree without moving any source branch. Add a context-only trusted-origin boundary at the Lambda adapter, initialize the runtime once during cold start, bound long Google work, expose immutable revision proof, and make Linux image builds non-deploying and reproducible.

**Tech Stack:** Go 1.27, Templ, HTMX, Task, Docker BuildKit, AWS Lambda Go runtime, API Gateway HTTP API payload v2, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md`

## Global Constraints

- Do not deploy the new application release to App Runner.
- Keep `/Users/craigjohnson/repos/portfolio`, `/Users/craigjohnson/repos/portfolio-codebase-cleanup`, and every existing source branch recoverable.
- Do not use the AWS root identity for a plan, apply, image push, or runtime inspection.
- Do not globally trust `X-Forwarded-Proto`, `CF-Connecting-IP`, `Host`, or another client-controlled forwarding header.
- The API Gateway integration ceiling is 30 seconds; the Lambda timeout is 29 seconds and Google add/result-sync external work is bounded at 24 seconds.
- Templ and Tailwind generated files remain ignored artifacts. Edit sources and use `task generate`, `task build`, and `task ci`.
- Stage only named paths. Never use `git add .`, `git add -A`, or `git add --all`.
- Every commit uses Conventional Commits.
- Execute this plan only after its four reviewed planning documents are committed
  together as exactly one commit directly after candidate `c5419d40`; an
  uncommitted planning tree is not a valid integration input.
- Approval of this plan does not authorize a later GitHub mutation. At each
  **Approval gate**, present exact targets, text, command, and rollback and wait
  for current-session approval.

---

### Task 1: Create the clean integration worktree and candidate snapshot commit

**Files:**

- Modify through squash merge: complete candidate tree relative to `main`
- Preserve: `/Users/craigjohnson/repos/portfolio-codebase-cleanup`
- Create worktree: `/Users/craigjohnson/repos/portfolio-lambda-integration`

**Interfaces:**

- Consumes: `origin/main@24ab9ace530e6dd8a1736f34e4f078afc63e480b` and candidate `c5419d40716796531ad5da0dd836bb05e0adb010`
- Produces: branch `codex/integrate-lambda-cutover` whose first source commit tree is byte-equivalent to the candidate, followed by the reviewed planning-documents commit

- [ ] **Step 1: Refresh refs without pruning and assert every reviewed SHA**

```bash
repo=/Users/craigjohnson/repos/portfolio
candidate_wt=/Users/craigjohnson/repos/portfolio-codebase-cleanup
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
candidate_commit=c5419d40716796531ad5da0dd836bb05e0adb010

git -C "$repo" fetch origin
test "$(git -C "$repo" rev-parse origin/main)" = "24ab9ace530e6dd8a1736f34e4f078afc63e480b"
test "$(git -C "$repo" rev-parse origin/refactor)" = "005cc87da0b0b1a2c536a9b0ed53c3d936a9bc38"
test "$(git -C "$repo" rev-parse origin/copilot/explore-non-aws-app-runner-options)" = "3f17a0c1a9f1915cf2fe6837676940e40fdec77c"
git -C "$candidate_wt" merge-base --is-ancestor "$candidate_commit" HEAD
test -z "$(git -C "$candidate_wt" status --porcelain)"

plan_docs_commit=$(git -C "$candidate_wt" rev-parse HEAD)
test "$(git -C "$candidate_wt" rev-list --count \
  "$candidate_commit..$plan_docs_commit")" = "1"
expected_docs=$(printf '%s\n' \
  docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md \
  docs/superpowers/plans/2026-08-21-portfolio-integration-lambda-readiness.md \
  docs/superpowers/plans/2026-08-21-development-lambda-cutover.md \
  docs/superpowers/plans/2026-08-21-production-lambda-cutover.md | sort)
actual_docs=$(git -C "$candidate_wt" diff --name-only \
  "$candidate_commit" "$plan_docs_commit" | sort)
test "$actual_docs" = "$expected_docs"
test ! -e "$integration_wt"
```

Expected: every assertion succeeds, the candidate worktree is clean, and its only commits after `c5419d40` contain the four reviewed planning documents. If `origin/main` moved, stop and re-audit the new commit instead of weakening the assertion.

- [ ] **Step 2: Create an isolated branch from current reviewed main**

```bash
repo=/Users/craigjohnson/repos/portfolio
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
git -C "$repo" worktree add \
  -b codex/integrate-lambda-cutover \
  "$integration_wt" \
  24ab9ace530e6dd8a1736f34e4f078afc63e480b
```

Expected: the new worktree reports branch `codex/integrate-lambda-cutover` and a clean status.

- [ ] **Step 3: Squash the complete candidate stack**

```bash
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
git -C "$integration_wt" merge --squash \
  c5419d40716796531ad5da0dd836bb05e0adb010
```

Expected: modify/delete conflicts only for root `main.go` and `main_test.go`.

- [ ] **Step 4: Prove the candidate contains the current-main behaviors**

```bash
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
rg -n 'Minutes: +40' "$integration_wt/internal/google"
rg -n 'MislabelledLPSZuluTime|time\.Now\(' "$integration_wt/internal/app" -g '*_test.go'
```

Expected: the 40-minute reminder and time-relative schedule fixtures are present under the modular packages.

- [ ] **Step 5: Resolve only the expected deleted root files**

```bash
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
git -C "$integration_wt" rm -- main.go main_test.go
git -C "$integration_wt" diff --cached --check
git -C "$integration_wt" diff --cached --exit-code \
  c5419d40716796531ad5da0dd836bb05e0adb010 --
```

Expected: the final command has no output, proving the staged tree equals the audited candidate.

- [ ] **Step 6: Run the candidate baseline in the integration worktree**

```bash
cd /Users/craigjohnson/repos/portfolio-lambda-integration || exit 1
go mod download
task ci
git diff --exit-code
go mod tidy -diff
go mod verify
git diff --cached --check
```

Expected: every command succeeds and only the squash changes are staged.

- [ ] **Step 7: Commit the exact candidate snapshot**

```bash
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
git -C "$integration_wt" commit \
  -m "feat: integrate portfolio refactor and Lambda deployment" \
  -m "Consolidates PRs #39 and #42 plus audited cleanup c5419d40 on current main. Preserves the refactored equivalents of PRs #41 and #43."
```

Expected: one commit on top of reviewed `main`; `git status --short` is empty.

- [ ] **Step 8: Carry the reviewed planning documents into the integration branch**

```bash
candidate_wt=/Users/craigjohnson/repos/portfolio-codebase-cleanup
integration_wt=/Users/craigjohnson/repos/portfolio-lambda-integration
plan_docs_commit=$(git -C "$candidate_wt" rev-parse HEAD)
git -C "$integration_wt" cherry-pick "$plan_docs_commit"
git -C "$integration_wt" diff --check HEAD~1 HEAD
git -C "$integration_wt" status --short
```

Expected: the cherry-pick changes only the four paths asserted in Step 1, `git diff --check` succeeds, and status is clean.

---

### Task 2: Add a real pull-request CI gate

**Files:**

- Create: `.github/workflows/ci.yml`
- Delete through Task 1 candidate merge: `.github/workflows/copilot-setup-steps.yml`

**Interfaces:**

- Consumes: pinned versions in `go.mod`, `Taskfile.yaml`, and repository documentation
- Produces: required job name `CI / repository`

- [ ] **Step 1: Create the workflow with pinned tool versions**

```yaml
name: CI

on:
  pull_request:
  push:
    branches:
      - main

permissions:
  contents: read

concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  repository:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - name: Checkout
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Install repository tools
        run: |
          go install github.com/go-task/task/v3/cmd/task@v3.53.1
          go install github.com/a-h/templ/cmd/templ@v0.3.1020
          go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.0
          task install-tailwind

      - name: Run repository gate
        run: |
          task ci
          git diff --exit-code

      - name: Verify module graph
        run: |
          go mod tidy -diff
          go mod verify
```

- [ ] **Step 2: Validate YAML structure and the local command sequence**

```bash
ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml", aliases: true)'
task ci
git diff --exit-code
go mod tidy -diff
go mod verify
```

Expected: YAML parses and all repository commands pass.

- [ ] **Step 3: Commit the workflow**

```bash
git add -- .github/workflows/ci.yml
git commit -m "ci: add portfolio pull request gate"
```

---

### Task 3: Add the context-only trusted origin primitive

**Files:**

- Create: `internal/httpx/trusted_origin.go`
- Modify: `internal/httpx/request.go`
- Modify: `internal/httpx/request_test.go`

**Interfaces:**

- Produces: `httpx.TrustedOrigin`, `httpx.WithTrustedOrigin(*http.Request, TrustedOrigin) *http.Request`
- Consumed by: Lambda adapter middleware in Task 4 and all existing `RequestIsHTTPS`, `RequestBaseURL`, and `NewSecureCookie` callers

- [ ] **Step 1: Write failing trusted-origin tests**

```go
func TestTrustedOriginOverridesUntrustedTransportMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.invalid/soccer", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req = WithTrustedOrigin(req, TrustedOrigin{Scheme: "https", Host: "dev.craigdevjohnson.com"})

	if !RequestIsHTTPS(req) {
		t.Fatal("trusted API Gateway origin was not treated as HTTPS")
	}
	if got := RequestBaseURL(req); got != "https://dev.craigdevjohnson.com" {
		t.Fatalf("RequestBaseURL = %q", got)
	}
}

func TestTrustedOriginRejectsInvalidValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	for _, origin := range []TrustedOrigin{
		{Scheme: "javascript", Host: "example.com"},
		{Scheme: "https", Host: ""},
	} {
		got := WithTrustedOrigin(req, origin)
		if RequestIsHTTPS(got) {
			t.Fatalf("invalid origin became trusted: %#v", origin)
		}
	}
}
```

Retain and extend `TestRequestIsHTTPSOnlyTrustsProxiedHeader` so a public source with `X-Forwarded-Proto: https` remains HTTP.

- [ ] **Step 2: Run the tests and confirm failure**

Run: `go test ./internal/httpx -run 'Test(TrustedOrigin|RequestIsHTTPS)' -count=1`

Expected: compile failure because `TrustedOrigin` and `WithTrustedOrigin` do not exist.

- [ ] **Step 3: Implement the unforgeable context marker**

```go
package httpx

import (
	"context"
	"net/http"
	"strings"
)

type trustedOriginContextKey struct{}

type TrustedOrigin struct {
	Scheme string
	Host   string
}

func WithTrustedOrigin(request *http.Request, origin TrustedOrigin) *http.Request {
	scheme := strings.ToLower(strings.TrimSpace(origin.Scheme))
	host := strings.TrimSpace(origin.Host)
	if request == nil || (scheme != "http" && scheme != "https") || host == "" {
		return request
	}
	origin = TrustedOrigin{Scheme: scheme, Host: host}
	ctx := context.WithValue(request.Context(), trustedOriginContextKey{}, origin)
	return request.WithContext(ctx)
}

func requestTrustedOrigin(request *http.Request) (TrustedOrigin, bool) {
	if request == nil {
		return TrustedOrigin{}, false
	}
	origin, ok := request.Context().Value(trustedOriginContextKey{}).(TrustedOrigin)
	return origin, ok
}
```

Update `RequestIsHTTPS` to check `requestTrustedOrigin` first. Update `RequestBaseURL` to return `origin.Scheme + "://" + origin.Host` when present, then retain the existing fallback.

- [ ] **Step 4: Run focused and package tests**

```bash
go test ./internal/httpx -count=1
go test ./internal/google ./internal/soccer ./internal/portal -count=1
```

Expected: all pass, including public forwarded-header spoof rejection.

- [ ] **Step 5: Commit**

```bash
git add -- internal/httpx/trusted_origin.go internal/httpx/request.go internal/httpx/request_test.go
git commit -m "fix: model trusted request origins"
```

---

### Task 4: Move Lambda construction into cold start and attach the gateway origin

**Files:**

- Modify: `cmd/lambda/main.go`
- Create: `cmd/lambda/main_test.go`
- Modify: `internal/app/server.go`
- Modify: affected `internal/app/*_test.go` call sites

**Interfaces:**

- Consumes: `httpx.WithTrustedOrigin` from Task 3
- Produces: `proxyV2`, `initializeLambda(context.Context) (proxyV2, error)`, `newLambdaHandler(proxyV2)`, `withAPIGatewayOrigin(http.Handler) http.Handler`

- [ ] **Step 1: Write failing gateway-event and one-time-initialization tests**

Build a real event with these fields:

```go
event := events.APIGatewayV2HTTPRequest{
	RawPath: "/soccer/google/connect",
	Headers: map[string]string{"host": "dev.craigdevjohnson.com"},
	RequestContext: events.APIGatewayV2HTTPRequestContext{
		DomainName: "dev.craigdevjohnson.com",
		HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
			Method:   http.MethodGet,
			Path:     "/soccer/google/connect",
			Protocol: "HTTP/1.1",
			SourceIP: "203.0.113.10",
		},
	},
}
```

Run table-driven V2 events through
`httpadapter.NewV2(withAPIGatewayOrigin(mux))`. Mount production handler or
cookie code behind test-only routes:

- configured `google.Handler.ConnectHandler`: require `303`, parse `Location`,
  require `redirect_uri=https://dev.craigdevjohnson.com/soccer`, and require the
  real `google_oauth_state` cookie to be `Secure`, `HttpOnly`, path `/soccer`,
  and SameSite Lax;
- `google.SetConnectionCookie`: require the real `google_connection` cookie to
  be `Secure`;
- configured `soccer.Handler.LogoutHandler`: require the real expired
  `lps_session` cookie to remain `Secure`; and
- configured `portal.Handler.LogoutHandler`: require the real expired
  `mgmt_session` cookie to be `Secure`, path `/`, and SameSite Strict.

Construct the portal handler with an empty config and nil OIDC/AWS clients,
assert `PortalEnabled()` is false, and do not register `/login`,
`/auth/callback`, or `/mgmt`. This proves the shared origin behavior remains
portal-compatible without enabling or exposing the portal. Retain a small probe
handler assertion for the trusted base URL. Add fake `proxyV2` tests for
nil-event rejection and warm reuse.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./cmd/lambda -run 'Test(APIGateway|LambdaHandler)' -count=1`

Expected: compile failure for the new interfaces and middleware.

- [ ] **Step 3: Implement the gateway boundary and cold-start functions**

Use the adapter's typed context, never request headers:

```go
func withAPIGatewayOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayContext, ok := core.GetAPIGatewayV2ContextFromContext(r.Context())
		domain := strings.TrimSpace(gatewayContext.DomainName)
		if !ok || domain == "" {
			http.Error(w, "gateway request context missing", http.StatusInternalServerError)
			return
		}
		r = httpx.WithTrustedOrigin(r, httpx.TrustedOrigin{Scheme: "https", Host: domain})
		next.ServeHTTP(w, r)
	})
}
```

Replace the mutex and package adapter with:

```go
type proxyV2 interface {
	ProxyWithContext(context.Context, events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)
}

type lambdaHandlerFunc func(context.Context, *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)

func initializeLambda(ctx context.Context) (proxyV2, error) {
	if err := resolveSSMSecrets(ctx); err != nil {
		return nil, fmt.Errorf("resolve SSM secrets: %w", err)
	}
	handler, err := app.NewLambdaHandler(ctx)
	if err != nil {
		return nil, fmt.Errorf("construct application: %w", err)
	}
	return httpadapter.NewV2(withAPIGatewayOrigin(handler)), nil
}

func newLambdaHandler(proxy proxyV2) lambdaHandlerFunc {
	return func(ctx context.Context, request *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		if request == nil {
			return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusBadRequest, Body: "invalid request"}, nil
		}
		return proxy.ProxyWithContext(ctx, *request)
	}
}
```

In `main`, use an eight-second initialization context, log an initialization error, exit nonzero, then call `lambda.Start(newLambdaHandler(proxy))`.

Change `app.NewLambdaHandler()` to `app.NewLambdaHandler(ctx context.Context)`. Pass the same context to store construction and return configured-store errors in Lambda mode. Keep the regular server's background degraded behavior by logging returned errors.

- [ ] **Step 4: Run Lambda and application tests**

```bash
go test -race ./cmd/lambda ./internal/httpx ./internal/app -count=1
go test ./internal/google ./internal/soccer ./internal/portal -count=1
```

Expected: all pass; the V2 event produces HTTPS and `Secure` without a forwarding header.

- [ ] **Step 5: Commit**

```bash
git add -- cmd/lambda/main.go cmd/lambda/main_test.go internal/app/server.go
git diff --cached --name-only
git commit -m "fix: initialize Lambda behind a trusted gateway origin"
```

Before committing, unstage any application file not required by the signature change.

---

### Task 5: Make SSM resolution atomic and testable

**Files:**

- Modify: `cmd/lambda/secrets.go`
- Create: `cmd/lambda/secrets_test.go`

**Interfaces:**

- Produces: private `ssmParameterGetter` interface and `resolveSSMSecretsWithClient(context.Context, ssmParameterGetter) error`
- Consumed by: `initializeLambda`

- [ ] **Step 1: Write failing tests for complete, missing, literal, and partial responses**

Use `t.Setenv` for the three known keys. A fake client implements:

```go
type fakeSSMGetter struct {
	output *ssm.GetParametersOutput
	err    error
}

func (fake *fakeSSMGetter) GetParameters(
	_ context.Context,
	_ *ssm.GetParametersInput,
	_ ...func(*ssm.Options),
) (*ssm.GetParametersOutput, error) {
	return fake.output, fake.err
}
```

Assert that a missing second parameter leaves all three environment variables at their original path values.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./cmd/lambda -run 'TestResolveSSM' -count=1`

Expected: compile failure for the injected-client function.

- [ ] **Step 3: Validate the entire response before calling `os.Setenv`**

Implement the AWS SDK interface, collect environment-name-to-path pairs, call
`GetParameters` once, reject `InvalidParameters`, build a path map, verify every
requested path exists, then update the known environment keys in deterministic
order. Literal non-path values remain unchanged.

- [ ] **Step 4: Run focused and race tests**

```bash
go test ./cmd/lambda -run 'TestResolveSSM' -count=1
go test -race ./cmd/lambda -count=1
```

- [ ] **Step 5: Commit**

```bash
git add -- cmd/lambda/secrets.go cmd/lambda/secrets_test.go
git commit -m "fix: resolve Lambda secrets atomically"
```

---

### Task 6: Await the Soccer audit write before returning

**Files:**

- Modify: `internal/soccer/auth.go`
- Create: `internal/soccer/auth_test.go`

**Interfaces:**

- Changes: `persistSessionRecord(context.Context, string, *types.SessionData) error`
- Preserves: successful import response even when the optional audit write fails

- [ ] **Step 1: Add a blocking fake-store regression test**

Create a fake `SoccerStore` whose `Put` closes `started`, waits on `release`, then
returns. Invoke `ImportHandler` in a goroutine. Assert no response completes
before `release` closes, then close it and assert the normal success fragment.

- [ ] **Step 2: Run the test and prove the detached goroutine fails it**

Run: `go test ./internal/soccer -run 'TestImportHandlerWaitsForSessionPersistence' -count=1`

Expected: failure because the response completes before the fake store is released.

- [ ] **Step 3: Make the write bounded and synchronous**

Replace `context.WithoutCancel` plus `go` with a direct call using the request
context and a three-second child timeout. Return the store error from
`persistSessionRecord`; log it as a warning and continue rendering the successful
browser-session response because DynamoDB is an audit baseline, not the session
source of truth.

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/soccer ./internal/app -count=1`

- [ ] **Step 5: Commit**

```bash
git add -- internal/soccer/auth.go internal/soccer/auth_test.go
git diff --cached --name-only
git commit -m "fix: finish Soccer persistence within Lambda requests"
```

---

### Task 7: Add liveness and immutable revision proof

**Files:**

- Create: `internal/buildinfo/buildinfo.go`
- Create: `internal/buildinfo/buildinfo_test.go`
- Create: `internal/app/health.go`
- Create: `internal/app/health_test.go`
- Modify: `internal/app/server.go`
- Modify: `internal/app/server_render_smoke_test.go`
- Modify later in Task 9: `Dockerfile`, `Dockerfile.lambda`

**Interfaces:**

- Produces: `buildinfo.Revision() string`, `GET /healthz`
- Response: `200`, `application/json`, `Cache-Control: no-store`, body `{"status":"ok","revision":"..."}`

- [ ] **Step 1: Write failing endpoint tests**

```go
func TestHealthHandlerReturnsRevisionWithoutDependencyProbe(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	healthHandler("0123456789abcdef").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Body.String(); got != "{\"revision\":\"0123456789abcdef\",\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", got)
	}
}
```

Add `/healthz` to the public route smoke table.

- [ ] **Step 2: Run and prove failure**

Run: `go test ./internal/app -run 'TestHealth|TestBuildMuxPublicRouteRenderingSmoke' -count=1`

Expected: compile or route failure.

- [ ] **Step 3: Implement build info and the dependency-free handler**

```go
package buildinfo

import "strings"

var revision = "development"

func Revision() string {
	value := strings.TrimSpace(revision)
	if value == "" {
		return "development"
	}
	return value
}
```

```go
func healthHandler(revision string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"revision": revision,
			"status":   "ok",
		})
	}
}
```

Register `GET /healthz` using `buildinfo.Revision()` before feature-dependent routes.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/buildinfo ./internal/app -count=1`

- [ ] **Step 5: Commit**

```bash
git add -- internal/buildinfo/buildinfo.go internal/buildinfo/buildinfo_test.go internal/app/health.go internal/app/health_test.go internal/app/server.go internal/app/server_render_smoke_test.go
git commit -m "feat: expose immutable release health proof"
```

---

### Task 8: Bound Google Calendar mutations and preserve partial progress

**Files:**

- Modify: `internal/google/oauth.go`
- Modify: `internal/google/calendar_handlers.go`
- Modify: `internal/google/calendar_events.go`
- Modify: `internal/google/add_handler_test.go`
- Modify: `internal/app/handlers_soccer_schedule_test.go`

**Interfaces:**

- Produces: `calendarMutationResult`
- Changes: `insertCalendarEvents(context.Context, *http.Request, *ConnectionRecord, *oauth2.Token, []types.Game) (calendarMutationResult, error)`
- Configuration: `Handler.CalendarMutationTimeout`, default `24*time.Second`

- [ ] **Step 1: Add cancellation, partial-count, and safe-retry tests**

Use a fake HTTP transport that succeeds for one canonical game, then blocks on
`request.Context().Done()` for the next. Set `CalendarMutationTimeout` to
`20*time.Millisecond`. Exercise both `AddHandler` and `SyncResultsHandler`.
Assert each response reports one completed mutation and a safe retry. Retry
with both games and assert the first event is updated or matched, not duplicated.

- [ ] **Step 2: Run and prove failure**

Run: `go test ./internal/google -run 'Test(AddHandler|SyncResultsHandler)(Deadline|Retry)' -count=1`

Expected: existing tuple results discard partial counts and no handler budget ends the request.

- [ ] **Step 3: Introduce the explicit result type and context parameter**

```go
type calendarMutationResult struct {
	added        int
	updated      int
	skipped      int
	authRejected bool
}
```

Increment the result after each completed game. Return the accumulated result
with any later error. Pass the caller's bounded context through OAuth and every
Google HTTP request instead of constructing a fresh background context.

- [ ] **Step 4: Apply the 24-second work budget at both handler entries**

In both `AddHandler` and `SyncResultsHandler`, derive:

```go
timeout := h.CalendarMutationTimeout
if timeout <= 0 {
	timeout = 24 * time.Second
}
workCtx, cancel := context.WithTimeout(r.Context(), timeout)
defer cancel()
workRequest := r.Clone(workCtx)
```

Use `workRequest` for form parsing, connection lookup, Soccer selection or result
resolution, token refresh, and calendar calls. Keep the original request for
final HTMX rendering. When
`errors.Is(err, context.DeadlineExceeded)` or `errors.Is(err, context.Canceled)`,
render the completed count and this statement: `The request reached its time limit. Retry to finish; existing games will be matched instead of duplicated.`

- [ ] **Step 5: Run focused tests repeatedly**

```bash
go test -race ./internal/google -count=1
go test ./internal/google -run 'Test(AddHandler|SyncResultsHandler)(Deadline|Retry)' -count=10
go test ./internal/app -run 'TestResolveSyncResultsGames' -count=10
```

- [ ] **Step 6: Commit**

```bash
git add -- internal/google/oauth.go internal/google/calendar_handlers.go internal/google/calendar_events.go internal/google/add_handler_test.go
if ! git diff --quiet -- internal/app/handlers_soccer_schedule_test.go; then
  git add -- internal/app/handlers_soccer_schedule_test.go
fi
git diff --cached --name-only
git commit -m "fix: bound synchronous Google Calendar mutations"
```

The conditional stages `internal/app/handlers_soccer_schedule_test.go` by its
exact name only when its context regression changes, and does so before the
commit and clean-tree release gate.

---

### Task 9: Make both Linux image builds reproducible and non-deploying

**Files:**

- Modify: `Dockerfile`
- Modify: `Dockerfile.lambda`
- Modify: `docker-compose.yml`
- Modify: `Taskfile.yaml`
- Modify: `tests/docker-runtime-ca.sh`
- Create: `tests/docker-lambda-image.sh`

**Interfaces:**

- Produces Task commands: `task build-image`, `task build-lambda-image`, `task test-images`
- Inputs: `IMAGE_TAG`, `BUILD_REVISION`, `DOCKER_BUILD_CA_CERT_PATH`, optional `DOCKER_RUNTIME_CA_CERT_PATH`

- [ ] **Step 1: Extend image contract tests before changing Dockerfiles**

Create `tests/docker-lambda-image.sh` with:

```sh
#!/bin/sh
set -eu

image="portfolio-lambda-contract:$$"
container="portfolio-lambda-contract-$$"
tmp_dir=$(mktemp -d)
cleanup() {
	docker rm -f "$container" >/dev/null 2>&1 || true
	docker image rm "$image" >/dev/null 2>&1 || true
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

docker build --platform linux/amd64 \
	--build-arg BUILD_REVISION=0123456789abcdef0123456789abcdef01234567 \
	-f Dockerfile.lambda -t "$image" .

test "$(docker image inspect --format '{{.Architecture}}' "$image")" = "amd64"
docker create --name "$container" "$image" >/dev/null
docker cp "$container:/var/task/bootstrap" "$tmp_dir/bootstrap"
file "$tmp_dir/bootstrap" | grep -Eq 'x86-64|x86_64'
```

- [ ] **Step 2: Run the new contract and capture the current result**

Run: `sh tests/docker-lambda-image.sh`

Expected in an intercepted network: the current Lambda Dockerfile may fail
`wget` or `go mod download` with an untrusted Gateway CA. If it succeeds, the
test still establishes the pre-change architecture baseline.

- [ ] **Step 3: Remove target-architecture defaults and add build-only CA support**

In `Dockerfile`, declare `ARG TARGETOS` and `ARG TARGETARCH` without `linux` or
`arm64` defaults. In both Dockerfiles, accept a BuildKit secret named
`build_ca_bundle`, append it only in the builder stage before network calls, and
use `ARG BUILD_CA_BUNDLE_DIGEST=empty` to invalidate the relevant layer when the
secret changes.

In `Dockerfile.lambda`, add:

```dockerfile
ARG BUILD_REVISION=development

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath \
  -ldflags="-s -w -X portfolio/internal/buildinfo.revision=${BUILD_REVISION}" \
  -o /bootstrap ./cmd/lambda
```

Do not copy the builder certificate bundle into the Lambda runtime image.

Give the regular Dockerfile the same `BUILD_REVISION=development` argument and
`-X portfolio/internal/buildinfo.revision=${BUILD_REVISION}` linker injection so
its `/healthz` response identifies the built source too.

- [ ] **Step 4: Separate optional regular-runtime trust and prove the regular image architecture**

Give the regular Dockerfile a distinct `runtime_ca_bundle` secret. Its final
trust store receives only that bundle. Update Compose so local environments may
point both secrets at the same Gateway CA file without making that coupling part
of the image contract. Update `tests/docker-runtime-ca.sh` to build with
`--platform linux/amd64`, pass both secrets, assert the image inspection reports
`amd64`, copy `/app/portfolio-server` from the stopped test container and use
`file` to require `x86-64` or `x86_64`, then prove only the runtime secret appears
in the final trust store.

- [ ] **Step 5: Add non-deploying Taskfile build commands**

Both commands create a private temporary build-CA bundle, prefer
`DOCKER_BUILD_CA_CERT_PATH`, otherwise export the macOS `Gateway CA`, compute its
digest, and call `docker build` with a BuildKit secret. Both commands always use
`--platform linux/amd64`, the supplied `IMAGE_TAG`, and either the supplied
`BUILD_REVISION` or the full `git rev-parse HEAD`. `build-lambda-image` selects
`Dockerfile.lambda`.

`test-images` runs both shell contract files. No new build task logs certificate
contents or environment secrets.

- [ ] **Step 6: Run image and repository gates**

```bash
task build-image IMAGE_TAG=portfolio:local-proof
task build-lambda-image IMAGE_TAG=portfolio-lambda:local-proof
task test-images
task ci
```

Expected: both image manifests report `amd64`, both Go executables are x86-64,
the regular runtime trust test passes, and repository CI passes.

- [ ] **Step 7: Commit**

```bash
git add -- Dockerfile Dockerfile.lambda docker-compose.yml Taskfile.yaml tests/docker-runtime-ca.sh tests/docker-lambda-image.sh
git commit -m "build: make Linux images immutable and verifiable"
```

---

### Task 10: Run the readiness gate and open the replacement draft PR

**Files:**

- Modify: `README.md`
- Modify: `DEPLOY-INSTRUCTIONS.md`
- Modify: `docs/deployment/aws-lambda-api-gateway.md`
- Create: `docs/pull-requests/2026-08-21-lambda-integration.md`
- Create through GitHub: replacement draft PR
- Close through GitHub after replacement checks pass: PRs #39, #42, and #43

**Interfaces:**

- Consumes: all Task 1-9 commits
- Produces: one reviewed replacement PR, hosted CI evidence, and reversible superseding closures

- [ ] **Step 1: Update runtime documentation to match implemented behavior**

Document `/healthz`, the context-backed origin, 29-second Lambda timeout,
24-second Google add and result-sync budget, non-deploying image tasks, and that the old
`deploy-lambda` and `redeploy-lambda` commands operate only on legacy shared
state and must not be used for replacement environments.

- [ ] **Step 2: Create a concise PR body**

```markdown
## Summary

- consolidate PRs #39 and #42 plus audited cleanup `c5419d40` on current `main`
- preserve current-main reminder behavior and the useful time-relative fixtures from #43
- make API Gateway HTTPS, cookies, Lambda initialization, long Google work, and image builds release-safe
- add a real repository CI gate

## Source audit

- base: `24ab9ace530e6dd8a1736f34e4f078afc63e480b`
- PR #39 head: `005cc87da0b0b1a2c536a9b0ed53c3d936a9bc38`
- PR #42 head: `3f17a0c1a9f1915cf2fe6837676940e40fdec77c`
- cleanup candidate: `c5419d40716796531ad5da0dd836bb05e0adb010`
- expected modify/delete conflicts resolved by retaining the candidate's modular deletion of root `main.go` and `main_test.go`

## Verification

- `task ci`
- targeted race tests for Lambda, HTTP origin, Google, Soccer, and application packages
- repeated Google sync and schedule tests
- `go mod tidy -diff` and `go mod verify`
- Linux amd64 Lambda bootstrap inspection
- Linux amd64 regular-server inspection and runtime CA contract

## Deployment

Opening or merging this PR does not deploy AWS infrastructure or change DNS.
```

- [ ] **Step 3: Run the full local release gate**

```bash
task ci
go test -race ./cmd/lambda ./internal/httpx ./internal/google ./internal/soccer ./internal/app
go test ./internal/google -run 'Test(EventPayload|(AddHandler|SyncResultsHandler)(Deadline|Retry))' -count=10
go mod tidy -diff
go mod verify
task test-images
git diff --check
git diff --check origin/main...HEAD
```

Expected: every command passes.

- [ ] **Step 4: Commit documentation and confirm a clean status**

```bash
git add -- README.md DEPLOY-INSTRUCTIONS.md docs/deployment/aws-lambda-api-gateway.md docs/pull-requests/2026-08-21-lambda-integration.md
git commit -m "docs: define the Lambda readiness contract"
git status --short
```

Expected: no status output.

- [ ] **Step 5: Push the exact branch and create a draft PR**

**Approval gate:** Stop and present the exact branch SHA, repository, base,
head, title, and draft PR body. Push and create the PR only after the user
approves those GitHub mutations in the current execution session.

```bash
git push -u origin codex/integrate-lambda-cutover

pr_url=$(gh pr create \
  --repo CraigDevJohnson/portfolio \
  --draft \
  --base main \
  --head codex/integrate-lambda-cutover \
  --title "feat: integrate portfolio and prepare Lambda runtime" \
  --body-file docs/pull-requests/2026-08-21-lambda-integration.md)
echo "$pr_url"
```

- [ ] **Step 6: Wait for and inspect hosted checks**

```bash
pr_url=$(gh pr view --json url --jq .url)
gh pr checks "$pr_url" --watch --interval 10
local_head_sha=$(git rev-parse HEAD)
remote_head_sha=$(gh pr view "$pr_url" --json headRefOid --jq .headRefOid)
test "$remote_head_sha" = "$local_head_sha"
gh pr view "$pr_url" --json number,baseRefName,headRefName,headRefOid,isDraft,mergeable,mergeStateStatus,statusCheckRollup,url
```

Expected: base `main`, correct head, draft status, mergeable, and `CI / repository` successful.

- [ ] **Step 7: Close the three superseded PRs without deleting branches**

**Approval gate:** Stop and present the verified replacement PR number and SHA,
the three exact PR numbers, and the complete closure comments below. Close them
only after the user approves these reversible GitHub mutations in the current
execution session.

```bash
pr_url=$(gh pr view --json url --jq .url)
replacement_number=$(gh pr view "$pr_url" --json number --jq .number)
replacement_sha=$(git rev-parse HEAD)
test "$(gh pr view "$pr_url" --json headRefOid --jq .headRefOid)" = "$replacement_sha"

gh pr close 42 --repo CraigDevJohnson/portfolio \
  --comment "Superseded by #${replacement_number}. The replacement starts from current main and carries the complete tree through 3f17a0c1 plus audited cleanup c5419d40. Verification passed at ${replacement_sha}. Closing only; the source branch remains for provenance and recovery."

test "$(gh pr view "$pr_url" --json headRefOid --jq .headRefOid)" = "$replacement_sha"
gh pr close 39 --repo CraigDevJohnson/portfolio \
  --comment "Superseded by #${replacement_number}, which carries refactor@005cc87d in the consolidated candidate tree on current main. Closing this draft only; the source branch remains."

test "$(gh pr view "$pr_url" --json headRefOid --jq .headRefOid)" = "$replacement_sha"
gh pr close 43 --repo CraigDevJohnson/portfolio \
  --comment "Superseded by #${replacement_number}. Its time-relative fixture behavior is present in the refactored internal/app schedule tests. Closing only; the source branch remains."
```

- [ ] **Step 8: Verify the GitHub end state**

```bash
gh pr list --repo CraigDevJohnson/portfolio --state open \
  --json number,title,baseRefName,headRefName,isDraft,statusCheckRollup,url
git status --short --branch
```

Expected: only the replacement draft remains open for this stack; all source branches still exist.
