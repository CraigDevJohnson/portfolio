#!/usr/bin/env python3
"""Exercise protected production approval against independent GitHub responses."""

import copy
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "approval", Path(__file__).resolve().parents[1] / "scripts/collect-production-approval.py")
approval = importlib.util.module_from_spec(spec)
spec.loader.exec_module(approval)


class ProductionApprovalTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.evidence = Path(self.directory.name)
        self.source = "a" * 40
        self.context = {
            "SOURCE_SHA": self.source, "GITHUB_RUN_ID": "123",
            "GITHUB_RUN_ATTEMPT": "1", "GITHUB_REPOSITORY": approval.REPOSITORY,
            "GITHUB_EVENT_NAME": "workflow_run", "GITHUB_REF": "refs/heads/main", "GITHUB_SHA": self.source,
        }
        self.plan = b"saved binary production plan"
        (self.evidence / "prod.tfplan").write_bytes(self.plan)
        (self.evidence / "release-identity.json").write_text(json.dumps({
            "promotion_sha": self.source, "planning_run_id": "123",
            "planning_run_attempt": "1", "planning_environment": "production-plan",
            "plan_sha256": hashlib.sha256(self.plan).hexdigest(),
        }))
        self.context["PLANNED_PLAN_SHA256"] = hashlib.sha256(self.plan).hexdigest()
        self.context["PLANNED_IDENTITY_SHA256"] = approval.digest(self.evidence / "release-identity.json")
        user = {"id": 42454849, "login": "CraigDevJohnson", "type": "User"}
        self.responses = {
            "branches/main": {"protected": True, "commit": {"sha": self.source}},
            "actions/runs/123/attempts/1": {
                "id": 123, "run_attempt": 1, "workflow_id": 346157322,
                "path": ".github/workflows/release.yml", "event": "workflow_run",
                "head_branch": "main", "head_sha": self.source, "status": "in_progress",
                "conclusion": None, "repository": {"full_name": approval.REPOSITORY},
                "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "head_repository": {"full_name": approval.REPOSITORY},
            },
            "environments/production": {
                "id": 456, "can_admins_bypass": False,
                "deployment_branch_policy": {"protected_branches": True,
                                             "custom_branch_policies": False},
                "protection_rules": [{"type": "required_reviewers", "prevent_self_review": False,
                                      "reviewers": [{"type": "User", "reviewer": user}]}],
            },
            "actions/runs/123/approvals": [{
                "state": "approved", "user": user,
                "environments": [{"id": 456, "name": "production"}],
            }],
        }

    def collect(self):
        return approval.collect(self.evidence, self.context,
                                lambda endpoint: copy.deepcopy(self.responses[endpoint]))

    def test_actual_approval_binds_plan_and_run(self):
        result = self.collect()
        self.assertEqual(result["plan_sha256"], hashlib.sha256(self.plan).hexdigest())
        self.assertEqual(result["planning_run_id"], "123")
        self.assertEqual(result["reviewer_login"], "CraigDevJohnson")
        self.assertTrue(result["approval_id"].startswith("github-123-1-"))

    def test_supplied_success_file_cannot_replace_missing_github_approval(self):
        (self.evidence / "approval.json").write_text(json.dumps({"approved": True}))
        self.responses["actions/runs/123/approvals"] = []
        with self.assertRaisesRegex(ValueError, "lacks Craig"):
            self.collect()

    def test_tampered_binary_is_rejected(self):
        (self.evidence / "prod.tfplan").write_bytes(b"different binary")
        with self.assertRaisesRegex(ValueError, "planning-job outputs"):
            self.collect()

    def test_coordinated_binary_and_identity_substitution_is_rejected(self):
        (self.evidence / "prod.tfplan").write_bytes(b"replacement")
        path = self.evidence / "release-identity.json"
        identity = json.loads(path.read_text())
        identity["plan_sha256"] = approval.digest(self.evidence / "prod.tfplan")
        path.write_text(json.dumps(identity))
        with self.assertRaisesRegex(ValueError, "planning-job outputs"):
            self.collect()

    def test_reused_previous_attempt_approval_is_rejected(self):
        self.context["GITHUB_RUN_ATTEMPT"] = "2"
        with self.assertRaisesRegex(ValueError, "first-attempt"):
            self.collect()

    def test_source_changed_while_waiting(self):
        self.responses["branches/main"]["commit"]["sha"] = "b" * 40
        with self.assertRaisesRegex(ValueError, "current protected main"):
            self.collect()

    def test_cancelled_run_cannot_apply(self):
        self.responses["actions/runs/123/attempts/1"]["conclusion"] = "cancelled"
        with self.assertRaisesRegex(ValueError, "active trusted"):
            self.collect()

    def test_different_workflow_is_rejected(self):
        self.responses["actions/runs/123/attempts/1"]["workflow_id"] = 999
        with self.assertRaisesRegex(ValueError, "active trusted"):
            self.collect()

    def test_expired_execution_window_cannot_be_reapproved_in_place(self):
        self.responses["actions/runs/123/attempts/1"]["created_at"] = "2000-01-01T00:00:00Z"
        with self.assertRaisesRegex(ValueError, "eight-hour"):
            self.collect()

    def test_admin_bypass_cannot_replace_protected_review(self):
        self.responses["environments/production"]["can_admins_bypass"] = True
        with self.assertRaisesRegex(ValueError, "protection changed"):
            self.collect()

    def test_wrong_environment_review_does_not_approve_production(self):
        self.responses["actions/runs/123/approvals"][0]["environments"][0]["id"] = 999
        with self.assertRaisesRegex(ValueError, "lacks Craig"):
            self.collect()

    def test_matching_login_with_wrong_user_id_is_rejected(self):
        self.responses["actions/runs/123/approvals"][0]["user"] = {
            "id": 999, "login": "CraigDevJohnson", "type": "User"}
        with self.assertRaisesRegex(ValueError, "lacks Craig"):
            self.collect()

    def test_later_rejection_invalidates_earlier_approval(self):
        rejected = copy.deepcopy(self.responses["actions/runs/123/approvals"][0])
        rejected["state"] = "rejected"
        self.responses["actions/runs/123/approvals"].append(rejected)
        with self.assertRaisesRegex(ValueError, "rejection"):
            self.collect()


if __name__ == "__main__":
    unittest.main()
