# Requirements Document

## Introduction

The EC2 Management Portal adds a protected administration section to the existing Go + Templ + HTMX portfolio web application. It consists of two new route families: an authentication flow at `/login` and `/auth/callback`, and a management dashboard at `/mgmt`. Access to `/mgmt` requires a valid, unexpired session established through the OAuth 2.0 Authorization Code + PKCE flow with AWS Cognito.

Authentication is delegated entirely to the Cognito_Hosted_UI. The application never sees or stores operator passwords — Cognito handles user identity, invitations, password resets, and MFA. The number of operators is unbounded; operators are managed in the Cognito_User_Pool via the AWS console or CLI. Once authenticated, operators can perform EC2 instance management actions — start, stop, restart, view metrics, and view logs — directly from the browser using HTMX-driven interactions backed by the AWS SDK for Go.

The feature reuses the project's existing AES-256-GCM session cookie infrastructure and integrates with Cognito via standard OAuth 2.0 and JWKS-based JWT validation.

---

## Glossary

- **Portal**: The combined `/login`, `/auth/callback`, and `/mgmt` subsystem described in this document.
- **Operator**: An authenticated human user of the Portal, whose identity is managed in the Cognito_User_Pool.
- **Cognito_User_Pool**: The AWS Cognito User Pool that owns operator identities and handles invitations, MFA, and password management.
- **Cognito_Hosted_UI**: The Cognito-managed login page that operators are redirected to for authentication.
- **JWKS_Cache**: A server-side cache of the Cognito JSON Web Key Set, refreshed at most once per hour, used to validate ID token signatures.
- **OAuth_State_Cookie**: A short-lived HttpOnly cookie named `mgmt_oauth_state` that stores the PKCE `code_verifier` and OAuth `state` nonce during the authorization flow.
- **ID_Token**: A JWT issued by Cognito after successful authentication, used to establish the operator's identity.
- **Session_Cookie**: An HTTP-only, Secure, SameSite=Strict cookie that carries an AES-256-GCM-encrypted JSON session payload.
- **Portal_Session**: The decrypted session payload carried by the Session_Cookie; contains username and expiry.
- **Auth_Middleware**: An HTTP middleware component that validates the Session_Cookie before allowing access to protected handlers.
- **EC2_Client**: The AWS SDK v2 EC2 service client used to list instances and execute instance actions.
- **Metrics_Client**: The AWS SDK v2 CloudWatch client used to fetch CPU utilization data for EC2 instances.
- **Logs_Client**: The AWS SDK v2 CloudWatch Logs client used to retrieve log streams and events for EC2 instances.
- **Instance_Action**: One of: start, stop, or restart (stop-then-start) applied to a single EC2 instance.
- **Management_Handler**: The Go HTTP handler package responsible for the `/mgmt` route family.
- **Login_Handler**: The Go HTTP handler responsible for the `/login` and `/auth/callback` routes.

---

## Requirements

### Requirement 1: User Authentication — Cognito Login Initiation

**User Story:** As an operator, I want to be redirected to the AWS Cognito Hosted UI when I access `/login`, so that I can authenticate using my Cognito account without the application managing my credentials.

#### Acceptance Criteria

1. WHEN an unauthenticated `GET /login` request is received, THE Login_Handler SHALL redirect the browser to the Cognito Hosted UI authorization endpoint with the OAuth 2.0 Authorization Code + PKCE parameters: `client_id`, `redirect_uri` (pointing to `/auth/callback`), `response_type=code`, `scope=openid email profile`, a cryptographically random `state` parameter (stored in a short-lived HttpOnly cookie), and a PKCE `code_challenge` derived from a cryptographically random `code_verifier`.
2. WHEN an authenticated `GET /login` request is received (valid Portal_Session cookie present), THE Login_Handler SHALL redirect the browser to `/mgmt` with HTTP 302.
3. THE Login_Handler SHALL store the `state` value and `code_verifier` in an HttpOnly, Secure, SameSite=Lax cookie named `mgmt_oauth_state` with a 10-minute expiry, for validation during the callback.
4. IF `MGMT_COGNITO_DOMAIN`, `MGMT_COGNITO_CLIENT_ID`, or `MGMT_COGNITO_REDIRECT_URI` is not configured, THEN THE Login_Handler SHALL return HTTP 503.

---

### Requirement 2: User Authentication — OAuth Callback

**User Story:** As an operator, I want the application to complete the OAuth flow after I authenticate with Cognito, so that I receive a session cookie and am redirected to the management dashboard.

#### Acceptance Criteria

1. WHEN a `GET /auth/callback` request is received with a `code` and `state` query parameter, THE Login_Handler SHALL validate that the `state` matches the value stored in the `mgmt_oauth_state` cookie; IF the `state` does not match or the cookie is absent, THE Login_Handler SHALL return HTTP 400 with an error page.
2. WHEN the `state` is valid, THE Login_Handler SHALL exchange the authorization `code` for tokens by calling the Cognito token endpoint, passing the `code_verifier` retrieved from the `mgmt_oauth_state` cookie.
3. WHEN the token exchange succeeds, THE Login_Handler SHALL validate the ID_Token JWT: verify the signature against the Cognito JWKS endpoint, verify the `iss` claim matches the expected Cognito issuer URL, verify the `aud` claim matches `MGMT_COGNITO_CLIENT_ID`, and verify the token is not expired.
4. WHEN JWT validation succeeds, THE Login_Handler SHALL extract the `email` claim (or `cognito:username` if email is absent) as the operator's username, create a Portal_Session, clear the `mgmt_oauth_state` cookie, set the Session_Cookie with HttpOnly and Secure attributes and a 12-hour expiry, and redirect the browser to `/mgmt` with HTTP 302.
5. IF the token exchange or JWT validation fails for any reason, THE Login_Handler SHALL return HTTP 401 with an error page and log the failure with structured fields including the error type.
6. THE Login_Handler SHALL cache the Cognito JWKS response and refresh it no more than once per hour to avoid hammering the JWKS endpoint.

---

### Requirement 3: User Authentication — Logout

**User Story:** As an operator, I want a logout action, so that I can end my session and prevent unauthorized access from a shared browser.

#### Acceptance Criteria

1. WHEN a `POST /logout` request is received with a valid Session_Cookie, THE Login_Handler SHALL clear the Session_Cookie by setting it to an expired value and redirect the browser to the Cognito logout endpoint with `client_id` and `logout_uri` parameters pointing back to `/login`, so that the Cognito session is also terminated.
2. WHEN a `POST /logout` request is received without a valid Session_Cookie, THE Login_Handler SHALL redirect the browser to `/login` with HTTP 302.
3. IF the Cognito logout endpoint URL is not configured, THEN THE Login_Handler SHALL clear the Session_Cookie and redirect to `/login` with HTTP 302 (local logout only).

---

### Requirement 4: Protected Route — Auth Middleware

**User Story:** As a server operator, I want all `/mgmt` routes protected by authentication, so that unauthenticated users cannot access or invoke EC2 actions.

#### Acceptance Criteria

1. THE Auth_Middleware SHALL intercept all requests to `/mgmt` and `/mgmt/*` paths.
2. WHEN a request to a `/mgmt` or `/mgmt/*` path arrives without a valid, unexpired Session_Cookie, THE Auth_Middleware SHALL redirect the browser to `/login` with HTTP 302, which itself redirects to the Cognito_Hosted_UI.
3. WHEN a request arrives with a valid, unexpired Session_Cookie, THE Auth_Middleware SHALL pass the request to the downstream handler with the Operator's username injected into the request context under the key `portal_username`.
4. WHEN a Session_Cookie is present but its AES-256-GCM decryption fails, THE Auth_Middleware SHALL clear the Session_Cookie by setting it with Max-Age=0 and redirect the browser to `/login` with HTTP 302.
5. WHEN a Session_Cookie is present but the Portal_Session's `expires_at` field is less than or equal to the current server time, THE Auth_Middleware SHALL clear the Session_Cookie by setting it with Max-Age=0 and redirect the browser to `/login` with HTTP 302.
6. THE Auth_Middleware SHALL use the same AES-256-GCM session key infrastructure as the existing soccer session, loaded from the `MGMT_SESSION_KEY` environment variable (a 64-character hex string encoding 32 bytes).
7. IF `MGMT_SESSION_KEY` is not configured at startup, THEN THE Auth_Middleware SHALL return HTTP 503 for all requests to `/login` and `/mgmt/*`.
8. A Portal_Session SHALL be considered valid only if it contains a non-empty `username` field and an `expires_at` field that is strictly greater than the current server time.

---

### Requirement 5: EC2 Instance Listing

**User Story:** As an operator, I want to see all EC2 instances in the configured AWS region, so that I can select instances for management actions.

#### Acceptance Criteria

1. WHEN an authenticated `GET /mgmt` request is received, THE Management_Handler SHALL render the management dashboard with a list of EC2 instances retrieved from the EC2_Client, sorted in ascending lexicographic order by instance ID.
2. THE Management_Handler SHALL display for each instance: instance ID, instance name (from the `Name` tag, or `—` if the tag is absent), current state (one of: `pending`, `running`, `stopping`, `stopped`, `shutting-down`, or `terminated`), instance type, and availability zone.
3. WHEN the EC2_Client returns zero instances, THE Management_Handler SHALL render the dashboard with a message indicating no instances were found in the configured region.
4. IF the EC2_Client returns an error or any other failure occurs that prevents successful instance retrieval (including network timeouts and authentication failures), THEN THE Management_Handler SHALL render the dashboard with an alert indicating retrieval failure and log the error with structured fields including instance region, AWS error code, and error message.
5. IF `MGMT_AWS_REGION` is not set, THEN THE Management_Handler SHALL use `us-east-1` as the default region.
6. WHERE `MGMT_AWS_REGION` is set, THE Management_Handler SHALL use the region value specified by that environment variable.

---

### Requirement 6: EC2 Instance Actions — Start, Stop, Restart

**User Story:** As an operator, I want to start, stop, and restart EC2 instances from the dashboard, so that I can manage instance lifecycle without using the AWS console.

#### Acceptance Criteria

1. WHEN an authenticated `POST /mgmt/instances/{id}/start` request is received, THE Management_Handler SHALL invoke the EC2_Client `StartInstances` API for the specified instance ID.
2. WHEN an authenticated `POST /mgmt/instances/{id}/stop` request is received, THE Management_Handler SHALL invoke the EC2_Client `StopInstances` API for the specified instance ID.
3. WHEN an authenticated `POST /mgmt/instances/{id}/restart` request is received, THE Management_Handler SHALL invoke the EC2_Client `StopInstances` API, and only if `StopInstances` succeeds, SHALL invoke the EC2_Client `StartInstances` API for the specified instance ID.
4. WHEN an Instance_Action succeeds, THE Management_Handler SHALL return HTTP 200 with an HTMX-compatible HTML fragment showing the updated instance state and a success message.
5. IF an Instance_Action API call returns an error, THEN THE Management_Handler SHALL return HTTP 200 with an HTMX-compatible HTML fragment containing an error alert indicating the AWS error, and SHALL log the error with structured fields including instance ID and action name.
6. IF an Instance_Action request is received where the instance ID does not match the pattern `^i-[0-9a-f]{8,17}$`, THEN THE Management_Handler SHALL return HTTP 400 with an HTMX-compatible HTML error fragment and SHALL NOT invoke any EC2_Client API.
7. THE Management_Handler SHALL log each Instance_Action with structured fields: operator username, instance ID, action name, and outcome (success or error).
8. IF a restart action's `StopInstances` call returns an error, THEN THE Management_Handler SHALL return HTTP 200 with an HTMX-compatible HTML fragment containing an error alert indicating the stop failure, SHALL NOT invoke `StartInstances`, and SHALL log the error with structured fields including instance ID and action name.

---

### Requirement 7: EC2 Instance Metrics

**User Story:** As an operator, I want to view CPU utilization metrics for an EC2 instance, so that I can assess instance health before or after taking an action.

#### Acceptance Criteria

1. WHEN an authenticated `GET /mgmt/instances/{id}/metrics` request is received with a valid instance ID, THE Management_Handler SHALL query the Metrics_Client for the `CPUUtilization` CloudWatch metric for the specified instance over the past 60 minutes at 5-minute resolution.
2. WHEN the Metrics_Client returns one or more data points, THE Management_Handler SHALL return HTTP 200 with an HTML fragment (containing no `<html>` or `<body>` wrapper) with a table of metric data points, each showing the timestamp in ISO 8601 UTC format and the CPU utilization percentage rounded to two decimal places.
3. WHEN the Metrics_Client returns zero data points, THE Management_Handler SHALL return HTTP 200 with an HTML fragment containing a "No data available" message.
4. IF the Metrics_Client returns an error, THEN THE Management_Handler SHALL return an HTML fragment containing an error alert indicating a metrics retrieval failure and SHALL log the error with structured fields including instance ID and error message.
5. IF the instance ID in the request does not match the pattern `^i-[0-9a-f]{8,17}$`, THEN THE Management_Handler SHALL return HTTP 400 with an HTML fragment containing an error message and SHALL NOT query the Metrics_Client.

---

### Requirement 8: EC2 Instance Logs

**User Story:** As an operator, I want to view recent CloudWatch log events for an EC2 instance, so that I can diagnose issues without leaving the portal.

#### Acceptance Criteria

1. WHEN an authenticated `GET /mgmt/instances/{id}/logs` request is received and the instance ID matches the pattern `^i-[0-9a-f]{8,17}$`, THE Management_Handler SHALL query the Logs_Client for log events from the log group `/ec2/{id}` over the past 30 minutes.
2. WHEN the Logs_Client returns one or more events, THE Management_Handler SHALL return HTTP 200 with an HTMX-compatible HTML fragment containing up to 100 log events in reverse-chronological order, each showing a timestamp in RFC 3339 format and the event message.
3. WHEN the Logs_Client returns zero events, THE Management_Handler SHALL return HTTP 200 with a fragment containing a "No recent log events" message.
4. IF the Logs_Client returns a `ResourceNotFoundException`, THEN THE Management_Handler SHALL return HTTP 200 with a fragment containing a "Log group not found" message rather than a generic error.
5. IF the Logs_Client returns any other error, THEN THE Management_Handler SHALL return HTTP 200 with a fragment containing an error alert and SHALL log the error with structured fields including instance ID and log group name.
6. IF the instance ID does not match the pattern `^i-[0-9a-f]{8,17}$`, THEN THE Management_Handler SHALL return HTTP 400 with an error fragment without querying the Logs_Client.

---

### Requirement 9: Management Dashboard UI

**User Story:** As an operator, I want a clean, functional dashboard UI consistent with the portfolio's existing design, so that the portal feels native to the application.

#### Acceptance Criteria

1. THE Management_Handler SHALL render the `/mgmt` dashboard using a Templ layout and page component styled with Tailwind CSS, consistent with the portfolio's existing dark-first theme.
2. THE Management_Handler SHALL render instance action buttons (start, stop, restart) using HTMX `hx-post` attributes that target a response fragment inline without a full page reload.
3. THE Management_Handler SHALL render metrics and log load triggers using HTMX `hx-get` attributes that load content into a designated panel without a full page reload.
4. THE Management_Handler SHALL display the authenticated operator's username in the dashboard header.
5. THE Management_Handler SHALL provide a logout form button in the dashboard header that submits `POST /logout`, which clears the session and redirects through the Cognito logout flow.
6. WHERE an instance is in the `stopped` state, THE Management_Handler SHALL render the stop and restart action buttons with the HTML `disabled` attribute set.
7. WHERE an instance is in the `running` state, THE Management_Handler SHALL render the start action button with the HTML `disabled` attribute set.
8. WHERE an instance is in a transitional state (`pending`, `stopping`, `shutting-down`), THE Management_Handler SHALL render all three action buttons (start, stop, restart) with the HTML `disabled` attribute set.

---

### Requirement 10: Portal Configuration and Startup

**User Story:** As a server operator, I want all portal configuration loaded from environment variables, so that the portal integrates with the existing env-driven config pattern.

#### Acceptance Criteria

1. WHEN the server starts, THE Portal SHALL read the following environment variables: `MGMT_SESSION_KEY`, `MGMT_COGNITO_DOMAIN` (the Cognito Hosted UI domain, e.g. `https://myapp.auth.us-east-1.amazoncognito.com`), `MGMT_COGNITO_CLIENT_ID`, `MGMT_COGNITO_REDIRECT_URI` (the full callback URL, e.g. `https://example.com/auth/callback`), `MGMT_COGNITO_LOGOUT_URI` (optional, the post-logout redirect URL), and `MGMT_AWS_REGION`.
2. THE Portal SHALL extend the existing `Config` struct in `internal/config/config.go` with portal-specific fields covering the session key, Cognito domain, client ID, redirect URI, logout URI, and AWS region.
3. WHEN the server starts and portal configuration is loaded, THE Portal SHALL emit a single structured log entry at INFO level containing: portal enabled status (true/false) and the resolved AWS region string, regardless of whether the portal is enabled or disabled.
4. THE Portal SHALL guarantee that all existing portfolio routes continue to operate normally regardless of any portal configuration errors, missing environment variables, or disabled portal state; portal configuration issues SHALL NOT affect non-portal routes.
5. IF `MGMT_SESSION_KEY` is set but is not a valid 64-character hex string, THEN THE Portal SHALL log a warning at WARN level, set portal enabled status to false, disable all portal routes, and continue serving all other routes.
6. IF `MGMT_SESSION_KEY` is absent or empty, THEN THE Portal SHALL set portal enabled status to false and disable all portal routes without logging a warning.
7. IF `MGMT_COGNITO_DOMAIN` or `MGMT_COGNITO_CLIENT_ID` is absent or empty while `MGMT_SESSION_KEY` is valid, THEN THE Portal SHALL log a warning at WARN level, set portal enabled status to false, and disable all portal routes.
8. IF `MGMT_COGNITO_LOGOUT_URI` is absent or empty, THEN THE Portal SHALL perform local-only logout (clear session cookie and redirect to `/login`) without attempting to contact the Cognito logout endpointI .
