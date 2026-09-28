#!/usr/bin/env python3
"""Offline contract tests; fixtures never count as production acceptance."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("production", Path(__file__).resolve().parents[1] / "scripts/observe-lambda-production.py")
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)


class Clock:
    def __init__(self):
        self.seconds = 0

    def now(self):
        return self.seconds

    def wall(self):
        return 1800000000 + self.seconds

    def sleep(self, seconds):
        self.seconds += seconds


class ProductionObservation(unittest.TestCase):
    def setUp(self):
        route = patch.object(p, "route_probe", return_value={"state": "ENABLED"})
        route.start()
        self.addCleanup(route.stop)

    def run_window(self, sample=None, duration=1800, interval=30, clock=None):
        clock = clock or Clock()
        return p.observe_window(sample or (lambda start, end: {"lambda_version": "8"}),
                                duration, interval, clock.now, clock.wall, clock.sleep)

    def test_exact_minimum_and_no_smoke_shortcut(self):
        observations = self.run_window()
        self.assertEqual(len(observations), 61)
        self.assertEqual(observations[-1]["elapsed_seconds"], 1800)
        for duration, interval in ((1799, 30), (1800, 0), (1800, 31), (1800, -1)):
            with self.subTest(duration=duration, interval=interval), self.assertRaises(ValueError):
                self.run_window(duration=duration, interval=interval)

    def test_first_last_and_penultimate_failure_invalidate_window(self):
        for failure in (0, 1799, 1800):
            clock = Clock()
            def sample(start, end):
                if clock.seconds == failure:
                    raise ValueError("required probe failed")
                return {"lambda_version": "8"}
            with self.subTest(failure=failure), self.assertRaises(ValueError):
                self.run_window(sample, interval=1, clock=clock)
        self.assertEqual(self.run_window()[-1]["elapsed_seconds"], 1800)

    def test_alias_change_and_observation_gap(self):
        clock = Clock()
        with self.assertRaisesRegex(ValueError, "alias changed"):
            self.run_window(lambda start, end: {"lambda_version": "9" if clock.seconds else "8"}, clock=clock)
        clock = Clock()
        def slow(start, end):
            clock.seconds += 61
            return {"lambda_version": "8"}
        with self.assertRaisesRegex(ValueError, "maximum gap"):
            self.run_window(slow, clock=clock)
        clock = Clock()
        original_sleep = clock.sleep
        clock.sleep = lambda seconds: original_sleep(61)
        with self.assertRaisesRegex(ValueError, "gap exceeds"):
            self.run_window(clock=clock)

    def test_wall_clock_manipulation(self):
        clock = Clock()
        clock.wall = lambda: 1800000000 + clock.seconds * 2
        with self.assertRaisesRegex(ValueError, "wall clock changed"):
            self.run_window(clock=clock)

    def test_slow_first_probe_not_counted_toward_window(self):
        clock = Clock()
        def sample(start, end):
            clock.seconds += 1
            return {"lambda_version": "8"}
        samples = self.run_window(sample, clock=clock)
        self.assertGreaterEqual(samples[-1]["elapsed_seconds"] - samples[0]["elapsed_seconds"], 1800)

    def fake_fetch(self, base, route, origin=""):
        if base == p.WWW:
            return 308, {"location": p.APEX + route}, b""
        if route == "/healthz":
            return 200, {"content-type": "application/json", "cache-control": "no-store"}, b'{"status":"ok","revision":"abc"}'
        if route.endswith(".css"):
            return 200, {"content-type": "text/css"}, b"body{}"
        if route.endswith(".jpg"):
            return 200, {"content-type": "image/jpeg"}, b"\xff\xd8\xff"
        return 200, {"content-type": "text/html", "cache-control": "no-store"}, b"<!doctype html><html></html>"

    def test_real_routes_assets_and_permanent_redirect(self):
        with patch.object(p, "fetch", self.fake_fetch):
            p.public_probe("abc")
        for failure in ("temporary", "query-lost", "wrong-host", "missing", "wrong-revision", "fake-asset"):
            def fetch(base, route, origin=""):
                status, headers, body = self.fake_fetch(base, route, origin)
                if base == p.WWW:
                    if failure == "temporary": status = 302
                    if failure == "query-lost": headers["location"] = p.APEX + route.split("?")[0]
                    if failure == "wrong-host": headers["location"] = "https://example.com" + route
                    if failure == "missing": headers = {}
                if failure == "wrong-revision" and route == "/healthz": body = b'{"status":"ok","revision":"wrong"}'
                if failure == "fake-asset" and route.endswith(".jpg"): body = b"<html>"
                return status, headers, body
            with self.subTest(failure=failure), patch.object(p, "fetch", fetch), self.assertRaises(ValueError):
                p.public_probe("abc")

    def test_alarm_and_error_signal_failure(self):
        alarms = [{"AlarmName": p.FUNCTION + "-" + name, "StateValue": "OK"} for name in p.SUFFIXES]
        def cli(*args):
            return {"MetricAlarms": alarms} if args[1] == "describe-alarms" else {"Datapoints": [{"Sum": 0, "Timestamp": p.utc(0)}]}
        with patch.object(p, "aws", cli):
            self.assertEqual(p.telemetry_probe(0, 60, "api")["Errors"]["observed_sum"], 0)
            alarms[0]["StateValue"] = "ALARM"
            with self.assertRaises(ValueError): p.telemetry_probe(0, 60, "api")
            alarms[0]["StateValue"] = "OK"
        def error_cli(*args):
            return {"MetricAlarms": alarms} if args[1] == "describe-alarms" else {"Datapoints": [{"Sum": 1}]}
        with patch.object(p, "aws", error_cli), self.assertRaises(ValueError):
            p.telemetry_probe(0, 60, "api")
        def absent_cli(*args):
            return {"MetricAlarms": alarms} if args[1] == "describe-alarms" else {"Datapoints": []}
        with patch.object(p, "aws", absent_cli):
            self.assertIsNone(p.telemetry_probe(0, 60, "api")["Errors"]["observed_sum"])

    def test_identity_digest_and_weighted_alias(self):
        alias = {"FunctionVersion": "8"}
        code = {"Code": {"ResolvedImageUri": "repo@sha256:abc"}}
        with patch.object(p, "aws", side_effect=lambda *args: alias if args[1] == "get-alias" else code):
            self.assertEqual(p.identity_probe("sha256:abc"), "8")
            with self.assertRaises(ValueError): p.identity_probe("sha256:bad")
            alias["RoutingConfig"] = {"AdditionalVersionWeights": {"9": 0.1}}
            with self.assertRaises(ValueError): p.identity_probe("sha256:abc")

    def test_failed_observation_leaves_no_success_and_retry_needs_new_directory(self):
        with tempfile.TemporaryDirectory() as directory:
            config = {"SOURCE_SHA": "a" * 40, "PROMOTION_SHA": "b" * 40,
                      "IMAGE_DIGEST": "sha256:" + "c" * 64, "PRODUCTION_DEPLOYMENT_ID": "123",
                      "API_ID": "api", "APEX_ORIGIN_HOST": "api.execute-api.us-west-2.amazonaws.com",
                      "EVIDENCE_DIR": directory}
            with patch.dict(os.environ, config, clear=True), patch.object(p, "public_probe"), \
                    patch.object(p, "observe_window", side_effect=ValueError("late failure")):
                with self.assertRaises(ValueError): p.observe()
                self.assertTrue((Path(directory) / "window-failed.json").exists())
                self.assertFalse((Path(directory) / "ci-origin-window.json").exists())
                self.assertFalse((Path(directory) / "production-verification.json").exists())
                with self.assertRaises(FileExistsError): p.observe()

    def test_ci_first_complete_sample_is_flushed_for_operator_coordination(self):
        with tempfile.TemporaryDirectory() as directory:
            config = {"SOURCE_SHA": "a" * 40, "PROMOTION_SHA": "b" * 40,
                      "IMAGE_DIGEST": "sha256:" + "c" * 64, "PRODUCTION_DEPLOYMENT_ID": "123",
                      "API_ID": "api", "APEX_ORIGIN_HOST": "api.execute-api.us-west-2.amazonaws.com",
                      "EVIDENCE_DIR": directory}
            samples = self.run_window()
            def observe(*args, **kwargs):
                for item in samples:
                    kwargs["on_sample"](item)
                return samples
            with patch.dict(os.environ, config, clear=True), patch.object(p, "public_probe"), \
                    patch.object(p, "observe_window", side_effect=observe), patch("builtins.print") as printed:
                p.observe()
            first = printed.call_args_list[0]
            self.assertTrue(first.kwargs["flush"])
            self.assertEqual(json.loads(first.args[0]), {
                "ci_origin_window_id": p.window_id(config["PROMOTION_SHA"], "123"),
                "first_complete_sample_at": samples[0]["observed_at"]})
            self.assertEqual(len(printed.call_args_list), 2)

    def test_success_persists_unverified_window_without_finalizing(self):
        with tempfile.TemporaryDirectory() as directory:
            config = {"SOURCE_SHA": "a" * 40, "PROMOTION_SHA": "b" * 40,
                      "IMAGE_DIGEST": "sha256:" + "c" * 64, "PRODUCTION_DEPLOYMENT_ID": "123",
                      "API_ID": "api", "APEX_ORIGIN_HOST": "api.execute-api.us-west-2.amazonaws.com",
                      "EVIDENCE_DIR": directory}
            with patch.dict(os.environ, config, clear=True), patch.object(p, "public_probe"), \
                    patch.object(p, "observe_window", return_value=self.run_window()):
                p.observe()
                result = json.loads((Path(directory) / "ci-origin-window.json").read_text())
                self.assertEqual(result["status"], "APPLIED_NOT_VERIFIED")
                self.assertEqual(result["browser_evidence"], "pending")
                self.assertEqual(result["metric_coverage"], "pending_final_ingestion_check")
                self.assertFalse((Path(directory) / "production-verification.json").exists())


class BrowserEvidence(unittest.TestCase):
    def setUp(self):
        self.start = 1800000000
        self.window = {"schema_version": 1, "ci_origin_window": "passed", "window_id": "id",
                       "production_deployment_id": "123", "promotion_sha": "p", "source_sha": "s",
                       "image_digest": "d", "lambda_version": "8", "base_url": p.APEX,
                       "started_at": p.utc(self.start), "ended_at": p.utc(self.start + 1800),
                       "observations": [{"observed_at": p.utc(self.start + offset), "elapsed_seconds": offset,
                                         "lambda_version": "8"} for offset in range(0, 1801, 30)]}
        self.receipt = {key: self.window[key] for key in ("schema_version", "window_id", "production_deployment_id",
                        "promotion_sha", "source_sha", "image_digest", "lambda_version", "base_url")}
        self.receipt.update(operator="CraigDevJohnson", observations=[{
            "observed_at": p.utc(self.start + offset),
            "soccer": "authorized_current_data", "google_calendar": "created_read_back_and_deleted_test_event" if offset in (0, 1800) else "connected_calendar_read",
            "cookie": {"secure": True, "http_only": True, "same_site": "Lax", "path": "/soccer"},
            "authenticated_cache_control": "no-store"} for offset in range(0, 1801, 30)])

    def validate(self):
        return p.validate_browser(self.window, self.receipt, now=self.start + 1801)

    def test_receipt_remains_untrusted_pending_transport(self):
        result = self.validate()
        self.assertEqual(result["provenance"], "pending")
        self.assertEqual(result["status"], "APPLIED_NOT_VERIFIED")

    def test_cache_directive_parsing(self):
        for policy in ("no-store, max-age=0", "private, no-store", "NO-STORE, max-age=0"):
            self.receipt["observations"][0]["authenticated_cache_control"] = policy
            self.validate()
        for policy in ("no-store, public", "no-store, max-age=10", "no-store, s-maxage=0", "private"):
            self.receipt["observations"][0]["authenticated_cache_control"] = policy
            with self.assertRaises(ValueError): self.validate()

    def test_expired_auth_cookie_upstream_failure_and_bad_binding(self):
        for field, value in (("soccer", "expired"),
                             ("google_calendar", "provider_error"), ("cookie", {}),
                             ("authenticated_cache_control", "public")):
            with self.subTest(field=field):
                original = self.receipt["observations"][-1][field]
                self.receipt["observations"][-1][field] = value
                with self.assertRaises(ValueError): self.validate()
                self.receipt["observations"][-1][field] = original
        self.receipt["production_deployment_id"] = "999"
        with self.assertRaises(ValueError): self.validate()

    def test_missing_duplicate_reordered_future_gapped_timestamps(self):
        original = copy.deepcopy(self.receipt["observations"])
        mutations = [original[3:], original[:-3], original[:1] + original[4:],
                     [original[0], original[0]] + original[1:], list(reversed(original))]
        future = copy.deepcopy(original)
        future[-1]["observed_at"] = p.utc(self.start + 1802)
        mutations.append(future)
        for observations in mutations:
            self.receipt["observations"] = observations
            with self.assertRaises(ValueError): self.validate()

    def test_automated_timestamp_and_elapsed_tampering(self):
        original = copy.deepcopy(self.window["observations"])
        for field, value in (("observed_at", p.utc(self.start + 1700)), ("elapsed_seconds", 4000),
                             ("lambda_version", "9")):
            self.window["observations"] = copy.deepcopy(original)
            self.window["observations"][-1][field] = value
            with self.assertRaises(ValueError): self.validate()
        self.window["observations"] = original[:1] + original[4:]
        with self.assertRaises(ValueError): self.validate()

    def test_final_metric_coverage_includes_both_endpoints(self):
        self.window["api_id"] = "api"
        self.window["origin_host"] = "api.execute-api.us-west-2.amazonaws.com"
        periods = list(range(self.start, self.start + 1801, 60))
        signals = {metric: {"periods": periods, "observed_sum": 0} for metric in ("Errors", "Throttles", "5xx")}
        points = [{"Timestamp": p.utc(period), "Sum": 1} for period in periods]
        with patch.object(p, "aws", return_value={"Datapoints": points}), \
                patch.object(p.time, "time", return_value=self.start + 2000), \
                patch.object(p, "telemetry_probe", return_value=signals), \
                patch.object(p, "identity_probe", return_value="8"), patch.object(p, "public_probe"):
            self.assertEqual(p.final_metrics(self.window)["metric_coverage"], "passed")
            points.pop()
            with self.assertRaises(ValueError): p.final_metrics(self.window)
        with patch.object(p.time, "time", return_value=self.start + 1801), self.assertRaises(ValueError):
            p.final_metrics(self.window)

    def test_short_window_future_window_unknown_secret_field(self):
        self.window["ended_at"] = p.utc(self.start + 1799)
        with self.assertRaises(ValueError): self.validate()
        self.window["ended_at"] = p.utc(self.start + 1900)
        with self.assertRaises(ValueError): self.validate()
        self.window["ended_at"] = p.utc(self.start + 1800)
        self.receipt["token"] = "must-not-be-retained"
        with self.assertRaises(ValueError): self.validate()


if __name__ == "__main__":
    unittest.main()
