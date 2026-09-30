# Cloudflare DNS records

<!-- markdownlint-disable MD013 -->

Cloudflare is the registrar and authoritative DNS for `craigdevjohnson.com`.
There is no Route 53 hosted zone. The portfolio's records are managed by hand in
the Cloudflare dashboard (D19); the Cloudflare OpenTofu provider would need an
API token. This page is the record of what the portfolio owns.

Other records in the zone belong to other owners and are out of scope here:
`foundry` (a DNS-only record owned by the foundry repo) and the Fastmail mail
records.

## Records

| Name | Type | Target | Proxy status | Source of the target |
| --- | --- | --- | --- | --- |
| `craigdevjohnson.com` (apex) | CNAME (flattened at the apex) | Prod API Gateway regional domain, `d-*.execute-api.us-west-2.amazonaws.com` | Proxied | `api_gateway_domain_targets["craigdevjohnson.com"]` from the prod root |
| `www` | CNAME | Prod API Gateway regional domain for `www.craigdevjohnson.com` | Proxied | `api_gateway_domain_targets["www.craigdevjohnson.com"]` from the prod root |
| `dev` | CNAME | Dev API Gateway regional domain, `d-*.execute-api.us-west-2.amazonaws.com` | Proxied | `api_gateway_domain_targets["dev.craigdevjohnson.com"]` from the dev root |
| ACM validation for `dev.craigdevjohnson.com` | CNAME | `_*.acm-validations.aws.` | DNS only | `acm_validation_records` from the dev root |
| ACM validation for `craigdevjohnson.com` | CNAME | `_*.acm-validations.aws.` | DNS only | `acm_validation_records` from the prod root |
| ACM validation for `www.craigdevjohnson.com` | CNAME | `_*.acm-validations.aws.` | DNS only | `acm_validation_records` from the prod root |

Read the targets from the environment roots:

```sh
tofu -chdir=infra/lambda/environments/prod output -json api_gateway_domain_targets
tofu -chdir=infra/lambda/environments/prod output -json acm_validation_records
tofu -chdir=infra/lambda/environments/dev output -json api_gateway_domain_targets
tofu -chdir=infra/lambda/environments/dev output -json acm_validation_records
```

- **ACM validation records stay DNS only** (grey cloud) and stay in place for as
  long as the certificate is in use: ACM renews the certificate through them.
  Enter the record name and value exactly as ACM reports them.
- **Traffic records are proxied** (orange cloud). Cloudflare terminates the
  visitor's TLS and connects to the API Gateway custom domain with its ACM
  certificate for the same hostname.
- **Redirect rules** in Cloudflare send `www` to the apex with a 301 and HTTP to
  HTTPS with a 308. They do not change when the origin moves.
- `api_gateway_domain_targets` is empty while an environment is planned with
  `activate_custom_domain=false` (`ACTIVATE_CUSTOM_DOMAIN=false` on both the
  plan and the apply task; see
  [DEPLOY-INSTRUCTIONS.md](../../DEPLOY-INSTRUCTIONS.md#operator-workflow));
  use its `api_default_url` for testing then.
- An API Gateway custom domain name is unique within a Region across all AWS
  accounts, so each hostname can exist in only one account at a time.
