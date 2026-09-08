# Development authentication: external setup review

Status: AWS policy installation completed and verified on September 7, 2026.
Temporary administrator installation access was removed. Google consent
configuration is created; the dedicated OAuth client form is prepared and awaits
creation approval. No Cognito stack, session parameter or runtime activation
has been applied.

## Completed prerequisites

The non-root `portfolio-auth-policy-admin` profile uses the dedicated
`PortfolioAuthPolicyAdministrator` Identity Center permission set in account
`180294223248`, region `us-west-2`, with one-hour role sessions. Its current
grants permit only policy inspection/validation and scoped prerequisite reads.
SSO provisioning succeeded, the installed policy matched its prepared document,
and STS verified the expected assumed role after the shared session refresh.
It has no policy-writing, state-content, session-secret or deployment grants.
Private assignment and bootstrap evidence remains outside the repository.

Final live checks on September 7, 2026 established:

- All three [policy candidates](../../infra/lambda/bootstrap/candidates/README.md)
  passed Access Analyzer `IDENTITY_POLICY` validation with zero findings.
- Both the deployer permission-set inline document and its effective role
  policy match the reviewed development management document below. The sole
  managed-policy reference and effective role attachment are the auth setup
  policy. The deployer has no permissions boundary and is provisioned only to
  this account.
- The execution boundary defaults to `v2`, matches the reviewed management
  document and retains baseline `v1`. It is attached only as the boundary of
  `portfolio-lambda-dev-execution`; every production statement is unchanged.
- `/portfolio/setup/PortfolioAuthDevelopmentSetup` defaults to `v1` and matches
  the reviewed auth setup document.
- State bucket `portfolio-tofu-state-180294223248` uses AES256 encryption,
  has versioning enabled and all four public-access blocks enabled. Listing
  `portfolio-lambda-http-api/auth/dev/terraform.tfstate` returned zero keys.
- Cognito returned an empty domain description for
  `portfolio-lambda-dev-mgmt-180294223248`; no current domain was found.
- Google Cloud confirms project `portoflio-dev-508000` and the expected signed-in
  account. The user completed consent configuration; no OAuth client has been
  created. The client form contains the reviewed name and sole redirect below.

The regular `portfolio-deployer` profile passed STS identity, bucket security,
auth-prefix listing and domain lookup checks after installation. Independent
read-back verified all three installed documents and administrator cleanup.
These reads and policy validation do not prove future state writes or Cognito
mutations; verify those during separately approved planning and provisioning.
Recheck live identity and domain availability immediately before their uses.

## Completed AWS policy installation

The non-root administrator profile installed these exact reviewed documents:

| Installed target | Document | SHA-256 |
| --- | --- | --- |
| Managed policy `/portfolio/setup/PortfolioAuthDevelopmentSetup` default `v1`, attached only to `PortfolioDeployer` | `portfolio-deployer-auth-development-setup-candidate.json` | `69eca6d9d1bca4b6d444200fc18e56e20f5c1256f7454c2d4de9e04ccbcf2811` |
| `PortfolioDeployer` inline policy | `portfolio-deployer-development-management-candidate.json` | `68b54a348a07e7a27cda04499c08c79936e0e54a3cbb2887c3bb9c068d9af613` |
| Default `v2` of `/portfolio/boundaries/PortfolioLambdaExecutionBoundary` | `portfolio-lambda-execution-boundary-management-candidate.json` | `31ceb135cf378a4b033f51bba6192d01d74b140f58a409a48621f784499be8dc` |

The 6,606-character deployer document remains in the Identity Center inline slot;
the 2,951-character auth setup document is a separate managed policy. Deployer
provisioning to account `180294223248` reached `SUCCEEDED`. Installed documents,
attachments, production statements and the effective role matched the reviewed
inputs. Existing policy files above the `candidates/` directory remain historical
baselines; do not reinstall them as current inputs.

The account bootstrap administrator temporarily granted and then removed the
installer permissions on `PortfolioAuthPolicyAdministrator`. The installed
read-only document is `auth-policy-admin-read-policy.json`, SHA-256
`6e0647dc4f3f7a1dd2e4ffbcec4fca7579eecdb38439ee22355263965d43a17e`.
Final administrator provisioning reached `SUCCEEDED`; both its permission-set
inline and effective role inline match that document, with no managed-policy
attachments. Read-back establishes the configured removal; IAM caches may take
time to converge.

The successful private installer was `auth-policy-admin-install-corrected.json`
in `/Users/craigjohnson/.config/portfolio/cognito-dev`, SHA-256
`0662421fb0ce21750316fc2cfad8d23cb7e9fabdd888b2ada12c3b2c5f8d7ea9`.
It passed Access Analyzer with zero findings and independent scope review.
The initial `auth-policy-admin-install-candidate.json` remains private history;
it omitted a management-account provisioning dependency.

The temporary grant covered creation of the exact setup managed policy,
write/attach/detach operations on the exact deployer permission set, provisioning
of this account and its status read, and version creation/default selection on
the exact boundary. Provisioning in the management account also required
`iam:PutRolePolicy` on the existing deployer role and `iam:AttachRolePolicy` /
`iam:DetachRolePolicy` on that same role, conditioned on `iam:PolicyARN` equaling
the exact setup policy ARN. It granted no direct changes to its own permission
set or assignment. These dependencies changed no approved installation target
or business-policy document. See the
[AWS management-account permission-set guidance](https://docs.aws.amazon.com/singlesignon/latest/userguide/iam-auth-access-using-id-policies.html#id-policies-example-manage-permission-sets).

Early attempts encountered denied provisioning and policy updates. After adding
the required IAM dependency, allowing propagation and retrying the same scoped
operations, installation and cleanup both passed. Existing verified policy
versions were reused. Future retries must reconcile live state first and allow
for [IAM propagation](https://docs.aws.amazon.com/IAM/latest/UserGuide/troubleshoot.html#troubleshoot_general_eventual-consistency);
a fixed delay is not a guarantee. Private attempt, rollback and final read-back
evidence is retained outside the repository.

The reviewed rollback procedure was to restore the captured deployer inline,
detach only the new setup reference, reprovision the deployer, and restore
boundary `v1` if changed, retaining the setup policy and nondefault boundary
version for inspection. The completed installation retained both boundary
versions and included no deletion.

**Authority limit:** IAM scopes these operations to targets, not exact document
contents. During installation the writer could grant broader powers through the
deployer (including indirect administrator access), choose another managed-policy
reference, or alter the shared boundary's production ceiling. Reviewed hashes,
read-back and removal of the temporary grant constrain the intended operation;
they are not an IAM-enforced content restriction. The explicit installation
approval covered this temporary policy administration. No Cognito apply, secret injection, production
operation or application deployment was performed as part of this installation.

## Google configuration and pending client creation

| Setting | Value |
| --- | --- |
| Project ID | `portoflio-dev-508000` (exact spelling) |
| Consent application name | `Portfolio Development Management` |
| Audience / publishing status | External / Testing |
| Support and developer contact | `craigdevjohnson@gmail.com` |
| Sole test user | `craigdevjohnson@gmail.com` |
| Client type / name | Web application / `portfolio-lambda-dev-mgmt-google` |
| Scopes | `openid`, email, profile |
| Authorized JavaScript origins | None; exchange is server-side through Cognito |
| Redirect URI | `https://portfolio-lambda-dev-mgmt-180294223248.auth.us-west-2.amazoncognito.com/oauth2/idpresponse` |

The user accepted the
[Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy)
and Google confirmed that consent configuration was created. The dedicated web
client form is prepared with the exact name and redirect above; its Create
action has not been submitted. Client creation, the sole test-user entry and
private credential delivery await the requested Google setup approval. Keep the
audience in Testing and configure only the listed identity scopes and test user.
Do not reuse the Calendar client or add Calendar scopes.

Deliver the new credentials directly into a regular mode `0600` file named
`google.json` inside the operator-owned mode `0700` directory above. The private
input must contain exactly `client_id` and `client_secret`; convert a downloaded
Google credential document locally without printing its values. Do not expose
the credential dialog or file contents in chat, source control or command output.

After credential delivery, the next step is a separate
private saved auth plan under `portfolio-deployer`. Its state-lock write,
resource actions and saved-plan apply still require the approvals in the
[provisioning runbook](cognito-google-dev.md). The full implementation remains in
[draft PR 71](https://github.com/CraigDevJohnson/portfolio/pull/71).
