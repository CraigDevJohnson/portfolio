#!/usr/bin/env python3
"""Bind a saved production plan to an actual protected GitHub run approval."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

REPOSITORY = "CraigDevJohnson/portfolio"
REVIEWER_ID = 42454849
WORKFLOW_ID = 346157322


def require(condition, message):
    if not condition:
        raise ValueError(message)


def github(endpoint):
    result = subprocess.run(
        ["gh", "api", f"repos/{REPOSITORY}/{endpoint}"],
        capture_output=True, text=True, check=False,
    )
    require(result.returncode == 0, "GitHub approval metadata is unavailable")
    return json.loads(result.stdout)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def collect(evidence, context, read=github):
    source = context.get("SOURCE_SHA", "")
    run_id = context.get("GITHUB_RUN_ID", "")
    require(context.get("GITHUB_REPOSITORY") == REPOSITORY, "unexpected repository")
    require(context.get("GITHUB_EVENT_NAME") == "workflow_run"
            and context.get("GITHUB_REF") == "refs/heads/main"
            and context.get("GITHUB_SHA") == source, "unexpected production execution context")
    require(re.fullmatch(r"[0-9a-f]{40}", source), "invalid promotion SHA")
    require(re.fullmatch(r"[1-9][0-9]*", run_id), "invalid run ID")
    # GitHub's approval history has no attempt or approval timestamp. A retry
    # cannot prove that an earlier approval covers its newly generated plan.
    require(context.get("GITHUB_RUN_ATTEMPT") == "1", "production requires a fresh first-attempt run")
    identity = json.loads((evidence / "release-identity.json").read_text())
    plan_digest = digest(evidence / "prod.tfplan")
    require(plan_digest == context.get("PLANNED_PLAN_SHA256")
            and digest(evidence / "release-identity.json") == context.get("PLANNED_IDENTITY_SHA256"),
            "artifacts differ from trusted planning-job outputs")
    require(identity.get("promotion_sha") == source
            and identity.get("planning_run_id") == run_id
            and identity.get("planning_run_attempt") == "1"
            and identity.get("planning_environment") == "production-plan"
            and identity.get("plan_sha256") == plan_digest,
            "saved plan is not bound to this planning run")
    branch = read("branches/main")
    require(branch.get("protected") is True and branch.get("commit", {}).get("sha") == source,
            "promotion is no longer current protected main")
    run = read(f"actions/runs/{run_id}/attempts/1")
    require(run.get("id") == int(run_id) and run.get("run_attempt") == 1
            and run.get("workflow_id") == WORKFLOW_ID
            and run.get("path") == ".github/workflows/release.yml"
            and run.get("event") == "workflow_run" and run.get("head_branch") == "main"
            and run.get("head_sha") == source and run.get("status") == "in_progress"
            and run.get("conclusion") is None
            and run.get("repository", {}).get("full_name") == REPOSITORY
            and run.get("head_repository", {}).get("full_name") == REPOSITORY,
            "not the active trusted production Release run")
    started = datetime.strptime(run["created_at"], "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
    require(0 <= (datetime.now(timezone.utc) - started).total_seconds() <= 8 * 3600,
            "production run exceeds its eight-hour execution window")
    environment = read("environments/production")
    require(environment.get("can_admins_bypass") is False
            and environment.get("deployment_branch_policy") == {
                "protected_branches": True, "custom_branch_policies": False},
            "production Environment protection changed")
    rules = [rule for rule in environment.get("protection_rules", [])
             if rule.get("type") == "required_reviewers"]
    require(len(rules) == 1 and rules[0].get("prevent_self_review") is False
            and len(rules[0].get("reviewers", [])) == 1,
            "production reviewer policy changed")
    reviewer = rules[0]["reviewers"][0]
    require(reviewer.get("type") == "User"
            and reviewer.get("reviewer", {}).get("id") == REVIEWER_ID
            and reviewer["reviewer"].get("login") == "CraigDevJohnson",
            "production reviewer changed")
    history = read(f"actions/runs/{run_id}/approvals")
    require(isinstance(history, list), "invalid approval history")
    approvals = [entry for entry in history if any(
        item.get("id") == environment.get("id") and item.get("name") == "production"
        for item in entry.get("environments", []))]
    require(approvals and all(
        entry.get("state") == "approved"
        and entry.get("user", {}).get("id") == REVIEWER_ID
        and entry["user"].get("login") == "CraigDevJohnson"
        and entry["user"].get("type") == "User" for entry in approvals),
        "this production run lacks Craig's protected approval or has a rejection")
    approval_digest = hashlib.sha256(json.dumps(
        approvals, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    return {
        "schema_version": 1, "environment": "production",
        "promotion_sha": source, "plan_sha256": plan_digest,
        "planning_run_id": run_id, "planning_run_attempt": "1",
        "reviewer_login": "CraigDevJohnson",
        "approval_id": f"github-{run_id}-1-{approval_digest}",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", required=True, type=Path)
    parser.add_argument("--verify", action="store_true")
    args = parser.parse_args()
    try:
        approval = collect(args.evidence_dir, os.environ)
        output = args.evidence_dir / "approval.json"
        if args.verify:
            require(json.loads(output.read_text()) == approval,
                    "saved approval does not match authoritative GitHub history")
        else:
            with output.open("x") as file:
                output.chmod(0o600)
                json.dump(approval, file, indent=2)
                file.write("\n")
    except (ValueError, OSError, KeyError, TypeError) as error:
        print(f"Production approval rejected: {error}", file=sys.stderr)
        return 1
    print("Protected production approval verified")
    return 0


if __name__ == "__main__":
    sys.exit(main())
