"""Offline security regressions: no credentials or AWS calls are used."""
import contextlib
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('wrapper', Path(__file__).with_name('create-cognito-dev-plan.py'))
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)
c = w.contract
SENTINEL = 'SENTINEL-PRIVATE-GOOGLE-CREDENTIAL'


def fixture():
    resources = []
    config = []
    for address, values in c.RESOURCES.items():
        after = copy.deepcopy(values)
        if address.endswith('provider.google'):
            after['provider_details'] = dict(authorize_scopes='openid email profile', client_id=SENTINEL, client_secret=SENTINEL)
        unknown = {}
        if address in {'aws_cognito_user_pool.management', 'aws_cognito_user_pool_client.management'}:
            unknown['id'] = True
        if address != 'aws_cognito_user_pool.management':
            unknown['user_pool_id'] = True
        if 'branding' in address:
            unknown['client_id'] = True
        resources.append(dict(address=address, mode='managed', provider_name='registry.opentofu.org/hashicorp/aws', change=dict(actions=['create'], after=after, after_unknown=unknown)))
        expressions = {}
        if address != 'aws_cognito_user_pool.management':
            expressions['user_pool_id'] = {'references': ['aws_cognito_user_pool.management.id', 'aws_cognito_user_pool.management']}
        if 'branding' in address:
            expressions['client_id'] = {'references': ['aws_cognito_user_pool_client.management.id', 'aws_cognito_user_pool_client.management']}
        config.append(dict(address=address, expressions=expressions))
    return dict(resource_changes=resources, configuration=dict(provider_config={'aws': {'expressions': {'region': {'constant_value': c.REGION}, 'allowed_account_ids': {'constant_value': [c.ACCOUNT]}}}}, root_module={'resources': config, 'outputs': {k: {} for k in ['cognito_user_pool_id', 'cognito_domain', 'cognito_issuer', 'cognito_client_id', 'google_redirect_uri', 'session_parameter_path', 'management_runtime']}}))


class ContractTests(unittest.TestCase):
    def test_safe_summary(self):
        self.assertNotIn(SENTINEL, json.dumps(c.check(fixture())))

    def test_tampered_contracts(self):
        cases = [
            lambda p: p['configuration']['root_module']['outputs'].update(secret_output={}),
            lambda p: p['resource_changes'][0]['change']['after'].update(lambda_config=[{'pre_sign_up': 'arn:aws:lambda:us-west-2:180294223248:function:other'}]),
            lambda p: p['resource_changes'][1]['change']['after']['provider_details'].update(token_url='https://evil.example'),
            lambda p: p['configuration']['provider_config']['aws']['expressions'].update(allowed_account_ids={'constant_value': ['000000000000']}),
            lambda p: p['resource_changes'][0]['change'].update(actions=['delete', 'create']),
            lambda p: p['resource_changes'][0].update(address='aws_iam_role.unrelated'),
            lambda p: p['resource_changes'][2]['change']['after'].update(generate_secret=True),
            lambda p: p['resource_changes'][2]['change']['after'].update(allowed_oauth_flows=['implicit']),
            lambda p: p['resource_changes'][2]['change']['after'].update(callback_urls=['https://evil.example/callback']),
            lambda p: p['resource_changes'][1]['change']['after'].update(attribute_mapping={'email': 'name'}),
            lambda p: p['configuration']['provider_config']['aws']['expressions'].update(region={'constant_value': 'us-east-1'}),
            lambda p: p['configuration']['root_module']['resources'][1]['expressions'].update(user_pool_id={'constant_value': 'another-pool'}),
        ]
        for mutate in cases:
            with self.subTest(mutate=mutate):
                plan = fixture(); mutate(plan)
                with self.assertRaises(ValueError): c.check(plan)

    def known_fixture(self, action='no-op'):
        plan = fixture()
        for resource in plan['resource_changes']:
            change = resource['change']
            change['actions'] = [action]
            for key in list(change['after_unknown']):
                change['after'][key] = 'client123' if key == 'client_id' or (key == 'id' and 'pool_client' in resource['address']) else 'us-west-2_reviewed'
            change['after_unknown'] = {}
        return plan

    def test_known_relationships(self):
        for action in ['create', 'update', 'no-op']:
            with self.subTest(action=action):
                self.assertEqual(len(c.check(self.known_fixture(action))), 5)
        for index, key, value in [(1, 'user_pool_id', 'us-west-2_unrelated'), (2, 'user_pool_id', 'us-west-2_unrelated'), (3, 'user_pool_id', 'us-west-2_unrelated'), (4, 'user_pool_id', 'us-west-2_unrelated'), (4, 'client_id', 'otherclient123'), (0, 'id', 'us-east-1_reviewed'), (2, 'id', 'invalid-client!')]:
            with self.subTest(index=index, key=key):
                plan = self.known_fixture()
                plan['resource_changes'][index]['change']['after'][key] = value
                with self.assertRaises(ValueError): c.check(plan)

    def test_provider_default_email_configuration_converges(self):
        plan = self.known_fixture('no-op')
        plan['resource_changes'][0]['change']['after']['email_configuration'] = [{
            'configuration_set': None,
            'email_sending_account': 'COGNITO_DEFAULT',
            'from_email_address': '',
            'reply_to_email_address': None,
            'source_arn': '',
        }]
        self.assertEqual(len(c.check(plan)), 5)

    def test_custom_email_configuration_is_rejected(self):
        cases = [
            {'email_sending_account': 'DEVELOPER'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'source_arn': 'arn:aws:ses:us-west-2:180294223248:identity/example.com'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'from_email_address': 'Portfolio <portfolio@example.com>'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'reply_to_email_address': 'reply@example.com'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'configuration_set': 'production'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'unexpected': ''},
        ]
        for email_configuration in cases:
            with self.subTest(email_configuration=email_configuration):
                plan = self.known_fixture('no-op')
                plan['resource_changes'][0]['change']['after']['email_configuration'] = [email_configuration]
                with self.assertRaises(ValueError):
                    c.check(plan)

    def test_unknown_relationship_policy(self):
        self.assertEqual(len(c.check(fixture())), 5)
        # Only an explicitly unknown create may omit an ID. A known dependency
        # cannot contradict an unknown source, and existing objects need IDs.
        for index, key, value in [(1, 'user_pool_id', 'us-west-2_unrelated'), (4, 'client_id', 'client123')]:
            plan = fixture(); change = plan['resource_changes'][index]['change']
            change['after'][key] = value; change['after_unknown'].pop(key)
            with self.assertRaises(ValueError): c.check(plan)
        for mutation in ['missing-marker', 'unknown-update', 'unknown-against-known']:
            plan = fixture()
            if mutation == 'missing-marker': plan['resource_changes'][0]['change']['after_unknown'] = {}
            elif mutation == 'unknown-update': plan['resource_changes'][0]['change']['actions'] = ['update']
            else:
                plan = self.known_fixture('create')
                change = plan['resource_changes'][1]['change']
                change['after'].pop('user_pool_id'); change['after_unknown']['user_pool_id'] = True
            with self.subTest(mutation=mutation), self.assertRaises(ValueError): c.check(plan)

    def test_backend_tamper(self):
        good = {'backend': {'type': 's3', 'config': {k: v for k, v in c.BACKEND.items() if k != 'type'}}}
        self.assertEqual(w.backend(good), c.BACKEND)
        for key, value in [('key', 'other/state'), ('encrypt', False), ('endpoints', {'s3': 'https://evil.example'}), ('profile', 'admin')]:
            bad = copy.deepcopy(good); bad['backend']['config'][key] = value
            with self.assertRaises(ValueError): w.backend(bad)


class WrapperTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve()
        self.root.chmod(0o700)
        self.cred = self.root / 'credentials.json'
        w.write_private(self.cred, json.dumps(dict(client_id=SENTINEL, client_secret=SENTINEL)).encode())
        self.plan = self.root / 'plan'
        self.env = dict(PATH=os.environ['PATH'], HOME=os.environ['HOME'], AWS_PROFILE='portfolio-deployer', AWS_REGION=c.REGION, COGNITO_PRIVATE_DIR=str(self.root), GOOGLE_OAUTH_CREDENTIALS_FILE=str(self.cred), PLAN_FILE=str(self.plan), APPROVED_STATE_LOCK_URI=f"s3://{c.BACKEND['bucket']}/{c.BACKEND['key']}.tflock")
        self.calls = []
        self.fail = False
        self.fail_at = None

    def tearDown(self):
        self.temp.cleanup()

    def run_fake(self, args, **kwargs):
        self.calls.append((args, kwargs['env']))
        self.assertNotIn(SENTINEL, ' '.join(args))
        data = b''
        if args[0] == 'aws':
            data = json.dumps(dict(Account=c.ACCOUNT, Arn=f'arn:aws:sts::{c.ACCOUNT}:assumed-role/AWSReservedSSO_PortfolioDeployer_123/test')).encode()
        elif args[2] == 'init':
            (Path(kwargs['env']['TF_DATA_DIR']) / 'terraform.tfstate').write_text(json.dumps({'backend': {'type': 's3', 'config': {k: v for k, v in c.BACKEND.items() if k != 'type'}}}))
        elif args[2] == 'workspace': data = b'default\n'
        elif args[2] == 'plan':
            self.assertEqual(kwargs['env']['TF_VAR_google_client_secret'], SENTINEL)
            Path(args[-1].split('=', 1)[1]).write_bytes(SENTINEL.encode())
            data = SENTINEL.encode()
        elif args[2] == 'show': data = json.dumps(fixture()).encode()
        return subprocess.CompletedProcess(args, 1 if self.fail or (len(args) > 2 and args[2] == self.fail_at) else 0, data, SENTINEL.encode())

    def invoke(self, mode='plan'):
        out, err = io.StringIO(), io.StringIO()
        with patch.dict(os.environ, self.env, clear=True), patch.object(w.sys, 'argv', ['wrapper', mode]), patch.object(w.subprocess, 'run', self.run_fake), contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            try: w.main()
            except Exception:
                print('Cognito operation rejected or failed; inspect private artifacts locally.', file=err)
        self.assertNotIn(SENTINEL, out.getvalue() + err.getvalue())
        return out.getvalue(), err.getvalue()

    def test_cli_failure_diagnostic_never_echoes_input(self):
        result = subprocess.run([sys.executable, str(Path(__file__).with_name('create-cognito-dev-plan.py')), 'plan'], env=dict(self.env, TF_LOG=SENTINEL), capture_output=True, check=False)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn(SENTINEL.encode(), result.stdout + result.stderr)
        self.assertIn(b'Cognito operation rejected or failed', result.stderr)

    def test_plan_and_apply_secret_confinement(self):
        out, err = self.invoke()
        self.assertFalse(err)
        review = json.loads(out)
        self.assertEqual(self.plan.stat().st_mode & 0o777, 0o600)
        for args, env in self.calls:
            if args[0] == 'aws' or args[2] != 'plan': self.assertNotIn('TF_VAR_google_client_secret', env)
        self.env.update(APPROVED_PLAN_SHA256=review['plan_sha256'], APPROVED_PROVENANCE_SHA256=review['provenance_sha256'])
        out, err = self.invoke('apply')
        self.assertFalse(err)
        self.assertTrue(any(args[2:3] == ['apply'] for args, _ in self.calls))

    def test_apply_tamper_before_aws(self):
        out, _ = self.invoke(); review = json.loads(out)
        self.env.update(APPROVED_PLAN_SHA256=review['plan_sha256'], APPROVED_PROVENANCE_SHA256=review['provenance_sha256'])
        self.plan.write_bytes(b'tampered'); self.calls.clear()
        _, err = self.invoke('apply')
        self.assertTrue(err); self.assertFalse(self.calls)

    def test_provenance_tamper_before_aws(self):
        out, _ = self.invoke(); review = json.loads(out)
        self.env.update(APPROVED_PLAN_SHA256=review['plan_sha256'], APPROVED_PROVENANCE_SHA256=review['provenance_sha256'])
        Path(str(self.plan) + '.provenance.json').write_text('{}'); self.calls.clear()
        _, err = self.invoke('apply')
        self.assertTrue(err); self.assertFalse(self.calls)

    def test_export_rejects_extra_fields(self):
        value = dict(cognito_domain=f'https://{c.PREFIX}.auth.{c.REGION}.amazoncognito.com', cognito_issuer='https://cognito-idp.us-west-2.amazonaws.com/us-west-2_example', cognito_client_id='example123', redirect_uri='https://dev.craigdevjohnson.com/callback', logout_uri='https://dev.craigdevjohnson.com/login', allowed_emails=['craigdevjohnson@gmail.com'], allow_local_callback=False, ec2_management_tag_key='PortfolioManagement', ec2_management_tag_value='dev')
        self.assertEqual(c.runtime(value), value)
        value['client_secret'] = SENTINEL
        with self.assertRaises(ValueError): c.runtime(value)

    def test_provider_failure_is_private(self):
        self.fail = True
        out, err = self.invoke()
        self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.plan.exists())

    def test_plan_and_show_failures_are_private(self):
        for phase in ['plan', 'show']:
            with self.subTest(phase=phase):
                self.fail_at = phase
                out, err = self.invoke()
                self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.plan.exists())

    def test_unsafe_credentials_before_aws(self):
        for mode in ['readable', 'symlink', 'malformed', 'missing']:
            with self.subTest(mode=mode):
                self.cred.unlink(missing_ok=True)
                if mode == 'readable': self.cred.write_text('{}'); self.cred.chmod(0o644)
                elif mode == 'symlink': self.cred.symlink_to(self.root / 'missing')
                elif mode == 'malformed': w.write_private(self.cred, SENTINEL.encode())
                self.calls.clear()
                out, err = self.invoke()
                self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.calls)

    def test_existing_relative_and_env_overrides(self):
        for key, value in [('PLAN_FILE', 'relative'), ('PLAN_FILE', str(self.root / 'child' / '..' / 'plan')), ('COGNITO_PRIVATE_DIR', str(self.root / 'child' / '..')), ('TF_LOG', 'TRACE'), ('AWS_ENDPOINT_URL', 'https://evil.example')]:
            with self.subTest(key=key):
                original = dict(self.env); self.env[key] = value
                _, err = self.invoke(); self.assertTrue(err); self.assertFalse(self.calls)
                self.env = original
        self.plan.write_bytes(b'existing')
        _, err = self.invoke(); self.assertTrue(err); self.assertFalse(self.calls)


if __name__ == '__main__':
    unittest.main()
