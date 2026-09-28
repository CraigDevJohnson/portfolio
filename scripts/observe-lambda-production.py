#!/usr/bin/env python3
"""Production observations and sanitized operator receipt validation.

observe uses real elapsed time and direct-origin traffic. The identified
operator collects public HTTP observations separately without origin overrides. validate-browser validates
an operator's observations, not their provenance. Only a protected finalizer
that trusts the submitting operator/transport may turn these into success.
Neither command writes production-verification.json or records a deployment.
"""

import datetime as dt
import json
import hashlib
import math
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import uuid

APEX = "https://craigdevjohnson.com"
WWW = "https://www.craigdevjohnson.com"
FUNCTION = "portfolio-lambda-prod"
SUFFIXES = ("api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles")
MIN_WINDOW = 1800
MAX_GAP = 60
PUBLIC_FRESHNESS = 300
PUBLIC_CHECKS = ["apex-health-source-no-store", "apex-home", "apex-soccer-no-store",
                 "apex-css", "apex-jpeg", "www-root-redirect", "www-path-query-redirect"]
BINDING = ("window_id", "production_deployment_id", "promotion_sha", "source_sha",
           "image_digest", "lambda_version", "base_url")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def utc(timestamp):
    return dt.datetime.fromtimestamp(timestamp, dt.timezone.utc).isoformat()


def timestamp(value):
    require(isinstance(value, str), "missing UTC timestamp")
    parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    require(parsed.utcoffset() == dt.timedelta(0), "timestamp must be UTC")
    return parsed.timestamp()


def save(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")


def aws(*args):
    result = subprocess.run(["aws", *args, "--output", "json"], check=True,
                            capture_output=True, text=True, timeout=20)
    return json.loads(result.stdout)


def fetch(base, route, origin=""):
    # Raw response headers can contain cookies. They stay in a private temporary
    # directory and never enter the saved evidence or error messages.
    with tempfile.TemporaryDirectory() as directory:
        headers, body = Path(directory) / "headers", Path(directory) / "body"
        command = ["curl", "--disable", "--noproxy", "*", "-sS", "--connect-timeout", "5", "--max-time", "10",
                   "--max-redirs", "0", "-D", str(headers), "-o", str(body),
                   "--write-out", "%{http_code}"]
        if origin:
            command += ["--connect-to", f"craigdevjohnson.com:443:{origin}:443"]
        result = subprocess.run(command + [base + route], check=True,
                                capture_output=True, text=True, timeout=12)
        # Ignore interim HTTP responses such as proxy CONNECT headers.
        blocks = headers.read_text().replace("\r\n", "\n").strip().split("\n\n")
        fields = {}
        for line in blocks[-1].splitlines()[1:]:
            if ":" in line:
                key, value = line.split(":", 1)
                key = key.lower()
                # Cloudflare may repeat list-valued Report-To telemetry. It is
                # not used by any acceptance assertion; decision headers stay strict.
                require(key not in fields or key in {"set-cookie", "report-to"}, "duplicate response header")
                fields[key] = value.strip()
        return int(result.stdout), fields, body.read_bytes()


def public_probe(source, origin=""):
    routes = (("/healthz", "application/json", "health"), ("/", "text/html", "html"),
              ("/soccer", "text/html", "html"),
              ("/static/css/tailwind.css", "text/css", "css"),
              ("/static/images/backgrounds/home-hero.jpg", "image/jpeg", "jpeg"))
    for route, content_type, kind in routes:
        status, headers, body = fetch(APEX, route, origin)
        require(status == 200, f"{route}: unexpected HTTP status {status}")
        require(headers.get("content-type", "").split(";")[0].lower() == content_type,
                f"{route}: incorrect content type")
        require(body, f"{route}: empty body")
        if kind == "health":
            health = json.loads(body)
            require(health.get("status", health.get("Status")) == "ok" and
                    health.get("revision", health.get("Revision")) == source,
                    "health revision differs from selected application source")
            require(headers.get("cache-control", "").lower() == "no-store", "health must not be cached")
        elif kind == "html":
            if route == "/soccer":
                require(headers.get("cache-control", "").lower() == "no-store", "Soccer must not be cached")
            require(re.search(rb"<(!doctype\s+html|html)(\s|>)", body, re.I), f"{route}: not HTML")
        elif kind == "css":
            require(b"{" in body and b"}" in body, "stylesheet is not CSS")
        else:
            require(body.startswith(b"\xff\xd8\xff"), "hero asset is not JPEG")
    if not origin:
        for route in ("/", "/soccer?production_probe=1&return=%2Fsoccer"):
            status, headers, _ = fetch(WWW, route)
            require(status in (301, 308) and headers.get("location") == APEX + route,
                    "www must permanently redirect to apex preserving path and query")

    return list(PUBLIC_CHECKS) if not origin else list(PUBLIC_CHECKS[:5])

def identity_probe(digest):
    alias = aws("lambda", "get-alias", "--function-name", FUNCTION, "--name", "live")
    version = alias.get("FunctionVersion", "")
    require(re.fullmatch(r"[1-9][0-9]*", version), "live alias is not immutable")
    require(not alias.get("RoutingConfig", {}).get("AdditionalVersionWeights"), "weighted alias is not accepted")
    function = aws("lambda", "get-function", "--function-name", FUNCTION, "--qualifier", version)
    image = function.get("Code", {}).get("ResolvedImageUri", function.get("Code", {}).get("ImageUri", ""))
    require(image.endswith("@" + digest), "live image differs from selected digest")
    return version


def route_probe():
    result = subprocess.run(["python3", str(Path(__file__).with_name("check-foundation-alarm-route.py")), "--json"],
                            capture_output=True, check=True, timeout=45)
    return json.loads(result.stdout)


def telemetry_probe(start, end, api_id):
    notification_route = route_probe()
    names = [FUNCTION + "-" + suffix for suffix in SUFFIXES]
    alarms = aws("cloudwatch", "describe-alarms", "--alarm-names", *names, "--no-paginate").get("MetricAlarms", [])
    require(sorted(a.get("AlarmName", "") for a in alarms) == sorted(names) and
            all(a.get("StateValue") == "OK" for a in alarms), "required production alarm is absent or unhealthy")
    signals = {}
    for namespace, metric, dimension in (("AWS/Lambda", "Errors", "FunctionName=" + FUNCTION),
                                          ("AWS/Lambda", "Throttles", "FunctionName=" + FUNCTION),
                                          ("AWS/ApiGateway", "5xx", "ApiId=" + api_id)):
        name, value = dimension.split("=", 1)
        data = aws("cloudwatch", "get-metric-statistics", "--namespace", namespace,
                   "--metric-name", metric, "--dimensions", f"Name={name},Value={value}",
                   "--start-time", utc(start), "--end-time", utc(end), "--period", "60",
                   "--statistics", "Sum").get("Datapoints")
        require(isinstance(data, list), "missing metric response")
        require(all(type(p.get("Sum")) in (int, float) and math.isfinite(p["Sum"]) and
                    p["Sum"] == 0 for p in data), f"{metric}: production error signal")
        # Sparse metrics can legitimately have no datapoints. Preserve that gap;
        # it is not proof of zero errors and cannot finalize acceptance.
        signals[metric] = {"datapoints": len(data), "observed_sum": 0 if data else None,
                           "periods": sorted(timestamp(point["Timestamp"]) for point in data)}
    signals["notification_route"] = notification_route
    return signals


def observe_window(sample, duration=MIN_WINDOW, interval=30, clock=time.monotonic,
                   wall=time.time, sleep=time.sleep, on_sample=lambda item: None):
    require(type(duration) is int and MIN_WINDOW <= duration <= 3600, "window must be 1800 to 3600 seconds")
    require(type(interval) is int and 1 <= interval <= 30, "interval must be 1 to 30 seconds")
    start, started = clock(), wall()
    observations = []
    version = None
    while True:
        before = clock()
        observed = sample(started, wall())
        now, at = clock(), wall()
        require(now >= before >= start, "monotonic clock moved backward")
        elapsed = now - start
        require(abs((at - started) - elapsed) <= 2, "wall clock changed during observation")
        require(now - before <= MAX_GAP, "observation exceeded maximum gap")
        previous = observations[-1]["elapsed_seconds"] if observations else 0
        require(elapsed - previous <= MAX_GAP, "observation gap exceeds 60 seconds")
        require(not observations or elapsed > previous, "duplicate or out-of-order observation")
        if version is None:
            version = observed["lambda_version"]
        require(observed["lambda_version"] == version, "live alias changed during observation")
        item = dict(observed, observed_at=utc(at), elapsed_seconds=elapsed)
        observations.append(item)
        on_sample(item)
        # The window begins when the first complete sample finishes. This avoids
        # counting pre-observation probe time toward the acceptance duration.
        if elapsed - observations[0]["elapsed_seconds"] >= duration:
            return observations
        sleep(max(0, min(interval - (now - before), duration - (elapsed - observations[0]["elapsed_seconds"]))))


def validate_window(window, now=None):
    now = time.time() if now is None else now
    require(window.get("schema_version") == 1 and window.get("ci_origin_window") == "passed",
            "automated observation window did not pass")
    start, end = timestamp(window["started_at"]), timestamp(window["ended_at"])
    require(MIN_WINDOW <= end - start <= 3600 + MAX_GAP and end <= now and now - end <= 8 * 3600,
            "invalid or future observation window")
    samples = window.get("observations", [])
    require(isinstance(samples, list) and len(samples) >= 2, "automated observations are missing")
    previous_at = previous_elapsed = None
    first_elapsed = samples[0].get("elapsed_seconds")
    require(type(first_elapsed) in (float, int) and math.isfinite(first_elapsed) and
            0 <= first_elapsed <= MAX_GAP, "invalid first elapsed time")
    for sample in samples:
        at, elapsed = timestamp(sample["observed_at"]), sample["elapsed_seconds"]
        require(type(elapsed) in (float, int) and math.isfinite(elapsed), "invalid elapsed time")
        require(start <= at <= end and abs((at - start) - (elapsed - first_elapsed)) <= 2,
                "automated timestamp differs from elapsed time")
        require(previous_at is None or (0 < at - previous_at <= MAX_GAP and
                0 < elapsed - previous_elapsed <= MAX_GAP), "invalid automated observation gap or ordering")
        require(sample["lambda_version"] == window["lambda_version"], "automated alias changed")
        previous_at, previous_elapsed = at, elapsed
    require(timestamp(samples[0]["observed_at"]) == start and
            timestamp(samples[-1]["observed_at"]) == end and
            samples[-1]["elapsed_seconds"] - first_elapsed >= MIN_WINDOW,
            "automated samples do not cover the entire window")
    return start, end


def final_metrics(window):
    start, end = validate_window(window)
    # Include both partial endpoint minutes. Conservative failure on unrelated
    # errors in either endpoint minute is preferable to missing a late error.
    first, last = math.floor(start / 60) * 60, math.floor(end / 60) * 60
    require(time.time() >= last + 180, "wait for final CloudWatch metric ingestion")
    signals = telemetry_probe(first, last + 60, window["api_id"])
    expected = set(range(first, last + 1, 60))
    coverage = {}
    # Error counters can be sparse. Positive traffic counters establish that
    # telemetry arrived for every minute; absent error points remain explicitly
    # "no reported errors", never synthetic zero datapoints.
    for namespace, metric, name, value in (("AWS/Lambda", "Invocations", "FunctionName", FUNCTION),
                                            ("AWS/ApiGateway", "Count", "ApiId", window["api_id"])):
        points = aws("cloudwatch", "get-metric-statistics", "--namespace", namespace,
                     "--metric-name", metric, "--dimensions", f"Name={name},Value={value}",
                     "--start-time", utc(first), "--end-time", utc(last + 60), "--period", "60",
                     "--statistics", "Sum").get("Datapoints", [])
        periods = [timestamp(point["Timestamp"]) for point in points
                   if type(point.get("Sum")) in (int, float) and math.isfinite(point["Sum"]) and point["Sum"] > 0]
        require(len(periods) == len(set(periods)) and expected <= set(periods),
                f"{metric}: incomplete metric coverage; production remains unverified")
        coverage[metric] = sorted(periods)
    require(identity_probe(window["image_digest"]) == window["lambda_version"], "alias changed after window")
    public_probe(window["source_sha"], window["origin_host"])
    return {"schema_version": 1, "window_id": window["window_id"],
            "production_deployment_id": window["production_deployment_id"],
            "metric_coverage": "passed", "error_assessment": "no_reported_errors",
            "traffic_periods": coverage, "error_signals": signals, "checked_at": utc(time.time())}


def validate_browser(window, receipt, now=None, coverage=None):
    """Validate sanitized observations. This deliberately makes no provenance claim."""
    now = time.time() if now is None else now
    start, end = validate_window(window, now)
    if coverage is not None:
        common_start, common_end = timestamp(coverage["started_at"]), timestamp(coverage["ended_at"])
        require(start <= common_start < common_end <= end and common_end - common_start >= MIN_WINDOW, "invalid browser coverage")
        start, end = common_start, common_end
    binding = BINDING
    require(receipt.get("schema_version") == 1, "unsupported browser evidence schema")
    require(set(receipt) == set(binding) | {"schema_version", "operator", "observations"},
            "browser evidence has unknown or missing fields; omit secrets and personal data")
    for key in binding:
        require(receipt.get(key) == window.get(key) and receipt.get(key) is not None, f"browser evidence {key} mismatch")
    require(isinstance(receipt["operator"], str) and re.fullmatch(r"[A-Za-z0-9_-]{1,39}", receipt["operator"]),
            "operator must be a GitHub login")
    observations = receipt["observations"]
    require(isinstance(observations, list) and len(observations) >= 2, "start and end browser observations are required")
    previous = None
    for index, observation in enumerate(observations):
        require(set(observation) == {"observed_at", "soccer", "google_calendar", "cookie", "authenticated_cache_control"},
                "browser observation has unknown or missing fields")
        at = timestamp(observation["observed_at"])
        require(start <= at <= end and (previous is None or at > previous), "browser observations must be ordered inside the window")
        require(previous is None or at - previous <= MAX_GAP, "authenticated browser observation gap exceeds 60 seconds")
        previous = at
        require(observation["soccer"] == "authorized_current_data", "real authorized Soccer data was not observed")
        expected_calendar = ("created_read_back_and_deleted_test_event" if index in (0, len(observations) - 1)
                             else "connected_calendar_read")
        require(observation["google_calendar"] == expected_calendar,
                "real Google Calendar operation was not observed")
        require(observation["cookie"] == {"secure": True, "http_only": True, "same_site": "Lax", "path": "/soccer"},
                "invalid Soccer cookie attributes")
        cache_control = observation["authenticated_cache_control"]
        require(isinstance(cache_control, str), "missing authenticated cache policy")
        directives = {part.strip().lower() for part in cache_control.split(",")}
        require("no-store" in directives and "public" not in directives and
                not any(part.startswith(("s-maxage=", "stale-while-revalidate=", "stale-if-error=")) or
                        (part.startswith("max-age=") and part != "max-age=0") for part in directives),
                "authenticated response must not be cached")
    require(timestamp(observations[0]["observed_at"]) - start <= MAX_GAP and
            end - timestamp(observations[-1]["observed_at"]) <= MAX_GAP,
            "browser evidence does not cover start and end of the window")
    return {"schema_version": 1, **{key: receipt[key] for key in binding},
            "operator": receipt["operator"], "browser_contract": "passed",
            "provenance": "pending", "status": "APPLIED_NOT_VERIFIED"}


def observe():
    config = {name: os.environ[name] for name in ("SOURCE_SHA", "IMAGE_DIGEST", "PROMOTION_SHA",
              "PRODUCTION_DEPLOYMENT_ID", "API_ID", "APEX_ORIGIN_HOST", "EVIDENCE_DIR")}
    for name in ("SOURCE_SHA", "PROMOTION_SHA"):
        require(re.fullmatch(r"[a-f0-9]{40}", config[name]), f"invalid {name}")
    require(re.fullmatch(r"sha256:[a-f0-9]{64}", config["IMAGE_DIGEST"]), "invalid IMAGE_DIGEST")
    require(re.fullmatch(r"[1-9][0-9]*", config["PRODUCTION_DEPLOYMENT_ID"]), "invalid deployment ID")
    require(re.fullmatch(r"[a-z0-9]+", config["API_ID"]), "invalid API ID")
    require(re.fullmatch(r"[a-z0-9.-]+\.execute-api\.[a-z0-9-]+\.amazonaws\.com", config["APEX_ORIGIN_HOST"]), "invalid origin host")
    directory = Path(config["EVIDENCE_DIR"])
    require(directory.is_absolute(), "EVIDENCE_DIR must be absolute")
    directory.mkdir(parents=True, exist_ok=True)
    # Never reuse evidence after failure or retry, even if the old run passed.
    marker = directory / "window.json"
    with marker.open("x") as stream:
        identity = {"schema_version": 1, "window_id": window_id(config["PROMOTION_SHA"], config["PRODUCTION_DEPLOYMENT_ID"]),
                    "production_deployment_id": config["PRODUCTION_DEPLOYMENT_ID"],
                    "promotion_sha": config["PROMOTION_SHA"], "source_sha": config["SOURCE_SHA"],
                    "image_digest": config["IMAGE_DIGEST"], "base_url": APEX,
                    "status": "APPLIED_NOT_VERIFIED", "api_id": config["API_ID"],
                    "origin_host": config["APEX_ORIGIN_HOST"]}
        json.dump(identity, stream)
    try:
        duration = int(os.environ.get("PRODUCTION_WINDOW_SECONDS", "2100"))
        interval = int(os.environ.get("PRODUCTION_INTERVAL_SECONDS", "30"))
        require(MIN_WINDOW <= duration <= 3600 and 1 <= interval <= 30, "invalid production window or interval")
        public_probe(config["SOURCE_SHA"], config["APEX_ORIGIN_HOST"])
        def sample(start, end):
            public_probe(config["SOURCE_SHA"], config["APEX_ORIGIN_HOST"])
            return {"lambda_version": identity_probe(config["IMAGE_DIGEST"]),
                    "error_signals": telemetry_probe(start - 60, max(end, start + 1), config["API_ID"])}
        with (directory / "observations.jsonl").open("x") as output:
            def append(item):
                output.write(json.dumps(item) + "\n")
                output.flush()
                if not append.started:
                    print(json.dumps({"ci_origin_window_id": identity["window_id"],
                                      "first_complete_sample_at": item["observed_at"]}), flush=True)
                    append.started = True
            append.started = False
            observations = observe_window(sample, duration, interval, on_sample=append)
        result = dict(identity, ci_origin_window="passed",
                      started_at=observations[0]["observed_at"], ended_at=observations[-1]["observed_at"],
                      lambda_version=observations[0]["lambda_version"], observations=observations,
                      browser_evidence="pending", provenance="pending",
                      metric_coverage="pending_final_ingestion_check")
        save(directory / "ci-origin-window.json", result)
        print("CI origin/AWS window passed; operator public HTTP, browser evidence, final metric coverage, and protected provenance remain pending")
    except Exception:
        save(directory / "window-failed.json", dict(identity, ci_origin_window="failed"))
        raise


def window_id(promotion, deployment):
    return str(uuid.uuid5(uuid.NAMESPACE_URL, f"{APEX}/production/{promotion}/{deployment}"))


def validate_binding(value):
    require(value.get("base_url") == APEX, "public base URL must be canonical")
    for key in ("promotion_sha", "source_sha"):
        require(isinstance(value.get(key), str) and re.fullmatch(r"[a-f0-9]{40}", value[key]), "invalid public source binding")
    require(re.fullmatch(r"sha256:[a-f0-9]{64}", value.get("image_digest", "")), "invalid public digest binding")
    for key in ("production_deployment_id", "lambda_version"):
        require(isinstance(value.get(key), str) and re.fullmatch(r"[1-9][0-9]*", value[key]), "invalid public deployment binding")
    require(value.get("window_id") == window_id(value["promotion_sha"], value["production_deployment_id"]),
            "public window identifier differs from deployment")


def checked_operator(binding):
    validate_binding(binding)
    root = Path(__file__).resolve().parents[1]
    head = subprocess.run(["git", "-C", str(root), "rev-parse", "HEAD"], check=True,
                          capture_output=True, text=True).stdout.strip()
    require(head == binding["promotion_sha"], "collector checkout differs from promotion")
    subprocess.run(["git", "-C", str(root), "diff", "--exit-code", "HEAD", "--", "scripts/observe-lambda-production.py"],
                   check=True, capture_output=True)
    operator = subprocess.run(["gh", "api", "user", "--jq", ".login"], check=True,
                              capture_output=True, text=True, timeout=20).stdout.strip()
    require(operator == "CraigDevJohnson", "public collector requires the reviewed GitHub operator")
    return operator


def validate_public(window, receipt, now=None, require_fresh=True):
    now = time.time() if now is None else now
    start, end = validate_window(window, now)
    validate_binding(receipt)
    require(set(receipt) == set(BINDING) | {"schema_version", "operator", "collector_sha", "operator_public_window",
            "started_at", "ended_at", "interval_seconds", "observations", "fresh_public_read"},
            "public receipt has unknown or missing fields")
    require(receipt["schema_version"] == 1 and receipt["operator"] == "CraigDevJohnson"
            and receipt["collector_sha"] == window["promotion_sha"]
            and receipt["operator_public_window"] == "passed", "public collector contract did not pass")
    require(all(receipt[key] == window[key] for key in BINDING), "public receipt binding mismatch")
    require(type(receipt["interval_seconds"]) is int and 1 <= receipt["interval_seconds"] <= 30,
            "invalid public sampling interval")
    # lambda_version in these samples is a coordinate, not an operator AWS read.
    for sample in receipt["observations"]:
        require(set(sample) == {"observed_at", "elapsed_seconds", "lambda_version", "checks"}
                and sample["checks"] == PUBLIC_CHECKS, "incomplete public HTTP sample")
    public_start, public_end = validate_window(dict(receipt, ci_origin_window="passed"), now)
    overlap_start, overlap_end = max(start, public_start), min(end, public_end)
    require(overlap_end - overlap_start >= MIN_WINDOW, "public and CI windows lack 1800 seconds common coverage")
    fresh = receipt["fresh_public_read"]
    require(set(fresh) == {"observed_at", "duration_seconds", "checks", "binding_sha256"}
            and fresh["binding_sha256"] == binding_digest(receipt)
            and fresh["checks"] == PUBLIC_CHECKS and type(fresh["duration_seconds"]) in (int, float)
            and math.isfinite(fresh["duration_seconds"]) and 0 <= fresh["duration_seconds"] <= MAX_GAP,
            "invalid fresh public read")
    at = timestamp(fresh["observed_at"])
    require(public_end <= at <= now and (not require_fresh or now - at <= PUBLIC_FRESHNESS),
            "fresh public read is stale or future")
    return {"started_at": utc(overlap_start), "ended_at": utc(overlap_end)}


def binding_digest(binding):
    return hashlib.sha256(json.dumps({key: binding[key] for key in BINDING},
                                    sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def public_read(binding):
    before, started = time.monotonic(), time.time()
    checks = public_probe(binding["source_sha"])  # Public DNS/TLS only, without AWS or origin overrides.
    elapsed, ended = time.monotonic() - before, time.time()
    require(0 <= elapsed <= MAX_GAP and abs((ended - started) - elapsed) <= 2,
            "public read timing changed")
    return {"observed_at": utc(ended), "duration_seconds": elapsed, "checks": checks,
            "binding_sha256": binding_digest(binding)}


def observe_public(binding, output, duration=2100, interval=30):
    binding = dict(binding)
    binding.setdefault("window_id", window_id(binding["promotion_sha"], binding["production_deployment_id"]))
    require(set(binding) == set(BINDING), "unknown or missing public binding fields")
    operator = checked_operator(binding)
    require(not output.exists(), "public output already exists")
    # Preserve failed attempts without a success-shaped receipt; retry in a new file.
    with output.open("x") as stream:
        stream.write('{}\n')
    def sample(start, end):
        return {"lambda_version": binding["lambda_version"], "checks": public_probe(binding["source_sha"])}
    first_sample = True
    def announce(item):
        nonlocal first_sample
        if first_sample:
            print(json.dumps({"operator_public_window_id": binding["window_id"],
                              "first_complete_sample_at": item["observed_at"]}), flush=True)
            first_sample = False
    observations = observe_window(sample, duration, interval, on_sample=announce)
    result = {**{key: binding[key] for key in BINDING}, "schema_version": 1,
              "operator": operator, "collector_sha": binding["promotion_sha"],
              "operator_public_window": "passed", "interval_seconds": interval,
              "started_at": observations[0]["observed_at"], "ended_at": observations[-1]["observed_at"],
              "observations": observations, "fresh_public_read": public_read(binding)}
    save(output, result)


def refresh_public(window, receipt, output):
    checked_operator(receipt)
    validate_public(window, receipt, require_fresh=False)
    require(not output.exists(), "public refresh output already exists")
    receipt = dict(receipt, fresh_public_read=public_read(receipt))
    validate_public(window, receipt)
    with output.open("x") as stream:
        json.dump(receipt, stream, separators=(",", ":"))
        stream.write("\n")


def dispatch_inputs(window, public, browser, apply_run, output):
    require(re.fullmatch(r"[1-9][0-9]*", apply_run), "invalid apply run ID")
    coverage = validate_public(window, public)
    validate_browser(window, browser, coverage=coverage)
    compact = lambda value: json.dumps(value, separators=(",", ":"))
    payload = {"ref": "main", "inputs": {"apply_run_id": apply_run,
               "public_receipt_json": compact(public), "browser_receipt_json": compact(browser)}}
    raw = compact(payload)
    require(len(raw.encode()) <= 60000, "combined acceptance inputs exceed the conservative dispatch limit")
    with output.open("x") as stream:
        stream.write(raw + "\n")


def validate_public_before_post(directory):
    result = json.loads((directory / "production-verification.json").read_text())
    window_path, receipt_path = directory / "ci-origin-window.json", directory / "public-receipt.json"
    for path, field in ((window_path, "ci_origin_window_sha256"), (receipt_path, "public_receipt_sha256")):
        require(hashlib.sha256(path.read_bytes()).hexdigest() == result[field], "public evidence changed before success POST")
    window, receipt = json.loads(window_path.read_text()), json.loads(receipt_path.read_text())
    for key in BINDING:
        if key != "base_url":
            require(str(result[key]) == receipt[key], "public result binding changed before success POST")
    validate_public(window, receipt)


def main():
    if sys.argv[1:] == ["observe"]:
        observe()
    elif len(sys.argv) in (4, 5) and sys.argv[1] == "observe-public":
        observe_public(json.loads(Path(sys.argv[2]).read_text()), Path(sys.argv[3]),
                       duration=int(sys.argv[4]) if len(sys.argv) == 5 else 2100)
    elif len(sys.argv) == 3 and sys.argv[1] == "check-public-before-post":
        validate_public_before_post(Path(sys.argv[2]))
    elif len(sys.argv) == 5 and sys.argv[1] == "refresh-public":
        refresh_public(json.loads(Path(sys.argv[2]).read_text()), json.loads(Path(sys.argv[3]).read_text()), Path(sys.argv[4]))
    elif len(sys.argv) == 7 and sys.argv[1] == "dispatch-inputs":
        dispatch_inputs(*(json.loads(Path(path).read_text()) for path in sys.argv[2:5]), sys.argv[5], Path(sys.argv[6]))
    elif len(sys.argv) == 4 and sys.argv[1] == "final-metrics":
        window = json.loads(Path(sys.argv[2]).read_text())
        save(Path(sys.argv[3]), final_metrics(window))
    elif len(sys.argv) == 5 and sys.argv[1] == "validate-browser":
        window = json.loads(Path(sys.argv[2]).read_text())
        receipt = json.loads(Path(sys.argv[3]).read_text())
        save(Path(sys.argv[4]), validate_browser(window, receipt))
    else:
        raise ValueError("usage: observe-lambda-production.py observe | validate-browser WINDOW RECEIPT OUTPUT | final-metrics WINDOW OUTPUT | observe-public BINDING OUTPUT [SECONDS] | refresh-public WINDOW RECEIPT OUTPUT | dispatch-inputs WINDOW PUBLIC BROWSER APPLY_RUN OUTPUT")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        # Do not include command outputs or browser-supplied strings in diagnostics.
        print("Production observation failed: " + (str(error) if isinstance(error, ValueError) else type(error).__name__), file=sys.stderr)
        sys.exit(1)
