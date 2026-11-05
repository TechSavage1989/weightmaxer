"""VERSION, release notes and README download instructions agree."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


class ReleaseMetadataTests(unittest.TestCase):
    def setUp(self):
        self.version = (ROOT / 'VERSION').read_text(encoding='utf-8').strip()

    def test_version_format(self):
        self.assertRegex(self.version, r'^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$')

    def test_release_notes_exist_with_bullets(self):
        notes = ROOT / 'docs' / 'releases' / f'{self.version}.md'
        self.assertTrue(notes.is_file(), f'add {notes.relative_to(ROOT)}')
        text = notes.read_text(encoding='utf-8')
        self.assertRegex(text, r'(?m)^- \S', 'release notes need "- " bullets for the in-app What\'s new list')

    def test_readme_links(self):
        readme = (ROOT / 'README.md').read_text(encoding='utf-8')
        self.assertEqual(readme.count('<h1'), 1)
        for anchor in ['#download', '#getting-started', '#features', '#screenshots', '#updates-and-settings', '#development']:
            self.assertIn(f'href="{anchor}"', readme)
        heading_ids = {re.sub(r'[^a-z0-9 -]', '', h.lower()).strip().replace(' ', '-')
                       for h in re.findall(r'(?m)^## (.+)$', readme)}
        for anchor in ['download', 'getting-started', 'features', 'screenshots', 'updates-and-settings', 'development']:
            self.assertIn(anchor, heading_ids)
        for img in re.findall(r'src="(docs/images/[^"]+)"', readme):
            self.assertTrue((ROOT / img).is_file(), img)
        self.assertIn('releases/latest', readme)
        self.assertNotRegex(readme, r'releases/download/v\d', 'link the latest release, not a version-specific asset')


if __name__ == '__main__':
    unittest.main()
