// Recaptures the README screenshots in docs/images/ from the real interface
// (web/index.html, the page the WeightMaxxer window shows) with fictional data.
// Run after visible UI changes: node tests/browser/screenshots.cjs
const path = require("path");
const fs = require("fs");
const { chromium } = require("playwright");
const VERSION = fs.readFileSync(path.resolve(__dirname, "..", "..", "VERSION"), "utf8").trim();
const NOTES = fs.readFileSync(path.resolve(__dirname, "..", "..", "docs", "releases", `${VERSION}.md`), "utf8")
  .split("\n").filter((line) => line.startsWith("- ")).map((line) => line.slice(2));

const PAGE = "file://" + path.resolve(__dirname, "..", "..", "web", "index.html").replace(/\\/g, "/");
const OUT = path.resolve(__dirname, "..", "..", "docs", "images");
const sample = {
  schema: 1, units: "imp", mode: "pace",
  profile: { sex: "f", age: "38", hft: "5", hin: "6", w: "196", g: "165", act: "1.375", rate: "1", start: "2026-09-07", macro: "balanced" },
  log: [["2026-09-07", 196], ["2026-09-14", 194.6], ["2026-09-21", 194.0], ["2026-09-28", 192.8], ["2026-10-05", 192.2],
        ["2026-10-12", 191.4], ["2026-10-19", 190.0], ["2026-10-26", 189.6], ["2026-11-02", 188.4]].map(([d, lb]) => ({ d, kg: lb * 0.45359237 })),
};

async function page(browser, doc, upd) {
  const p = await browser.newPage({ viewport: { width: 1280, height: 860 }, deviceScaleFactor: 1, locale: "en-US", timezoneId: "UTC" });
  await p.clock.setFixedTime(new Date("2026-11-23T17:00:00Z"));
  await p.addInitScript(([doc, upd, version]) => {
    window.wmxLoad = () => doc;
    window.wmxSave = () => {};
    window.wmxInfo = () => ({ version, dataFolder: "C:\\Users\\alex\\AppData\\Roaming\\Weightmaxer" });
    window.wmxReady = () => window.wmxUpdate(upd);
    try { localStorage.clear(); } catch (e) {}
  }, [JSON.stringify(doc), upd, VERSION]);
  await p.goto(PAGE);
  await p.waitForTimeout(300);
  return p;
}

(async () => {
  const browser = await chromium.launch(process.env.CI_CHROME_PATH ? { executablePath: process.env.CI_CHROME_PATH } : {});
  const idle = { current: VERSION, phase: "idle", check: { kind: "up-to-date", at: "2026-11-23T17:00:00Z" }, progress: {} };

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

  const avail = { current: "1.2.0", latest: VERSION, phase: "available", notes: NOTES.slice(0, 4), more: NOTES.length > 4, check: { kind: "available", at: "2026-11-23T17:00:00Z" }, progress: {} };
  p = await page(browser, sample, avail);
  await p.screenshot({ path: path.join(OUT, "update-banner.png"), clip: { x: 0, y: 0, width: 1280, height: 420 } });
  await p.close();

  await browser.close();
  console.log("screenshots written to docs/images/");
})();
