# Development Cognito and Google provisioning

The independent `infra/lambda/auth/dev` root owns five Cognito resources. Its state,
saved plans, provider logs, and plan JSON can contain the Google client secret.
Never put those artifacts in Git, chat, CI artifacts, or release workflow logs.
Auth provisioning is an operator task, separate from Lambda release jobs.

## Live prerequisites and approvals

The pool is **not provisioned** in the workloads account. The dated records
below describe the earlier management-account setup; they are history.

Use the workloads account (`AWS_ACCOUNT_ID` in `Taskfile.yaml`), region
`us-west-2`, and Craig's `workloads-admin` SSO profile (role
`AWSReservedSSO_WorkloadsAdmin_*`). The cognito tasks set that profile, and the
wrapper refuses any other identity or account. Never use root credentials.

The backend is `portfolio-tofu-state-<AWS_ACCOUNT_ID>`, which the account root
manages. The managed-login domain prefix defaults to
`portfolio-lambda-dev-mgmt-<AWS_ACCOUNT_ID>`; review its availability before
planning. The wrappers enforce backend encryption and locking configuration.

Craig selected Google Cloud project `portoflio-dev-508000` (spelling intentional).
The project and signed-in account are verified, and the user completed consent
configuration. The dedicated web OAuth client `portfolio-lambda-dev-mgmt-google`
is created, with its credentials delivered to the private operator input. The
sole test-user entry is `craigdevjohnson@gmail.com`; only the three basic identity
scopes are saved, with no sensitive or restricted scopes. External/Testing and
the application/support/contact fields were verified. No JavaScript origins
are registered. The client's Google redirect URI is the `google_redirect_uri`
output, `https://portfolio-lambda-dev-mgmt-<AWS_ACCOUNT_ID>.auth.us-west-2.amazoncognito.com/oauth2/idpresponse`.
The redirect URI registered for the earlier management-account pool must be
replaced with the workloads one before a workloads apply.

Google exempts basic identity scopes from the Testing test-user allowlist, so
application access must still be enforced by the verified-email allowlist.
See [Google's app-state guidance](https://developers.google.com/identity/protocols/oauth2/production-readiness/overview).

Callback registration is `https://dev.craigdevjohnson.com/callback`; logout is
`https://dev.craigdevjohnson.com/login`. The callback path must be exactly
`/callback`, and the logout return path must be exactly `/login`. Visiting
`/login` renders a signed-out page; only its explicit
sign-in button starts OAuth through `POST /login`. These wrappers deliberately admit only
the reviewed domain and no loopback callback. A domain or callback change needs
an updated reviewed checker contract.

## Private inputs and plan review

The private `google.json` input was delivered and validated for the earlier
management-account review ([initial plan review](2026-09-07-cognito-initial-plan-review.md)).
No auth-state write, Cognito apply, session-key injection or runtime activation
has occurred in either account.

Create an operator-owned directory outside the checkout with mode `0700`. Set
`COGNITO_PRIVATE_DIR` to its absolute path. Have the credential delivery channel
write a regular mode `0600` JSON file there with exactly `client_id` and
`client_secret` string keys. Never paste its contents into a terminal command,
chat, tfvars, or logs. Set `GOOGLE_OAUTH_CREDENTIALS_FILE` to that file's absolute
path. Symlinked paths, public-readable files/directories, malformed JSON and
existing plan paths are rejected. Parent directories must not be symlinks;
on macOS use the canonical `/private/...` path instead of `/tmp/...`.

Run tasks from this checkout using environment variables (not Task CLI variable
assignments):

```sh
export COGNITO_PRIVATE_DIR=/absolute/private/operator-directory
export GOOGLE_OAUTH_CREDENTIALS_FILE=/absolute/private/operator-directory/google.json
export PLAN_FILE=/absolute/private/operator-directory/auth.tfplan
export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-<AWS_ACCOUNT_ID>/portfolio-lambda-http-api/auth/dev/terraform.tfstate.tflock
task cognito-dev-init
task cognito-dev-plan
```

The lock acknowledgement is not approval to create a plan or apply resources.
Each task initializes fresh private provider/backend data with a read-only
provider lockfile. No default CLI configuration or environment overrides are
accepted. Google variables are passed only to the auth plan subprocess. The
wrapper prints a fixed resource/action summary, exact backend/account identity,
and SHA-256 checksums. Raw stdout/stderr, plan JSON and provider data remain in
new mode `0700` directories beneath `COGNITO_PRIVATE_DIR`. Failures print only
a generic diagnostic; inspect those private artifacts locally when needed.

Review the five create/update/no-op actions, exact names and URLs, Google scopes
and attribute mapping, public code-flow client, backend and private state
handling. Deletions, replacements, extra resources, and changed backend settings
are rejected. Capture both printed checksums as the reviewed approval record.
The plan's adjacent `.provenance.json` binds its checksum to the auth backend.

## Separately approved saved apply

Stop for explicit live-apply approval. Once approved, set
`APPROVED_PLAN_SHA256` and `APPROVED_PROVENANCE_SHA256` to the two exact reviewed
checksums, keep the same absolute `PLAN_FILE`, and run:

```sh
task cognito-dev-apply
```

Apply snapshots the saved plan and provenance using guarded reads, verifies both
checksums before AWS access, repeats identity/backend checks, and rechecks the
snapshot's resource contract. Only that snapshot is applied, with a five-minute
lock timeout. No fresh apply plan is generated. Raw apply output stays private.
Verify the live resources, then run `cognito-dev-plan` again with a fresh
`PLAN_FILE` and confirm all five actions converge to no-op. Do not print all
outputs or raw state to perform this verification.

## Public runtime handoff

After verified provisioning:

```sh
task --silent cognito-dev-export > /absolute/path/management-runtime.tfvars.json
```

This calls only `tofu output -json management_runtime`, validates the nine exact
public fields, and emits `{"management": {...}}` for the development runtime
root. It never exports all outputs, provider details or state. Verify export
success before using the file; a failed redirected command may leave an empty
file. Treat the allowlisted email as personal data when sharing this public
configuration.

Runtime deployment remains a separate reviewed Lambda plan. Export the object
for the development plan with
`export TF_VAR_management="$(jq -c .management /absolute/path/management-runtime.tfvars.json)"`,
then run `task lambda-dev-plan` and `task lambda-dev-apply`. After that apply,
set the GitHub **development** environment variable `MANAGEMENT_RUNTIME_JSON`
to the same bare `.management` object so CI releases keep it; the release job
validates it and forwards it as `TF_VAR_management`. Omit it or use `null`
while the portal is disabled. A CI release cannot enable the portal, because
its plan may change only the Lambda image and `live` alias. Keep image and
unrelated Lambda changes out of the auth plan.
Provision the `/portfolio/lambda/dev/MGMT_SESSION_KEY` SecureString value through
the separately approved secret channel before enabling the runtime. The auth
root does not create or read that secret. Do not add auth-state access or Google
credentials to automatic release workflows.

## Offline checks

`task cognito-dev-tooling-test` uses synthetic sentinels and mocked subprocesses;
it does not read credentials or call AWS. Private run directories are retained
for local audit; remove them only after the review/retention requirement ends.


## PR #71 review fixes

[The review-fix record](2026-09-07-cognito-pr71-review-fixes.md) covers optional
session-key failure isolation, the signed-out landing page, exact callback
validation and tag-aware instance controls. The management key must still be
provisioned before portal activation; if it becomes unavailable during a cold
start, only the portal is disabled. Untagged instances remain visible for reads
with start/stop/restart disabled. These source fixes do not apply infrastructure
or change the installed IAM documents.
