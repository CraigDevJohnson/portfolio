# Initial development Cognito plan review

Status: prerequisites verified; initialization and saved-plan execution await
approval. No live auth plan has been created or applied.

## Verified prerequisites

The approved Google web client `portfolio-lambda-dev-mgmt-google` exists in
`portoflio-dev-508000`, with only the reviewed Cognito redirect and no JavaScript
origins. Its private credentials are delivered and validated. Google is in
External/Testing mode, with Craig as the sole configured test user and only
OpenID, email and profile scopes. The application email allowlist remains the
portal authorization gate.

The three approved AWS policies are installed. Fresh `portfolio-deployer` reads
verified account/role identity, AES256 backend encryption, enabled versioning,
all four public-access blocks, an empty auth-state prefix and an empty proposed
Cognito domain description. Those reads do not establish successful writes or
resource creation. Source and infrastructure CI passed at `87668807`; this
review adds documentation only.

## Exact proposed operation

Run `task cognito-dev-init`, then `task cognito-dev-plan` from the isolated
`codex/cognito-google-dev-setup` checkout using these public coordinates:

| Setting | Value |
| --- | --- |
| AWS profile | `portfolio-deployer` |
| Account / region | `180294223248` / `us-west-2` |
| Root | `infra/lambda/auth/dev` |
| Workspace | `default` |
| Backend bucket | `portfolio-tofu-state-180294223248` |
| State key | `portfolio-lambda-http-api/auth/dev/terraform.tfstate` |
| Native lock object | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/auth/dev/terraform.tfstate.tflock` |
| Private directory | `/Users/craigjohnson/.config/portfolio/cognito-dev` |
| Credential input | `google.json` in that private directory |
| New saved plan | `auth-initial-20260907.tfplan` in that private directory |
| Plan provenance | `auth-initial-20260907.tfplan.provenance.json` beside the plan |

The directory is operator-owned mode `0700`; the regular credential file is
mode `0600` and contains exactly `client_id` and `client_secret`. Both proposed
plan paths are absent. The wrapper rejects alternate AWS/Tofu configuration,
implicit tfvars, symlinks, public-readable paths and existing plan files.

This operation initializes private provider/backend data, reads the auth root's
live state, and creates a private saved plan. It requires permission to create
and remove the exact S3 native lock object above. The plan and raw provider
output may contain the Google client secret and stay private. The public result
contains only the checked resource/action summary and plan/provenance SHA-256
checksums. No Cognito resource apply is included in this operation.

## Expected review result

The checker must admit only these five resources; actual actions have not yet
been generated:

| Resource | Expected initial configuration |
| --- | --- |
| User pool | `portfolio-lambda-dev-mgmt`, Essentials tier, no self-service signup |
| Google provider | Only OpenID/email/profile, reviewed attribute mapping and private client credentials |
| Public app client | `portfolio-lambda-dev-mgmt-web`, code flow, Google identity provider, no client secret |
| Cognito domain | `portfolio-lambda-dev-mgmt-180294223248`, managed login version 2 |
| Managed login branding | Cognito-provided values for this pool and client |

The only callback is `https://dev.craigdevjohnson.com/callback`; logout is
`https://dev.craigdevjohnson.com/login`. The Google redirect is
`https://portfolio-lambda-dev-mgmt-180294223248.auth.us-west-2.amazoncognito.com/oauth2/idpresponse`.
Local callback is disabled. Deletions, replacements, extra resources or changed
backend/account/region are rejected.

After the private plan passes, present its five actions and both checksums for
separate saved-apply approval. Session-secret injection, runtime activation,
source release and EC2 changes remain later reviewed operations. See the
[provisioning runbook](cognito-google-dev.md) and
[completed external setup record](2026-09-07-cognito-external-setup-review.md).
