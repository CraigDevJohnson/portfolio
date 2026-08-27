# Reviewed AWS bootstrap policy inputs

This directory tracks the non-secret policy documents reviewed for the initial
replacement Lambda bootstrap. Their presence in Git grants no AWS access and
does not approve any live use. Provisioning, assignment, migration, planning,
applying, tightening, and reprovisioning each require their own current-session
review and approval.

The tracked files are the authoritative current reviewed inputs. Git history
preserves each earlier candidate:

- `portfolio-deployer-development-bootstrap-policy.json` is the current
  development-only IAM Identity Center inline-policy candidate. It contains
  temporary controlled statements and no production or custom-domain
  authority. Its reviewed non-whitespace count must remain within the
  10,240-byte Identity Center quota enforced by `policy_contract_test.go`.
  Its release-repository statement is also exact: it includes the documented
  ECR push contract, including `ecr:BatchGetImage`, plus only the read actions
  used by the artifact plan and immutable-release guard.
- `portfolio-lambda-execution-boundary-policy.json` is the root-owned
  permissions boundary for deterministic Lambda execution roles. The boundary
  is a ceiling, not a grant; its production statements remain dormant until a
  separately authorized production role and runtime policy exist.

The boundary is provisioned at the account-owned path
`/portfolio/boundaries/PortfolioLambdaExecutionBoundary`. A deployer may never
create, edit, remove, or bypass it.

## Temporary statement gates

The development bootstrap policy is intentionally not a standing final policy.
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
- remove `TD`, `TE`, `TF`, `TG`, `TH`, and `D` after the exact development
  SecureString migration and metadata/application verification;
- replace `T8` with exact API-ID child resources after the API ID is captured;
  then remove `T4`, `T5`, `T7`, `T9`, `TA`, `TB`, `TI`, `TK`, and `TL` after the
  direct development service, tags, policies, logging, alarms, and read-backs
  pass their gates.

Every tightened or post-bootstrap file revision is a new policy artifact.
Recompute its SHA-256 and byte counts, repeat static and live IAM Access
Analyzer review, obtain approval, update the permission set, reprovision it,
wait for `SUCCEEDED`, and verify effective access before using it.
Never restore this initial document after a temporary statement has been
removed. Custom-domain work requires its own just-in-time development-only
candidate and approval.

Do not store an IAM Identity Center user ID, ownership or MFA evidence, live
assignment coordinates, decrypted parameter value, saved plan, session data,
or approval record here. Keep those details in the private approval evidence.
See [the replacement deployment instructions](../../../DEPLOY-INSTRUCTIONS.md)
for the surrounding approval-gated workflow.
