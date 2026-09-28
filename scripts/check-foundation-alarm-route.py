#!/usr/bin/env python3
"""Check the exact configured shared route; this does not prove inbox delivery."""

import argparse
import hashlib
import json
import subprocess
import sys

ACCOUNT = "180294223248"
RULE = "foundation-notifications-services"
RULE_ARN = f"arn:aws:events:us-west-2:{ACCOUNT}:rule/{RULE}"
BUS_ARN = f"arn:aws:events:us-east-2:{ACCOUNT}:event-bus/foundation-notifications"
ROLE_ARN = f"arn:aws:iam::{ACCOUNT}:role/FoundationNotificationForward"
ALARMS = {f"arn:aws:cloudwatch:us-west-2:{ACCOUNT}:alarm:portfolio-lambda-prod-{suffix}"
          for suffix in ("api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles")}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(rule, targets):
    require(rule.get("Arn") == RULE_ARN and rule.get("State") == "ENABLED",
            "foundation notification rule is absent, disabled or in another account")
    pattern = json.loads(rule["EventPattern"])
    expected = {"account": [ACCOUNT], "$or": [
        {"source": ["aws.guardduty"], "detail-type": ["GuardDuty Finding"]},
        {"source": ["aws.backup"],
         "detail-type": ["Backup Job State Change", "Copy Job State Change", "Restore Job State Change"],
         "detail": {"state": ["FAILED", "ABORTED", "EXPIRED", "PARTIAL"]}},
        {"source": ["aws.cloudwatch"], "detail-type": ["CloudWatch Alarm State Change"],
         "resources": sorted(ALARMS) + [
             f"arn:aws:cloudwatch:us-east-2:{ACCOUNT}:alarm:foundation-health-unhealthy",
             f"arn:aws:cloudwatch:us-east-2:{ACCOUNT}:alarm:foundation-health-missing"],
         "detail": {"state": {"value": ["ALARM", "OK"]}}},
    ]}
    def canonical(value):
        if isinstance(value, dict):
            return {key: canonical(item) for key, item in value.items()}
        if isinstance(value, list):
            return sorted((canonical(item) for item in value), key=lambda item: json.dumps(item, sort_keys=True))
        return value
    require(canonical(pattern) == canonical(expected),
            "foundation notification pattern has missing or unexpected selectors or constraints")
    actual = targets.get("Targets", [])
    require(len(actual) == 1 and actual[0].get("Id") == "foundation-notifications"
            and actual[0].get("Arn") == BUS_ARN and actual[0].get("RoleArn") == ROLE_ARN,
            "notification forwarding destination or role changed")
    require(all(actual[0].get(key) in (None, "") for key in ("Input", "InputPath", "InputTransformer")),
            "native notification events must be forwarded without replacement")
    return {"rule_arn": rule["Arn"], "state": rule["State"], "target_bus_arn": actual[0]["Arn"],
            "forwarding_role_arn": actual[0]["RoleArn"], "covered_alarm_arns": sorted(ALARMS),
            "pattern_sha256": hashlib.sha256(json.dumps(pattern, sort_keys=True, separators=(",", ":")).encode()).hexdigest()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    def aws(operation):
        result = subprocess.run(["aws", "events", operation, "--rule" if operation == "list-targets-by-rule" else "--name",
                                 RULE, "--region", "us-west-2", "--output", "json"],
                                capture_output=True, check=True, timeout=20)
        return json.loads(result.stdout)
    result = validate(aws("describe-rule"), aws("list-targets-by-rule"))
    print(json.dumps(result) if args.json else "The five production alarms have the expected foundation forwarding configuration")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError):
        print("Production foundation alarm route is unverified", file=sys.stderr)
        sys.exit(1)
