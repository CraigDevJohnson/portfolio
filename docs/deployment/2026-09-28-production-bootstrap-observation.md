# Read-only production bootstrap observation

A production apply can succeed before acceptance fails. A later promotion must
then compare its saved plan with the newly observed live alias, rather than
reuse bootstrap metadata from before that apply. An applied version is not a
verified production predecessor.

The **Production bootstrap observation** workflow runs only by manual dispatch
from current protected main, by Craig's GitHub identity. It uses the existing
protected `production-plan` environment and exact production planner role in
account `180294223248`, `us-west-2`. Shared non-cancelling `lambda-production`
concurrency separates the observation from deployment and acceptance work.

The workflow reads the `portfolio-lambda-prod` function and `live` alias. It
rejects mutable versions or image tags, other functions/repositories, weighted
aliases, and alias movement during the observation. AWS CLI queries retain only
the required metadata, excluding function environment variables and code URLs.
No new IAM permission, AWS mutation, repository-variable update or deployment
status write is included.

A successful run uploads the immutable artifact
`production-bootstrap-observation-<run-id>-1`, containing:

- `bootstrap.json`: the existing five-field planner bootstrap input.
- `provenance.json`: the workflow/run/source/actor, observation time, fixed
  account/region, snapshot checksum and explicit observed-only assessment.

Before using the snapshot in a separately authorized fresh promotion, review the
successful workflow run and its artifact ID/digest and compare the exact
snapshot with the intended account and release. The observation does not set
`PRODUCTION_BOOTSTRAP_EVIDENCE_JSON`, authorize apply, or create verified release
history. Existing apply-time alias/revision/image checks remain necessary.

This reader preparation does not change public probe vantage, Cloudflare
protection, acceptance requirements, or the existing bounded change window.
