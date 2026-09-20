# Production Lambda promotion

Production release preparation is in progress. Production application is hard-disabled
in the apply script, in addition to having no workflow job. Environment variables
cannot enable it. A successful `production-plan` job is not a production
deployment and does not authorize any live mutation.

## Authority boundaries

- `portfolio-production-planner-ci`, trusted only by `production-plan`, validates
  the manifest and creates evidence. It cannot write production state or services.
- `portfolio-production-deployer-ci`, trusted only by `production`, is defined but
  must not be provisioned or used until the readiness and activation gates below.
  It can write only the exact production state object and release the existing
  production Lambda function and `live` alias.
- `portfolio-deployer` remains the guarded local SSO path for separately approved
  bootstrap or recovery work. It is not a substitute for protected CI promotion.
- Release automation must not change IAM/OIDC resources, state backends, runtime
  parameters, DNS/Cloudflare, API Gateway domains, ACM, App Runner, Amplify, or
  unrelated environments.

## Readiness gate

Before activation, retain sanitized evidence proving all of the following without
recording parameter or secret values:

1. The production OpenTofu root and state already own the function, `live` alias,
   API Gateway, custom domains, certificate, logs, tables, and five alarms.
2. Required runtime parameters exist, are encrypted as approved, and are readable
   by the production execution role.
3. The certificate covers `craigdevjohnson.com` and `www.craigdevjohnson.com` and
   both API Gateway mappings pass direct-origin HTTPS probes.
4. OAuth callback and logout allowlists use the exact production HTTPS URLs, and
   secure session-cookie behavior has been tested at the production origin.
5. The alarm actions are configured and all five alarms are healthy.
6. The current public origin and exact DNS/Cloudflare rollback coordinates have
   been recorded, and that rollback origin has been tested.
7. The public cutover and rollback procedures have been reviewed. DNS/Cloudflare
   changes remain manual, exact-record operations outside release automation.

## Separate activation gate

The maintainer must separately authorize each operation below in the session where
it occurs. Issue assignment, this document, a code review, or a plan-only run is
not authorization.

1. Create and review an isolated CI-role plan. Confirm it adds only
   `portfolio-production-deployer-ci` and its exact inline policy, then separately
   authorize applying the checksum- and provenance-bound plan.
2. Verify all four deterministic role ARNs with `task lambda-ci-roles-verify` and
   test that the production deployer can assume only from the exact `production`
   Environment subject. Test representative denied builder, development,
   bootstrap, DNS, IAM, SSM-write, and unrelated-state actions.
3. Create the `production` GitHub Environment before any workflow references it.
   Require the designated reviewer, protected branches, and no administrator
   bypass. Set only `AWS_PRODUCTION_DEPLOYER_ROLE_ARN` to the verified ARN.
4. Review the live Environment through the GitHub API. Stop if its reviewer,
   branch policy, bypass setting, or role variable differs from this contract.
5. Merge a separate activation PR that adds the production apply/verify job. That
   job must use `environment: production`, `deployments: write`, `id-token: write`,
   the non-cancelling `lambda-production` concurrency group, and the evidence from
   the same successful planning run. PR-head jobs receive no AWS credentials.

## Promotion and failure handling

Create a new manifest-only promotion PR after revalidating the current development
deployment, successful latest status, source SHA, ECR digest, completed scan, live
development alias/version, and verification evidence. Do not reuse an old manifest
merely because it once passed, rebuild the image, or use a mutable tag.

The plan evidence includes `promotion.json`, `scan.json`, plan binary/JSON/text,
checksum, policy result, durable prior verified deployment evidence, and
`release-identity.json`. A bundle with `apply_authorized: false` is rehearsal evidence
only. Application must use `task lambda-ci-apply-production`; it binds the current
manifest and workflow identity, verifies the checksum and plan policy, checks
current `main`, checks the durable production coordinate and current alias, and repeats both mutable-state checks
immediately before applying the saved plan.

Before application, create the protected GitHub production deployment through
`task lambda-ci-record-production`. On success, verify and retain the exact alias,
version, digest, `/healthz` revision, direct-origin route probes, public apex and
`www` routes, required pages/assets, OAuth and cookies, and all alarm samples.
Only then record a successful deployment and public-cutover verdict.

On any apply or verification failure, preserve the evidence, record failure on the
same GitHub deployment, and stop. A production rollback plan may move only the
`live` alias to the recorded prior verified version; it must pass policy and
checksum validation. Applying rollback requires a new operator authorization and
must never be automatic.

## Remaining live work before activation

This preparation is not activation. Live AWS state, runtime parameters, certificate
and OAuth configuration, both public routes, alarm actions, the protected GitHub
Environment, and any first-cutover bootstrap coordinate remain unverified. The
later activation change must review those facts and must not merely remove the hard
stop.

The active planner retains its existing authority and reads the digest-bound scan
summary through its existing `ecr:DescribeImages` permission. Missing, incomplete,
or critical scan evidence fails planning closed. No role policy has been applied by
this repository change.
