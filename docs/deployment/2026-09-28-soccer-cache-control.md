# Soccer response caching correction, September 28, 2026

Production readiness inspection found that the current image served Soccer
responses without an explicit Cache-Control policy. Cloudflare reported DYNAMIC,
but that does not prohibit browser storage of authenticated player and calendar
data. The production acceptance contract requires no-store.

All /soccer and /soccer/ routes now pass through one no-store boundary, including
authentication callbacks, HTML fragments, calendar operations, redirects and
error responses. Portfolio pages and static asset caching are unchanged. Route
tests exercise signed-in and signed-out pages, import validation, logout, method
rejection and unknown endpoints, plus an unrelated portfolio page.

This is an application confidentiality correction within Issue #75 readiness.
It changes no IAM authority, resources, secrets, retention, account placement or
fixed infrastructure cost. Existing baseline and root-key exception boundaries
remain unchanged. The session encryption key remains unchanged per Craig's
explicit decision.

The replacement image must first pass the trusted development deployment and
verification. Production then promotes that exact immutable digest in its own
manifest-only change; the previous image's smoke checks do not prove the fix.
Production activation and the full protected 30-minute acceptance remain pending.
