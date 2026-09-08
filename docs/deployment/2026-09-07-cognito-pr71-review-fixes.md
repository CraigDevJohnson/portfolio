# PR #71 review fixes

All four code-review findings on `8c5ccee8` were reproduced or verified against
the approved development design. No cloud policy or resource changes are part
of these fixes.

| Finding | Result |
| --- | --- |
| [Optional portal key stops Lambda startup](https://github.com/CraigDevJohnson/portfolio/pull/71#discussion_r3954223032) | Required secrets remain an atomic batch; the portal key resolves separately. Missing/invalid SSM responses, access errors and optional-only AWS configuration failures clear the portal key and log a fixed warning. Key-format validation still disables the portal through configuration. |
| [Logout immediately signs in again](https://github.com/CraigDevJohnson/portfolio/pull/71#discussion_r3954223046) | `GET /login` renders a signed-out page. Only an explicit `POST /login` begins the Google code/PKCE flow. Logout clears both the session and pending OAuth state. Registered callback and logout URLs remain unchanged. |
| [Callback path accepts an unregistered route](https://github.com/CraigDevJohnson/portfolio/pull/71#discussion_r3954223050) | Portal enablement requires the literal `/callback` path. Legacy, alternate, encoded and query/fragment variants fail configuration validation; explicit HTTP loopback support remains. |
| [Untagged instances expose unavailable actions](https://github.com/CraigDevJohnson/portfolio/pull/71#discussion_r3954223058) | Exact `PortfolioManagement=dev` eligibility and lifecycle state determine available controls. Other instances remain visible with read-only metrics/logs and disabled start/stop/restart. IAM remains authoritative for requests. |

Cognito logout does not end a user's Google session, so automatically starting
OAuth at the logout return URL can sign them in again. The explicit button
prevents that automatic restart. See the
[AWS logout endpoint documentation](https://docs.aws.amazon.com/cognito/latest/developerguide/logout-endpoint.html).

Focused regressions cover secret isolation and warning redaction; GET/POST
sign-in routing, PKCE state binding and logout cookies; callback path rejection;
and rendered tagged/untagged instance controls. Live Google/Cognito sign-in and
EC2 operations remain part of the later separately reviewed deployment proof.

Validation completed: `task ci`, `task infrastructure-ci`, focused regression
tests and independent cross-review passed. The signed-out page was checked in a
local browser with fixture configuration and no live AWS/Google credentials.
It rendered the explicit sign-in button without redirecting to an identity
provider. Live sign-in and EC2 operations were not performed.
