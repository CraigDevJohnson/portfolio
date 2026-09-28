# Production observation from CI and the operator

Cloudflare Bot Fight Mode can challenge GitHub runners. Keep all Cloudflare security settings unchanged. A challenge, HTTP 403, missing route or transient failure fails the public observation; direct-origin requests cannot substitute for public evidence.

CI continuously checks the TLS origin, health revision and cache policy, homepage, Soccer, CSS and JPEG, immutable alias/image, five alarms, foundation forwarding route and error metrics. `ci-origin-window.json` identifies this evidence accurately. Its default window is 2100 seconds, allowing the operator to begin shortly after the deployment coordinate appears. It remains unverified pending public/browser evidence, positive traffic coverage for every observed minute after ingestion, and protected finalization. The original failed deployments stay failed.

The operator uses the checked-in observer at the exact promotion commit on a trusted machine where ordinary public requests succeed. `git` verifies that commit and unchanged collector source; `gh api user` must identify Craig. The collector needs no AWS credentials. It disables curl configuration files and proxies, uses normal public DNS and verified TLS, and never sets an origin override. No response bodies, headers, cookies, OAuth codes or tokens enter evidence. Only coordinates, timestamps, monotonic elapsed times and an exact list of successful required checks are retained.

## Collect the overlapping public window

After the protected apply creates its GitHub production deployment, obtain the actual deployment ID, promotion/source/image coordinates and published Lambda version from the release evidence or existing read-only deployment metadata. Create a local binding JSON with exactly these fields (strings):

- `production_deployment_id`, `promotion_sha`, `source_sha`, `image_digest`, `lambda_version`
- `base_url`: `https://craigdevjohnson.com`

Do not substitute the current application source for the manifest-only promotion SHA. The collector derives a deterministic window ID from the promotion and original deployment; the protected CI artifact must match it. Start promptly while CI is observing:

```sh
python3 scripts/observe-lambda-production.py observe-public binding.json public-window.json 2100
```

The optional final argument selects 1800–3600 actual monotonic seconds; the default is 2100. Use 2100 seconds for both operator and CI so start offsets up to five minutes still allow 1800 seconds of common coverage. CI flushes a nonsecret window-ID/first-complete-sample timestamp line for coordination; deployment creation alone does not mean that observation has started. The duration counts between the first and last complete observations. Sampling targets a 30-second cadence, subtracting probe time from the next sleep. It rejects any observation or gap over 60 seconds, clock jumps, failed route, incorrect source/cache policy, incorrect assets or www redirects. Never edit times, truncate failed intervals or resume a partial file. Failed attempts leave an unusable partial output; use a new output filename and, if necessary, a fresh authorized release window. A delayed start may leave less than 1800 seconds of overlap even when both individual windows are long enough; finalization rejects that.

Perform the real authenticated Soccer and Google Calendar checks throughout the eventual common interval, with no gap over 60 seconds. Preserve the existing sanitized browser receipt schema and exact coordinates. Both endpoint observations require actual creation, readback and deletion of a test event; restoring or reading an existing event does not satisfy creation. Interior observations require connected Calendar reads. All observations require current authorized Soccer data, secure/HttpOnly/SameSite=Lax/path=/soccer cookies and non-cacheable authenticated responses. Do not put private data in either receipt.

## Refresh, package and finalize

After the Release run succeeds, download its immutable production evidence and locate `ci-origin-window.json`. Check that the operator and CI windows share at least 1800 seconds. Wait for final metric ingestion (at least 180 seconds after the final observed minute boundary), then make a new actual public read:

```sh
python3 scripts/observe-lambda-production.py refresh-public ci-origin-window.json public-window.json public-refreshed.json
python3 scripts/observe-lambda-production.py dispatch-inputs ci-origin-window.json public-refreshed.json browser-receipt.json APPLY_RUN_ID acceptance-inputs.json
```

`APPLY_RUN_ID` is the original successful Release run, not the acceptance run. Packaging creates compact workflow inputs and rejects more than 60000 total encoded bytes, leaving room below GitHub's combined dispatch limit. Submit that JSON to the existing `production-acceptance.yml` workflow through the authorized GitHub workflow-dispatch operation. This procedure does not itself authorize dispatch or approval.

The authenticated submitting actor and protected reviewer must remain Craig. The finalizer binds both receipts to the digest-verified original workflow artifact, current protected main, exact promotion/source/image/version/deployment/window and existing approval provenance. It requires actual continuous common coverage, the browser contract, complete final ingested metric coverage, a fresh CI origin/AWS probe and a fresh operator public read **no older than 300 seconds at finalization**, rechecked after remote status/main checks immediately before every new success POST, including retries. An already-recorded identical remote success remains idempotent and requires no new POST. Future, stale, gapped, partial or substituted receipts fail closed. Refresh immediately before dispatch and complete protected review within that bound; stale evidence requires a new public read and a new acceptance run, not relaxed checks.

The verification record uses schema 3 and distinct `ci_origin_window` and `operator_public_window` verdicts. It hashes both original windows/receipts and final metrics before recording the original deployment as successful. This is trusted single-operator evidence, like the existing browser receipt, not a claim of autonomous public verification by the CI runner. No new grants, credentials, services or Cloudflare exceptions are required. Infrastructure execution deadlines are unchanged.
