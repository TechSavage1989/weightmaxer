"""Keep the repository root compact without hiding unreviewed new files.

A genuinely required new root entry is added to ALLOWED in the same reviewed
pull request, with the reason recorded in AGENTS.md. Never replace the list
with a pattern.
"""
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
ALLOWED = {
    'README.md', 'AGENTS.md', 'CLAUDE.md', 'DEVELOPMENT_HANDOFF.md', 'VERSION',
    'go.mod', 'go.sum', '.gitignore', '.github',
    'cmd', 'internal', 'web', 'assets', 'docs', 'scripts', 'tests',
}
# Local outputs that are git-ignored; skipped only when no Git index exists.
GENERATED = {'.git', 'build', 'dist', 'site', 'node_modules', 'test-results'}


def root_entries(root):
    """Top-level names of tracked files, or of the files present in a source archive."""
    if (root / '.git').exists():
        out = subprocess.check_output(['git', 'ls-files', '-z'], cwd=root).decode()
        return {p.split('/')[0] for p in out.split('\0') if p}
    return {p.name for p in root.iterdir() if p.name not in GENERATED}


def unexpected(names):
    return sorted(set(names) - ALLOWED)


class RepositoryLayoutTests(unittest.TestCase):
    def test_root_allowlist(self):
        self.assertEqual(unexpected(root_entries(ROOT)), [], 'Group new files under existing folders; read AGENTS.md')

    def test_guard_rejects_new_root_clutter(self):
        # Negative control in an isolated folder without .git (like a source archive).
        with tempfile.TemporaryDirectory() as d:
            d = Path(d)
            for name in ['README.md', 'go.mod', 'scratch.txt', 'notes.md']:
                (d / name).write_text('x')
            (d / 'build').mkdir()
            (d / 'temp').mkdir()
            self.assertEqual(unexpected(root_entries(d)), ['notes.md', 'scratch.txt', 'temp'])
            for name in ['scratch.txt', 'notes.md']:
                (d / name).unlink()
            (d / 'temp').rmdir()
            self.assertEqual(unexpected(root_entries(d)), [])

    def test_required_documents(self):
        for path in ['README.md', 'AGENTS.md', 'CLAUDE.md', 'DEVELOPMENT_HANDOFF.md',
                     'docs/REPOSITORY_STANDARDS.md', 'docs/images/hero.png']:
            self.assertTrue((ROOT / path).is_file(), path)

    def test_readme_icon_matches_app_icon(self):
        self.assertEqual((ROOT / 'docs/images/icon.png').read_bytes(), (ROOT / 'assets/icon.png').read_bytes(),
                         'run node scripts/render_icon.cjs after changing the icon')

    def test_claude_imports_agents(self):
        self.assertIn('@AGENTS.md', (ROOT / 'CLAUDE.md').read_text(encoding='utf-8'))


if __name__ == '__main__':
    unittest.main()
