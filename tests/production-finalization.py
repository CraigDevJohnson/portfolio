#!/usr/bin/env python3
"""Test acceptance provenance and safe immutable-artifact handling."""

import copy
from datetime import datetime, timezone
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import stat
import tempfile
import time
import unittest
from unittest import mock
import zipfile

spec = importlib.util.spec_from_file_location(
    "finalization", Path(__file__).resolve().parents[1] / "scripts/finalize-production.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FinalizationTest(unittest.TestCase):
    def setUp(self):
        self.source = "a" * 40
        self.context = {"GITHUB_SHA": self.source, "GITHUB_RUN_ID": "22", "APPLY_RUN_ID": "11",
                        "GITHUB_REPOSITORY": module.REPOSITORY, "GITHUB_EVENT_NAME": "workflow_dispatch",
                        "GITHUB_REF": "refs/heads/main", "GITHUB_RUN_ATTEMPT": "1"}
        reviewer = copy.deepcopy(module.REVIEWER)
        self.responses = {
            "branches/main": {"protected": True, "commit": {"sha": self.source}},
            "environments/production": {
                "id": 33, "can_admins_bypass": False,
                "deployment_branch_policy": {"protected_branches": True, "custom_branch_policies": False},
                "protection_rules": [{"type": "required_reviewers", "prevent_self_review": False,
                                      "reviewers": [{"type": "User", "reviewer": reviewer}]}],
            },
            "actions/runs/11/artifacts?per_page=100": {
                "total_count": 1, "artifacts": [{"id": 44, "name": f"production-evidence-{self.source}",
                                                "expired": False, "digest": "sha256:" + "b" * 64}]},
        }
        for run_id, path, event, status, conclusion in (
            (22, "production-acceptance.yml", "workflow_dispatch", "in_progress", None),
            (11, "release.yml", "workflow_run", "completed", "success"),
        ):
            self.responses[f"actions/runs/{run_id}/attempts/1"] = {
                "id": run_id, "run_attempt": 1, "head_sha": self.source, "head_branch": "main",
                "path": ".github/workflows/" + path, "event": event, "status": status, "conclusion": conclusion,
                "actor": copy.deepcopy(reviewer), "repository": {"full_name": module.REPOSITORY},
                "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "head_repository": {"full_name": module.REPOSITORY},
            }
            self.responses[f"actions/runs/{run_id}/approvals"] = [{
                "state": "approved", "user": copy.deepcopy(reviewer),
                "environments": [{"id": 33, "name": "production"}]}]

    def provenance(self):
        return module.provenance(self.context, lambda endpoint: copy.deepcopy(self.responses[endpoint]))

    def test_exact_protected_run_selects_one_artifact(self):
        self.assertEqual(self.provenance()["id"], 44)

    def test_untrusted_actor_cannot_attest_browser_actions(self):
        self.responses["actions/runs/22/attempts/1"]["actor"]["id"] = 99
        with self.assertRaisesRegex(ValueError, "submitted by Craig"):
            self.provenance()

    def test_failed_or_manual_original_release_is_rejected(self):
        for field, value in (("conclusion", "failure"), ("event", "workflow_dispatch"),
                             ("run_attempt", 2), ("path", ".github/workflows/other.yml")):
            with self.subTest(field=field):
                original = copy.deepcopy(self.responses["actions/runs/11/attempts/1"])
                self.responses["actions/runs/11/attempts/1"][field] = value
                with self.assertRaises(ValueError):
                    self.provenance()
                self.responses["actions/runs/11/attempts/1"] = original

    def test_current_main_must_still_match(self):
        self.responses["branches/main"]["commit"]["sha"] = "c" * 40
        with self.assertRaisesRegex(ValueError, "current protected main"):
            self.provenance()

    def test_both_runs_require_real_protected_approval(self):
        for run_id in (11, 22):
            key = f"actions/runs/{run_id}/approvals"
            original = self.responses[key]
            self.responses[key] = []
            with self.assertRaisesRegex(ValueError, "protected production approval"):
                self.provenance()
            self.responses[key] = original

    def test_admin_bypass_is_not_approval(self):
        self.responses["environments/production"]["can_admins_bypass"] = True
        with self.assertRaisesRegex(ValueError, "protection changed"):
            self.provenance()

    def test_duplicate_or_expired_artifacts_are_rejected(self):
        data = self.responses["actions/runs/11/artifacts?per_page=100"]
        data["artifacts"][0]["expired"] = True
        with self.assertRaises(ValueError):
            self.provenance()
        data["artifacts"][0]["expired"] = False
        data["artifacts"].append(copy.deepcopy(data["artifacts"][0]))
        with self.assertRaises(ValueError):
            self.provenance()

    def test_artifact_substitution_and_unsafe_entries(self):
        for name, mode, wrong_digest in (("plan.json", 0, True), ("../escaped", 0, False),
                                        ("/absolute", 0, False), ("symlink", stat.S_IFLNK, False)):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, "w") as bundle:
                    info = zipfile.ZipInfo(name)
                    info.external_attr = mode << 16
                    bundle.writestr(info, "not trustworthy")
                archive = buffer.getvalue()
                expected = "sha256:" + ("0" * 64 if wrong_digest else hashlib.sha256(archive).hexdigest())
                target = Path(temporary) / "evidence"
                with self.assertRaises(ValueError):
                    module.unpack(archive, expected, target)
                self.assertFalse(target.exists())

    def test_safe_artifact_is_extracted_once(self):
        with tempfile.TemporaryDirectory() as temporary:
            buffer = io.BytesIO()
            with zipfile.ZipFile(buffer, "w") as bundle:
                bundle.writestr("plan.json", json.dumps({"reviewed": True}))
            archive = buffer.getvalue()
            expected = "sha256:" + hashlib.sha256(archive).hexdigest()
            target = Path(temporary) / "evidence"
            module.unpack(archive, expected, target)
            self.assertEqual(json.loads((target / "plan.json").read_text()), {"reviewed": True})
            with self.assertRaisesRegex(ValueError, "already exists"):
                module.unpack(archive, expected, target)

    def evidence_bundle(self, directory):
        observer = module.observer_module()
        start = time.time() - 2400
        identity = {
            "promotion_sha": self.source, "planning_run_id": "11", "planning_run_attempt": "1",
            "development_source_sha": "d" * 40, "development_deployment_id": 90,
            "image_digest": "sha256:" + "e" * 64,
            "plan_sha256": hashlib.sha256(b"plan").hexdigest(),
            "scan_sha256": hashlib.sha256(b"scan").hexdigest(),
        }
        (directory / "prod.tfplan").write_bytes(b"plan")
        (directory / "scan.json").write_bytes(b"scan")
        (directory / "release-identity.json").write_text(json.dumps(identity))
        deployment = dict(identity, source_sha=self.source, production_deployment_id=91,
                          status="in_progress", status_recorded=True, approval_id="approved-11",
                          reviewer_login="CraigDevJohnson",
                          release_identity_sha256=module.digest(directory / "release-identity.json"))
        (directory / "github-production-deployment.json").write_text(json.dumps(deployment))
        (directory / "approval.json").write_text(json.dumps({"approval_id": "approved-11"}))
        window = {
            "schema_version": 1, "window_id": observer.window_id(self.source, "91"), "production_deployment_id": "91",
            "promotion_sha": self.source, "source_sha": identity["development_source_sha"],
            "image_digest": identity["image_digest"], "lambda_version": "8",
            "base_url": "https://craigdevjohnson.com", "ci_origin_window": "passed",
            "started_at": observer.utc(start), "ended_at": observer.utc(start + 1800),
            "observations": [{"observed_at": observer.utc(start + step * 30),
                              "elapsed_seconds": step * 30, "lambda_version": "8"}
                             for step in range(61)],
        }
        (directory / "ci-origin-window.json").write_text(json.dumps(window))
        receipt = {key: window[key] for key in (
            "schema_version", "window_id", "production_deployment_id", "promotion_sha", "source_sha",
            "image_digest", "lambda_version", "base_url")}
        receipt.update(operator="CraigDevJohnson", observations=[{
            "observed_at": observer.utc(start + step * 30), "soccer": "authorized_current_data",
            "google_calendar": ("created_read_back_and_deleted_test_event" if step in (0, 60)
                                else "connected_calendar_read"),
            "cookie": {"secure": True, "http_only": True, "same_site": "Lax", "path": "/soccer"},
            "authenticated_cache_control": "no-store, max-age=0",
        } for step in range(61)])
        public = {key: window[key] for key in observer.BINDING}
        public.update(schema_version=1, operator="CraigDevJohnson", collector_sha=self.source,
                      operator_public_window="passed", started_at=window["started_at"], ended_at=window["ended_at"],
                      interval_seconds=30, observations=[dict(item, checks=observer.PUBLIC_CHECKS)
                                                        for item in window["observations"]],
                      fresh_public_read={"observed_at": observer.utc(time.time()), "duration_seconds": 1,
                                         "checks": observer.PUBLIC_CHECKS, "binding_sha256": observer.binding_digest(window)})
        self.public = public
        return observer, receipt

    def test_finalization_records_original_deployment_only_after_actual_contract_checks(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            observer, receipt = self.evidence_bundle(directory)
            context = dict(self.context, EVIDENCE_DIR=str(directory), BROWSER_RECEIPT_JSON=json.dumps(receipt, separators=(",", ":")),
                           PUBLIC_RECEIPT_JSON=json.dumps(self.public, separators=(",", ":")))
            with mock.patch.dict(module.os.environ, context, clear=True), \
                    mock.patch.object(module.sys, "argv", ["finalize-production.py", "finalize"]), \
                    mock.patch.object(module, "provenance", return_value={"id": 44}), \
                    mock.patch.object(module, "observer_module", return_value=observer), \
                    mock.patch.object(observer, "final_metrics", return_value={"metric_coverage": "passed"}), \
                    mock.patch.object(module.subprocess, "run") as record:
                module.main()
            record.assert_called_once()
            self.assertEqual(record.call_args.kwargs["env"]["DEPLOYMENT_STATE"], "success")
            result = json.loads((directory / "production-verification.json").read_text())
            self.assertEqual(result["production_deployment_id"], 91)
            self.assertEqual(result["acceptance_run_id"], "22")
            self.assertEqual(result["browser_receipt_sha256"], module.digest(directory / "browser-receipt.json"))

    def test_invalid_public_receipts_never_record_success(self):
        for failure in ("missing", "route", "stale", "binding", "oversized"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                directory = Path(temporary)
                observer, receipt = self.evidence_bundle(directory)
                public = copy.deepcopy(self.public)
                if failure == "missing": public = {}
                if failure == "route": public["observations"][-1]["checks"] = []
                if failure == "stale": public["fresh_public_read"]["observed_at"] = observer.utc(time.time() - 301)
                if failure == "binding": public["image_digest"] = "sha256:" + "0" * 64
                if failure == "oversized": public["padding"] = "x" * 60000
                context = dict(self.context, EVIDENCE_DIR=str(directory), BROWSER_RECEIPT_JSON=json.dumps(receipt),
                               PUBLIC_RECEIPT_JSON=json.dumps(public))
                with mock.patch.dict(module.os.environ, context, clear=True), \
                        mock.patch.object(module.sys, "argv", ["finalize-production.py", "finalize"]), \
                        mock.patch.object(module, "provenance", return_value={"id": 44}), \
                        mock.patch.object(module, "observer_module", return_value=observer), \
                        mock.patch.object(observer, "final_metrics") as metrics, \
                        mock.patch.object(module.subprocess, "run") as record:
                    with self.assertRaises((ValueError, KeyError)):
                        module.main()
                record.assert_not_called()
                metrics.assert_not_called()
                self.assertFalse((directory / "production-verification.json").exists())

    def test_public_freshness_is_rechecked_after_final_remote_checks(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            observer, receipt = self.evidence_bundle(directory)
            context = dict(self.context, EVIDENCE_DIR=str(directory), BROWSER_RECEIPT_JSON=json.dumps(receipt),
                           PUBLIC_RECEIPT_JSON=json.dumps(self.public))
            real_validate = observer.validate_public
            calls = []
            def validate(*args):
                calls.append(True)
                if len(calls) == 2:
                    raise ValueError("fresh public read is stale")
                return real_validate(*args)
            with mock.patch.dict(module.os.environ, context, clear=True), \
                    mock.patch.object(module.sys, "argv", ["finalize-production.py", "finalize"]), \
                    mock.patch.object(module, "provenance", return_value={"id": 44}), \
                    mock.patch.object(module, "observer_module", return_value=observer), \
                    mock.patch.object(observer, "validate_public", side_effect=validate), \
                    mock.patch.object(observer, "final_metrics", return_value={}), \
                    mock.patch.object(module.subprocess, "run") as record:
                with self.assertRaisesRegex(ValueError, "stale"):
                    module.main()
            record.assert_not_called()
            self.assertFalse((directory / "production-verification.json").exists())

    def test_failed_browser_journey_never_records_success(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            observer, receipt = self.evidence_bundle(directory)
            receipt["observations"][-1]["soccer"] = "session_expired"
            context = dict(self.context, EVIDENCE_DIR=str(directory), BROWSER_RECEIPT_JSON=json.dumps(receipt, separators=(",", ":")),
                           PUBLIC_RECEIPT_JSON=json.dumps(self.public, separators=(",", ":")))
            with mock.patch.dict(module.os.environ, context, clear=True), \
                    mock.patch.object(module.sys, "argv", ["finalize-production.py", "finalize"]), \
                    mock.patch.object(module, "provenance", return_value={"id": 44}), \
                    mock.patch.object(module, "observer_module", return_value=observer), \
                    mock.patch.object(module.subprocess, "run") as record:
                with self.assertRaisesRegex(ValueError, "Soccer data"):
                    module.main()
            record.assert_not_called()
            self.assertFalse((directory / "production-verification.json").exists())


if __name__ == "__main__":
    unittest.main()
