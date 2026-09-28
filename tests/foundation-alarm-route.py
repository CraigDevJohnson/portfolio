#!/usr/bin/env python3
import copy
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "route", Path(__file__).resolve().parents[1] / "scripts/check-foundation-alarm-route.py")
route = importlib.util.module_from_spec(spec)
spec.loader.exec_module(route)


class FoundationAlarmRouteTest(unittest.TestCase):
    def setUp(self):
        self.pattern = {"account": [route.ACCOUNT], "$or": [{
            "source": ["aws.cloudwatch"], "detail-type": ["CloudWatch Alarm State Change"],
            "resources": sorted(route.ALARMS) + [
                f"arn:aws:cloudwatch:us-east-2:{route.ACCOUNT}:alarm:foundation-health-unhealthy",
                f"arn:aws:cloudwatch:us-east-2:{route.ACCOUNT}:alarm:foundation-health-missing"], "detail": {"state": {"value": ["ALARM", "OK"]}},
        }]}
        self.pattern["$or"] += [
            {"source": ["aws.guardduty"], "detail-type": ["GuardDuty Finding"]},
            {"source": ["aws.backup"],
             "detail-type": ["Backup Job State Change", "Copy Job State Change", "Restore Job State Change"],
             "detail": {"state": ["FAILED", "ABORTED", "EXPIRED", "PARTIAL"]}},
        ]
        self.rule = {"Arn": route.RULE_ARN, "State": "ENABLED"}
        self.targets = {"Targets": [{"Id": "foundation-notifications", "Arn": route.BUS_ARN,
                                    "RoleArn": route.ROLE_ARN}]}

    def validate(self):
        route.validate(dict(self.rule, EventPattern=json.dumps(self.pattern)), self.targets)

    def test_exact_shared_forwarding_route(self):
        self.validate()

    def test_missing_or_disabled_rule(self):
        self.rule["State"] = "DISABLED"
        with self.assertRaises(ValueError):
            self.validate()

    def test_every_production_alarm_is_selected(self):
        self.pattern["$or"][0]["resources"].remove(sorted(route.ALARMS)[0])
        with self.assertRaises(ValueError):
            self.validate()

    def test_account_and_native_state_filters_remain(self):
        self.pattern["account"] = ["222222222222"]
        with self.assertRaises(ValueError):
            self.validate()
        self.pattern["account"] = [route.ACCOUNT]
        self.pattern["$or"][0]["detail"]["state"]["value"] = ["ALARM"]
        with self.assertRaises(ValueError):
            self.validate()

    def test_additional_pattern_constraints_and_branches_are_rejected(self):
        original = copy.deepcopy(self.pattern)
        for change in (
            lambda p: p.update(resources=["different alarm"]),
            lambda p: p.update(detail={"state": {"value": ["INSUFFICIENT_DATA"]}}),
            lambda p: p["$or"].append({"source": ["unapproved.service"]}),
            lambda p: p["$or"][0].update(region=["us-east-1"]),
            lambda p: p["$or"][1].update(source=["different.service"]),
        ):
            self.pattern = copy.deepcopy(original)
            change(self.pattern)
            with self.assertRaises(ValueError):
                self.validate()

    def test_semantic_array_order_is_irrelevant(self):
        self.pattern["$or"].reverse()
        self.pattern["$or"][-1]["resources"].reverse()
        self.validate()

    def test_destination_role_or_input_substitution_is_rejected(self):
        original = copy.deepcopy(self.targets)
        for field, value in (("Arn", "arn:aws:sns:us-west-2:180294223248:other"),
                             ("RoleArn", "arn:aws:iam::180294223248:role/other"),
                             ("Input", "replacement event")):
            with self.subTest(field=field):
                self.targets = copy.deepcopy(original)
                self.targets["Targets"][0][field] = value
                with self.assertRaises(ValueError):
                    self.validate()


if __name__ == "__main__":
    unittest.main()
