# Lambda HTTP API infrastructure

This directory contains the replacement infrastructure layout for the Lambda
HTTP API. It deliberately does not change or initialize the legacy `infra/`
root.

- `artifacts` owns only immutable release storage.
- `environments/dev` and `environments/prod` use distinct backend keys.
- `modules/service` contains no backend or provider configuration.

Saved plans can contain sensitive configuration, so keep them in private
temporary directories. Always run planning and applying as separate commands.
