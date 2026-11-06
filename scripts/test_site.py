"""The GitHub Pages site builds as one self-contained page."""
from pathlib import Path
import re
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_site  # noqa: E402


class SiteTests(unittest.TestCase):
    def test_site_is_self_contained(self):
        with tempfile.TemporaryDirectory() as d:
            out = build_site.build(Path(d) / 'site')
            self.assertEqual(sorted(p.name for p in out.iterdir()), ['icon.png', 'index.html'])
            html = (out / 'index.html').read_text(encoding='utf-8')
            self.assertNotIn('href="app.css"', html)
            self.assertNotIn('src="app.js"', html)
            self.assertIn('<link rel="icon" type="image/png" href="icon.png">', html)
            # No third-party scripts, styles or trackers.
            self.assertEqual(re.findall(r'<script[^>]+src=', html), [])
            self.assertEqual(re.findall(r'<link[^>]+stylesheet[^>]+https?:', html), [])
            self.assertIn('const desktop = host.has("wmxLoad")', html)
            self.assertIn(f'<link rel="canonical" href="{build_site.SITE_URL}">', html)
            self.assertTrue(build_site.SITE_URL.startswith('https://') and build_site.SITE_URL.endswith('/'))


if __name__ == '__main__':
    unittest.main()
