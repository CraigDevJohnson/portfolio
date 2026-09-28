#!/usr/bin/env python3
"""Exercise the recovery gate with real Git history and fake GitHub responses."""

import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location(
    "recovery", ROOT / "scripts/validate-development-recovery.py"
)
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.old_cwd = Path.cwd()
        os.chdir(self.repo)
        self.addCleanup(os.chdir, self.old_cwd)
        self.git("init", "-q")
        self.git("config", "user.email", "fixture@example.invalid")
        self.git("config", "user.name", "Recovery test")
        (self.repo / "deploy").mkdir()
        self.manifest = b'{"source_sha":"historical-only"}\n'
        (self.repo / "deploy/production-release.json").write_bytes(self.manifest)
        self.base = self.commit("verified development")
        (self.repo / "app.txt").write_text("updated application\n")
        self.anchor = self.commit("undeployed runtime backlog")
        (self.repo / "scripts").mkdir()
        (self.repo / "scripts/validate-development-recovery.py").write_text("# reviewed gate\n")
        self.source = self.commit("reviewed recovery")
        self.env = {
            "GITHUB_REPOSITORY": "CraigDevJohnson/portfolio",
            "GITHUB_EVENT_NAME": "workflow_dispatch",
            "GITHUB_REF": "refs/heads/main",
            "GITHUB_SHA": self.source,
            "GITHUB_RUN_ID": "901",
            "GITHUB_RUN_ATTEMPT": "1",
        }
        self.ci = {
            "id": 800, "workflow_id": 340100411, "run_attempt": 1,
            "head_sha": self.source, "head_branch": "main", "event": "push",
            "path": ".github/workflows/ci.yml", "status": "completed",
            "conclusion": "success", "created_at": "2026-09-25T12:00:00Z",
            "repository": {"full_name": self.env["GITHUB_REPOSITORY"]},
            "head_repository": {"full_name": self.env["GITHUB_REPOSITORY"]},
        }
        self.run = dict(self.ci, id=901, workflow_id=346157322,
                        event="workflow_dispatch", path=".github/workflows/release.yml",
                        status="in_progress", conclusion=None)
        self.approvals = [{
            "state": "approved",
            "user": {"login": "CraigDevJohnson", "id": 42454849, "type": "User"},
            "environments": [{"name": "release-review"}],
        }]
        self.environment = {
            "name": "release-review", "can_admins_bypass": False,
            "deployment_branch_policy": {"protected_branches": True, "custom_branch_policies": False},
            "protection_rules": [{"type": "required_reviewers", "prevent_self_review": False,
                                  "reviewers": [{"type": "User", "reviewer": {
                                      "login": "CraigDevJohnson", "id": 42454849}}]},
                                 {"type": "branch_policy"}],
        }
        self.main = self.source
        self.development = self.base
        self.pull_base = self.anchor
        self.latest_ci = [self.ci]
        self.run_reads = []
        self.constants = patch.multiple(
            recovery, DEVELOPMENT_BASE=self.base, BACKLOG_ANCHOR=self.anchor,
            MANIFEST_SHA256=hashlib.sha256(self.manifest).hexdigest(),
        )
        self.constants.start()
        self.addCleanup(self.constants.stop)
        self.now = patch.object(recovery, "now_epoch", return_value=recovery.timestamp("2026-09-25T12:05:00Z"))
        self.now.start()
        self.addCleanup(self.now.stop)

    def git(self, *args):
        return subprocess.check_output(["git", *args], text=True).strip()

    def commit(self, message):
        self.git("add", ".")
        self.git("commit", "-qm", message)
        return self.git("rev-parse", "HEAD")

    def github(self, endpoint, paginate=False):
        if endpoint == "commits/main":
            return {"sha": self.main}
        if endpoint.startswith("commits/"):
            return [{"state": "closed", "merged_at": "2026-09-25T12:00:00Z",
                     "merge_commit_sha": self.source,
                     "base": {"ref": "main", "sha": self.pull_base}}]
        if endpoint.startswith("actions/workflows/ci.yml/runs?"):
            return [{"workflow_runs": copy.deepcopy(self.latest_ci)}]
        if endpoint.endswith("/approvals"):
            return copy.deepcopy(self.approvals)
        if endpoint.startswith("actions/runs/"):
            self.run_reads.append(endpoint)
            return copy.deepcopy(self.run)
        if endpoint == "environments/release-review":
            return copy.deepcopy(self.environment)
        raise AssertionError(f"Unexpected GitHub read: {endpoint}")

    def validate(self, phase="preflight"):
        with patch.dict(os.environ, self.env), \
             patch.object(recovery, "github", side_effect=self.github), \
             patch.object(recovery, "development_base", return_value=self.development):
            return recovery.validate(self.source, phase, "901")

    def test_reviewed_pending_backlog_is_eligible_but_has_no_aws_authority(self):
        self.approvals = []
        result = self.validate()
        self.assertEqual(result["classification"], "development-reviewed")
        self.assertEqual(result["source_sha"], self.source)
        self.assertEqual(result["ci_run_id"], 800)
        self.assertFalse(result["approval_verified"])

    def test_approved_recovery_reuses_same_source(self):
        self.assertTrue(self.validate("approved")["approval_verified"])

    def test_requires_craig_approval_and_rejects_rejection(self):
        for reviews in [[], [dict(self.approvals[0], state="rejected")],
                        [dict(self.approvals[0], user={"login": "someone", "id": 3, "type": "User"})],
                        self.approvals + [dict(self.approvals[0], state="rejected")]]:
            with self.subTest(reviews=reviews):
                original = self.approvals
                self.approvals = reviews
                with self.assertRaises(ValueError):
                    self.validate("approved")
                self.approvals = original

    def test_consumed_recovery_cannot_run_again(self):
        self.development = self.source
        with self.assertRaisesRegex(ValueError, "development base"):
            self.validate("approved")

    def test_stale_main_or_changed_reviewed_base_is_rejected(self):
        self.main = self.anchor
        with self.assertRaisesRegex(ValueError, "current main"):
            self.validate()
        self.main = self.source
        self.pull_base = self.base
        with self.assertRaisesRegex(ValueError, "reviewed"):
            self.validate()

    def test_runtime_and_manifest_changes_after_anchor_are_rejected(self):
        for path, contents in [("app.txt", "extra runtime\n"),
                               ("deploy/production-release.json", "{}\n")]:
            with self.subTest(path=path):
                self.git("reset", "--hard", self.source)
                (self.repo / path).write_text(contents)
                bad_source = self.commit("out of recovery scope")
                with patch.dict(os.environ, dict(self.env, GITHUB_SHA=bad_source)), \
                     patch.object(recovery, "github", side_effect=self.github):
                    with self.assertRaises(ValueError):
                        recovery.validate(bad_source, "preflight", "901")
        self.git("reset", "--hard", self.source)

    def test_latest_failed_ci_cannot_be_hidden_by_older_success(self):
        self.latest_ci.append(dict(self.ci, id=801, created_at="2026-09-25T12:01:00Z",
                                   conclusion="failure"))
        with self.assertRaisesRegex(ValueError, "CI"):
            self.validate()

    def test_wrong_ci_run_and_fork_are_rejected(self):
        for change in [{"event": "pull_request"}, {"workflow_id": 1},
                       {"head_sha": self.anchor}, {"conclusion": "cancelled"},
                       {"head_repository": {"full_name": "fork/portfolio"}}]:
            with self.subTest(change=change):
                self.latest_ci = [dict(self.ci, **change)]
                with self.assertRaises(ValueError):
                    self.validate()

    def test_wrong_context_attempt_expiry_or_cancelled_run_is_rejected(self):
        for change in [{"GITHUB_EVENT_NAME": "pull_request"}, {"GITHUB_RUN_ATTEMPT": "2"},
                       {"GITHUB_REF": "refs/heads/other"}, {"GITHUB_SHA": self.anchor}]:
            with self.subTest(change=change):
                original = dict(self.env)
                self.env.update(change)
                with self.assertRaises(ValueError):
                    self.validate()
                self.env = original
        self.run["status"] = "completed"
        self.run["conclusion"] = "cancelled"
        with self.assertRaises(ValueError):
            self.validate("approved")
        self.run.update(status="in_progress", conclusion=None, created_at="2026-09-24T12:00:00Z")
        with self.assertRaisesRegex(ValueError, "eight-hour"):
            self.validate()

    def test_environment_bypass_or_missing_reviewers_is_rejected(self):
        self.environment["can_admins_bypass"] = True
        with self.assertRaises(ValueError):
            self.validate()
        self.environment["can_admins_bypass"] = False
        self.environment["protection_rules"] = []
        with self.assertRaises(ValueError):
            self.validate()

    def test_completed_provenance_survives_consumption_and_new_main(self):
        self.run.update(status="completed", conclusion="success")
        self.main = self.anchor
        self.development = self.source
        result = self.validate("completed")
        self.assertTrue(result["approval_verified"])
        self.assertNotIn("classification", result)

    def test_failed_completed_run_is_not_scan_provenance(self):
        self.run.update(status="completed", conclusion="failure")
        with self.assertRaises(ValueError):
            self.validate("completed")


class ScanProvenanceTests(unittest.TestCase):
    def test_manual_scan_requires_successful_recovery_provenance_before_download(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            bindir = root / "bin"
            bindir.mkdir()
            source = "a" * 40
            digest = "sha256:" + "b" * 64
            scan = {"repositoryName": "portfolio-lambda-releases",
                    "imageId": {"imageDigest": digest}, "imageScanStatus": {"status": "COMPLETE"},
                    "imageScanFindings": {"findingSeverityCounts": {}}}
            with zipfile.ZipFile(root / "scan.zip", "w") as archive:
                archive.writestr("scan.json", json.dumps(scan))
            (root / "runs.json").write_text(json.dumps({"total_count": 1, "workflow_runs": [{
                "id": 901, "run_attempt": 1, "created_at": "2026-09-25T12:00:00Z",
                "head_sha": source, "event": "workflow_dispatch", "status": "completed",
                "conclusion": "success"}]}))
            (root / "artifacts.json").write_text(json.dumps({"total_count": 1, "artifacts": [{
                "id": 77, "name": "release-" + source, "expired": False}]}))
            (bindir / "gh").write_text("""#!/bin/sh
printf '%s\n' "$*" >> "$FIXTURE/calls"
case "$*" in
  *workflows/release.yml/runs*) cat "$FIXTURE/runs.json" ;;
  *actions/runs/901/artifacts*) cat "$FIXTURE/artifacts.json" ;;
  *actions/artifacts/77/zip*) cat "$FIXTURE/scan.zip" ;;
  *) exit 9 ;;
esac
""")
            (bindir / "python3").write_text("""#!/bin/sh
case "$*" in
  *validate-development-recovery.py*'--phase completed --run-id 901') ;;
  *) exit 9 ;;
esac
printf '%s\n' verified-recovery >> "$FIXTURE/calls"
test "$ALLOW_RECOVERY" = true
""")
            for executable in bindir.iterdir():
                executable.chmod(0o700)
            env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.environ["PATH"],
                       FIXTURE=str(root), GITHUB_REPOSITORY="CraigDevJohnson/portfolio",
                       DEVELOPMENT_SOURCE_SHA=source, IMAGE_DIGEST=digest,
                       SCAN_FILE=str(root / "result.json"))
            for allowed in ("false", "true"):
                with self.subTest(approved=allowed):
                    (root / "calls").write_text("")
                    result = subprocess.run(["sh", str(ROOT / "scripts/fetch-ci-lambda-release-scan.sh")],
                                            env=dict(env, ALLOW_RECOVERY=allowed), capture_output=True)
                    calls = (root / "calls").read_text()
                    if allowed == "false":
                        self.assertNotEqual(result.returncode, 0)
                        self.assertNotIn("actions/artifacts/77/zip", calls)
                        self.assertFalse((root / "result.json").exists())
                    else:
                        self.assertEqual(result.returncode, 0, result.stderr.decode())
                        self.assertEqual(json.loads((root / "result.json").read_text()), scan)
                        self.assertLess(calls.index("verified-recovery"), calls.index("actions/artifacts/77/zip"))


if __name__ == "__main__":
    unittest.main()
