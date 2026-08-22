<!-- markdownlint-disable MD013 -->
# Management portal preview

Management portal preview lets a local reviewer inspect representative EC2 inventory states, request harmless preview actions, load sample metrics and logs, and view empty, retrieval-error, and interruption states without Cognito or AWS clients.

## Sub-features

- `portal-dashboard` renders six representative instance states and operator context.
- `portal-actions` returns inline preview-only start, stop, or restart feedback.
- `portal-metrics` loads sample CPU values into the selected instance's detail region.
- `portal-logs` loads recent sample events in descending time order.
- `portal-states` renders empty, retrieval-error, fragment-error, and full interruption fixtures.
- `portal-exit` returns from the operator shell to the public portfolio.

## How to get to it (user POV)

- Under the explicit local preview launch, open `/mgmt` directly.
- Review the `Local preview — no AWS actions are sent` banner before using any control.
- In the Instances table, choose a state-appropriate action, `Load metrics`, or `Load logs`.
- Choose `Exit preview` or `Back to portfolio` to return to `/`.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes and explicitly validates `/mgmt` plus its preview safety banner.
- The server was launched by `control-portfolio` with an empty inherited environment and loopback-only preview mode.
- Browser request evidence is retained. This recipe must never be repointed at a live portal URL.

- **Dashboard.** Run `goto "$VERIFY_URL/mgmt"` and snapshot. Require the H1 `Management portal`, username `local.preview@portfolio.test`, safety banner `Local preview — no AWS actions are sent`, instance count `6 total`, and six `[data-portal-instance]` rows.
- **Action.** Click `"button[aria-label='stop instance Portfolio web (i-0f1e2d3c4b5a69788)']"`, wait for `#action-status-i-0f1e2d3c4b5a69788`, and snapshot. The feedback says `Preview only — no AWS stop action was sent for i-0f1e2d3c4b5a69788.` and the browser request is only the loopback POST.
- **Metrics.** Click `"button[aria-label='Load metrics for Portfolio web (i-0f1e2d3c4b5a69788)']"`. Wait for `#metrics-i-0f1e2d3c4b5a69788 table`, then require four CPU samples and `aria-expanded="true"` on the triggering control.
- **Logs.** Click `"button[aria-label='Load logs for Portfolio web (i-0f1e2d3c4b5a69788)']"`. Wait for `#logs-i-0f1e2d3c4b5a69788 .portal-log-list`, then require three events in descending timestamp order and `aria-expanded="true"`.
- **Fragment error.** Point one preview metrics or logs control at the same route with `?fixture=error`, process it with HTMX, and activate it. Require HTTP `500`, `X-Portal-Fragment-Error: true`, useful inline error feedback, and the open scoped detail region.
- **Dashboard states.** Open `/mgmt?fixture=empty` and require `No instances found`; open `/mgmt?fixture=retrieval-error` and require `Unable to load instances.`.
- **Interruption state.** Open `/__preview/portal/error`; expect HTTP `503` and the full operator interruption page headed `Something interrupted the connection`. Capture this with an HTTP response body/status alongside browser evidence because a successful-looking screenshot cannot prove the status code.
- **Exit.** Return to `/mgmt`, click `"a.portal-session-action[href='/']"`, and require the Home title and `data-layout="systems-overlook"`.
- **Proof.** Retain the dashboard before-action evidence, inline action result, metrics/log result snapshots, browser requests, response status for the interruption route, server log, and launch metadata under `evidence/management-portal-preview/`.

## Gotchas

- The preview intentionally has no authentication. That proves only local operator UI behavior, not Cognito login or authorization enforcement.
- Preview action feedback is not evidence of an EC2 state change. Its value is proving the request path, safety copy, and inline swap.
- Metrics and logs are adjacent detail regions. Scope assertions to the instance ID and detail kind.
- HTTP error fragments carry `X-Portal-Fragment-Error: true` so HTMX swaps meaningful error content. Preserve the response header when verifying an error path.
- The preview must bind to loopback. If doctor sees any other listener, stop; do not drive it.
