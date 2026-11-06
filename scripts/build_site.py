#!/usr/bin/env python3
"""Build the GitHub Pages website from web/ into site/ (or --out).

The site is the same interface the Windows app shows. Without the Windows
host it runs as the web version: data stays in the browser's storage and the
update controls are hidden.

    python3 scripts/build_site.py
"""
import argparse
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
# The website's public address (custom domain set in Settings › Pages).
SITE_URL = 'https://weightmaxer.techsavage3d.com/'
HEAD_EXTRA = (
    '<meta name="description" content="Plan how long it takes to lose or gain weight, '
    'your daily calories and macros, and track your weigh-ins. Runs in your browser; your data stays on your device.">\n'
    '<meta name="theme-color" content="#0e6f7a">\n'
    '<link rel="icon" type="image/png" href="icon.png">\n'
    '<link rel="apple-touch-icon" href="icon.png">\n'
    f'<link rel="canonical" href="{SITE_URL}">\n'
    '<meta property="og:title" content="Weightmaxer">\n'
    f'<meta property="og:url" content="{SITE_URL}">\n'
    f'<meta property="og:image" content="{SITE_URL}icon.png">\n'
)


def build(out: Path) -> Path:
    web = ROOT / 'web'
    html = (web / 'index.html').read_text(encoding='utf-8')
    css = (web / 'app.css').read_text(encoding='utf-8')
    js = (web / 'app.js').read_text(encoding='utf-8')
    link, script = '<link rel="stylesheet" href="app.css">', '<script src="app.js"></script>'
    if link not in html or script not in html or '</title>\n' not in html:
        raise SystemExit('web/index.html no longer has the expected stylesheet, script or title tags')
    html = html.replace(link, '<style>\n' + css + '\n</style>', 1)
    html = html.replace(script, '<script>\n' + js + '\n</script>', 1)
    html = html.replace('</title>\n', '</title>\n' + HEAD_EXTRA, 1)
    if out.exists():
        shutil.rmtree(out)
    out.mkdir(parents=True)
    (out / 'index.html').write_text(html, encoding='utf-8', newline='\n')
    shutil.copy(ROOT / 'assets' / 'icon.png', out / 'icon.png')
    return out


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--out', default='site')
    a = ap.parse_args()
    print(f'built {build((ROOT / a.out).resolve())}')
