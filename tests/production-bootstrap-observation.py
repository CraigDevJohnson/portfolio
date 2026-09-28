#!/usr/bin/env python3
"""Offline production metadata fixtures; no AWS or GitHub calls."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("observation", ROOT / "scripts/observe-production-bootstrap.py")
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class ObservationTests(unittest.TestCase):
    def setUp(self):
        self.alias = {"AliasArn": m.FUNCTION_ARN + ":live", "Name": "live", "FunctionVersion": "2",
                      "RevisionId": "revision-2", "RoutingConfig": {"AdditionalVersionWeights": {}}}
        self.function = {"FunctionName": m.FUNCTION, "FunctionArn": m.FUNCTION_ARN + ":2", "Version": "2",
                         "ImageUri": m.IMAGE_PREFIX + "sha256:" + "a" * 64,
                         "ResolvedImageUri": m.IMAGE_PREFIX + "sha256:" + "a" * 64}
        self.calls = []
        self.context = {"GITHUB_SHA": "b" * 40, "GITHUB_RUN_ID": "71", "GITHUB_RUN_ATTEMPT": "1",
                        "GITHUB_REPOSITORY": m.REPOSITORY, "GITHUB_REF": "refs/heads/main",
                        "GITHUB_EVENT_NAME": "workflow_dispatch", "GITHUB_ACTOR_ID": str(m.ACTOR_ID)}
        self.branch = {"protected": True, "commit": {"sha": self.context["GITHUB_SHA"]}}
        self.run = {"id": 71, "run_attempt": 1, "event": "workflow_dispatch", "path": m.WORKFLOW,
                    "head_branch": "main", "head_sha": self.context["GITHUB_SHA"], "status": "in_progress",
                    "conclusion": None, "actor": {"id": m.ACTOR_ID}, "triggering_actor": {"id": m.ACTOR_ID},
                    "repository": {"full_name": m.REPOSITORY}, "head_repository": {"full_name": m.REPOSITORY}}

    def aws(self, arguments):
        self.calls.append(arguments)
        self.assertEqual(arguments[:2], ["aws", "lambda"])
        self.assertIn(arguments[2], ("get-alias", "get-function"))
        self.assertIn("--query", arguments)
        return copy.deepcopy(self.alias if arguments[2] == "get-alias" else self.function)

    def github(self, endpoint):
        return self.branch if endpoint == "branches/main" else self.run

    def test_exact_sanitized_snapshot_uses_only_metadata_reads(self):
        result = m.observe(self.aws)
        self.assertEqual(result, {"function_name": m.FUNCTION, "alias_name": "live", "alias_version": "2",
                                  "alias_revision_id": "revision-2", "image_uri": self.function["ImageUri"]})
        self.assertEqual([call[2] for call in self.calls], ["get-alias", "get-function", "get-alias"])
        self.assertNotIn("Environment", json.dumps(self.calls))

    def test_wrong_alias_version_weight_and_revision(self):
        for field, value in (("AliasArn", m.FUNCTION_ARN + ":other"), ("Name", "other"),
                             ("FunctionVersion", "$LATEST"), ("FunctionVersion", "0"), ("RevisionId", ""),
                             ("RoutingConfig", {"AdditionalVersionWeights": {"3": 0.1}})):
            with self.subTest(field=field):
                original = self.alias[field]
                self.alias[field] = value
                with self.assertRaises(ValueError): m.observe(self.aws)
                self.alias[field] = original

    def test_wrong_function_mutable_or_mismatched_image(self):
        for field, value in (("FunctionName", "other"), ("FunctionArn", m.FUNCTION_ARN), ("Version", "3"),
                             ("ImageUri", "repo:latest"), ("ImageUri", "other@sha256:" + "a" * 64),
                             ("ResolvedImageUri", m.IMAGE_PREFIX + "sha256:" + "c" * 64)):
            with self.subTest(field=field):
                original = self.function[field]
                self.function[field] = value
                with self.assertRaises(ValueError): m.observe(self.aws)
                self.function[field] = original

    def test_alias_race_rejected(self):
        def moving_alias(arguments):
            result = self.aws(arguments)
            if len(self.calls) == 3:
                result["RevisionId"] = "changed"
            return result
        with self.assertRaises(ValueError): m.observe(moving_alias)

    def test_current_main_first_attempt_craig_and_workflow_authority(self):
        self.assertEqual(m.authorize(self.context, self.github), ("b" * 40, "71"))
        for field, value in (("GITHUB_ACTOR_ID", "1"), ("GITHUB_REF", "refs/heads/other"),
                             ("GITHUB_EVENT_NAME", "push"), ("GITHUB_RUN_ATTEMPT", "2")):
            with self.subTest(field=field), self.assertRaises(ValueError):
                m.authorize(dict(self.context, **{field: value}), self.github)
        for field, value in (("head_sha", "c" * 40), ("path", ".github/workflows/other.yml"),
                             ("actor", {"id": 1}), ("triggering_actor", {"id": 1}), ("run_attempt", 2)):
            original = self.run[field]
            self.run[field] = value
            with self.assertRaises(ValueError): m.authorize(self.context, self.github)
            self.run[field] = original
        self.branch["commit"]["sha"] = "c" * 40
        with self.assertRaises(ValueError): m.authorize(self.context, self.github)

    def test_artifact_has_observed_only_provenance_and_cannot_be_reused(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "observation"
            with patch.object(m, "authorize", return_value=("b" * 40, "71")), \
                    patch.object(m, "observe", return_value=m.observe(self.aws)), \
                    patch.object(m.sys, "argv", ["observe", "--output-dir", str(output)]):
                self.assertEqual(m.main(), 0)
                self.assertEqual(set(path.name for path in output.iterdir()), {"bootstrap.json", "provenance.json"})
                provenance = json.loads((output / "provenance.json").read_text())
                self.assertEqual(provenance["assessment"], "observed_only_not_verified_production_history")
                self.assertEqual(provenance["bootstrap_sha256"], m.hashlib.sha256((output / "bootstrap.json").read_bytes()).hexdigest())
                self.assertEqual(m.main(), 1)

    def test_workflow_is_dispatch_only_and_cannot_publish_deployment_status(self):
        workflow = (ROOT / m.WORKFLOW).read_text()
        self.assertIn("  workflow_dispatch:", workflow)
        for forbidden in ("workflow_run:", "  push:", "deployments:", "write-all", "tofu", "PRODUCTION_BOOTSTRAP_EVIDENCE_JSON"):
            self.assertNotIn(forbidden, workflow)
        self.assertIn("environment: production-plan", workflow)
        self.assertIn("group: lambda-production", workflow)
        self.assertIn("cancel-in-progress: false", workflow)
        self.assertLess(workflow.index("--authorize-only"), workflow.index("uses: aws-actions/configure-aws-credentials"))
        self.assertIn("role/portfolio-production-planner-ci", workflow)


if __name__ == "__main__":
    unittest.main()
