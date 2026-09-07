# Development Cognito and Google sign-in design

**Status:** Approved design, pending implementation-plan review

**Approved:** 2026-09-03

**Scope:** Development Lambda environment, the existing `/mgmt` portal, and Google-federated sign-in. Production remains unchanged.

## Purpose

Enable the existing management portal at `dev.craigdevjohnson.com` without introducing portfolio-managed user passwords. Google authenticates the user. Cognito brokers the OAuth and OpenID Connect exchange and issues the ID token that the Go application already validates with Authorization Code plus PKCE.

## Decision

The development environment owns a dedicated Cognito user pool. It has a public OAuth app client, a Cognito managed-login domain, and Google as its only interactive identity provider.

The app client permits the authorization-code OAuth flow with `openid`, `email`, and `profile`. It does not enable Cognito username/password, SRP, administrator password, or self-service sign-up flows.

The portal remains anonymous outside `/mgmt`. After a successful Cognito token validation, the Go callback handler compares the verified email claim with the development allowlist before it writes an encrypted portal session cookie. The initial allowlist contains one Google address owned by Craig. A rejected user does not receive a portal session.

This leaves the user pool able to create a federated profile when an unapproved Google account reaches Cognito. That profile grants no site access. A Cognito pre-sign-up trigger that prevents those profile records is deferred until it provides value beyond the single-user development rollout.

## Architecture

```text
Browser
  -> portfolio development Lambda `/login`
  -> Cognito managed login
  -> Google sign-in
  -> Cognito authorization code callback at `/callback`
  -> Go PKCE exchange and signed-ID-token validation
  -> exact email allowlist check
  -> encrypted session cookie for `/mgmt`
```

The existing Go OIDC client is a public client and uses PKCE. It requires the Cognito domain, app-client ID, redirect URI, logout URI, session key, and allowlist at runtime. It does not require a Cognito app-client secret.

## Infrastructure and secret handling

The development Lambda module will provision the user pool, managed-login domain, app client, Google provider binding, and the parameter names consumed by the Lambda. It will add the corresponding `MGMT_*` environment variables and extend the Lambda startup resolver to fetch the management session key from Parameter Store.

The Google OAuth client is created in Craig's Google Cloud project. Its client ID and client secret are needed to configure the Cognito Google provider. They must enter the deployment through the existing deployment secret channel and must never be committed, printed, or saved in an `.auto.tfvars` file. The Terraform state backend must remain encrypted and access-controlled because an identity-provider configuration may retain a provider client secret in state.

The session key is a SecureString Parameter Store value. Its path is managed in OpenTofu; the plaintext value is injected separately. The Lambda receives the path, then resolves and decrypts the value during cold-start initialization.

## Portal authorization

`MGMT_ALLOWED_EMAILS` is an environment-owned, comma-separated exact-email allowlist. The development root supplies a single address. The config layer normalizes and validates the list, and the callback handler denies an empty, unverified, or non-allowlisted email before creating a portal session.

The Lambda role gains only the management permissions that the existing portal uses. `ec2:DescribeInstances`, CloudWatch metric lookup, and log filtering are read operations. Instance start and stop are limited to instances bearing a dedicated opt-in management tag, allowing additional development targets later without expanding the role to every instance.

## Callback and logout URLs

The development Cognito app client registers:

- `https://dev.craigdevjohnson.com/callback` for sign-in.
- `https://dev.craigdevjohnson.com/login` for logout.
- A loopback callback for local development only when explicitly enabled.

The Google Cloud OAuth client registers Cognito's documented user-pool identity-provider callback URI, not the portfolio callback URI. Cognito, rather than Google, redirects the browser back to the application.

## Error handling and observability

Malformed, absent, unverified, or disallowed identity claims return the existing generic portal sign-in failure page. Logs record a reason category but not raw tokens or authorization codes. The allowlist must never be accepted as an alternate identity source. Cognito token signature, issuer, audience, and expiry validation stays mandatory before the allowlist decision.

## Testing and proof

Tests cover configuration parsing, exact allowlist matching, rejection of missing or unverified email claims, and the callback's refusal to create a session for an unapproved address. Terraform tests assert the separate dev user pool, Google-only provider selection, authorization-code and PKCE client configuration, registered URLs, Lambda runtime variables, SSM permissions, and tag-limited EC2 actions.

Before any live apply, review the development plan and identify the AWS account, region, current resources, Cognito domain prefix, Google client configuration, Parameter Store paths, and exact affected EC2 tag. The live apply and the Google Cloud OAuth-client creation remain separate, explicit external actions.

## Non-goals

- Production Cognito resources or production portal enablement.
- Cognito-managed passwords, password reset, MFA policy, or public sign-up.
- A second Lambda solely for a Cognito pre-sign-up trigger.
- Broad EC2 control over untagged instances.
- Changes to the public portfolio pages or the unrelated footer work.
