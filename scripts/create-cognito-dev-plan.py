#!/usr/bin/env python3
"""Private Cognito lifecycle wrapper; raw subprocess streams stay private."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile

REPO = Path(__file__).resolve().parent.parent
ROOT = REPO / 'infra/lambda/auth/dev'
spec = importlib.util.spec_from_file_location('auth_contract', REPO / 'scripts/check-cognito-dev-plan.py')
contract = importlib.util.module_from_spec(spec)
spec.loader.exec_module(contract)
require = contract.require


def private_path(raw, existing=True):
    path = Path(raw)
    require(path.is_absolute() and path == path.resolve() and not path.is_relative_to(REPO))
    # Reject symbolic links in every component, including the final component.
    require(all(not p.is_symlink() for p in [path, *path.parents]))
    parent = path.parent.stat()
    require(parent.st_uid == os.getuid() and stat.S_ISDIR(parent.st_mode) and parent.st_mode & 0o077 == 0)
    if existing:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as source:
            info = os.fstat(source.fileno())
            require(stat.S_ISREG(info.st_mode) and info.st_uid == os.getuid() and info.st_mode & 0o077 == 0)
            require(info.st_size <= 128 * 1024 * 1024)
            return source.read()
    require(not path.exists())
    return path


def write_private(path, data):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'wb') as out:
        out.write(data)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def environment():
    require(os.environ.get('AWS_PROFILE') == 'portfolio-deployer')
    require(os.environ.get('AWS_REGION') == contract.REGION)
    # Refuse all alternate credential, endpoint, CLI, variable and log channels.
    for key in os.environ:
        require(not key.startswith(('TF_', 'TOFU_')))
        require(not key.startswith('AWS_') or key in {'AWS_PROFILE', 'AWS_REGION', 'AWS_PAGER'})
    env = {k: v for k, v in os.environ.items() if k not in {'GOOGLE_OAUTH_CREDENTIALS_FILE', 'PLAN_FILE', 'APPROVED_PLAN_SHA256', 'APPROVED_PROVENANCE_SHA256'}}
    env['AWS_PAGER'] = ''
    return env


def backend(metadata):
    b = metadata['backend']
    require(b['type'] == 's3')
    expected = {k: v for k, v in contract.BACKEND.items() if k != 'type'}
    require(all(contract.same(b['config'].get(k), v) for k, v in expected.items()))
    def unset(value):
        return value in (None, '', False, [], {}) or (isinstance(value, dict) and all(unset(v) for v in value.values()))
    require(all(k in expected or unset(v) for k, v in b['config'].items()))
    return contract.BACKEND


def main():
    os.umask(0o077)
    mode = sys.argv[1] if len(sys.argv) == 2 else 'plan'
    require(mode in {'init', 'plan', 'apply', 'export'})
    env = environment()
    require(os.environ.get('APPROVED_STATE_LOCK_URI') == f"s3://{contract.BACKEND['bucket']}/{contract.BACKEND['key']}.tflock")
    private_dir = Path(os.environ['COGNITO_PRIVATE_DIR'])
    require(private_dir.is_absolute() and private_dir == private_dir.resolve() and not private_dir.is_relative_to(REPO))
    require(all(not p.is_symlink() for p in [private_dir, *private_dir.parents]))
    info = private_dir.stat()
    require(stat.S_ISDIR(info.st_mode) and info.st_uid == os.getuid() and info.st_mode & 0o077 == 0)
    # Do not allow implicit tfvars or executable override configuration.
    require(not list(ROOT.glob('*.tfvars*')) and not list(ROOT.glob('*override.tf*')))
    plan_path = None
    snapshot = None
    if mode == 'plan':
        plan_path = private_path(os.environ['PLAN_FILE'], False)
        private_path(str(plan_path) + '.provenance.json', False)
        credentials = json.loads(private_path(os.environ['GOOGLE_OAUTH_CREDENTIALS_FILE']))
        require(set(credentials) == {'client_id', 'client_secret'})
        require(all(isinstance(v, str) and v.strip() and '\x00' not in v for v in credentials.values()))
    if mode == 'apply':
        snapshot = private_path(os.environ['PLAN_FILE'])
        approved = os.environ['APPROVED_PLAN_SHA256']
        require(re.fullmatch('[0-9a-f]{64}', approved) is not None and digest(snapshot) == approved)
        provenance = private_path(os.environ['PLAN_FILE'] + '.provenance.json')
        require(digest(provenance) == os.environ['APPROVED_PROVENANCE_SHA256'])
        require(json.loads(provenance) == dict(schema='portfolio.cognito-dev-plan/v1', plan_sha256=approved, backend=contract.BACKEND, workspace='default', account=contract.ACCOUNT))
    run_dir = Path(tempfile.mkdtemp(prefix='cognito-', dir=private_dir))
    data_dir = run_dir / 'data'
    data_dir.mkdir(mode=0o700)
    cli = run_dir / 'tofurc'
    write_private(cli, b'')
    env.update(TF_DATA_DIR=str(data_dir), TF_CLI_CONFIG_FILE=str(cli))
    count = 0
    def run(command, run_env=None):
        nonlocal count
        count += 1
        result = subprocess.run(command, env=run_env or env, cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        write_private(run_dir / f'{count}.stdout', result.stdout)
        write_private(run_dir / f'{count}.stderr', result.stderr)
        require(result.returncode == 0)
        return result.stdout
    identity = json.loads(run(['aws', '--profile', 'portfolio-deployer', '--region', contract.REGION, 'sts', 'get-caller-identity', '--output', 'json']))
    require(identity['Account'] == contract.ACCOUNT)
    require(re.fullmatch(r'arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_[A-Za-z0-9]+/[^/]+', identity['Arn']) is not None)
    tofu = ['tofu', f'-chdir={ROOT}']
    run(tofu + ['init', '-backend-config=backend.hcl', '-reconfigure', '-lockfile=readonly', '-input=false'])
    require(run(tofu + ['workspace', 'show']).strip() == b'default')
    backend(json.loads((data_dir / 'terraform.tfstate').read_bytes()))
    if mode == 'init':
        print(json.dumps(dict(account=contract.ACCOUNT, backend=contract.BACKEND, workspace='default')))
        return
    if mode == 'export':
        value = contract.runtime(json.loads(run(tofu + ['output', '-json', 'management_runtime'])))
        print(json.dumps({'management': value}, sort_keys=True))
        return
    saved = run_dir / 'review.tfplan'
    if mode == 'plan':
        secret_env = dict(env, TF_VAR_google_client_id=credentials['client_id'], TF_VAR_google_client_secret=credentials['client_secret'])
        run(tofu + ['plan', '-lock-timeout=5m', '-input=false', f'-out={saved}'], secret_env)
    else:
        write_private(saved, snapshot)
    plan = json.loads(run(tofu + ['show', '-json', str(saved)]))
    summary = contract.check(plan)
    backend(json.loads((data_dir / 'terraform.tfstate').read_bytes()))
    plan_bytes = saved.read_bytes()
    sha = digest(plan_bytes)
    if mode == 'apply':
        require(sha == os.environ['APPROVED_PLAN_SHA256'])
        run(tofu + ['apply', '-lock-timeout=5m', '-input=false', str(saved)])
        print('Reviewed Cognito saved plan applied; verify convergence separately.')
        return
    provenance = json.dumps(dict(schema='portfolio.cognito-dev-plan/v1', plan_sha256=sha, backend=contract.BACKEND, workspace='default', account=contract.ACCOUNT), sort_keys=True).encode()
    write_private(plan_path, plan_bytes)
    write_private(Path(str(plan_path) + '.provenance.json'), provenance)
    print(json.dumps(dict(account=contract.ACCOUNT, backend=contract.BACKEND, actions=summary, plan_sha256=sha, provenance_sha256=digest(provenance)), sort_keys=True))

if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Cognito operation rejected or failed; inspect private artifacts locally.', file=sys.stderr)
        sys.exit(1)
