#!/usr/bin/env python3
"""Synthetic public observation evidence never represents live acceptance."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('observer', Path(__file__).resolve().parents[1] / 'scripts/observe-lambda-production.py')
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)


class PublicEvidence(unittest.TestCase):
    def setUp(self):
        self.start = 1800000000
        self.binding = dict(promotion_sha='a' * 40, source_sha='b' * 40, image_digest='sha256:' + 'c' * 64,
                            production_deployment_id='123', lambda_version='8', base_url=p.APEX)
        self.binding['window_id'] = p.window_id(self.binding['promotion_sha'], '123')
        self.window = dict(self.binding, schema_version=1, ci_origin_window='passed',
                           started_at=p.utc(self.start), ended_at=p.utc(self.start + 2100),
                           observations=[dict(observed_at=p.utc(self.start + t), elapsed_seconds=t,
                                              lambda_version='8') for t in range(0, 2101, 30)])
        self.receipt = dict(self.binding, schema_version=1, operator='CraigDevJohnson',
                            collector_sha=self.binding['promotion_sha'], operator_public_window='passed',
                            started_at=p.utc(self.start + 30), ended_at=p.utc(self.start + 1830), interval_seconds=30,
                            observations=[dict(observed_at=p.utc(self.start + 30 + t), elapsed_seconds=t,
                                               lambda_version='8', checks=list(p.PUBLIC_CHECKS)) for t in range(0, 1801, 30)],
                            fresh_public_read=dict(observed_at=p.utc(self.start + 2110), duration_seconds=2,
                                                   checks=list(p.PUBLIC_CHECKS), binding_sha256=p.binding_digest(self.binding)))

    def validate(self):
        return p.validate_public(self.window, self.receipt, now=self.start + 2120)

    def test_independent_windows_require_actual_common_coverage(self):
        self.assertEqual(self.validate(), {'started_at': p.utc(self.start + 30), 'ended_at': p.utc(self.start + 1830)})
        self.receipt['observations'] = self.receipt['observations'][1:]
        self.receipt['started_at'] = self.receipt['observations'][0]['observed_at']
        with self.assertRaises(ValueError): self.validate()

    def test_complete_but_nonoverlapping_windows_fail(self):
        for item in self.receipt['observations']:
            item['observed_at'] = p.utc(p.timestamp(item['observed_at']) - 600)
        self.receipt['started_at'] = self.receipt['observations'][0]['observed_at']
        self.receipt['ended_at'] = self.receipt['observations'][-1]['observed_at']
        with self.assertRaisesRegex(ValueError, 'common coverage'): self.validate()

    def test_flags_do_not_replace_route_elapsed_or_gap_evidence(self):
        original = copy.deepcopy(self.receipt)
        mutations = [lambda r: r.update(observations=[]),
                     lambda r: r['observations'][5]['checks'].pop(),
                     lambda r: r['observations'][5].update(checks=['challenge-accepted']),
                     lambda r: r['observations'][5].update(elapsed_seconds=0),
                     lambda r: r.update(observations=r['observations'][:2]+r['observations'][5:]),
                     lambda r: r.update(interval_seconds=31),
                     lambda r: r.update(token='do-not-save'),
                     lambda r: r['fresh_public_read']['checks'].pop(),
                     lambda r: r['fresh_public_read'].update(binding_sha256="f"*64)]
        for change in mutations:
            self.receipt = copy.deepcopy(original)
            change(self.receipt)
            with self.subTest(change=change), self.assertRaises(ValueError): self.validate()

    def test_stale_future_and_substituted_evidence_fail(self):
        original = copy.deepcopy(self.receipt)
        for field, value in [('source_sha', 'd'*40), ('promotion_sha', 'e'*40), ('production_deployment_id', '999'),
                             ('window_id', 'different'), ('image_digest', 'sha256:'+'e'*64),
                             ('lambda_version', '9'), ('operator', 'someone'), ('collector_sha', 'f'*40)]:
            self.receipt = copy.deepcopy(original)
            self.receipt[field] = value
            with self.subTest(field=field), self.assertRaises(ValueError): self.validate()
        for offset in (1800, 2121):
            self.receipt = copy.deepcopy(original)
            self.receipt['fresh_public_read']['observed_at'] = p.utc(self.start + offset)
            with self.assertRaises(ValueError): self.validate()
        self.receipt = original
        with self.assertRaises(ValueError): p.validate_public(self.window, self.receipt, now=self.start + 40000)

    def test_refresh_rechecks_all_public_routes_and_preserves_coordinates(self):
        with tempfile.TemporaryDirectory() as folder:
            target = Path(folder) / 'receipt.json'
            fresh = dict(observed_at=p.utc(self.start + 2120), duration_seconds=2, checks=p.PUBLIC_CHECKS, binding_sha256=p.binding_digest(self.binding))
            with patch.object(p, 'checked_operator'), patch.object(p.time, 'time', return_value=self.start+2120), \
                    patch.object(p, 'public_read', return_value=fresh) as read:
                p.refresh_public(self.window, self.receipt, target)
                read.assert_called_once_with(self.receipt)
                self.assertEqual(json.loads(target.read_text())['fresh_public_read'], fresh)
                with self.assertRaises(ValueError): p.refresh_public(self.window, self.receipt, target)

    def test_collector_failure_is_never_success_and_has_no_origin_override(self):
        with tempfile.TemporaryDirectory() as folder:
            target = Path(folder) / 'receipt.json'
            with patch.object(p, 'checked_operator', return_value='CraigDevJohnson'), \
                    patch.object(p, 'public_probe', side_effect=ValueError('HTTP 403')) as probe:
                with self.assertRaises(ValueError): p.observe_public(self.binding, target)
                probe.assert_called_once_with(self.binding['source_sha'])
                self.assertEqual(json.loads(target.read_text()), {})
                with self.assertRaises(ValueError): p.observe_public(self.binding, target)

    def test_compact_dispatch_payload_preserves_both_receipts(self):
        browser = {key: self.binding[key] for key in p.BINDING}
        browser.update(schema_version=1, operator="CraigDevJohnson", observations=[{
            "observed_at": sample["observed_at"], "soccer": "authorized_current_data",
            "google_calendar": "created_read_back_and_deleted_test_event" if index in (0, 60) else "connected_calendar_read",
            "cookie": {"secure": True, "http_only": True, "same_site": "Lax", "path": "/soccer"},
            "authenticated_cache_control": "no-store"}
            for index, sample in enumerate(self.receipt["observations"])])
        with tempfile.TemporaryDirectory() as folder, patch.object(p.time, "time", return_value=self.start+2120):
            target = Path(folder) / "inputs.json"
            p.dispatch_inputs(self.window, self.receipt, browser, "777", target)
            payload = json.loads(target.read_text())
            self.assertLessEqual(target.stat().st_size, 60000)
            self.assertEqual(json.loads(payload["inputs"]["public_receipt_json"]), self.receipt)
            self.assertEqual(json.loads(payload["inputs"]["browser_receipt_json"]), browser)
            self.assertEqual(payload["inputs"]["apply_run_id"], "777")
            with self.assertRaises(FileExistsError):
                p.dispatch_inputs(self.window, self.receipt, browser, "777", target)

    def test_cli_duration_defaults_to_35_minutes_and_accepts_explicit_duration(self):
        with tempfile.TemporaryDirectory() as folder:
            binding, output = Path(folder) / "binding.json", Path(folder) / "public.json"
            binding.write_text(json.dumps(self.binding))
            for extra, expected in (([], 2100), (["2400"], 2400)):
                with patch.object(p.sys, "argv", ["observer", "observe-public", str(binding), str(output), *extra]), \
                        patch.object(p, "observe_public") as collect:
                    p.main()
                    self.assertEqual(collect.call_args.kwargs["duration"], expected)
            for invalid in (1799, 3601):
                target = Path(folder) / str(invalid)
                with patch.object(p, "checked_operator", return_value="CraigDevJohnson"), \
                        patch.object(p, "public_probe") as probe:
                    with self.assertRaises(ValueError): p.observe_public(self.binding, target, duration=invalid)
                    probe.assert_not_called()

    def test_public_curl_disables_user_config_proxies_and_origin_override(self):
        with patch.object(p.subprocess, 'run', side_effect=RuntimeError('capture invocation')) as run:
            with self.assertRaises(RuntimeError): p.fetch(p.APEX, '/healthz')
            command = run.call_args.args[0]
            self.assertEqual(command[:4], ['curl', '--disable', '--noproxy', '*'])
            self.assertNotIn('--connect-to', command)
            self.assertNotIn('--resolve', command)
            self.assertNotIn('-k', command)


if __name__ == '__main__':
    unittest.main()
