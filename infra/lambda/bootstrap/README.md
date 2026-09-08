# Reviewed AWS bootstrap policy inputs

This directory tracks the non-secret policy documents reviewed for the initial
replacement Lambda bootstrap. Their presence in Git grants no AWS access and
does not approve any live use. Provisioning, assignment, migration, planning,
applying, tightening, and reprovisioning each require their own current-session
review and approval.

The current installed inputs are the three unchanged documents in
[candidates/](candidates/README.md), despite their retained candidate filenames.
Installation and independent read-back completed on September 7, 2026:

- `portfolio-deployer-development-management-candidate.json` is the installed
  `PortfolioDeployer` Identity Center inline policy and effective role inline.
- `portfolio-deployer-auth-development-setup-candidate.json` is version `v1` of
  `/portfolio/setup/PortfolioAuthDevelopmentSetup`, attached only to the human
  deployer permission set and role.
- `portfolio-lambda-execution-boundary-management-candidate.json` is default
  version `v2` of the execution boundary. Its production statements remain
  unchanged; its only current attachment is the development execution role.

The original `portfolio-deployer-development-bootstrap-policy.json` and
`portfolio-lambda-execution-boundary-policy.json` are retained pre-Cognito
baselines, not current installation defaults. Do not reinstall them as the
current policy set. The management inline preserves the bootstrap grants and
adds session injection; the separate setup policy grants development auth-state
and Cognito setup access. The boundary remains a ceiling, not a grant. Keep the
6,606-character management policy in the Identity Center inline slot and the
2,951-character setup policy separately managed.

The temporary installation authority on `portfolio-auth-policy-admin` was
removed after verification; its SSO and effective role policies were restored
to the reviewed read-only document. No deployment or Cognito apply follows
merely from installing these policies. See the
[external setup record](../../../docs/deployment/2026-09-07-cognito-external-setup-review.md).

The boundary is provisioned at the account-owned path
`/portfolio/boundaries/PortfolioLambdaExecutionBoundary`. A deployer may never
create, edit, remove, or bypass it.

## Temporary statement gates

The following gates document tightening of the retained pre-Cognito bootstrap
baseline. Preserve their removals in every later installed policy revision.
Remove or replace its controlled SIDs as soon as each reviewed purpose is
complete:

- `TJ` was removed after the exact legacy-state metadata check; never restore
  it. The replacement backend list grant is limited to `env:/` and the exact
  artifacts and development state keys. Do not gate that statement with
  `s3:max-keys`: the missing-object `HeadObject` authorization check requires
  `s3:ListBucket` but does not supply that request field;
- `T1` was removed after state-bucket versioning was verified `Enabled`; never
  restore it;
- `T2` and `T3` were removed after the artifact apply, live read-back, and
  empty convergence plan; the permission set was reprovisioned and its
  effective role verified, so never restore them;
- `TD`, `TE`, `TF`, `TG`, `TH`, `D`, and `P` were removed after the exact
  development SecureString migration and metadata/application verification;
- `T7` and `TL` were removed after API `048o6alxh8` was created and captured;
  `T8` was replaced by `RAPI`, which grants only `apigateway:GET` on that exact
  API and its subresources;
- direct-stack gates `T4`, `T5`, `TI`, `TK`, `TN`, `T9`, `TM`, `TA`, and `TB`
  were removed after the development service, tags, policies, logging, alarms,
  and read-backs passed their gates; and
- `TCRequest` and `TCTags` were removed after the public, DNS-validated,
  AWS-managed certificate was issued and captured. `TCRead` now grants only
  `DescribeCertificate` and `ListTagsForCertificate` on that exact certificate
  ARN, with the reviewed development tags and region still required. No
  statement grants certificate mutation or deletion; and
- `DomainCreate`, `DomainCreateTagAuthorization`, and `ApiMappingCreate` were
  removed immediately after `dev.craigdevjohnson.com` reached `AVAILABLE` and
  target `d-vgozrre45c.execute-api.us-west-2.amazonaws.com` and root mapping
  `u1ettj` were captured. During activation, the API Gateway V2 service
  authorization table does not support `aws:RequestTag` or `aws:TagKeys` on the
  `DomainNames` collection resource used by `DomainCreate`; live IAM rejected
  the exact provider request when those unsupported conditions were attached.
  The tagged create required the exact four development tag conditions on the
  separate, exact encoded `DomainCreateTagAuthorization` resource. That resource
  encodes the public
  domain ARN `/domainnames/dev.craigdevjohnson.com`; the API's HTTP `/v2` prefix
  is not part of the resource ARN. The live `CreateDomainName` request also
  required `apigateway:PUT` on that exact tag resource, so the temporary
  statement granted only `POST` and `PUT` there; no domain or mapping resource
  received `PUT`. `DomainCreate` constrained every supplied
  endpoint and security-policy value to Regional and TLS 1.2 and required
  `us-west-2`. Do not add `Null` checks for the optional mTLS request keys: the
  provider emitted `mutualTlsAuthentication: null`, and live IAM rejected the
  otherwise exact request while those checks were attached. No candidate granted
  `AddCertificateToDomain`, and the checked HCL and saved plan contained no mTLS
  configuration. Because the collection resource also has no
  request-domain condition key, that short-lived statement could authorize an
  untagged domain create elsewhere in the region. The exact HCL hostname and
  certificate, fresh checked saved plan, checksum-bound apply, and immediate
  post-create removal were mandatory compensating controls. The provider request
  and saved plan contained both endpoint and security-policy fields. Do not
  add `Null=false` presence checks for those two documented
  `ArrayOfString` keys: live IAM and the policy simulator both rejected the
  otherwise exact provider request when those checks were present. The current
  `DomainRead` can read only `dev.craigdevjohnson.com`, and `ApiMappingRead` can
  read only captured mapping `u1ettj`. The API Gateway IAM model could not
  constrain the mapping request
  body to API `048o6alxh8` and stage `$default`, so the checked HCL, environment
  contract, fresh saved-plan review, and checksum-bound apply are mandatory
  compensating controls. No statement now grants domain or mapping create,
  update, tag, policy, certificate-attachment, or delete actions.

Every tightened or post-bootstrap file revision is a new policy artifact.
Recompute its SHA-256 and byte counts, repeat static and live IAM Access
Analyzer review, obtain approval, update the permission set, reprovision it,
wait for `SUCCEEDED`, and verify effective access before using it.
Never restore this initial document after a temporary statement has been
removed. Each later custom-domain phase requires its own just-in-time,
development-only candidate and approval.

Do not store an IAM Identity Center user ID, ownership or MFA evidence, live
assignment coordinates, decrypted parameter value, saved plan, session data,
or approval record here. Keep those details in the private approval evidence.
See [the replacement deployment instructions](../../../DEPLOY-INSTRUCTIONS.md)
for the surrounding approval-gated workflow.
