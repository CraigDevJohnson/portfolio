# Development App Runner Retirement Implementation Plan

<!-- markdownlint-disable MD013 -->

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make App Runner retirement reviewable and reproducible without the
former seven-day development observation window, while retaining every shared
or Lambda resource.

**Architecture:** Preserve the historical failed observation unchanged, encode
the accepted risk in a superseding design, remove App Runner from the legacy
OpenTofu configuration, and route the destructive operation through an
exact-action saved-plan checker. Current operator documentation and the future
production plan consume the superseding decision rather than the retired
development gate.

**Tech Stack:** OpenTofu, Task, POSIX shell, jq, GitHub Actions, Markdown

**Spec:** `docs/superpowers/specs/2026-08-29-development-app-runner-retirement-design.md`

## Global Constraints

- Preserve commit `927a11835dc217cc228361b383e54565af73c2cb` and its two evidence files byte-for-byte.
- Leave `development.observation_completed_at` as `null`; never relabel the failed sample as passed.
- Permit exactly the eight managed-resource deletes and four root-output deletes named in the spec.
- Permit data-source reads and no-op actions, but reject every other create, update, replacement, or delete action.
- Retain both ECR repositories, both legacy DynamoDB tables, shared DynamoDB IAM policies, all SSM parameters, the legacy Lambda/API Gateway resources, every `infra/lambda/` resource, Cloudflare, Google OAuth configuration, and App Runner log groups.
- Keep the shared production observation scripts, tests, and production tasks intact; retire only the development observation task entry points that require App Runner.
- Do not run a remote OpenTofu plan, write a state lock, disassociate a custom domain, apply a plan, deploy, push, open a pull request, merge, or mutate Cloudflare/AWS during implementation.
- Do not use `tofu destroy`, `-destroy`, `-target`, or automatic approval for retirement.
- Use `Taskfile.yaml` as the command source of truth and run the repository-required checks before completion.

---

### Task 1: Preserve the observation evidence

**Files:**

- Create unchanged: `docs/deployment/evidence/development-observation.jsonl`
- Create unchanged: `docs/deployment/evidence/releases/4db774fac83c23af5a872bcf703ba3b021a2e5c4.json`

**Interfaces:**

- Consumes: local commit `927a11835dc217cc228361b383e54565af73c2cb`
- Produces: truthful historical evidence referenced by the retirement design and operator docs

- [ ] **Step 1: Record the expected evidence object IDs**

```bash
git ls-tree -r 927a11835dc217cc228361b383e54565af73c2cb \
  docs/deployment/evidence/development-observation.jsonl \
  docs/deployment/evidence/releases/4db774fac83c23af5a872bcf703ba3b021a2e5c4.json
```

Expected: two blob IDs, one for each evidence file.

- [ ] **Step 2: Cherry-pick the historical evidence commit**

```bash
git cherry-pick 927a11835dc217cc228361b383e54565af73c2cb
```

Expected: one local `docs: record development Lambda observation sample`
commit with no conflict.

- [ ] **Step 3: Prove the evidence bytes are unchanged**

```bash
test "$(git rev-parse HEAD:docs/deployment/evidence/development-observation.jsonl)" = \
  "$(git rev-parse 927a11835dc217cc228361b383e54565af73c2cb:docs/deployment/evidence/development-observation.jsonl)"
test "$(git rev-parse HEAD:docs/deployment/evidence/releases/4db774fac83c23af5a872bcf703ba3b021a2e5c4.json)" = \
  "$(git rev-parse 927a11835dc217cc228361b383e54565af73c2cb:docs/deployment/evidence/releases/4db774fac83c23af5a872bcf703ba3b021a2e5c4.json)"
jq -e '.development.observation_completed_at == null' \
  docs/deployment/evidence/releases/4db774fac83c23af5a872bcf703ba3b021a2e5c4.json
jq -e '.passed == false and .unresolved_blockers == ["rollback origin failed"]' \
  docs/deployment/evidence/development-observation.jsonl
```

Expected: every command exits zero.

---

### Task 2: Enforce the exact retirement-plan contract

**Files:**

- Create: `tests/app-runner-retirement-plan.sh`
- Create: `scripts/check-app-runner-retirement-plan.sh`
- Modify: `.github/workflows/ci.yml:79-97`

**Interfaces:**

- Consumes: `PLAN_JSON`, an absolute path to `tofu show -json` output
- Produces: a POSIX checker that exits zero only for the spec's exact actionable resource and output changes

- [ ] **Step 1: Write the failing shell contract test**

Create `tests/app-runner-retirement-plan.sh` following the existing
`tests/lambda-plan-contract.sh` harness. Its valid fixture contains the eight
exact managed addresses with `change.actions == ["delete"]`, non-null `before`,
null `after`, the four output deletes, retained-resource no-ops, and one
data-source read. Add mutations rejected for:

```text
missing expected delete
unexpected delete
create action
update action
replacement action
expected address with a non-delete action
delete with a non-null after value
unexpected output delete
missing expected output delete
relative PLAN_JSON path
missing PLAN_JSON file
missing resource_changes
missing output_changes
```

The test runs the real checker and asserts exit status; it does not inspect
checker source text.

- [ ] **Step 2: Run the focused test and verify RED**

```bash
sh tests/app-runner-retirement-plan.sh
```

Expected: FAIL because `scripts/check-app-runner-retirement-plan.sh` does not
exist.

- [ ] **Step 3: Implement the minimal POSIX checker**

Create `scripts/check-app-runner-retirement-plan.sh` with `set -eu`. Require an
absolute, existing `PLAN_JSON`, `.resource_changes` as an array, and
`.output_changes` as an object. For actionable managed resources require the
exact sorted eight-address set, exact `["delete"]` actions, non-null `before`,
and null `after`. For actionable outputs require the exact sorted four-name set,
exact `["delete"]` actions, non-null `before`, and null `after`. Treat only
`["no-op"]` and `["read"]` as non-actionable. Print
`App Runner retirement plan contract passed` only after every assertion passes.

- [ ] **Step 4: Run the focused test and verify GREEN**

```bash
sh tests/app-runner-retirement-plan.sh
```

Expected: every named case passes and the script prints a final pass count.

- [ ] **Step 5: Wire the offline contract into CI**

Add `sh tests/app-runner-retirement-plan.sh` to the offline deployment contract
step. Change the OpenTofu formatting check to
`tofu fmt -check -recursive infra`. Initialize and validate the retained legacy
root using:

```bash
tofu -chdir=infra init -backend=false -input=false
tofu -chdir=infra validate
```

- [ ] **Step 6: Re-run focused validation and commit**

```bash
sh tests/app-runner-retirement-plan.sh
tofu fmt -check -recursive infra
tofu -chdir=infra init -backend=false -input=false
tofu -chdir=infra validate
git add scripts/check-app-runner-retirement-plan.sh \
  tests/app-runner-retirement-plan.sh .github/workflows/ci.yml
git commit -m "test: enforce App Runner retirement plan"
```

Expected: all validation commands exit zero before the commit.

---

### Task 3: Remove App Runner and add saved-plan tasks

**Files:**

- Modify: `infra/main.tf:111-291`
- Modify: `infra/variables.tf:19-47`
- Modify: `infra/outputs.tf:6-34`
- Modify: `Taskfile.yaml:757-935`

**Interfaces:**

- Consumes: `scripts/check-app-runner-retirement-plan.sh`
- Produces: `legacy-apprunner-retirement-init`, `legacy-apprunner-retirement-plan`, and `legacy-apprunner-retirement-apply` Task interfaces

- [ ] **Step 1: Remove only App Runner-owned HCL**

Delete the eight managed resource blocks and four output blocks listed in the
spec. Delete variables `container_port`, `auto_deployments_enabled`,
`ecr_image_tag`, `app_runner_cpu`, and `app_runner_memory`. Preserve the two
shared DynamoDB IAM policy blocks and their legacy Lambda attachments.

- [ ] **Step 2: Retire unsafe or unusable task entry points**

Remove `lambda-dev-observation-sample`, `lambda-dev-observation-workflow`,
`lambda-dev-observation-gate`, `deploy`, `redeploy`, and `logs`. Preserve the
three production observation tasks, `_ecr-*`, `deploy-lambda`, and
`redeploy-lambda`.

- [ ] **Step 3: Add the legacy retirement task interfaces**

Add `legacy-apprunner-retirement-init` that runs `_lambda-identity-check`,
rejects OpenTofu CLI, workspace, and data-directory override variables, then
initializes `infra` with `-reconfigure -input=false` using the checked-in
backend block (not `backend.hcl`) and rejects a non-default workspace.

Add `legacy-apprunner-retirement-plan` that runs the identity check, requires
`APPROVED_STATE_LOCK_URI` to equal
`s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock`,
rejects the same OpenTofu override variables and non-default workspace, requires
a new absolute `PLAN_FILE`, runs a full-refresh locked saved
`tofu -chdir=infra plan`, converts it to temporary JSON, invokes the checker,
and prints the no-color plan.

Add `legacy-apprunner-retirement-apply` that runs the same identity and lock
checks plus the override and default-workspace guards. It requires the reviewed
`APPROVED_PLAN_SHA256`, validates it before inspecting fresh saved-plan JSON,
runs the retirement checker, validates the checksum again, and applies only the
unchanged `PLAN_FILE`.

- [ ] **Step 4: Validate configuration and task parsing**

```bash
tofu fmt -check -recursive infra
tofu -chdir=infra init -backend=false -input=false
tofu -chdir=infra validate
task --list >/dev/null
sh tests/app-runner-retirement-plan.sh
```

Expected: all commands exit zero without contacting or mutating live AWS.

- [ ] **Step 5: Commit**

```bash
git add infra/main.tf infra/variables.tf infra/outputs.tf Taskfile.yaml
git commit -m "refactor(infra): retire development App Runner"
```

---

### Task 4: Update the operative retirement documentation

**Files:**

- Modify: `README.md`
- Modify: `DEPLOY-INSTRUCTIONS.md`
- Modify: `docs/deployment/aws-lambda-api-gateway.md`
- Modify: `docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md`
- Modify: `docs/superpowers/plans/2026-08-21-production-lambda-cutover.md`

**Interfaces:**

- Consumes: the superseding design, preserved evidence, and final Task names
- Produces: operator guidance that cannot direct a future worker to recreate App Runner or wait on the waived gate

- [ ] **Step 1: Update current command documentation**

Remove `task logs`, `task deploy`, and `task redeploy` from the README command
table and legacy deployment prose. Document the three
`legacy-apprunner-retirement-*` tasks, lock URI, saved-plan checksum, and
separate custom-domain approval boundary.

- [ ] **Step 2: Replace destructive legacy instructions**

Rewrite `DEPLOY-INSTRUCTIONS.md` so the retained `infra/` root is described as
legacy Lambda/shared data plus a pending App Runner deletion. Remove App Runner
association, deployment, troubleshooting, full-stack `tofu plan -destroy`, and
`tofu destroy` instructions. Document the exact retirement sequence and state
that live custom-domain disassociation is out-of-band and separately approved.

- [ ] **Step 3: Align Lambda platform guidance**

Replace the App Runner rollback section in
`docs/deployment/aws-lambda-api-gateway.md` with the accepted retirement design,
retained legacy Lambda helpers, and saved-plan tasks. Remove legacy App Runner
route-verification guidance.

- [ ] **Step 4: Mark prior constraints as narrowly superseded**

Add a dated note near the top of the 2026-08-21 migration design linking the new
design and stating that only development App Runner seven-day and rollback
requirements are superseded. Production observation and Amplify retirement
requirements remain unchanged.

Revise the production plan's development prerequisite to require the accepted
retirement design, preserved failed evidence, current public health, and
matching release SHA instead of the removed `lambda-dev-observation-gate`.
Keep the production observation task unchanged.

- [ ] **Step 5: Audit stale operational references and commit**

```bash
rg -n "task (deploy|redeploy|logs)|lambda-dev-observation|app_runner_service_(url|arn|id)|tofu (plan -destroy|destroy)|associate-custom-domain" \
  README.md DEPLOY-INSTRUCTIONS.md docs/deployment/aws-lambda-api-gateway.md \
  docs/superpowers/plans/2026-08-21-production-lambda-cutover.md Taskfile.yaml
git diff --check
git add README.md DEPLOY-INSTRUCTIONS.md \
  docs/deployment/aws-lambda-api-gateway.md \
  docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md \
  docs/superpowers/plans/2026-08-21-production-lambda-cutover.md
git commit -m "docs: record App Runner retirement workflow"
```

Expected: no operative instruction remains for the retired interfaces and the
diff has no whitespace errors. Historical descriptions in the old design may
remain because the new superseding note explicitly scopes them.

---

### Task 5: Verify the integrated retirement change

**Files:**

- Verify only; no new production files

**Interfaces:**

- Consumes: Tasks 1-4
- Produces: fresh local evidence that the branch is reviewable without any live mutation

- [ ] **Step 1: Run deployment contract checks**

```bash
sh tests/lambda-observation-window.sh
sh tests/lambda-plan-contract.sh
sh tests/app-runner-retirement-plan.sh
```

- [ ] **Step 2: Validate all OpenTofu roots**

```bash
tofu fmt -check -recursive infra
tofu -chdir=infra init -backend=false -input=false
tofu -chdir=infra validate
go test ./infra/lambda -count=1
```

- [ ] **Step 3: Run repository-required checks**

```bash
task fmt
task lint
task test
task build
```

- [ ] **Step 4: Inspect final scope**

```bash
git diff --check origin/main...HEAD
git status --short --branch
git log --oneline --decorate origin/main..HEAD
```

Expected: no uncommitted source changes, no whitespace errors, only retirement
commits, and no live AWS, Cloudflare, or GitHub mutation.
