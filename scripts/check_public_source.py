#!/usr/bin/env python3
"""Fail if tracked files contain secrets, build output or private data.

Run from a Git checkout or an extracted source archive.
"""
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
SKIP_DIRS = {'.git', 'build', 'dist', 'node_modules', 'test-results'}
BINARY_OK = {'assets/icon.png'}
SECRET = re.compile(r'(gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|AKIA[0-9A-Z]{16}|'
                    r'-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|xox[baprs]-[A-Za-z0-9-]{10,}|sk-[A-Za-z0-9]{32,})')
FORBIDDEN_EXT = {'.exe', '.dll', '.zip', '.syso', '.msi', '.7z', '.log', '.env', '.pfx', '.p12', '.pem', '.key'}
EMAIL = re.compile(r'[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}')
EMAIL_OK = {'noreply@anthropic.com', 'john@jchw.io'}


def files():
    if (ROOT / '.git').exists():
        out = subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode()
        return [p for p in out.split('\0') if p]
    return [str(p.relative_to(ROOT)).replace('\\', '/') for p in ROOT.rglob('*')
            if p.is_file() and not (set(p.relative_to(ROOT).parts) & SKIP_DIRS)]


def main():
    problems = []
    for rel in files():
        path = ROOT / rel
        if not path.is_file():
            continue
        ext = path.suffix.lower()
        if ext in FORBIDDEN_EXT:
            problems.append(f'{rel}: build output, archive or key file must not be committed')
            continue
        if ext == '.png':
            if not (rel in BINARY_OK or re.fullmatch(r'docs/images/[a-z0-9-]+\.png', rel)):
                problems.append(f'{rel}: images belong in docs/images/ with kebab-case names')
            elif path.stat().st_size > 600_000:
                problems.append(f'{rel}: image is larger than 600 KB')
            continue
        text = path.read_text(encoding='utf-8', errors='replace')
        if SECRET.search(text):
            problems.append(f'{rel}: looks like it contains a secret')
        for email in set(EMAIL.findall(text)) - EMAIL_OK:
            if not email.endswith(('example.com', 'example.org')):
                problems.append(f'{rel}: contains an email address ({email})')
    if problems:
        print('\n'.join(problems))
        sys.exit(1)
    print(f'ok: {len(files())} files checked')


if __name__ == '__main__':
    main()
