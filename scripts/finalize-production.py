#!/usr/bin/env python3
"""Finalize an applied release from protected GitHub and real operator evidence."""

import hashlib
from datetime import datetime, timezone
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import zipfile

REPOSITORY = "CraigDevJohnson/portfolio"
REVIEWER = {"id": 42454849, "login": "CraigDevJohnson", "type": "User"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def github(endpoint, binary=False):
    result = subprocess.run(["gh", "api", f"repos/{REPOSITORY}/{endpoint}"],
                            capture_output=True, check=True, timeout=60)
    return result.stdout if binary else json.loads(result.stdout)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def approved(run_id, read):
    environment = read("environments/production")
    require(environment.get("can_admins_bypass") is False and
            environment.get("deployment_branch_policy") == {
                "protected_branches": True, "custom_branch_policies": False},
            "production environment protection changed")
    rules = [rule for rule in environment.get("protection_rules", [])
             if rule.get("type") == "required_reviewers"]
    require(len(rules) == 1 and rules[0].get("prevent_self_review") is False
            and len(rules[0].get("reviewers", [])) == 1, "production reviewer policy changed")
    reviewer = rules[0]["reviewers"][0]
    require(reviewer.get("type") == "User" and all(
        reviewer.get("reviewer", {}).get(key) == value for key, value in REVIEWER.items()),
        "production reviewer changed")
    reviews = [review for review in read(f"actions/runs/{run_id}/approvals") if any(
        item.get("id") == environment.get("id") and item.get("name") == "production"
        for item in review.get("environments", []))]
    require(reviews and all(review.get("state") == "approved" and all(
        review.get("user", {}).get(key) == value for key, value in REVIEWER.items())
        for review in reviews), "missing or rejected protected production approval")


def provenance(context, read=github):
    source = context.get("GITHUB_SHA", "")
    current_id = context.get("GITHUB_RUN_ID", "")
    apply_id = context.get("APPLY_RUN_ID", "")
    require(context.get("GITHUB_REPOSITORY") == REPOSITORY and
            context.get("GITHUB_EVENT_NAME") == "workflow_dispatch" and
            context.get("GITHUB_REF") == "refs/heads/main" and
            context.get("GITHUB_RUN_ATTEMPT") == "1", "unexpected acceptance execution context")
    require(re.fullmatch(r"[a-f0-9]{40}", source) and
            re.fullmatch(r"[1-9][0-9]*", current_id) and
            re.fullmatch(r"[1-9][0-9]*", apply_id) and current_id != apply_id,
            "invalid acceptance coordinates")
    branch = read("branches/main")
    require(branch.get("protected") is True and branch.get("commit", {}).get("sha") == source,
            "release is not current protected main")
    for run_id, path, event, status, conclusion in (
        (current_id, ".github/workflows/production-acceptance.yml", "workflow_dispatch", "in_progress", None),
        (apply_id, ".github/workflows/release.yml", "workflow_run", "completed", "success"),
    ):
        run = read(f"actions/runs/{run_id}/attempts/1")
        require(run.get("id") == int(run_id) and run.get("run_attempt") == 1
                and run.get("head_sha") == source and run.get("head_branch") == "main"
                and run.get("path") == path and run.get("event") == event
                and run.get("status") == status and run.get("conclusion") == conclusion
                and run.get("repository", {}).get("full_name") == REPOSITORY
                and run.get("head_repository", {}).get("full_name") == REPOSITORY,
                "untrusted, stale, failed or retried workflow run")
        started = datetime.strptime(run["created_at"], "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
        require(0 <= (datetime.now(timezone.utc) - started).total_seconds() <= 8 * 3600,
                "production run exceeds its eight-hour execution window")
        if run_id == current_id:
            require(all(run.get("actor", {}).get(key) == value for key, value in REVIEWER.items()),
                    "acceptance receipt was not submitted by Craig")
        approved(run_id, read)
    artifacts = read(f"actions/runs/{apply_id}/artifacts?per_page=100")
    require(artifacts.get("total_count", 101) <= 100, "unexpected artifact pagination")
    matching = [artifact for artifact in artifacts.get("artifacts", [])
                if artifact.get("name") == f"production-evidence-{source}"]
    require(len(matching) == 1 and matching[0].get("expired") is False
            and type(matching[0].get("id")) is int and matching[0]["id"] > 0
            and re.fullmatch(r"sha256:[a-f0-9]{64}", matching[0].get("digest", "")),
            "missing unique immutable production evidence")
    return matching[0]


def unpack(archive, expected, destination):
    require("sha256:" + hashlib.sha256(archive).hexdigest() == expected,
            "artifact archive digest mismatch")
    require(not destination.exists(), "acceptance evidence directory already exists")
    with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
        entries = bundle.infolist()
        require(len(entries) <= 500 and sum(item.file_size for item in entries) <= 50 * 1024 * 1024,
                "unexpected evidence archive size")
        names = [item.filename for item in entries]
        require(len(names) == len(set(names)), "duplicate evidence archive entries")
        for entry in entries:
            path = Path(entry.filename)
            require(not path.is_absolute() and ".." not in path.parts and "\\" not in entry.filename
                    and not stat.S_ISLNK(entry.external_attr >> 16), "unsafe evidence archive path")
        destination.mkdir(mode=0o700)
        bundle.extractall(destination)


def bind(directory, context):
    identity = json.loads((directory / "release-identity.json").read_text())
    deployment = json.loads((directory / "github-production-deployment.json").read_text())
    window = json.loads((directory / "automated-window.json").read_text())
    approval = json.loads((directory / "approval.json").read_text())
    source, run_id = context["GITHUB_SHA"], context["APPLY_RUN_ID"]
    require(identity["promotion_sha"] == deployment["source_sha"] == window["promotion_sha"] == source
            and identity["planning_run_id"] == deployment["planning_run_id"] == run_id
            and identity["planning_run_attempt"] == deployment["planning_run_attempt"] == "1"
            and identity["development_source_sha"] == deployment["development_source_sha"] == window["source_sha"]
            and identity["image_digest"] == deployment["image_digest"] == window["image_digest"]
            and str(deployment["production_deployment_id"]) == window["production_deployment_id"]
            and digest(directory / "release-identity.json") == deployment["release_identity_sha256"]
            and digest(directory / "prod.tfplan") == identity["plan_sha256"] == deployment["plan_sha256"]
            and digest(directory / "scan.json") == identity["scan_sha256"] == deployment["scan_sha256"]
            and approval["approval_id"] == deployment["approval_id"]
            and deployment["status"] == "in_progress" and deployment["status_recorded"] is True,
            "production evidence coordinates disagree")
    require(not (directory / "window-failed.json").exists(), "failed acceptance window cannot be finalized")
    return identity, deployment, window


def observer_module():
    spec = importlib.util.spec_from_file_location("production_observer",
                                                Path(__file__).with_name("observe-lambda-production.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    directory = Path(os.environ["EVIDENCE_DIR"])
    require(directory.is_absolute(), "evidence directory must be absolute")
    artifact = provenance(os.environ)
    if sys.argv[1:] == ["prepare"]:
        unpack(github(f"actions/artifacts/{artifact['id']}/zip", binary=True), artifact["digest"], directory)
        bind(directory, os.environ)
        print("Trusted applied-release evidence retrieved; production remains unverified")
        return
    require(sys.argv[1:] == ["finalize"], "expected prepare or finalize")
    identity, deployment, window = bind(directory, os.environ)
    raw = os.environ["BROWSER_RECEIPT_JSON"]
    require(len(raw.encode()) <= 50000, "browser receipt exceeds input limit")
    receipt = json.loads(raw)
    require(receipt.get("operator") == REVIEWER["login"], "browser operator differs from authenticated submitter")
    observer = observer_module()
    observer.validate_browser(window, receipt)
    metrics = observer.final_metrics(window)
    provenance(os.environ)
    result = {
        "schema_version": 2, "source_sha": identity["development_source_sha"],
        "promotion_sha": identity["promotion_sha"], "image_digest": identity["image_digest"],
        "lambda_version": window["lambda_version"], "production_deployment_id": deployment["production_deployment_id"],
        "window_id": window["window_id"], "started_at": window["started_at"], "ended_at": window["ended_at"],
        "automated_public_window": "passed", "browser_contract": "passed", "metric_coverage": "passed",
        "operator": REVIEWER["login"], "acceptance_run_id": os.environ["GITHUB_RUN_ID"],
        "provenance": "protected-github-operator", "status": "verified",
    }
    for name, value in (("browser-receipt.json", receipt), ("final-metrics.json", metrics)):
        (directory / name).write_text(json.dumps(value, indent=2) + "\n")
    result.update({
        "automated_window_sha256": digest(directory / "automated-window.json"),
        "browser_receipt_sha256": digest(directory / "browser-receipt.json"),
        "final_metrics_sha256": digest(directory / "final-metrics.json"),
    })
    (directory / "production-verification.json").write_text(json.dumps(result, indent=2) + "\n")
    context = os.environ.copy()
    context.update({
        "SOURCE_SHA": identity["promotion_sha"], "DEVELOPMENT_SOURCE_SHA": identity["development_source_sha"],
        "IMAGE_DIGEST": identity["image_digest"], "DEVELOPMENT_DEPLOYMENT_ID": str(identity["development_deployment_id"]),
        "PLAN_SHA256": identity["plan_sha256"], "RELEASE_IDENTITY_SHA256": deployment["release_identity_sha256"],
        "SCAN_SHA256": identity["scan_sha256"], "PLANNING_RUN_ID": identity["planning_run_id"],
        "PLANNING_RUN_ATTEMPT": identity["planning_run_attempt"], "APPROVAL_ID": deployment["approval_id"],
        "REVIEWER_LOGIN": deployment["reviewer_login"], "LAMBDA_VERSION": window["lambda_version"],
        "DEPLOYMENT_STATE": "success",
    })
    subprocess.run(["sh", "scripts/record-ci-lambda-production.sh"], env=context, check=True)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        print("Production acceptance failed: " + (str(error) if isinstance(error, ValueError) else type(error).__name__),
              file=sys.stderr)
        sys.exit(1)
