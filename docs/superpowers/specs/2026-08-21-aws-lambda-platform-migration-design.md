# AWS Lambda Portfolio Platform Migration Design

<!-- markdownlint-disable MD013 MD010 -->

**Status:** Approved

**Approved:** 2026-08-21

**Scope:** Git integration, AWS Lambda and API Gateway environments, development
and production cutovers, and retirement gates for App Runner and Amplify

> **2026-08-29 narrow supersession:** The approved
> [development App Runner retirement design](./2026-08-29-development-app-runner-retirement-design.md)
> supersedes only this document's development App Runner retention and seven-day
> observation requirements. Production observation and Amplify retirement
> requirements remain unchanged.

## Purpose

This specification moves the current Go, Templ, and HTMX portfolio onto a
long-term AWS platform without extending the App Runner proof of concept. The
latest audited application tree will be integrated into `main` once, deployed
first to an isolated Lambda development environment, and then promoted as the
same immutable image to production.

The existing App Runner development service and Amplify production application
remain untouched rollback origins until their replacement environments have
passed an observation window. Their eventual removal is a separate destructive
decision.

## Current state

- `craigdevjohnson.com` and `www.craigdevjohnson.com` are Cloudflare-proxied and
  serve the static Amplify application.
- `dev.craigdevjohnson.com` is Cloudflare-proxied and serves the Go application
  from the existing App Runner service.
- The App Runner runtime uses the existing `/portfolio/*` SSM parameters and the
  `portfolio-google-connections` and `portfolio-soccer-sessions` DynamoDB
  tables. These resources remain legacy rollback dependencies.
- The Amplify response currently advertises a one-year shared-cache lifetime.
  A production origin switch therefore requires an explicit Cloudflare purge.
- Pull requests #39, #42, and #43 overlap. The clean local candidate at
  `c5419d40716796531ad5da0dd836bb05e0adb010` contains the intended cumulative
  tree, including the current `main` branch's 40-minute Google Calendar reminder
  and the useful behavior from #43.
- The existing `infra/*.tf` state couples App Runner, Lambda, shared ECR,
  DynamoDB, and mutable image tags. It is retained as legacy infrastructure and
  is not the foundation for the replacement environments.
- The configured AWS CLI identity is the account root user. Routine deployment
  must not begin until a scoped, non-root deployment identity is available.

## Decision

The long-term runtime is an AWS Lambda container behind an API Gateway HTTP API.
Cloudflare remains the public DNS provider and may remain the edge proxy after
the origin path is proven.

```text
Cloudflare DNS and edge
          |
          v
API Gateway Regional custom domain
          |
          v
Lambda live alias -> published container version
          |
          +----> environment-owned DynamoDB tables
          |
          +----> environment-owned SSM parameter paths
```

Development and production use separate OpenTofu roots, backend keys, Lambda
functions, APIs, IAM roles, log groups, alarms, SSM paths, and DynamoDB tables.
They share one immutable ECR release repository so production can promote the
exact digest tested in development without rebuilding.

Standard ECS Fargate plus an Application Load Balancer is the documented escape
hatch. It becomes the preferred target only if measured Lambda cold starts are
unacceptable, required synchronous work cannot stay below API Gateway's
30-second integration ceiling, or the application acquires persistent or
background processing requirements. ECS Express is not used because its
always-on Fargate and load-balancer cost does not improve this low-traffic
application enough to offset its managed-resource boundary.

## Guiding constraints

1. Do not deploy the new application release to App Runner.
2. Do not mutate or import resources from the legacy `infra/` state into the
   replacement state roots.
3. Do not use OpenTofu workspaces, targeted applies, automatic approvals, or
   mutable release tags for the replacement platform.
4. Do not put decrypted SSM values into OpenTofu variables, state, saved plans,
   terminal output, or Git.
5. Do not use the AWS root identity for routine plans, applies, image pushes, or
   runtime inspection.
6. Every AWS or DNS apply uses a saved, reviewed plan or an exact record-level
   mutation list.
7. Development proves a release before production uses that same ECR digest.
8. App Runner and Amplify stay available through seven full days of verified
   replacement service unless a newly observed defect extends that window.
9. Deleting runtime resources, ECR images, branches, secrets, data, or legacy
   state requires a fresh destructive-action plan and approval.
10. Approval of this specification or its implementation plans does not approve
    future live execution. Immediately before each external mutation, present
    the provider, account/repository/zone, exact target, current and proposed
    values, command or console action, and rollback; continue only after approval
    of that exact mutation in the current execution session. Changed inputs
    require new approval.

## Subproject 1: clean integration and Lambda readiness

### Git integration

Create a new integration branch from the reviewed `main` commit. Apply the
complete candidate with `git merge --squash`; do not cherry-pick only
`c5419d40`, because it depends on the preceding PR stack. Resolve the expected
root `main.go` and `main_test.go` modify/delete conflicts by retaining the
candidate's modular application architecture after proving that their current
behavior exists under `internal/`.

The integration branch must first contain an exact candidate-tree commit. New
runtime, container, CI, and infrastructure changes follow as focused
conventional commits. This preserves a simple audit boundary between inherited
application work and migration work.

Open one replacement draft PR against `main`. Only after its local and hosted
checks pass may #39, #42, and #43 be closed as superseded. Their branches remain
present. The replacement PR is merged only after the direct development Lambda
endpoint passes its acceptance checks.

### Trusted request origin

The API Gateway V2 adapter constructs an HTTPS URL from the typed gateway event,
but the resulting request has no `TLS` state and uses the public client IP as
`RemoteAddr`. The generic helper consequently produces HTTP OAuth callbacks and
non-Secure cookies.

The Lambda adapter boundary will attach a context-only trusted origin derived
from `APIGatewayV2HTTPRequestContext.DomainName`. `RequestIsHTTPS` and
`RequestBaseURL` will prefer that marker, then retain their existing direct-TLS
and private-proxy behavior. No public forwarding header becomes trusted.

A real API Gateway V2 event with a public source IP must prove:

- the base origin is HTTPS;
- the Google redirect URI is `https://dev.craigdevjohnson.com/soccer` for the development-domain event;
- Soccer, Google, and portal-compatible cookies carry `Secure`; and
- a direct request from a public address still cannot spoof HTTPS with
  `X-Forwarded-Proto`.

### Lambda lifecycle correctness

Secrets and the application adapter initialize during the Lambda initialization
phase under one bounded context. Initialization failure exits the runtime rather
than retaining a partially initialized adapter. Warm invocations reuse the same
adapter and AWS clients.

SSM resolution validates the complete response before changing any environment
variable. A missing parameter cannot leave half the process environment updated.

The asynchronous Soccer audit write becomes a bounded synchronous operation on
Lambda-compatible request paths. Lambda may freeze the execution environment as
soon as a response is returned, so correctness cannot depend on a detached
goroutine.

### Request budget

Google Calendar add and result-sync work receives a 24-second external-operation
budget, leaving five seconds inside a 29-second Lambda timeout for response
rendering and adapter serialization. Partial mutation counts survive a deadline.
The response states that retry is safe because canonical Google event IDs and
the private `game_id` lookup converge instead of duplicating events.

The implementation stays serial initially. SQS and durable job state are a
separate design only if measurement shows that the synchronous path cannot meet
the budget.

### Health and release proof

`GET /healthz` returns dependency-free JSON containing `status=ok` and the full
source revision injected at image build time. It is not a DynamoDB readiness
probe. A valid release record contains all of:

- Git commit SHA;
- ECR repository and digest;
- Lambda published version;
- `live` alias target;
- API endpoint and custom domain; and
- `/healthz` response revision.

### Container and CI gates

Both Dockerfiles accept BuildKit target-platform arguments without architecture
defaults that override `linux/amd64`. Build-only proxy certificates and optional
runtime trust additions are separate inputs. The Lambda Dockerfile accepts the
build certificate secret without copying it into the final AWS runtime image.

The Taskfile gains non-deploying image-build and verification tasks. Deployment
tasks no longer hide `--auto-approve` or mutable tag updates. A GitHub Actions
workflow runs the authoritative repository gate on every replacement PR.

## Subproject 2: isolated development Lambda and cutover

### State and resource ownership

The legacy `infra/` root remains unchanged except for documentation that marks
its App Runner and shared Lambda tasks as legacy. New infrastructure lives under
`infra/lambda/`:

```text
infra/lambda/
├── artifacts/             immutable ECR release repository state
├── modules/service/       Lambda, API, data, IAM, logs, alarms, and domain
└── environments/
    ├── dev/               development root and backend key
    └── prod/              production root and backend key
```

Backend keys are:

- `portfolio-lambda-http-api/artifacts/terraform.tfstate`
- `portfolio-lambda-http-api/dev/terraform.tfstate`
- `portfolio-lambda-http-api/prod/terraform.tfstate`

The existing `portfolio/terraform.tfstate` object is never migrated or modified
by these roots. S3 bucket versioning and native lock files are prerequisites.

The Lambda execution role uses the account-root-owned permissions boundary
`arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary`.
The replacement roots derive its partition and account from the current AWS data
sources but never create, edit, or remove the boundary policy. A separately
reviewed Identity Center deployer permission set may create or pass only roles
that carry this boundary. It cannot create unbounded roles, attach managed
policies, manage the boundary, or mutate any legacy resource. The reviewed
non-secret initial deployer and boundary policy inputs are tracked under
[`infra/lambda/bootstrap/`](../../../infra/lambda/bootstrap/README.md). Their
presence grants no access; exact live identity, assignment, and approval
evidence remains private and every use is separately approved. The deployer
input is development-only and temporary; never restore it after a removal and
reprovisioning gate. The boundary's production statements are a dormant
runtime ceiling, not production deployment authority.

### Release repository

The artifact root owns `portfolio-lambda-releases`, its lifecycle policy, and
its repository policy. The repository uses immutable tags, scan-on-push,
`force_delete=false`, and no lifecycle rule that expires tagged releases. The
repository policy grants only `ecr:BatchGetImage` and
`ecr:GetDownloadUrlForLayer` to the `lambda.amazonaws.com` service principal.
It requires `aws:SourceAccount` equal to `180294223248` and `aws:SourceArn`
matching only
`arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*`. Image tags
use `git-` followed by the full 40-character source SHA, while Lambda receives
the digest-qualified URI. Provisioning the access package does not create or
import any ECR resource. Its temporary `T2` and `T3` SIDs authorize only the
later separately approved Terraform artifact saved plan and are removed after
verified convergence.

### Development service

Development physical names start with `portfolio-lambda-dev`. The service owns:

- a published x86-64 Lambda image function and `live` alias;
- execution role `${local.function_name}-execution`, inline runtime policy
  `${local.function_name}-runtime`, and the root-owned boundary above;
- HTTP API `${local.function_name}-http`, `$default` route and stage,
  alias-qualified integration, and alias-qualified invoke permission;
- environment-specific Google connection and Soccer session DynamoDB tables;
- exact SSM and KMS read permissions for `/portfolio/lambda/dev/*`;
- managed Lambda and API access log groups with 14-day retention;
- alarms for Lambda errors, throttles, and high duration plus API 5xx and high
  latency; and
- a Regional ACM certificate, API Gateway custom domain, and mapping for
  `dev.craigdevjohnson.com` after direct-endpoint acceptance.

The five alarm names are exactly `${local.function_name}-lambda-errors`,
`${local.function_name}-lambda-throttles`,
`${local.function_name}-lambda-duration`, `${local.function_name}-api-5xx`, and
`${local.function_name}-api-latency`. The inline runtime policy remains limited
to GetItem/PutItem/DeleteItem on the Google table, PutItem on the Soccer table,
GetParameters on the three exact environment paths, Decrypt through the exact
`alias/aws/ssm` key, and CreateLogStream/PutLogEvents in the precreated Lambda
log group. It has no legacy paths, deployment permissions, managed-policy
attachments, CreateLogGroup, or boundary-management actions.

The management portal remains disabled during the first development proof. It
requires a separate threat and IAM scope because the current portal can describe
all instances in its configured region.

### Development rollout

1. Establish and verify the non-root deployment identity.
2. Apply the reviewed artifact plan.
3. Build once, run container checks, push the commit tag, and record the digest.
4. Apply a reviewed development plan using that digest and no custom domain.
5. Verify `/`, `/soccer`, `/static/css/tailwind.css`, a representative binary
   image, `/healthz`, log records, alarms, published version, and alias target.
6. Add the direct API `/soccer` URI to Google OAuth and complete a full connect,
   callback, calendar selection, add, and result-sync exercise.
7. Request the ACM certificate, add only its DNS-only Cloudflare validation
   records, and activate the API Gateway custom domain after issuance.
8. Capture the existing `dev` Cloudflare record, switch it from App Runner to
   the API Gateway custom-domain target, and verify externally.
9. Keep App Runner running and unchanged for seven days.

Before step 8, commit a sanitized machine-readable development rollback record
containing the App Runner origin and exact Cloudflare record ID/type/name/value,
TTL, and proxy state. The immutable release JSON references this file; recurring
checks and any rollback load it rather than relying on a prior shell or log.

Cloudflare traffic starts DNS-only unless a reviewed proxy configuration proves
that dynamic Soccer responses are never cached. Enabling the orange-cloud proxy
does not authorize trusting `CF-Connecting-IP`; the app may intentionally log
the Cloudflare edge address until a bounded proxy-authentication design exists.

### Development rollback

- Before DNS: leave the new endpoint unused or move `live` to the previous
  published Lambda version.
- After DNS: restore the recorded App Runner DNS value, purge only affected
  Cloudflare cache entries, and verify the old App Runner endpoint.
- Do not destroy the failed Lambda environment during incident response; retain
  logs and release evidence for diagnosis.

## Subproject 3: production migration and retirement

### Production preparation

Before production infrastructure is applied:

- inspect and record the hidden Cloudflare origin, redirect rules, cache rules,
  TTLs, and proxy state;
- identify the old Amplify frontend's public Lambda integration dependency;
- choose explicitly between fresh Google connections or an independently
  reviewed encrypted-data migration;
- configure a confirmed alarm notification path;
- verify production table deletion protection and point-in-time recovery; and
- use the exact ECR digest accepted in development.

The default recommendation is fresh production Google connections and a new
session key. Existing encrypted Google rows are not copied because they are
bound to the legacy encryption key. Reusing that key or copying seven current
records requires a separate data-migration plan.

### Production rollout

1. Apply the production environment with 90-day logs, protected tables, alarms,
   and no public DNS change.
2. Exercise the execute-api endpoint and production custom-domain target.
3. Add the production Google OAuth redirect URIs and complete one full Soccer
   and Google Calendar workflow.
4. Record Amplify, Cloudflare, DNS, cache, and Lambda rollback coordinates.
5. Switch `craigdevjohnson.com` first, verify it, then switch or redirect `www`.
6. Purge the old one-year Cloudflare cache and verify the public artifact by
   source revision, digest, Lambda version, and response body.
7. Observe routes, Lambda/API alarms, logs, OAuth, cookies, and latency for seven
   days while Amplify remains available.

The production release JSON references two committed machine-readable inputs:
the sanitized Amplify/Cloudflare precutover record and the SNS delivery proof
containing only topic ARN, confirmed count, message ID, timestamps, and a receipt
token hash. Observation and rollback tooling refuses missing or inconsistent
references; subscriber endpoints and raw receipt tokens are never committed.

### Production rollback

Restore the recorded Amplify origin and `www` behavior, purge the affected
Cloudflare cache, and verify that the Amplify artifact is serving again. Keep the
failed Lambda version and logs for analysis. No data or infrastructure is
deleted during rollback.

### Retirement

After both observation windows pass, present exact removal plans for:

- the App Runner custom domain and service;
- Amplify domain association and application;
- legacy Lambda resources in the old shared state, if any;
- obsolete ECR images or repositories;
- obsolete SSM paths and DynamoDB tables; and
- superseded Git branches and worktrees.

Each removal identifies the exact resource, current consumers, recovery method,
and whether retained data will be exported. No retirement action is implied by
successful cutover.

## Acceptance criteria

1. One replacement PR from current `main` contains the audited candidate tree,
   Lambda-readiness changes, new CI, and the isolated infrastructure roots.
2. PRs #39, #42, and #43 are closed as superseded without deleting their source
   branches.
3. The API Gateway event path produces HTTPS OAuth URLs and Secure cookies while
   public direct requests still cannot spoof proxy headers.
4. `task ci`, targeted race tests, both Linux x86-64 image checks, OpenTofu
   formatting and validation, and the hosted PR check pass.
5. No replacement state contains an App Runner or Amplify address, decrypted
   secret, mutable image tag, or legacy workload resource.
6. `dev.craigdevjohnson.com` serves the accepted Lambda digest and passes the
   complete Soccer and Google Calendar workflow.
7. App Runner remains a tested rollback origin through the development
   observation window.
8. Production promotes the development-tested digest without rebuilding.
9. Apex and `www` behavior, HTTPS, static assets, OAuth, cookies, logs, alarms,
   and cache invalidation are externally verified after production cutover.
10. Amplify remains a tested rollback origin through the production observation
    window.
11. No legacy platform, data, secret, image, branch, or state is deleted without
    a separate approved retirement plan.
12. `infra/lambda/bootstrap/` contains the reviewed initial policy bodies and
    their contract test, while live principal, assignment, provisioning, and
    post-tightening evidence remains outside Git.
