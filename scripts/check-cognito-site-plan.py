#!/usr/bin/env python3
"""Offline plan and runtime contract for the site identity roots
(infra/lambda/auth/site/{dev,prod}). Diagnostics never include input values."""
import json
import os
import re
import sys
from types import SimpleNamespace

# The Taskfile supplies PORTFOLIO_ACCOUNT_ID; contract() refuses an empty value.
ACCOUNT = os.environ.get('PORTFOLIO_ACCOUNT_ID', '')
REGION = 'us-west-2'
# The reviewed site origin of each environment. The client registers exactly
# its callback and sign-out return; no loopback callback is admitted.
ORIGINS = {'dev': 'https://dev.craigdevjohnson.com', 'prod': 'https://craigdevjohnson.com'}
MODULE = 'module.site.'
POOL = MODULE + 'aws_cognito_user_pool.site'
GOOGLE = MODULE + 'aws_cognito_identity_provider.google'
CLIENT = MODULE + 'aws_cognito_user_pool_client.site'
DOMAIN = MODULE + 'aws_cognito_user_pool_domain.site'
BRANDING = MODULE + 'aws_cognito_managed_login_branding.site'
OUTPUTS = {'cognito_user_pool_id', 'google_redirect_uri', 'session_parameter_path', 'site_runtime'}
VARIABLES = {'aws_account_id', 'google_client_id', 'google_client_secret', 'cognito_domain_prefix', 'enable_local_callback'}
# Pool attributes AWS changes on its own. Google sign-in creates a federated
# user, so once anyone signs in, every refresh reports the user count as drift.
COMPUTED_POOL_DRIFT = {'estimated_number_of_users', 'last_modified_date'}


def require(ok):
    if not ok:
        raise ValueError('site identity contract rejected')


def contract(env):
    """The reviewed names, URLs and backend of one environment's site root."""
    require(env in ORIGINS and re.fullmatch(r'[0-9]{12}', ACCOUNT) is not None)
    name = f'portfolio-lambda-{env}-site'
    # Only the root's default prefix is admitted; an override needs a reviewed
    # change to this contract.
    prefix = f'{name}-{ACCOUNT}'
    origin = ORIGINS[env]
    backend = dict(type='s3', bucket=f'portfolio-tofu-state-{ACCOUNT}', key=f'portfolio-lambda-http-api/auth/site/{env}/terraform.tfstate', region=REGION, encrypt=True, use_lockfile=True)
    return SimpleNamespace(
        env=env,
        prefix=prefix,
        cognito_domain=f'https://{prefix}.auth.{REGION}.amazoncognito.com',
        callback=f'{origin}/auth/callback',
        logout=f'{origin}/sign-in',
        backend=backend,
        lock_uri=f"s3://{backend['bucket']}/{backend['key']}.tflock",
        resources={
            POOL: dict(name=name, user_pool_tier='ESSENTIALS', admin_create_user_config=[{'allow_admin_create_user_only': True}], username_configuration=[{'case_sensitive': False}]),
            GOOGLE: dict(provider_name='Google', provider_type='Google', attribute_mapping={'email': 'email', 'email_verified': 'email_verified', 'name': 'name', 'username': 'sub'}),
            CLIENT: dict(name=f'{name}-web', generate_secret=False, allowed_oauth_flows_user_pool_client=True, allowed_oauth_flows=['code'], allowed_oauth_scopes=['openid', 'email', 'profile'], supported_identity_providers=['Google'], explicit_auth_flows=['ALLOW_REFRESH_TOKEN_AUTH'], callback_urls=[f'{origin}/auth/callback'], logout_urls=[f'{origin}/sign-in'], read_attributes=['email', 'email_verified', 'name'], write_attributes=['email', 'name']),
            DOMAIN: dict(domain=prefix, managed_login_version=2),
            BRANDING: dict(use_cognito_provided_values=True),
        },
    )


def same(actual, expected):
    if isinstance(expected, list):
        if expected and isinstance(expected[0], dict):
            return isinstance(actual, list) and len(actual) == len(expected) and all(all(same(a.get(k), v) for k, v in e.items()) for a, e in zip(actual, expected))
        return isinstance(actual, list) and sorted(actual) == sorted(expected)
    return type(actual) is type(expected) and actual == expected


def default_email_configuration(value):
    if value in (None, []):
        return True
    if not isinstance(value, list) or len(value) != 1 or not isinstance(value[0], dict):
        return False
    configuration = value[0]
    optional = {'configuration_set', 'from_email_address', 'reply_to_email_address', 'source_arn'}
    return (set(configuration) <= optional | {'email_sending_account'}
            and configuration.get('email_sending_account') == 'COGNITO_DEFAULT'
            and all(configuration.get(key) in (None, '') for key in optional))


def linked_id(change, key, pattern):
    value = change['after'].get(key)
    unknown = change.get('after_unknown', {}).get(key, False)
    if unknown is True:
        require(value is None and change['actions'] == ['create'])
        return None
    require(unknown is False and isinstance(value, str) and re.fullmatch(pattern, value) is not None)
    return value


def check_links(changes):
    by_address = {r['address']: r['change'] for r in changes}
    pool_pattern = r'us-west-2_[A-Za-z0-9]+'
    client_pattern = r'[a-z0-9]{1,128}'
    pool_id = linked_id(by_address[POOL], 'id', pool_pattern)
    client_id = linked_id(by_address[CLIENT], 'id', client_pattern)
    for address, change in by_address.items():
        if address != POOL:
            require(linked_id(change, 'user_pool_id', pool_pattern) == pool_id)
        if address == BRANDING:
            require(linked_id(change, 'client_id', client_pattern) == client_id)


def check_configuration(plan, site):
    providers = plan['configuration']['provider_config']
    require(set(providers) == {'aws'})
    expressions = providers['aws']['expressions']
    require(expressions['region'] == {'constant_value': REGION})
    require(expressions['allowed_account_ids'] == {'references': ['var.aws_account_id']})
    require(not (set(expressions) - {'region', 'allowed_account_ids', 'default_tags'}))
    # The wrapper passes only the Google credentials, so the plan must use the
    # reviewed default domain prefix and no loopback callback.
    variables = plan.get('variables', {})
    require(set(variables) <= VARIABLES)
    require(variables.get('aws_account_id', {}).get('value') == ACCOUNT)
    require(variables.get('cognito_domain_prefix', {}).get('value') is None)
    require(variables.get('enable_local_callback', {'value': False}).get('value') is False)
    root = plan['configuration']['root_module']
    require(not root.get('resources'))
    require(set(root.get('outputs', {})) == OUTPUTS)
    calls = root.get('module_calls', {})
    require(set(calls) == {'site'})
    call = calls['site']
    require(call.get('source') == '../modules/pool')
    require(call.get('expressions', {}).get('environment') == {'constant_value': site.env})
    module = call.get('module', {})
    require(not module.get('module_calls'))
    resources = module.get('resources', [])
    require({MODULE + r['address'] for r in resources} == set(site.resources) and len(resources) == len(site.resources))
    for resource in resources:
        require(not resource.get('provisioners'))
        expr = resource.get('expressions', {})
        if MODULE + resource['address'] != POOL:
            require(set(expr.get('user_pool_id', {}).get('references', [])) == {'aws_cognito_user_pool.site.id', 'aws_cognito_user_pool.site'})
        if MODULE + resource['address'] == BRANDING:
            require(set(expr.get('client_id', {}).get('references', [])) == {'aws_cognito_user_pool_client.site.id', 'aws_cognito_user_pool_client.site'})


def benign_drift(address, key, before, after, site):
    """A refresh difference that changes nothing reviewed."""
    if address == POOL and key in COMPUTED_POOL_DRIFT:
        return True
    # After the first apply AWS reports an unset collection as empty.
    if before is None and after in ([], {}):
        return True
    # The pool learns its domain once the reviewed domain resource exists.
    return address == POOL and key == 'domain' and before in (None, '') and after == site.prefix


def check_drift(drift, site):
    """Admit only refresh differences that change nothing reviewed."""
    require(isinstance(drift, list))
    for entry in drift:
        require(isinstance(entry, dict) and entry.get('address') in site.resources)
        change = entry.get('change')
        require(isinstance(change, dict) and change.get('actions') == ['update'])
        before, after = change.get('before'), change.get('after')
        require(isinstance(before, dict) and isinstance(after, dict))
        changed = {key for key in set(before) | set(after) if before.get(key) != after.get(key)}
        require(all(benign_drift(entry['address'], key, before.get(key), after.get(key), site) for key in changed))


def check(plan, env):
    """Check one saved plan against env's contract; return a secret-free summary."""
    site = contract(env)
    require(plan.get('errored', False) is False)
    changes = plan.get('resource_changes', [])
    require(len(changes) == 5 and {r['address'] for r in changes} == set(site.resources))
    check_drift(plan.get('resource_drift') or [], site)
    check_configuration(plan, site)
    check_links(changes)
    summary = []
    for r in changes:
        require(r.get('mode') == 'managed' and r.get('provider_name') == 'registry.opentofu.org/hashicorp/aws' and r.get('module_address') == 'module.site')
        change = r['change']
        require(change['actions'] in [['create'], ['update'], ['no-op']])
        after = change['after']
        for key, value in site.resources[r['address']].items():
            require(same(after.get(key), value))
        # AWS provider 6 resources may name their own region; it must stay ours.
        require(change.get('after_unknown', {}).get('region') is True or after.get('region') in (None, REGION))
        if r['address'] == POOL:
            require(not after.get('lambda_config') and not change.get('after_unknown', {}).get('lambda_config'))
            require(not after.get('sms_configuration'))
            require(default_email_configuration(after.get('email_configuration')))
        if r['address'] == GOOGLE:
            details = after['provider_details']
            require(details.get('authorize_scopes') == 'openid email profile')
            defaults = {'attributes_url': 'https://people.googleapis.com/v1/people/me?personFields=', 'attributes_url_add_attributes': 'true', 'authorize_url': 'https://accounts.google.com/o/oauth2/v2/auth', 'oidc_issuer': 'https://accounts.google.com', 'token_request_method': 'POST', 'token_url': 'https://www.googleapis.com/oauth2/v4/token'}
            # Cognito adds these after create; declaring them keeps the plan converged.
            require(set(details) == {'authorize_scopes', 'client_id', 'client_secret'} | set(defaults))
            require(all(details[key] == value for key, value in defaults.items()))
            require(all(isinstance(details.get(k), str) and details[k].strip() for k in ['client_id', 'client_secret']))
        summary.append({'resource': r['address'], 'action': change['actions'][0]})
    return sorted(summary, key=lambda r: r['resource'])


RUNTIME_FIELDS = ['cognito_domain', 'cognito_issuer', 'cognito_client_id', 'redirect_uri', 'logout_uri', 'allow_local_callback']


def runtime(value, env):
    """Admit only env's reviewed non-secret site_runtime fields."""
    site = contract(env)
    require(isinstance(value, dict) and set(value) == set(RUNTIME_FIELDS))
    fixed = dict(cognito_domain=site.cognito_domain, redirect_uri=site.callback, logout_uri=site.logout, allow_local_callback=False)
    for key, expected in fixed.items():
        require(same(value[key], expected))
    require(isinstance(value['cognito_client_id'], str) and re.fullmatch(r'[a-z0-9]{1,128}', value['cognito_client_id']) is not None)
    require(isinstance(value['cognito_issuer'], str) and re.fullmatch(r'https://cognito-idp\.us-west-2\.amazonaws\.com/us-west-2_[A-Za-z0-9]+', value['cognito_issuer']) is not None)
    return value


def tfvars(value, env):
    """Render the checked runtime as the site block for <env>.auto.tfvars.
    Craig adds the environment's reviewed invitations to the same block."""
    checked = runtime(value, env)
    width = max(len(key) for key in RUNTIME_FIELDS)
    lines = [f'  {key.ljust(width)} = {json.dumps(checked[key])}' for key in RUNTIME_FIELDS]
    return '\n'.join(['site = {', *lines, '}']) + '\n'


if __name__ == '__main__':
    try:
        with open(os.environ['PLAN_JSON']) as source:
            result = check(json.load(source), os.environ['SITE_ENVIRONMENT'])
        print(json.dumps(result, sort_keys=True))
    except Exception:
        print('Site identity plan rejected; inspect private artifacts locally.', file=sys.stderr)
        sys.exit(1)
