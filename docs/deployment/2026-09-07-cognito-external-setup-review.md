# Development authentication: external setup review

Status: prepared for separate authorization. No policy candidate, Google OAuth
client, Cognito stack, session parameter or runtime activation has been applied.

## Completed prerequisites

The non-root `portfolio-auth-policy-admin` profile uses the dedicated
`PortfolioAuthPolicyAdministrator` Identity Center permission set in account
`180294223248`, region `us-west-2`, with one-hour role sessions. Its current
grants permit only policy inspection/validation and scoped prerequisite reads.
SSO provisioning succeeded, the installed policy matched its prepared document,
and STS verified the expected assumed role after the shared session refresh.
It has no policy-writing, state-content, session-secret or deployment grants.
Private assignment and bootstrap evidence remains outside the repository.

Live read-only checks on September 7, 2026 established:

- All three [policy candidates](../../infra/lambda/bootstrap/candidates/README.md)
  passed Access Analyzer `IDENTITY_POLICY` validation with zero findings.
- Both the deployer permission-set inline document and its effective role
  policy match the tracked approved bootstrap. It has no attached managed
  policies or permissions boundary and is provisioned only to this account.
- The execution boundary matches its tracked baseline, has only version `v1`,
  and is attached only as the boundary of `portfolio-lambda-dev-execution`.
  The candidate preserves every production statement.
- `/portfolio/setup/PortfolioAuthDevelopmentSetup` does not exist yet.
- State bucket `portfolio-tofu-state-180294223248` uses AES256 encryption,
  has versioning enabled and all four public-access blocks enabled. Listing
  `portfolio-lambda-http-api/auth/dev/terraform.tfstate` returned zero keys.
- Cognito returned an empty domain description for
  `portfolio-lambda-dev-mgmt-180294223248`; no current domain was found.
- Google Cloud confirms project `portoflio-dev-508000` and the expected signed-in
  account. It has no OAuth client or configured consent screen.

Validation checks policy syntax and supported constructs. Effective provisioning
permissions must be verified under `portfolio-deployer` after installation.
Recheck live baselines and domain availability immediately before their uses.

## Proposed AWS policy installation

Use the new administrator profile for the following exact document installations:

| Target | Candidate | SHA-256 |
| --- | --- | --- |
| New managed policy `/portfolio/setup/PortfolioAuthDevelopmentSetup`, attached only to `PortfolioDeployer` | `portfolio-deployer-auth-development-setup-candidate.json` | `69eca6d9d1bca4b6d444200fc18e56e20f5c1256f7454c2d4de9e04ccbcf2811` |
| `PortfolioDeployer` inline policy | `portfolio-deployer-development-management-candidate.json` | `68b54a348a07e7a27cda04499c08c79936e0e54a3cbb2887c3bb9c068d9af613` |
| New default version of `/portfolio/boundaries/PortfolioLambdaExecutionBoundary` | `portfolio-lambda-execution-boundary-management-candidate.json` | `31ceb135cf378a4b033f51bba6192d01d74b140f58a409a48621f784499be8dc` |

Keep the 6,606-character deployer document in the Identity Center inline slot;
the 2,951-character auth setup document is a separate managed policy. Reprovision
only account `180294223248` and wait for `SUCCEEDED`. Compare installed documents,
attachments, production statements and the effective deployer role against the
reviewed inputs. Then verify prerequisite reads as `portfolio-deployer`.

This requires a temporary installation grant on the administrator permission set,
installed and later removed by the account bootstrap administrator. The complete
private candidate is `auth-policy-admin-install-candidate.json` in the operator
directory `/Users/craigjohnson/.config/portfolio/cognito-dev`, SHA-256
`bd700e75681ffc7cb2405758d03992a5bd3a9efdf51cfb65270dfc137586d044`.
It passed Access Analyzer with zero findings. The current read-only document is
`auth-policy-admin-read-policy.json`, SHA-256
`6e0647dc4f3f7a1dd2e4ffbcec4fca7579eecdb38439ee22355263965d43a17e`.

The temporary grant adds only creation of the exact setup managed policy,
write/attach/detach operations on the exact deployer permission set, provisioning
of this account and its status read, and version creation/default selection on
the exact boundary. It grants no direct changes to its own permission set or
assignment. Restore the read-only document and reprovision the administrator
permission set after successful verification, or after rollback on failure.
Verify the restored effective policy before ending the operation.

If installation verification fails, restore the captured deployer inline,
detach only the new setup reference, reprovision the deployer, and restore
boundary `v1` if changed. Keep the unattached setup policy and nondefault boundary
version for inspection; this proposal includes no deletion.

**Authority limit:** IAM scopes these operations to targets, not exact document
contents. During installation the writer could grant broader powers through the
deployer (including indirect administrator access), choose another managed-policy
reference, or alter the shared boundary's production ceiling. Reviewed hashes,
read-back and removal of the temporary grant constrain the intended operation;
they are not an IAM-enforced content restriction. Approval must cover this
temporary policy administration. No Cognito apply, secret injection, production
operation or application deployment is part of this installation proposal.

## Proposed Google configuration

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

The consent wizard is prepared through its final agreement step in Chrome; it
has not been submitted. Completing it requires acceptance of the
[Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy).
Create the dedicated client after consent configuration, keep the audience in
Testing, and add only the listed test user. Do not reuse the Calendar client or
add Calendar scopes.

Deliver the new credentials directly into a regular mode `0600` file named
`google.json` inside the operator-owned mode `0700` directory above. The private
input must contain exactly `client_id` and `client_secret`; convert a downloaded
Google credential document locally without printing its values. Do not expose
the credential dialog or file contents in chat, source control or command output.

After policy installation and credential delivery, the next step is a separate
private saved auth plan under `portfolio-deployer`. Its state-lock write,
resource actions and saved-plan apply still require the approvals in the
[provisioning runbook](cognito-google-dev.md). The full implementation remains in
[draft PR 71](https://github.com/CraigDevJohnson/portfolio/pull/71).
