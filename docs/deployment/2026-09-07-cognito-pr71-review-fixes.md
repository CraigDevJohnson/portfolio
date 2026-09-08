# PR #71 review fixes

The original four code-review findings on `8c5ccee8` were reproduced or verified
against the approved development design and fixed in `df817a96`. The follow-up
finding on that commit was checked against current AWS documentation. No live
cloud policy or resource changes are part of this review work.

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

The follow-up request to [add `email_verified` to client write permissions](https://github.com/CraigDevJohnson/portfolio/pull/71#discussion_r3954397679)
is not applicable. AWS explicitly prohibits app-client write access to
`email_verified` and `phone_number_verified`; see
[app-client attribute permissions](https://docs.aws.amazon.com/cognito/latest/developerguide/user-pool-settings-client-apps.html).
AWS separately documents mapping Google's `email_verified` claim to obtain the
provider's verification status; see
[identity-provider attribute mapping](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-user-pools-specifying-attribute-mapping.html).
The current configuration follows both requirements: map and read
`email_verified`, and grant writes only to the ordinary mapped attributes
`email` and `name`. Adding the requested permission would contradict the
documented app-client restriction. The configuration comment now explains this
exception; the existing OpenTofu and saved-plan contracts retain the supported
attribute sets.

The still-unresolved logout and EC2-control threads were rechecked against
`df817a96`. Their fixes are present even though the comments are attached to
unchanged infrastructure lines: the registered logout URL now reaches the inert
GET handler, and the instance view model now enforces tag eligibility when
rendering controls. Focused routing, cookie, tag and lifecycle tests passed on
reinspection. No additional behavior changes were needed for those threads.

Focused regressions cover secret isolation and warning redaction; GET/POST
sign-in routing, PKCE state binding and logout cookies; callback path rejection;
and rendered tagged/untagged instance controls. Live Google/Cognito sign-in and
EC2 operations remain part of the later separately reviewed deployment proof.

Validation completed: `task ci`, `task infrastructure-ci`, focused regression
tests and independent cross-review passed. The signed-out page was checked in a
local browser with fixture configuration and no live AWS/Google credentials.
It rendered the explicit sign-in button without redirecting to an identity
provider. Live sign-in and EC2 operations were not performed.
