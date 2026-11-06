#!/usr/bin/env python3
"""Generate local credentials once; never overwrite an existing installation."""
from pathlib import Path
import os
import secrets

path = Path(__file__).resolve().parent / '.env'
values = {key: secrets.token_hex(32) for key in (
    'POSTGRES_PASSWORD', 'CLICKHOUSE_PASSWORD', 'REDIS_AUTH',
    'S3_SECRET_KEY', 'SALT', 'ENCRYPTION_KEY', 'NEXTAUTH_SECRET')}
values.update(LANGFUSE_HOST='http://localhost:1488',
              LANGFUSE_PUBLIC_KEY='pk-lf-' + secrets.token_hex(16),
              LANGFUSE_SECRET_KEY='sk-lf-' + secrets.token_hex(32),
              LANGFUSE_EMAIL='developer@walkthrough.local',
              LANGFUSE_PASSWORD=secrets.token_urlsafe(24))
try:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    print('Using existing langfuse/.env (credentials preserved).')
else:
    with os.fdopen(fd, 'w') as stream:
        stream.writelines(f'{key}={value}\n' for key, value in values.items())
    print('Created langfuse/.env. UI login: LANGFUSE_EMAIL / LANGFUSE_PASSWORD in that file.')
