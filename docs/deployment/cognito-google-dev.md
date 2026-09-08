# Development Cognito and Google provisioning

The independent `infra/lambda/auth/dev` root owns five Cognito resources. Its state,
saved plans, provider logs, and plan JSON can contain the Google client secret.
Never put those artifacts in Git, chat, CI artifacts, or release workflow logs.
Auth provisioning is an operator task, separate from Lambda release jobs.

## Live prerequisites and approvals

Use account `180294223248`, region `us-west-2`, and the `portfolio-deployer`
SSO profile (role `AWSReservedSSO_PortfolioDeployer_*`). Refresh that SSO session
before a live operation. The separate `portfolio-auth-policy-admin` SSO profile
now provides policy inspection/validation and prerequisite metadata reads with
one-hour sessions. It has no installation or provisioning authority. Its live
checks passed on September 7, 2026. The three reviewed policies are now installed;
all documents and effective deployer permissions matched their reviewed inputs,
and temporary administrator installation access was removed. See the
[external setup record](2026-09-07-cognito-external-setup-review.md) for exact
documents, versions and verification. Auth planning/applying still uses only
`portfolio-deployer`; do not substitute administrator or root credentials.

Verify encrypted, access-controlled backend bucket
`portfolio-tofu-state-180294223248`, versioning, public-access protection, effective
auth-prefix state read/write/lock permissions and required Cognito permissions.
Review domain availability for `portfolio-lambda-dev-mgmt-180294223248` before
planning. The wrappers enforce backend encryption and locking configuration;
they do not establish that the bucket's live security controls or permissions
are sufficient.

Craig selected Google Cloud project `portoflio-dev-508000` (spelling intentional).
The project and signed-in account are verified, and the user completed consent
configuration. The dedicated web OAuth client `portfolio-lambda-dev-mgmt-google`
is created, with its credentials delivered to the private operator input. The
sole test-user entry is `craigdevjohnson@gmail.com`; only the three basic identity
scopes are saved, with no sensitive or restricted scopes. External/Testing and
the application/support/contact fields were verified. No JavaScript origins
are registered. The client uses only this Google redirect URI:

`https://portfolio-lambda-dev-mgmt-180294223248.auth.us-west-2.amazoncognito.com/oauth2/idpresponse`

Google exempts basic identity scopes from the Testing test-user allowlist, so
application access must still be enforced by the verified-email allowlist.
See [Google's app-state guidance](https://developers.google.com/identity/protocols/oauth2/production-readiness/overview).

Callback registration is `https://dev.craigdevjohnson.com/callback`; logout is
`https://dev.craigdevjohnson.com/login`. These wrappers deliberately admit only
the reviewed domain and no loopback callback. A domain or callback change needs
an updated reviewed checker contract.

## Private inputs and plan review

The private `google.json` input has been delivered and validated; its raw download
is retained privately. Initialization, planning and the exact state-lock write
await separate approval of the
[initial plan review](2026-09-07-cognito-initial-plan-review.md). No auth-state
write, Cognito apply, session-key injection or runtime activation has occurred.

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
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
export COGNITO_PRIVATE_DIR=/absolute/private/operator-directory
export GOOGLE_OAUTH_CREDENTIALS_FILE=/absolute/private/operator-directory/google.json
export PLAN_FILE=/absolute/private/operator-directory/auth.tfplan
export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/auth/dev/terraform.tfstate.tflock
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

Runtime deployment remains a separate reviewed Lambda plan. Supply the exported
`management` object as the development root's variable and as
`EXPECTED_MANAGEMENT_JSON` to its checker, using Task4's runtime integration
instructions. For those tasks, set `EXPECTED_MANAGEMENT_JSON` to
`$(jq -c .management /absolute/path/management-runtime.tfvars.json)` in the
operator shell. The runtime wrappers forward this reviewed public value to
`TF_VAR_management`; use the same reviewed value for rollout and rollback.
Automatic workflow configuration must explicitly supply the public object;
its default remains `null`. After separately reviewing and applying runtime
enablement, set the GitHub **development** environment public variable
`MANAGEMENT_RUNTIME_JSON` to the bare `.management` object. The development
release step forwards this value as `EXPECTED_MANAGEMENT_JSON`; omit it or
use `null` while the portal is disabled. Ordinary rollouts cannot enable the
portal because their runtime environment diff remains restricted. Keep image and unrelated Lambda changes out of the auth plan.
Provision the `/portfolio/lambda/dev/MGMT_SESSION_KEY` SecureString value through
the separately approved secret channel before enabling the runtime. The auth
root does not create or read that secret. Do not add auth-state access or Google
credentials to automatic release workflows.

## Offline checks

`task cognito-dev-tooling-test` uses synthetic sentinels and mocked subprocesses;
it does not read credentials or call AWS. Private run directories are retained
for local audit; remove them only after the review/retention requirement ends.
