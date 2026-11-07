// Recaptures the README screenshots in docs/images/ from the real interface
// (web/index.html, the page the Weightmaxer window shows) with fictional data.
// Run after visible UI changes: node tests/browser/screenshots.cjs
const path = require("path");
const { chromium } = require("playwright");

const PAGE = "file://" + path.resolve(__dirname, "..", "..", "web", "index.html").replace(/\\/g, "/");
const OUT = path.resolve(__dirname, "..", "..", "docs", "images");
const sample = {
  schema: 1, units: "imp", mode: "pace",
  profile: { sex: "f", age: "38", hft: "5", hin: "6", w: "196", g: "165", act: "1.375", rate: "1", start: "2026-09-07", macro: "balanced" },
  log: [["2026-09-07", 196], ["2026-09-14", 194.6], ["2026-09-21", 194.0], ["2026-09-28", 192.8], ["2026-10-05", 192.2],
        ["2026-10-12", 191.4], ["2026-10-19", 190.0], ["2026-10-26", 189.6], ["2026-11-02", 188.4]].map(([d, lb]) => ({ d, kg: lb * 0.45359237 })),
};

async function page(browser, doc, upd) {
  const p = await browser.newPage({ viewport: { width: 1280, height: 860 }, deviceScaleFactor: 1 });
  await p.addInitScript(([doc, upd]) => {
    window.wmxLoad = () => doc;
    window.wmxSave = () => {};
    window.wmxInfo = () => ({ version: "1.0.0", dataFolder: "C:\\Users\\alex\\AppData\\Roaming\\Weightmaxer" });
    window.wmxReady = () => window.wmxUpdate(upd);
    try { localStorage.clear(); } catch (e) {}
  }, [JSON.stringify(doc), upd]);
  await p.goto(PAGE);
  await p.waitForTimeout(300);
  return p;
}

(async () => {
  const browser = await chromium.launch();
  const idle = { current: "1.0.0", phase: "idle", check: { kind: "up-to-date", at: new Date().toISOString() }, progress: {} };

  let p = await page(browser, sample, idle);
  await p.screenshot({ path: path.join(OUT, "hero.png") });
  await p.close();

  p = await page(browser, sample, idle);
  await p.click("#tab-log");
  await p.waitForTimeout(200);
  // Show the chart readout on the latest weigh-in.
  const pt = await p.evaluate(() => {
    const svg = document.getElementById("log-chart"), c = svg._chart, r = svg.getBoundingClientRect();
    const last = c.actual[c.actual.length - 1];
    return { x: r.left + (c.x(last.x) / c.W) * r.width, y: r.top + r.height / 2 };
  });
  await p.mouse.move(pt.x, pt.y);
  await p.waitForTimeout(100);
  await p.screenshot({ path: path.join(OUT, "weigh-ins.png") });
  await p.close();

  const gain = { ...sample, profile: { ...sample.profile, sex: "m", age: "27", hft: "5", hin: "11", w: "150", g: "165", rate: "0.5", act: "1.55" }, log: [] };
  p = await page(browser, gain, idle);
  await p.setViewportSize({ width: 1280, height: 1280 });
  await p.waitForTimeout(200);
  await p.screenshot({ path: path.join(OUT, "gain-and-macros.png") });
  await p.close();

  const avail = { current: "1.0.0", latest: "1.1.0", phase: "available", notes: ["Weekly averages on the weigh-in chart", "Export your weigh-ins as CSV"], more: true, check: { kind: "available", at: new Date().toISOString() }, progress: {} };
  p = await page(browser, sample, avail);
  await p.screenshot({ path: path.join(OUT, "update-banner.png"), clip: { x: 0, y: 0, width: 1280, height: 420 } });
  await p.close();

  await browser.close();
  console.log("screenshots written to docs/images/");
})();
