#!/usr/bin/env python3
"""Authorize the single reviewed September 2026 development backlog recovery."""

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
DEVELOPMENT_BASE = "9528b784088f71fa39d1d7fce8570278c0d3acaf"
BACKLOG_ANCHOR = "11166a9eaecf1a4c19ef96e51ab9915612d9b8bd"
MANIFEST_SHA256 = "d223aa79fcb7f803abbd8311a3a1027719845132688fcbcd53856387bdccad55"
ALLOWED_CHANGES = {
    ".github/workflows/release.yml",
    "Taskfile.yaml",
    "scripts/authorize-ci-lambda-release.sh",
    "scripts/check-current-main.sh",
    "scripts/validate-development-recovery.py",
    "scripts/fetch-ci-lambda-release-scan.sh",
    "tests/development-recovery.py",
    "tests/release-automation.sh",
    "DEPLOY-INSTRUCTIONS.md",
    "docs/deployment/aws-lambda-api-gateway.md",
    "docs/deployment/production-lambda-promotion.md",
    "docs/deployment/2026-09-25-production-launch-readiness.md",
    "docs/superpowers/plans/2026-08-21-production-lambda-cutover.md",
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def command(*args):
    result = subprocess.run(args, capture_output=True, text=True, check=False)
    require(result.returncode == 0, f"metadata command failed: {args[0]}")
    return result.stdout


def github(endpoint, paginate=False):
    args = ["gh", "api"]
    if paginate:
        args += ["--paginate", "--slurp"]
    return json.loads(command(*args, f"repos/{REPOSITORY}/{endpoint}"))


def timestamp(value):
    require(isinstance(value, str) and re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ", value),
            "invalid GitHub timestamp")
    return int(datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc).timestamp())


def now_epoch():
    return int(datetime.now(timezone.utc).timestamp())


def development_base(source):
    resolver = Path(__file__).with_name("resolve-development-release-base.sh")
    return command("sh", str(resolver), source).strip()


def trusted_run(run, source, workflow_id, path, event):
    return (
        run.get("head_sha") == source and run.get("head_branch") == "main"
        and run.get("workflow_id") == workflow_id and run.get("path") == path
        and run.get("event") == event
        and run.get("repository", {}).get("full_name") == REPOSITORY
        and run.get("head_repository", {}).get("full_name") == REPOSITORY
    )


def validate(source, phase, run_id):
    require(re.fullmatch(r"[0-9a-f]{40}", source), "source must be a full lowercase SHA")
    require(phase in {"preflight", "approved", "completed"}, "invalid recovery phase")
    require(re.fullmatch(r"[1-9][0-9]*", run_id), "invalid recovery run ID")
    require(os.environ.get("GITHUB_REPOSITORY") == REPOSITORY, "unexpected repository")
    historical = phase == "completed"
    if not historical:
        require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_dispatch"
                and os.environ.get("GITHUB_REF") == "refs/heads/main"
                and os.environ.get("GITHUB_SHA") == source
                and os.environ.get("GITHUB_RUN_ID") == run_id
                and os.environ.get("GITHUB_RUN_ATTEMPT") == "1", "unexpected recovery run context")
        require(command("git", "rev-parse", "HEAD").strip() == source, "checkout differs from selected source")
        require(not command("git", "status", "--porcelain", "--untracked-files=no").strip(),
                "recovery checkout has tracked changes")
    command("git", "merge-base", "--is-ancestor", DEVELOPMENT_BASE, BACKLOG_ANCHOR)
    command("git", "merge-base", "--is-ancestor", BACKLOG_ANCHOR, source)
    require(source != BACKLOG_ANCHOR, "select the reviewed recovery implementation commit")
    changes = command("git", "diff", "--no-renames", "--name-only", BACKLOG_ANCHOR, source).splitlines()
    require(changes and set(changes) <= ALLOWED_CHANGES, "source expands the reviewed recovery scope")
    for revision in (BACKLOG_ANCHOR, source):
        manifest = command("git", "show", f"{revision}:deploy/production-release.json").encode()
        require(hashlib.sha256(manifest).hexdigest() == MANIFEST_SHA256,
                "historical production manifest changed")
    pulls = github(f"commits/{source}/pulls")
    matches = [p for p in pulls if p.get("state") == "closed" and p.get("merged_at")
               and p.get("merge_commit_sha") == source and p.get("base", {}).get("ref") == "main"]
    require(len(matches) == 1 and matches[0]["base"].get("sha") == BACKLOG_ANCHOR,
            "source is not the uniquely reviewed recovery merge over the frozen anchor")
    if not historical:
        require(github("commits/main").get("sha") == source, "source is no longer current main")
        require(development_base(source) == DEVELOPMENT_BASE,
                "verified development base changed; recovery is consumed or requires new review")

    pages = github(f"actions/workflows/ci.yml/runs?head_sha={source}&event=push&branch=main&per_page=100",
                   paginate=True)
    require(isinstance(pages, list) and pages and all(isinstance(p, dict) for p in pages),
            "invalid CI pages")
    runs = [r for page in pages for r in page["workflow_runs"]]
    require(runs and all(type(r.get("id")) is int and r["id"] > 0 for r in runs), "missing valid CI runs")
    require(len({r["id"] for r in runs}) == len(runs), "duplicate CI runs")
    ci = max(runs, key=lambda r: (timestamp(r.get("created_at")), r["id"]))
    require(trusted_run(ci, source, 340100411, ".github/workflows/ci.yml", "push")
            and ci.get("status") == "completed" and ci.get("conclusion") == "success",
            "latest trusted CI for the selected source has not succeeded")

    run = github(f"actions/runs/{run_id}/attempts/1")
    require(run.get("id") == int(run_id) and run.get("run_attempt") == 1
            and trusted_run(run, source, 346157322, ".github/workflows/release.yml", "workflow_dispatch"),
            "not the exact first attempt of the trusted recovery Release run")
    if historical:
        require(run.get("status") == "completed" and run.get("conclusion") == "success",
                "recovery Release run did not complete successfully")
    else:
        require(run.get("status") == "in_progress" and run.get("conclusion") is None,
                "recovery Release run is not active")
        require(0 <= now_epoch() - timestamp(run.get("created_at")) <= 8 * 60 * 60,
                "recovery exceeds its eight-hour work window")

    environment = github("environments/release-review")
    require(environment.get("can_admins_bypass") is False
            and environment.get("deployment_branch_policy") == {
                "protected_branches": True, "custom_branch_policies": False},
            "release-review protection or administrator bypass changed")
    rules = [r for r in environment.get("protection_rules", []) if r.get("type") == "required_reviewers"]
    require(len(rules) == 1 and rules[0].get("prevent_self_review") is False
            and len(rules[0].get("reviewers", [])) == 1, "release-review reviewer contract changed")
    reviewer = rules[0]["reviewers"][0]
    require(reviewer.get("type") == "User" and reviewer.get("reviewer", {}).get("id") == 42454849
            and reviewer["reviewer"].get("login") == "CraigDevJohnson", "release-review reviewer changed")
    if phase != "preflight":
        approvals = github(f"actions/runs/{run_id}/approvals")
        require(isinstance(approvals, list), "invalid approval response")
        reviews = [a for a in approvals if any(e.get("name") == "release-review"
                                              for e in a.get("environments", []))]
        require(reviews and all(a.get("state") == "approved"
                               and a.get("user", {}).get("login") == "CraigDevJohnson"
                               and a["user"].get("id") == 42454849
                               and a["user"].get("type") == "User" for a in reviews),
                "Craig has not approved this recovery or a review was rejected")
    result = {"source_sha": source, "base_sha": BACKLOG_ANCHOR,
              "development_base_sha": DEVELOPMENT_BASE, "manifest_sha256": MANIFEST_SHA256,
              "ci_run_id": ci["id"], "release_run_id": int(run_id), "phase": phase,
              "approval_verified": phase != "preflight"}
    if not historical:
        result["classification"] = "development-reviewed"
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-sha", required=True)
    parser.add_argument("--phase", choices=("preflight", "approved", "completed"), required=True)
    parser.add_argument("--run-id", default=os.environ.get("GITHUB_RUN_ID", ""))
    parser.add_argument("--github-output", type=Path)
    args = parser.parse_args()
    try:
        result = validate(args.source_sha, args.phase, args.run_id)
        if args.github_output:
            require(args.phase == "preflight", "only preflight may emit job classification")
            with args.github_output.open("a") as output:
                for key in ("source_sha", "base_sha", "development_base_sha", "classification"):
                    output.write(f"{key}={result[key]}\n")
        print(json.dumps(result, sort_keys=True))
    except (ValueError, KeyError, TypeError, OSError) as error:
        print(f"Development recovery refused: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
