# Reviewed AWS bootstrap policy inputs

This directory tracks the non-secret policy documents reviewed for the initial
replacement Lambda bootstrap. Their presence in Git grants no AWS access and
does not approve any live use. Provisioning, assignment, migration, planning,
applying, tightening, and reprovisioning each require their own current-session
review and approval.

The tracked files are the authoritative initial inputs:

- `portfolio-deployer-development-bootstrap-policy.json` is the
  development-only IAM Identity Center inline policy. It contains temporary
  controlled statements and no production or custom-domain authority. Its
  reviewed non-whitespace count must remain within the 10,240-byte Identity
  Center quota enforced by `policy_contract_test.go`.
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

- remove `TJ` after the exact legacy-state metadata check and before remote
  OpenTofu initialization;
- remove `T1` after state-bucket versioning is verified `Enabled`;
- remove `T2` and `T3` after the artifact apply, read-back, and empty
  convergence plan;
- remove `TD`, `TE`, `TF`, `TG`, `TH`, and `D` after the exact development
  SecureString migration and metadata/application verification;
- replace `T8` with exact API-ID child resources after the API ID is captured;
  then remove `T4`, `T5`, `T7`, `T9`, `TA`, `TB`, `TI`, and `TK` after the
  direct development service, tags, policies, logging, alarms, and read-backs
  pass their gates.

Every tightened or post-bootstrap candidate is a new policy artifact. Recompute
its SHA-256 and byte counts, repeat static and live IAM Access Analyzer review,
obtain approval, update the permission set, reprovision it, wait for
`SUCCEEDED`, and verify effective access before using it.
Never restore this initial document after a temporary statement has been
removed. Custom-domain work requires its own just-in-time development-only
candidate and approval.

Do not store an IAM Identity Center user ID, ownership or MFA evidence, live
assignment coordinates, decrypted parameter value, saved plan, session data,
or approval record here. Keep those details in the private approval evidence.
See [the replacement deployment instructions](../../../DEPLOY-INSTRUCTIONS.md)
for the surrounding approval-gated workflow.
