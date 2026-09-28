#!/usr/bin/env python3
"""Read the current production alias without treating it as verified history."""

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
ACTOR_ID = 42454849
FUNCTION = "portfolio-lambda-prod"
FUNCTION_ARN = "arn:aws:lambda:us-west-2:180294223248:function:" + FUNCTION
IMAGE_PREFIX = "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@"
WORKFLOW = ".github/workflows/production-bootstrap-observation.yml"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def command_json(arguments):
    result = subprocess.run(arguments, capture_output=True, text=True, timeout=30, check=False)
    require(result.returncode == 0, "read-only metadata command failed")
    return json.loads(result.stdout)


def github(endpoint):
    return command_json(["gh", "api", f"repos/{REPOSITORY}/{endpoint}"])


def authorize(context, read=github):
    source, run_id = context.get("GITHUB_SHA", ""), context.get("GITHUB_RUN_ID", "")
    require(context.get("GITHUB_REPOSITORY") == REPOSITORY
            and context.get("GITHUB_EVENT_NAME") == "workflow_dispatch"
            and context.get("GITHUB_REF") == "refs/heads/main"
            and context.get("GITHUB_ACTOR_ID") == str(ACTOR_ID)
            and context.get("GITHUB_RUN_ATTEMPT") == "1", "unexpected observation context")
    require(re.fullmatch(r"[a-f0-9]{40}", source) and re.fullmatch(r"[1-9][0-9]*", run_id),
            "invalid observation source or run ID")
    branch = read("branches/main")
    require(branch.get("protected") is True and branch.get("commit", {}).get("sha") == source,
            "observation source is no longer current protected main")
    run = read(f"actions/runs/{run_id}/attempts/1")
    require(run.get("id") == int(run_id) and run.get("run_attempt") == 1
            and run.get("event") == "workflow_dispatch" and run.get("path") == WORKFLOW
            and run.get("head_branch") == "main" and run.get("head_sha") == source
            and run.get("status") == "in_progress" and run.get("conclusion") is None
            and run.get("actor", {}).get("id") == ACTOR_ID
            and run.get("triggering_actor", {}).get("id") == ACTOR_ID
            and run.get("repository", {}).get("full_name") == REPOSITORY
            and run.get("head_repository", {}).get("full_name") == REPOSITORY,
            "not Craig's active first-attempt observation workflow")
    return source, run_id


def alias_metadata(call=command_json):
    return call(["aws", "lambda", "get-alias", "--region", "us-west-2",
                 "--function-name", FUNCTION, "--name", "live", "--query",
                 "{AliasArn:AliasArn,Name:Name,FunctionVersion:FunctionVersion,RevisionId:RevisionId,RoutingConfig:RoutingConfig}",
                 "--output", "json"])


def validate_alias(alias):
    require(alias.get("AliasArn") == FUNCTION_ARN + ":live" and alias.get("Name") == "live",
            "unexpected production alias identity")
    require(isinstance(alias.get("FunctionVersion"), str) and
            re.fullmatch(r"[1-9][0-9]*", alias["FunctionVersion"]), "alias must use an immutable numbered version")
    require(isinstance(alias.get("RevisionId"), str) and re.fullmatch(r"[A-Za-z0-9-]+", alias["RevisionId"]),
            "missing or malformed alias revision")
    routing = alias.get("RoutingConfig")
    require(routing is None or routing == {} or routing == {"AdditionalVersionWeights": {}},
            "weighted production aliases are not accepted")


def observe(call=command_json):
    alias = alias_metadata(call)
    validate_alias(alias)
    version = alias["FunctionVersion"]
    function = call(["aws", "lambda", "get-function", "--region", "us-west-2",
                     "--function-name", FUNCTION, "--qualifier", version, "--query",
                     "{FunctionName:Configuration.FunctionName,FunctionArn:Configuration.FunctionArn,Version:Configuration.Version,ImageUri:Code.ImageUri,ResolvedImageUri:Code.ResolvedImageUri}",
                     "--output", "json"])
    require(function.get("FunctionName") == FUNCTION and function.get("FunctionArn") == FUNCTION_ARN + ":" + version
            and function.get("Version") == version, "unexpected immutable production function identity")
    image = function.get("ImageUri", "")
    require(isinstance(image, str) and re.fullmatch(re.escape(IMAGE_PREFIX) + r"sha256:[a-f0-9]{64}", image),
            "production image must be pinned to the exact release repository digest")
    require(function.get("ResolvedImageUri") == image, "resolved image differs from pinned production image")
    require(alias_metadata(call) == alias, "production alias changed during observation")
    return {"function_name": FUNCTION, "alias_name": "live", "alias_version": version,
            "alias_revision_id": alias["RevisionId"], "image_uri": image}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--authorize-only", action="store_true")
    parser.add_argument("--output-dir", type=Path)
    args = parser.parse_args()
    try:
        source, run_id = authorize(os.environ)
        if args.authorize_only:
            print("Read-only production observation authority verified")
            return 0
        require(args.output_dir is not None and args.output_dir.is_absolute(), "output directory must be absolute")
        snapshot = observe()
        authorize(os.environ)
        # New directories prevent retry artifacts from mixing old and new observations.
        args.output_dir.mkdir(parents=True, exist_ok=False)
        encoded = (json.dumps(snapshot, indent=2) + "\n").encode()
        (args.output_dir / "bootstrap.json").write_bytes(encoded)
        provenance = {"schema_version": 1, "repository": REPOSITORY, "workflow": WORKFLOW,
                      "run_id": run_id, "run_attempt": 1, "source_sha": source, "actor_id": ACTOR_ID,
                      "observed_at": datetime.now(timezone.utc).isoformat(),
                      "account_id": "180294223248", "region": "us-west-2",
                      "bootstrap_sha256": hashlib.sha256(encoded).hexdigest(),
                      "assessment": "observed_only_not_verified_production_history"}
        (args.output_dir / "provenance.json").write_text(json.dumps(provenance, indent=2) + "\n")
        print("Production bootstrap observation saved; no deployment status or configuration was changed")
    except (ValueError, OSError, KeyError, TypeError, subprocess.SubprocessError):
        print("Production bootstrap observation rejected; no accepted artifact was produced", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
