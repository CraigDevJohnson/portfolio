#!/usr/bin/env python3
"""Offline auth-plan contract. Diagnostics never include input values."""
import json
import os
import re
import sys

ACCOUNT = '180294223248'
REGION = 'us-west-2'
PREFIX = 'portfolio-lambda-dev-mgmt-180294223248'
BACKEND = dict(type='s3', bucket='portfolio-tofu-state-180294223248', key='portfolio-lambda-http-api/auth/dev/terraform.tfstate', region=REGION, encrypt=True, use_lockfile=True)
RESOURCES = {
 'aws_cognito_user_pool.management': dict(name='portfolio-lambda-dev-mgmt', user_pool_tier='ESSENTIALS', admin_create_user_config=[{'allow_admin_create_user_only': True}], username_configuration=[{'case_sensitive': False}]),
 'aws_cognito_identity_provider.google': dict(provider_name='Google', provider_type='Google', attribute_mapping={'email': 'email', 'email_verified': 'email_verified', 'name': 'name'}),
 'aws_cognito_user_pool_client.management': dict(name='portfolio-lambda-dev-mgmt-web', generate_secret=False, allowed_oauth_flows_user_pool_client=True, allowed_oauth_flows=['code'], allowed_oauth_scopes=['openid', 'email', 'profile'], supported_identity_providers=['Google'], explicit_auth_flows=['ALLOW_REFRESH_TOKEN_AUTH'], callback_urls=['https://dev.craigdevjohnson.com/callback'], logout_urls=['https://dev.craigdevjohnson.com/login'], read_attributes=['email', 'email_verified', 'name'], write_attributes=['email', 'name']),
 'aws_cognito_user_pool_domain.management': dict(domain=PREFIX, managed_login_version=2),
 'aws_cognito_managed_login_branding.management': dict(use_cognito_provided_values=True),
}

def require(ok):
    if not ok:
        raise ValueError('auth contract rejected')

def same(actual, expected):
    if isinstance(expected, list):
        if expected and isinstance(expected[0], dict):
            return isinstance(actual, list) and len(actual) == len(expected) and all(all(same(a.get(k), v) for k, v in e.items()) for a, e in zip(actual, expected))
        return isinstance(actual, list) and sorted(actual) == sorted(expected)
    return type(actual) is type(expected) and actual == expected

def check(plan):
    require(plan.get('errored', False) is False)
    changes = plan.get('resource_changes', [])
    require(len(changes) == 5 and {r['address'] for r in changes} == set(RESOURCES))
    providers = plan['configuration']['provider_config']
    require(set(providers) == {'aws'})
    expressions = providers['aws']['expressions']
    require(expressions['region'] == {'constant_value': REGION})
    require(expressions['allowed_account_ids'] == {'constant_value': [ACCOUNT]})
    require(not (set(expressions) - {'region', 'allowed_account_ids', 'default_tags'}))
    config = plan['configuration']['root_module']
    require(not config.get('module_calls'))
    require(set(config.get('outputs', {})) == {'cognito_user_pool_id', 'cognito_domain', 'cognito_issuer', 'cognito_client_id', 'google_redirect_uri', 'session_parameter_path', 'management_runtime'})
    require({r['address'] for r in config['resources']} == set(RESOURCES))
    require(not plan.get('resource_drift'))
    for resource in config['resources']:
        require(not resource.get('provisioners'))
        expr = resource['expressions']
        if resource['address'] != 'aws_cognito_user_pool.management':
            require(set(expr.get('user_pool_id', {}).get('references', [])) == {'aws_cognito_user_pool.management.id', 'aws_cognito_user_pool.management'})
        if resource['address'] == 'aws_cognito_managed_login_branding.management':
            require(set(expr.get('client_id', {}).get('references', [])) == {'aws_cognito_user_pool_client.management.id', 'aws_cognito_user_pool_client.management'})
    summary = []
    for r in changes:
        require(r.get('mode') == 'managed' and r.get('provider_name') == 'registry.opentofu.org/hashicorp/aws')
        change = r['change']
        require(change['actions'] in [['create'], ['update'], ['no-op']])
        after = change['after']
        for key, value in RESOURCES[r['address']].items():
            require(same(after.get(key), value))
        if r['address'] == 'aws_cognito_user_pool.management':
            require(not after.get('lambda_config') and not change.get('after_unknown', {}).get('lambda_config'))
            require(not after.get('sms_configuration'))
            require(not after.get('email_configuration'))
        if r['address'] == 'aws_cognito_identity_provider.google':
            details = after['provider_details']
            require(details.get('authorize_scopes') == 'openid email profile')
            defaults = {'attributes_url': 'https://people.googleapis.com/v1/people/me?personFields=', 'attributes_url_add_attributes': 'true', 'authorize_url': 'https://accounts.google.com/o/oauth2/v2/auth', 'oidc_issuer': 'https://accounts.google.com', 'token_request_method': 'POST', 'token_url': 'https://www.googleapis.com/oauth2/v4/token'}
            require(set(details) <= {'authorize_scopes', 'client_id', 'client_secret'} | set(defaults))
            require(all(key not in details or details[key] == value for key, value in defaults.items()))
            require(all(isinstance(details.get(k), str) and details[k].strip() for k in ['client_id', 'client_secret']))
        summary.append({'resource': r['address'], 'action': change['actions'][0]})
    return sorted(summary, key=lambda r: r['resource'])

def runtime(value):
    fixed = dict(cognito_domain=f'https://{PREFIX}.auth.{REGION}.amazoncognito.com', redirect_uri='https://dev.craigdevjohnson.com/callback', logout_uri='https://dev.craigdevjohnson.com/login', allowed_emails=['craigdevjohnson@gmail.com'], allow_local_callback=False, ec2_management_tag_key='PortfolioManagement', ec2_management_tag_value='dev')
    require(set(value) == set(fixed) | {'cognito_client_id', 'cognito_issuer'})
    for key, expected in fixed.items():
        require(same(value[key], expected))
    require(re.fullmatch(r'[a-z0-9]{1,128}', value['cognito_client_id']) is not None)
    require(re.fullmatch(r'https://cognito-idp\.us-west-2\.amazonaws\.com/us-west-2_[A-Za-z0-9]+', value['cognito_issuer']) is not None)
    return value

if __name__ == '__main__':
    try:
        with open(os.environ['PLAN_JSON']) as source:
            result = check(json.load(source))
        print(json.dumps(result, sort_keys=True))
    except Exception:
        print('Cognito plan rejected; inspect private artifacts locally.', file=sys.stderr)
        sys.exit(1)
