# Lambda HTTP API infrastructure

This directory contains the replacement infrastructure layout for the Lambda
HTTP API. It deliberately does not change or initialize the legacy `infra/`
root.

- `artifacts` owns only immutable release storage.
- `bootstrap/portfolio-deployer-development-bootstrap-policy.json` is the
  reviewed initial development-only deployer policy.
- `bootstrap/portfolio-lambda-execution-boundary-policy.json` is the reviewed
  input for the root-owned execution boundary.
- `environments/dev` and `environments/prod` use distinct backend keys.
- `modules/service` contains no backend or provider configuration.

The files under [`bootstrap`](./bootstrap/README.md) are authoritative initial
policy inputs only. Their presence grants nothing, and every live provisioning,
assignment, use, tightening, and reprovisioning step remains separately
approval-gated.

Saved plans can contain sensitive configuration, so keep them in private
temporary directories. Always run planning and applying as separate commands.
