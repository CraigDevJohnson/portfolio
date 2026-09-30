"""Offline security regressions for the site identity operator path.

No credentials, AWS, Cognito or Google calls are used: every subprocess is a
fake, and the private inputs are synthetic sentinels.
"""
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

# Synthetic account; the contract module reads PORTFOLIO_ACCOUNT_ID at import.
os.environ['PORTFOLIO_ACCOUNT_ID'] = '111122223333'
spec = importlib.util.spec_from_file_location('site_operator', Path(__file__).with_name('cognito-site-operator.py'))
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)
c = w.contract
SENTINEL = 'SENTINEL-PRIVATE-GOOGLE-CREDENTIAL'
ENVIRONMENTS = ['dev', 'prod']
POOL = 'module.site.aws_cognito_user_pool.site'
GOOGLE = 'module.site.aws_cognito_identity_provider.google'
CLIENT = 'module.site.aws_cognito_user_pool_client.site'
DOMAIN = 'module.site.aws_cognito_user_pool_domain.site'
BRANDING = 'module.site.aws_cognito_managed_login_branding.site'
ORIGIN = {'dev': 'https://dev.craigdevjohnson.com', 'prod': 'https://craigdevjohnson.com'}


def fixture(env='dev'):
    """A create plan for one site root, shaped like `tofu show -json`."""
    site = c.contract(env)
    resources = []
    config = []
    for address, values in site.resources.items():
        after = copy.deepcopy(values)
        if address == GOOGLE:
            after['provider_details'] = dict(authorize_scopes='openid email profile', client_id=SENTINEL, client_secret=SENTINEL)
        unknown = {}
        if address in {POOL, CLIENT}:
            unknown['id'] = True
        if address != POOL:
            unknown['user_pool_id'] = True
        if address == BRANDING:
            unknown['client_id'] = True
        resources.append(dict(address=address, module_address='module.site', mode='managed', provider_name='registry.opentofu.org/hashicorp/aws', change=dict(actions=['create'], after=after, after_unknown=unknown)))
        expressions = {}
        if address != POOL:
            expressions['user_pool_id'] = {'references': ['aws_cognito_user_pool.site.id', 'aws_cognito_user_pool.site']}
        if address == BRANDING:
            expressions['client_id'] = {'references': ['aws_cognito_user_pool_client.site.id', 'aws_cognito_user_pool_client.site']}
        config.append(dict(address=address.removeprefix('module.site.'), expressions=expressions))
    variables = {'aws_account_id': {'value': c.ACCOUNT}, 'cognito_domain_prefix': {'value': None}, 'google_client_id': {'value': SENTINEL}, 'google_client_secret': {'value': SENTINEL}}
    if env == 'dev':
        variables['enable_local_callback'] = {'value': False}
    module_call = dict(source='../modules/pool', expressions={'environment': {'constant_value': env}}, module={'resources': config})
    root = {'outputs': {k: {} for k in ['cognito_user_pool_id', 'google_redirect_uri', 'session_parameter_path', 'site_runtime']}, 'module_calls': {'site': module_call}}
    provider = {'aws': {'expressions': {'region': {'constant_value': c.REGION}, 'allowed_account_ids': {'references': ['var.aws_account_id']}}}}
    return dict(resource_changes=resources, variables=variables, configuration=dict(provider_config=provider, root_module=root))


def by_address(plan, address):
    return next(r for r in plan['resource_changes'] if r['address'] == address)


def reviewed_runtime(env='dev'):
    return dict(cognito_domain=f'https://portfolio-lambda-{env}-site-{c.ACCOUNT}.auth.{c.REGION}.amazoncognito.com', cognito_issuer='https://cognito-idp.us-west-2.amazonaws.com/us-west-2_example', cognito_client_id='example123', redirect_uri=f'{ORIGIN[env]}/auth/callback', logout_uri=f'{ORIGIN[env]}/sign-in', allow_local_callback=False)


class ContractTests(unittest.TestCase):
    def test_safe_summary(self):
        for env in ENVIRONMENTS:
            with self.subTest(env=env):
                summary = c.check(fixture(env), env)
                self.assertEqual(summary, sorted([{'resource': address, 'action': 'create'} for address in [POOL, GOOGLE, CLIENT, DOMAIN, BRANDING]], key=lambda r: r['resource']))
                self.assertNotIn(SENTINEL, json.dumps(summary))

    def test_each_environment_admits_only_its_own_names_and_urls(self):
        dev, prod = fixture('dev'), fixture('prod')
        self.assertEqual(by_address(dev, CLIENT)['change']['after']['callback_urls'], ['https://dev.craigdevjohnson.com/auth/callback'])
        self.assertEqual(by_address(prod, CLIENT)['change']['after']['callback_urls'], ['https://craigdevjohnson.com/auth/callback'])
        self.assertEqual(by_address(prod, DOMAIN)['change']['after']['domain'], 'portfolio-lambda-prod-site-111122223333')
        with self.assertRaises(ValueError): c.check(dev, 'prod')
        with self.assertRaises(ValueError): c.check(prod, 'dev')
        with self.assertRaises(ValueError): c.check(dev, 'staging')

    def test_tampered_contracts(self):
        cases = [
            lambda p: p['configuration']['root_module']['outputs'].update(secret_output={}),
            lambda p: by_address(p, POOL)['change']['after'].update(lambda_config=[{'pre_sign_up': 'arn:aws:lambda:us-west-2:111122223333:function:other'}]),
            lambda p: by_address(p, GOOGLE)['change']['after']['provider_details'].update(token_url='https://evil.example'),
            lambda p: p['configuration']['provider_config']['aws']['expressions'].update(allowed_account_ids={'constant_value': ['000000000000']}),
            lambda p: p['variables']['aws_account_id'].update(value='000000000000'),
            lambda p: by_address(p, POOL)['change'].update(actions=['delete', 'create']),
            lambda p: by_address(p, POOL).update(address='aws_iam_role.unrelated'),
            lambda p: by_address(p, CLIENT)['change']['after'].update(generate_secret=True),
            lambda p: by_address(p, CLIENT)['change']['after'].update(allowed_oauth_flows=['implicit']),
            lambda p: by_address(p, CLIENT)['change']['after'].update(callback_urls=['https://evil.example/auth/callback']),
            lambda p: by_address(p, CLIENT)['change']['after']['callback_urls'].append('http://localhost:8080/auth/callback'),
            lambda p: by_address(p, CLIENT)['change']['after'].update(logout_urls=['https://evil.example/sign-in']),
            lambda p: by_address(p, DOMAIN)['change']['after'].update(domain='portfolio-lambda-dev-site-reviewed'),
            lambda p: by_address(p, GOOGLE)['change']['after'].update(attribute_mapping={'email': 'name'}),
            lambda p: by_address(p, GOOGLE)['change']['after'].update(region='us-east-1'),
            lambda p: p['configuration']['provider_config']['aws']['expressions'].update(region={'constant_value': 'us-east-1'}),
            lambda p: p['configuration']['root_module']['module_calls']['site']['module']['resources'][1]['expressions'].update(user_pool_id={'constant_value': 'another-pool'}),
            lambda p: p['configuration']['root_module']['module_calls']['site'].update(source='git::https://evil.example/pool'),
            lambda p: p['configuration']['root_module']['module_calls']['site']['expressions'].update(environment={'constant_value': 'other'}),
            lambda p: p['configuration']['root_module']['module_calls'].update(other={'source': './other'}),
            lambda p: p['configuration']['root_module'].update(resources=[{'address': 'aws_iam_role.unrelated'}]),
            lambda p: p['configuration']['root_module']['module_calls']['site']['module']['resources'][0].update(provisioners=[{'type': 'local-exec'}]),
            lambda p: p['variables'].update(cognito_domain_prefix={'value': 'portfolio-lambda-dev-site-reviewed'}),
            lambda p: p['variables'].update(enable_local_callback={'value': True}),
            lambda p: p['variables'].update(unexpected={'value': 'x'}),
            lambda p: p.update(resource_drift=[{'address': POOL}]),
            lambda p: p.update(errored=True),
            lambda p: by_address(p, POOL).update(module_address='module.other'),
        ]
        for env in ENVIRONMENTS:
            for index, mutate in enumerate(cases):
                with self.subTest(env=env, case=index):
                    plan = fixture(env); mutate(plan)
                    with self.assertRaises(ValueError): c.check(plan, env)

    def known_fixture(self, env='dev', action='no-op'):
        plan = fixture(env)
        for resource in plan['resource_changes']:
            change = resource['change']
            change['actions'] = [action]
            for key in list(change['after_unknown']):
                change['after'][key] = 'client123' if key == 'client_id' or (key == 'id' and resource['address'] == CLIENT) else 'us-west-2_reviewed'
            change['after_unknown'] = {}
            change['after']['region'] = c.REGION
        return plan

    def test_known_relationships(self):
        for action in ['create', 'update', 'no-op']:
            with self.subTest(action=action):
                self.assertEqual(len(c.check(self.known_fixture('prod', action), 'prod')), 5)
        for address, key, value in [(GOOGLE, 'user_pool_id', 'us-west-2_unrelated'), (CLIENT, 'user_pool_id', 'us-west-2_unrelated'), (DOMAIN, 'user_pool_id', 'us-west-2_unrelated'), (BRANDING, 'user_pool_id', 'us-west-2_unrelated'), (BRANDING, 'client_id', 'otherclient123'), (POOL, 'id', 'us-east-1_reviewed'), (CLIENT, 'id', 'invalid-client!')]:
            with self.subTest(address=address, key=key):
                plan = self.known_fixture()
                by_address(plan, address)['change']['after'][key] = value
                with self.assertRaises(ValueError): c.check(plan, 'dev')

    def drift(self, address=POOL, actions=('update',), **changed):
        """A refresh difference as `tofu show -json` lists it in resource_drift."""
        before = dict(name='portfolio-lambda-dev-site', callback_urls=['https://dev.craigdevjohnson.com/auth/callback'], estimated_number_of_users=0, last_modified_date='2026-09-30T00:00:00Z')
        after = dict(before, **changed)
        return dict(address=address, module_address='module.site', mode='managed', provider_name='registry.opentofu.org/hashicorp/aws', change=dict(actions=list(actions), before=before, after=after, after_unknown={}, before_sensitive={}, after_sensitive={}))

    def test_computed_pool_drift_is_tolerated(self):
        # Google sign-in creates a federated user, so every later refresh
        # reports the pool's user count as drift; that is not a change.
        for changed in [dict(estimated_number_of_users=1), dict(estimated_number_of_users=1, last_modified_date='2026-10-01T00:00:00Z')]:
            with self.subTest(changed=changed):
                plan = self.known_fixture('dev', 'no-op')
                plan['resource_drift'] = [self.drift(**changed)]
                self.assertEqual(len(c.check(plan, 'dev')), 5)

    def test_configuration_drift_is_rejected(self):
        cases = [
            self.drift(callback_urls=['https://evil.example/auth/callback']),
            self.drift(estimated_number_of_users=1, name='portfolio-lambda-dev-other'),
            self.drift(estimated_number_of_users=1, lambda_config=[{'pre_sign_up': 'arn:aws:lambda:us-west-2:111122223333:function:other'}]),
            self.drift(address=CLIENT, estimated_number_of_users=1),
            self.drift(address='module.site.aws_iam_role.unrelated', estimated_number_of_users=1),
            self.drift(actions=('delete',), estimated_number_of_users=1),
            self.drift(actions=('delete', 'create'), estimated_number_of_users=1),
            dict(address=POOL),
            {'address': POOL, 'change': {'actions': ['update'], 'before': None, 'after': {'estimated_number_of_users': 1}}},
        ]
        for index, entry in enumerate(cases):
            with self.subTest(case=index):
                plan = self.known_fixture('dev', 'no-op')
                plan['resource_drift'] = [self.drift(estimated_number_of_users=1), entry]
                with self.assertRaises(ValueError): c.check(plan, 'dev')

    def test_provider_default_email_configuration_converges(self):
        plan = self.known_fixture('dev', 'no-op')
        by_address(plan, POOL)['change']['after']['email_configuration'] = [{
            'configuration_set': None,
            'email_sending_account': 'COGNITO_DEFAULT',
            'from_email_address': '',
            'reply_to_email_address': None,
            'source_arn': '',
        }]
        self.assertEqual(len(c.check(plan, 'dev')), 5)

    def test_custom_email_configuration_is_rejected(self):
        cases = [
            {'email_sending_account': 'DEVELOPER'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'source_arn': 'arn:aws:ses:us-west-2:111122223333:identity/example.com'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'from_email_address': 'Portfolio <portfolio@example.com>'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'reply_to_email_address': 'reply@example.com'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'configuration_set': 'production'},
            {'email_sending_account': 'COGNITO_DEFAULT', 'unexpected': ''},
        ]
        for email_configuration in cases:
            with self.subTest(email_configuration=email_configuration):
                plan = self.known_fixture('dev', 'no-op')
                by_address(plan, POOL)['change']['after']['email_configuration'] = [email_configuration]
                with self.assertRaises(ValueError):
                    c.check(plan, 'dev')

    def test_unknown_relationship_policy(self):
        self.assertEqual(len(c.check(fixture(), 'dev')), 5)
        # Only an explicitly unknown create may omit an ID. A known dependency
        # cannot contradict an unknown source, and existing objects need IDs.
        for address, key, value in [(GOOGLE, 'user_pool_id', 'us-west-2_unrelated'), (BRANDING, 'client_id', 'client123')]:
            plan = fixture(); change = by_address(plan, address)['change']
            change['after'][key] = value; change['after_unknown'].pop(key)
            with self.assertRaises(ValueError): c.check(plan, 'dev')
        for mutation in ['missing-marker', 'unknown-update', 'unknown-against-known']:
            plan = fixture()
            if mutation == 'missing-marker': by_address(plan, POOL)['change']['after_unknown'] = {}
            elif mutation == 'unknown-update': by_address(plan, POOL)['change']['actions'] = ['update']
            else:
                plan = self.known_fixture('dev', 'create')
                change = by_address(plan, GOOGLE)['change']
                change['after'].pop('user_pool_id'); change['after_unknown']['user_pool_id'] = True
            with self.subTest(mutation=mutation), self.assertRaises(ValueError): c.check(plan, 'dev')

    def test_backend_tamper(self):
        for env in ENVIRONMENTS:
            site = c.contract(env)
            good = {'backend': {'type': 's3', 'config': {k: v for k, v in site.backend.items() if k != 'type'}}}
            self.assertEqual(w.backend(good, site), site.backend)
            other = 'prod' if env == 'dev' else 'dev'
            for key, value in [('key', f'portfolio-lambda-http-api/auth/site/{other}/terraform.tfstate'), ('key', 'portfolio-lambda-http-api/auth/dev/terraform.tfstate'), ('encrypt', False), ('use_lockfile', False), ('endpoints', {'s3': 'https://evil.example'}), ('profile', 'admin')]:
                with self.subTest(env=env, key=key, value=value):
                    bad = copy.deepcopy(good); bad['backend']['config'][key] = value
                    with self.assertRaises(ValueError): w.backend(bad, site)

    def test_runtime_export_allowlist(self):
        for env in ENVIRONMENTS:
            with self.subTest(env=env):
                value = reviewed_runtime(env)
                self.assertEqual(c.runtime(value, env), value)
                for key, bad in [('client_secret', SENTINEL), ('invitations', {'craigdevjohnson@gmail.com': ['soccer']})]:
                    with self.assertRaises(ValueError): c.runtime(dict(value, **{key: bad}), env)
                for key, bad in [('allow_local_callback', True), ('redirect_uri', 'http://localhost:8080/auth/callback'), ('cognito_issuer', 'https://cognito-idp.us-east-1.amazonaws.com/us-east-1_example'), ('cognito_client_id', SENTINEL)]:
                    with self.assertRaises(ValueError): c.runtime(dict(value, **{key: bad}), env)
                other = 'prod' if env == 'dev' else 'dev'
                with self.assertRaises(ValueError): c.runtime(reviewed_runtime(other), env)


class WrapperTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve()
        self.root.chmod(0o700)
        self.cred = self.root / 'credentials.json'
        w.write_private(self.cred, json.dumps(dict(client_id=SENTINEL, client_secret=SENTINEL)).encode())
        self.plan = self.root / 'plan'
        self.use('dev')
        self.calls = []
        self.fail_every = False
        self.fail_at = None
        self.identity = dict(Account=c.ACCOUNT, Arn=f'arn:aws:sts::{c.ACCOUNT}:assumed-role/AWSReservedSSO_WorkloadsAdmin_123/test')
        self.runtime_output = None
        self.applied = []

    def use(self, env):
        self.site_env = env
        site = c.contract(env)
        self.env = dict(PATH=os.environ['PATH'], HOME=os.environ['HOME'], PORTFOLIO_ACCOUNT_ID=c.ACCOUNT, AWS_PROFILE=w.PROFILE, AWS_REGION=c.REGION, COGNITO_PRIVATE_DIR=str(self.root), GOOGLE_OAUTH_CREDENTIALS_FILE=str(self.cred), PLAN_FILE=str(self.plan), APPROVED_STATE_LOCK_URI=f"s3://{site.backend['bucket']}/{site.backend['key']}.tflock")

    def tearDown(self):
        self.temp.cleanup()

    def tofu_calls(self):
        return [args for args, _ in self.calls if args[0] == 'tofu']

    def run_fake(self, args, **kwargs):
        self.calls.append((args, kwargs['env']))
        self.assertNotIn(SENTINEL, ' '.join(args))
        data = b''
        if args[0] == 'aws':
            data = json.dumps(self.identity).encode()
        else:
            self.assertEqual(args[1], f'-chdir={w.SITE / self.site_env}')
            site = c.contract(self.site_env)
            if args[2] == 'init':
                (Path(kwargs['env']['TF_DATA_DIR']) / 'terraform.tfstate').write_text(json.dumps({'backend': {'type': 's3', 'config': {k: v for k, v in site.backend.items() if k != 'type'}}}))
            elif args[2] == 'workspace': data = b'default\n'
            elif args[2] == 'plan':
                self.assertEqual(kwargs['env']['TF_VAR_google_client_secret'], SENTINEL)
                Path(args[-1].split('=', 1)[1]).write_bytes(SENTINEL.encode())
                data = SENTINEL.encode()
            elif args[2] == 'show': data = json.dumps(fixture(self.site_env)).encode()
            elif args[2] == 'apply': self.applied.append(Path(args[-1]).read_bytes())
            elif args[2] == 'output': data = json.dumps(self.runtime_output).encode()
        return subprocess.CompletedProcess(args, 1 if self.fail_every or (len(args) > 2 and args[2] == self.fail_at) else 0, data, SENTINEL.encode())

    def invoke(self, mode='plan', env=None):
        out, err = io.StringIO(), io.StringIO()
        with patch.dict(os.environ, self.env, clear=True), patch.object(w.sys, 'argv', ['wrapper', env or self.site_env, mode]), patch.object(w.subprocess, 'run', self.run_fake), contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            try: w.main()
            except Exception:
                print('Site identity operation rejected or failed; inspect private artifacts locally.', file=err)
        self.assertNotIn(SENTINEL, out.getvalue() + err.getvalue())
        return out.getvalue(), err.getvalue()

    def reviewed_plan(self):
        out, err = self.invoke()
        self.assertFalse(err)
        review = json.loads(out)
        self.env.update(APPROVED_PLAN_SHA256=review['plan_sha256'], APPROVED_PROVENANCE_SHA256=review['provenance_sha256'])
        return review

    def test_cli_failure_diagnostic_never_echoes_input(self):
        # The real script runs here, so only stub aws and tofu are on PATH and
        # HOME hides the SSO cache: a regressed guard fails offline.
        bin_dir = self.root / 'bin'
        bin_dir.mkdir()
        called = self.root / 'called.log'
        for name in ['aws', 'tofu']:
            (bin_dir / name).write_text('#!/bin/sh\nprintf \'%s\\n\' "$0 $*" >> "$STUB_LOG"\nexit 1\n')
            (bin_dir / name).chmod(0o700)
        env = dict(self.env, PATH=str(bin_dir), HOME=str(self.root), STUB_LOG=str(called), TF_LOG=SENTINEL)
        result = subprocess.run([sys.executable, str(Path(__file__).with_name('cognito-site-operator.py')), 'dev', 'plan'], env=env, capture_output=True, check=False)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn(SENTINEL.encode(), result.stdout + result.stderr)
        self.assertIn(b'Site identity operation rejected or failed', result.stderr)
        # The TF_LOG refusal comes before any run directory or external command.
        self.assertFalse(called.exists())
        self.assertFalse(list(self.root.glob('cognito-site-*')))

    def test_plan_then_apply_exactly_the_saved_plan(self):
        for env in ENVIRONMENTS:
            with self.subTest(env=env):
                self.plan.unlink(missing_ok=True); Path(str(self.plan) + '.provenance.json').unlink(missing_ok=True)
                self.use(env); self.calls.clear(); self.applied.clear()
                review = self.reviewed_plan()
                self.assertEqual((review['environment'], review['account'], review['backend']), (env, c.ACCOUNT, c.contract(env).backend))
                self.assertEqual(self.plan.stat().st_mode & 0o777, 0o600)
                provenance = json.loads(Path(str(self.plan) + '.provenance.json').read_bytes())
                self.assertEqual(provenance['environment'], env)
                for args, run_env in self.calls:
                    if args[0] == 'aws' or args[2] != 'plan': self.assertNotIn('TF_VAR_google_client_secret', run_env)
                self.calls.clear()
                out, err = self.invoke('apply')
                self.assertFalse(err)
                # Apply never plans again: it applies only the reviewed bytes.
                self.assertEqual([args[2] for args in self.tofu_calls()], ['init', 'workspace', 'show', 'apply'])
                self.assertEqual(self.applied, [self.plan.read_bytes()])
                for args, run_env in self.calls:
                    self.assertFalse(any(key.startswith('TF_VAR_') for key in run_env))

    def test_apply_tamper_before_aws(self):
        self.reviewed_plan()
        self.plan.write_bytes(b'tampered'); self.calls.clear()
        _, err = self.invoke('apply')
        self.assertTrue(err); self.assertFalse(self.calls)

    def test_provenance_tamper_before_aws(self):
        self.reviewed_plan()
        Path(str(self.plan) + '.provenance.json').write_text('{}'); self.calls.clear()
        _, err = self.invoke('apply')
        self.assertTrue(err); self.assertFalse(self.calls)

    def test_apply_refuses_another_environments_plan_before_aws(self):
        self.reviewed_plan()
        approved = {k: self.env[k] for k in ['APPROVED_PLAN_SHA256', 'APPROVED_PROVENANCE_SHA256']}
        self.use('prod'); self.env.update(approved); self.calls.clear()
        _, err = self.invoke('apply')
        self.assertTrue(err); self.assertFalse(self.calls)
        # The development lock acknowledgement never unlocks production.
        self.use('dev'); self.env.update(approved); self.calls.clear()
        _, err = self.invoke('apply', env='prod')
        self.assertTrue(err); self.assertFalse(self.calls)

    def test_identity_or_account_refusal_before_tofu(self):
        for identity in [
            dict(Account='999999999999', Arn='arn:aws:sts::999999999999:assumed-role/AWSReservedSSO_WorkloadsAdmin_123/test'),
            dict(Account=c.ACCOUNT, Arn=f'arn:aws:sts::{c.ACCOUNT}:assumed-role/AWSReservedSSO_WorkloadsReadOnly_123/test'),
            dict(Account=c.ACCOUNT, Arn=f'arn:aws:iam::{c.ACCOUNT}:user/admin'),
        ]:
            for mode in ['init', 'plan', 'export']:
                with self.subTest(identity=identity['Arn'], mode=mode):
                    self.identity = identity; self.calls.clear()
                    out, err = self.invoke(mode)
                    self.assertFalse(out); self.assertTrue(err)
                    self.assertEqual([args[0] for args, _ in self.calls], ['aws'])
                    self.assertFalse(self.plan.exists())
        # The wrong profile, static keys or region are refused before any call.
        self.identity = dict(Account=c.ACCOUNT, Arn=f'arn:aws:sts::{c.ACCOUNT}:assumed-role/AWSReservedSSO_WorkloadsAdmin_123/test')
        for key, value in [('AWS_PROFILE', 'workloads-readonly'), ('AWS_PROFILE', None), ('AWS_ACCESS_KEY_ID', 'AKIAEXAMPLE'), ('AWS_SESSION_TOKEN', 'token'), ('AWS_REGION', 'us-east-1')]:
            with self.subTest(key=key, value=value):
                original = dict(self.env)
                if value is None: self.env.pop(key)
                else: self.env[key] = value
                self.calls.clear()
                _, err = self.invoke(); self.assertTrue(err); self.assertFalse(self.calls)
                self.env = original

    def test_private_directory_checks_before_aws(self):
        link = self.root.parent / (self.root.name + '-link')
        link.symlink_to(self.root)
        self.addCleanup(link.unlink)
        # A private checkout: a 0700 directory inside it passes every check
        # except the one that keeps private files out of the checkout.
        checkout = self.root / 'checkout'
        inside, outside = checkout / 'private', self.root / 'elsewhere'
        for directory in [checkout, inside, outside]:
            directory.mkdir(mode=0o700)
        self.runtime_output = reviewed_runtime()
        # init and export read no private file, so only the directory check
        # guards them.
        for operation in ['init', 'export', 'plan']:
            for label, value, mode in [('group-readable', str(self.root), 0o750), ('world-readable', str(self.root), 0o755), ('inside-checkout', str(inside), 0o700), ('symlink', str(link), 0o700), ('relative', 'private', 0o700), ('missing', str(self.root / 'missing'), 0o700)]:
                with self.subTest(operation=operation, label=label), patch.object(w, 'REPO', checkout):
                    self.root.chmod(mode); self.env['COGNITO_PRIVATE_DIR'] = value; self.calls.clear()
                    out, err = self.invoke(operation)
                    self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.calls)
                    self.root.chmod(0o700); self.env['COGNITO_PRIVATE_DIR'] = str(self.root)
            # The same private directory outside the checkout is accepted.
            with self.subTest(operation=operation, label='outside-checkout'), patch.object(w, 'REPO', checkout):
                self.env['COGNITO_PRIVATE_DIR'] = str(outside); self.calls.clear()
                out, err = self.invoke(operation)
                self.assertTrue(out); self.assertFalse(err); self.assertTrue(self.calls)
                self.env['COGNITO_PRIVATE_DIR'] = str(self.root)
                self.plan.unlink(missing_ok=True); Path(str(self.plan) + '.provenance.json').unlink(missing_ok=True)
        # A plan path inside the checkout is refused too, and the same path
        # outside it is accepted.
        for plan_file, accepted in [(checkout / 'site.tfplan', False), (outside / 'site.tfplan', True)]:
            with self.subTest(plan_file=plan_file.parent.name), patch.object(w, 'REPO', checkout):
                self.env['PLAN_FILE'] = str(plan_file); self.calls.clear()
                out, err = self.invoke()
                self.assertEqual((bool(out), bool(err), bool(self.calls), plan_file.exists()), (accepted, not accepted, accepted, accepted))

    def test_export_prints_only_the_reviewed_runtime_for_the_tfvars_handoff(self):
        expected = {
            'dev': '''site = {
  cognito_domain       = "https://portfolio-lambda-dev-site-111122223333.auth.us-west-2.amazoncognito.com"
  cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_example"
  cognito_client_id    = "example123"
  redirect_uri         = "https://dev.craigdevjohnson.com/auth/callback"
  logout_uri           = "https://dev.craigdevjohnson.com/sign-in"
  allow_local_callback = false
}
''',
            'prod': '''site = {
  cognito_domain       = "https://portfolio-lambda-prod-site-111122223333.auth.us-west-2.amazoncognito.com"
  cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_example"
  cognito_client_id    = "example123"
  redirect_uri         = "https://craigdevjohnson.com/auth/callback"
  logout_uri           = "https://craigdevjohnson.com/sign-in"
  allow_local_callback = false
}
''',
        }
        for env in ENVIRONMENTS:
            with self.subTest(env=env):
                self.use(env); self.calls.clear()
                self.runtime_output = reviewed_runtime(env)
                out, err = self.invoke('export')
                self.assertFalse(err)
                self.assertEqual(out, expected[env])
                # Only the one allowlisted output is read: never all outputs or state.
                self.assertEqual([args[2:] for args in self.tofu_calls()], [['init', '-backend-config=backend.hcl', '-reconfigure', '-lockfile=readonly', '-input=false'], ['workspace', 'show'], ['output', '-json', 'site_runtime']])

    def test_export_redacts_by_refusing_anything_beyond_the_allowlist(self):
        for runtime in [dict(reviewed_runtime(), client_secret=SENTINEL), dict(reviewed_runtime(), cognito_client_id=SENTINEL), reviewed_runtime('prod'), [SENTINEL], SENTINEL]:
            with self.subTest(runtime=type(runtime).__name__):
                self.runtime_output = runtime
                out, err = self.invoke('export')
                self.assertFalse(out); self.assertTrue(err)

    def test_provider_failure_is_private(self):
        self.fail_every = True
        out, err = self.invoke()
        self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.plan.exists())

    def test_plan_and_show_failures_are_private(self):
        for phase in ['plan', 'show']:
            with self.subTest(phase=phase):
                self.fail_at = phase
                out, err = self.invoke()
                self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.plan.exists())

    def test_unsafe_credentials_before_aws(self):
        for mode in ['readable', 'symlink', 'malformed', 'missing', 'extra-key']:
            with self.subTest(mode=mode):
                self.cred.unlink(missing_ok=True)
                if mode == 'readable': self.cred.write_text('{}'); self.cred.chmod(0o644)
                elif mode == 'symlink': self.cred.symlink_to(self.root / 'missing')
                elif mode == 'malformed': w.write_private(self.cred, SENTINEL.encode())
                elif mode == 'extra-key': w.write_private(self.cred, json.dumps(dict(client_id=SENTINEL, client_secret=SENTINEL, other=SENTINEL)).encode())
                self.calls.clear()
                out, err = self.invoke()
                self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.calls)

    def test_existing_relative_and_env_overrides(self):
        for key, value in [('PLAN_FILE', 'relative'), ('PLAN_FILE', str(self.root / 'child' / '..' / 'plan')), ('COGNITO_PRIVATE_DIR', str(self.root / 'child' / '..')), ('TF_LOG', 'TRACE'), ('TF_VAR_cognito_domain_prefix', 'portfolio-lambda-dev-site-other'), ('TF_VAR_enable_local_callback', 'true'), ('TOFU_LOG', 'TRACE'), ('AWS_ENDPOINT_URL', 'https://evil.example'), ('APPROVED_STATE_LOCK_URI', f's3://portfolio-tofu-state-{c.ACCOUNT}/portfolio-lambda-http-api/auth/dev/terraform.tfstate.tflock')]:
            with self.subTest(key=key):
                original = dict(self.env); self.env[key] = value
                _, err = self.invoke(); self.assertTrue(err); self.assertFalse(self.calls)
                self.env = original
        self.plan.write_bytes(b'existing')
        _, err = self.invoke(); self.assertTrue(err); self.assertFalse(self.calls)

    def test_unknown_environment_or_mode_before_aws(self):
        for env, mode in [('staging', 'plan'), ('dev', 'destroy'), ('dev', 'state')]:
            with self.subTest(env=env, mode=mode):
                self.calls.clear()
                out, err = self.invoke(mode, env=env)
                self.assertFalse(out); self.assertTrue(err); self.assertFalse(self.calls)

    def test_override_or_tfvars_files_before_aws(self):
        site = Path(self.temp.name) / 'site'
        for relative in ['dev', 'prod', 'modules/pool']:
            (site / relative).mkdir(parents=True)
        for path in ['dev/extra.auto.tfvars', 'dev/terraform.tfvars.json', 'dev/main_override.tf', 'modules/pool/override.tf']:
            with self.subTest(path=path), patch.object(w, 'SITE', site):
                (site / path).write_text('')
                self.calls.clear()
                _, err = self.invoke()
                self.assertTrue(err); self.assertFalse(self.calls)
                (site / path).unlink()


if __name__ == '__main__':
    unittest.main()
