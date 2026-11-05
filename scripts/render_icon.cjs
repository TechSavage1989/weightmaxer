// Renders assets/icon.svg to assets/icon.png (256x256, transparent) with Playwright,
// and copies it to docs/images/icon.png for the README header.
// Run after changing the SVG: node scripts/render_icon.cjs
const path = require("path");
const fs = require("fs");
const { chromium } = require("playwright");
(async () => {
  const svg = fs.readFileSync(path.join(__dirname, "..", "assets", "icon.svg"), "utf8");
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 256, height: 256 }, deviceScaleFactor: 1 });
  await page.setContent(`<html><body style="margin:0;background:transparent">${svg.replace("<svg ", '<svg width="256" height="256" ')}</body></html>`);
  await page.screenshot({ path: path.join(__dirname, "..", "assets", "icon.png"), omitBackground: true });
  await browser.close();
  fs.copyFileSync(path.join(__dirname, "..", "assets", "icon.png"), path.join(__dirname, "..", "docs", "images", "icon.png"));
})();
