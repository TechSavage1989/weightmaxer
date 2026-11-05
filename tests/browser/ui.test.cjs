// Browser tests for web/index.html with the Windows host replaced by stubs.
// Run: npm test --prefix tests   (or node tests/browser/ui.test.cjs)
const path = require("path");
const assert = require("assert");
const { chromium } = require("playwright");

const PAGE = "file://" + path.resolve(__dirname, "..", "..", "web", "index.html").replace(/\\/g, "/");
let failures = 0;

async function withPage(browser, opts, fn) {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: opts.dark ? "dark" : "light" });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  await page.addInitScript((saved) => {
    window.__saved = [];
    window.__calls = [];
    window.wmxLoad = () => saved;
    window.wmxSave = (doc) => { window.__saved.push(doc); };
    window.wmxInfo = () => ({ version: "1.0.0", dataFolder: "C:\\Users\\example\\AppData\\Roaming\\Weightmaxer" });
    window.wmxReady = () => window.__calls.push("ready");
    for (const n of ["wmxDownload", "wmxOpenNotes", "wmxDismissNotice", "wmxOpenDataFolder"]) window[n] = () => window.__calls.push(n);
    window.wmxInstall = () => { window.__calls.push("wmxInstall"); return ""; };
    window.wmxCheckNow = () => ({ accepted: false, message: "Checked just now. Try again in 40 seconds." });
  }, opts.saved || "{}");
  await page.goto(PAGE);
  await page.waitForFunction(() => window.__calls.includes("ready"));
  await fn(page);
  assert.deepStrictEqual(errors, [], "page errors");
  await page.close();
}

async function test(name, fn) {
  try { await fn(); console.log("ok   " + name); }
  catch (e) { failures++; console.log("FAIL " + name + "\n     " + (e.stack || e).toString().split("\n").slice(0, 4).join("\n     ")); }
}

const set = async (page, sel, value) => { await page.fill(sel, String(value)); await page.dispatchEvent(sel, "input"); };
const update = (page, u) => page.evaluate((u) => window.wmxUpdate(u), u);
const base = { current: "1.0.0", latest: "1.1.0", dev: false, check: { kind: "available", at: new Date().toISOString() }, notes: ["Faster charts"], more: false, progress: { done: 0, total: 0 } };

(async () => {
  const browser = await chromium.launch(process.env.CI_CHROME_PATH ? { executablePath: process.env.CI_CHROME_PATH } : {});

  await test("loss plan, gain plan and macros", () => withPage(browser, {}, async (page) => {
    assert.match(await page.textContent("#head"), /^Lose 31\.0 lb by /);
    assert.strictEqual(await page.textContent(".stat .v"), "1,682");
    assert.strictEqual(await page.locator(".macro").count(), 3);
    await set(page, "#g", 210);
    assert.match(await page.textContent("#head"), /^Gain 14\.0 lb by /);
    assert.strictEqual(await page.textContent("#rate-verb"), "Weekly gain");
    assert.ok(await page.isHidden("#floor-f"), "calorie minimum hidden for gain");
    const intake = parseInt((await page.textContent(".stat .v")).replace(/,/g, ""), 10);
    const maint = parseInt((await page.locator(".stat .v").nth(1).textContent()).replace(/,/g, ""), 10);
    assert.ok(intake > maint, "gain eats above maintenance");
  }));

  await test("lower calorie minimum is allowed with a warning, never below 800", () => withPage(browser, {}, async (page) => {
    await page.selectOption("#sex", "m"); await page.selectOption("#act", "1.2");
    await set(page, "#w", 180); await set(page, "#g", 150); await set(page, "#rate", 2);
    assert.strictEqual(await page.textContent(".stat .v"), "1,500");
    await set(page, "#floor", 1100);
    assert.strictEqual(await page.textContent(".stat .v"), "1,100");
    assert.match(await page.textContent("#alerts"), /below the usual minimum of 1,500/);
    await set(page, "#floor", 500);
    assert.strictEqual(await page.textContent(".stat .v"), "1,500", "values under 800 fall back to the standard minimum");
    assert.match(await page.textContent("#floor-hint"), /Enter 800–4000/);
  }));

  await test("metric units convert and the plan is unchanged", () => withPage(browser, {}, async (page) => {
    const head = await page.textContent("#head");
    await page.click("#u-met");
    assert.strictEqual(await page.inputValue("#w"), "88.9");
    assert.match(await page.textContent("#head"), /^Lose 14\.\d kg by /);
    // The pace rounds to 0.45 kg/week, so the date may move slightly.
    const days = Math.abs(new Date(head.split(" by ")[1]) - new Date((await page.textContent("#head")).split(" by ")[1])) / 864e5;
    assert.ok(days <= 14, `goal date moved ${days} days`);
  }));

  await test("saved data loads and weigh-ins save", () => withPage(browser, {
    saved: JSON.stringify({ schema: 1, units: "met", mode: "pace", profile: { w: "90", g: "80", sex: "m", age: "40", hcm: "180", act: "1.55", rate: "0.5", start: "2026-11-01" }, log: [{ d: "2026-11-01", kg: 90 }] }),
  }, async (page) => {
    assert.strictEqual(await page.inputValue("#w"), "90");
    assert.strictEqual(await page.getAttribute("#u-met", "aria-pressed"), "true");
    await page.click("#tab-log");
    assert.strictEqual(await page.locator(".log-row").count(), 1);
    await page.fill("#log-date", "2026-11-08"); await page.fill("#log-w", "89.2"); await page.click("#log-add");
    assert.strictEqual(await page.locator(".log-row").count(), 2);
    assert.match(await page.textContent("#log-sub"), /Down 0\.8 kg since the plan started/);
    const saved = JSON.parse(await page.evaluate(() => window.__saved[window.__saved.length - 1]));
    assert.deepStrictEqual(saved.log.map((e) => e.d), ["2026-11-01", "2026-11-08"]);
    await page.click(".log-row .del");
    assert.strictEqual(await page.locator(".log-row").count(), 1);
  }));

  await test("weigh-in chart shows the plan when Weigh-ins was the last tab", async () => {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await page.addInitScript(() => {
      try { localStorage.setItem("wmx-view", "log"); } catch (e) {}
      window.wmxLoad = () => "{}"; window.wmxSave = () => {}; window.wmxReady = () => {};
    });
    await page.goto(PAGE);
    await page.waitForFunction(() => document.getElementById("log-chart").innerHTML.length > 0);
    assert.ok(await page.isVisible("#view-log"));
    assert.ok((await page.locator("#log-chart path").count()) >= 2, "plan line drawn");
    assert.match(await page.textContent("#head"), /^Lose /);
    await ctx.close();
  });

  await test("update banner: versions, notes, Read more only when there is more", () => withPage(browser, {}, async (page) => {
    await update(page, { ...base, phase: "available" });
    const text = await page.textContent("#update-banner");
    assert.match(text, /Weightmaxer 1\.1\.0 is available\. You have 1\.0\.0\./);
    assert.ok(!/[→⇒>]/.test(text), "no version arrows");
    assert.strictEqual(await page.locator("#update-banner [data-act=notes]").count(), 0);
    await update(page, { ...base, phase: "available", more: true });
    assert.strictEqual(await page.locator("#update-banner [data-act=notes]").count(), 1);
    await page.click("[data-act=download]");
    assert.ok((await page.evaluate(() => window.__calls)).includes("wmxDownload"));
    await update(page, { ...base, phase: "downloading", progress: { done: 50, total: 200 } });
    assert.match(await page.textContent("#update-banner"), /25%/);
    await page.click("[data-act=later]").catch(() => {});
  }));

  await test("install waits for an unfinished weigh-in, then installs", () => withPage(browser, {}, async (page) => {
    await page.click("#tab-log"); await page.fill("#log-w", "190");
    await update(page, { ...base, phase: "ready" });
    assert.match(await page.textContent("#update-banner"), /will close and reopen/);
    await page.click("[data-act=install]");
    assert.match(await page.textContent("#install-msg"), /isn't added yet/);
    assert.ok(!(await page.evaluate(() => window.__calls)).includes("wmxInstall"));
    await page.fill("#log-w", "");
    await page.click("[data-act=install]");
    await page.waitForFunction(() => window.__calls.includes("wmxInstall"));
  }));

  await test("failed checks never say up to date", () => withPage(browser, {}, async (page) => {
    for (const kind of ["offline", "rate-limited", "error"]) {
      await update(page, { ...base, latest: "", phase: "idle", check: { kind, message: "Couldn't reach GitHub.", at: new Date().toISOString() } });
      const s = await page.textContent("#set-status");
      assert.ok(!/up to date/i.test(s), `${kind}: ${s}`);
      assert.match(s, /Couldn't check for updates/);
    }
    await update(page, { ...base, latest: "", phase: "idle", check: { kind: "up-to-date", at: new Date().toISOString() } });
    assert.match(await page.textContent("#set-status"), /^Up to date · checked just now/);
    await page.click("#tab-settings"); await page.click("#check-now");
    assert.match(await page.textContent("#check-msg"), /Try again in 40 seconds/);
  }));

  await test("notices after an update and after a rollback", () => withPage(browser, {}, async (page) => {
    await update(page, { ...base, latest: "", phase: "idle", notice: { ok: true, title: "Weightmaxer was updated to 1.1.0." } });
    assert.match(await page.textContent("#update-banner"), /updated to 1\.1\.0/);
    await update(page, { ...base, latest: "", phase: "idle", notice: { ok: false, title: "The update to 1.1.0 didn't finish.", detail: "Weightmaxer went back to version 1.0.0." } });
    assert.ok((await page.getAttribute("#update-banner", "class")).includes("bad"));
    await page.click("[data-act=dismiss-notice]");
    assert.ok(await page.isHidden("#update-banner"));
  }));

  await test("no nested vertical scroll areas, in light and dark", async () => {
    for (const dark of [false, true]) {
      await withPage(browser, { dark }, async (page) => {
        await page.setViewportSize({ width: 1000, height: 700 });
        for (const tab of ["plan", "log", "settings"]) {
          await page.click("#tab-" + tab);
          const nested = await page.evaluate(() => [...document.querySelectorAll("body *")].filter((el) => {
            const s = getComputedStyle(el);
            return /(auto|scroll)/.test(s.overflowY) && el.scrollHeight > el.clientHeight + 1;
          }).map((el) => el.id || el.className));
          assert.deepStrictEqual(nested, [], `${tab} (${dark ? "dark" : "light"})`);
          const wide = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
          assert.ok(!wide, `${tab} scrolls sideways`);
        }
      });
    }
  });

  await test("phone width: nothing cut off, tabs on one line, BMI labels apart", async () => {
    const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
    const page = await ctx.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(process.env.WMX_SITE_URL || PAGE);
    await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
    // Start and goal BMI close together, as in the reported screenshot.
    await page.selectOption("#sex", "m");
    await set(page, "#hft", 6); await set(page, "#hin", 0); await set(page, "#w", 176); await set(page, "#g", 169);
    await set(page, "#floor", 800);
    await page.click("#m-date");
    for (const tab of ["plan", "log", "settings"]) {
      await page.click("#tab-" + tab);
      const problems = await page.evaluate(() => {
        const vw = document.documentElement.clientWidth;
        const out = [];
        if (document.documentElement.scrollWidth > vw) out.push(`page is ${document.documentElement.scrollWidth}px wide in a ${vw}px screen`);
        for (const el of document.querySelectorAll(".topbar *, .view:not([hidden]) .card, .view:not([hidden]) input, .view:not([hidden]) select, .view:not([hidden]) .btn")) {
          const r = el.getBoundingClientRect();
          if (r.width && (r.right > vw + 0.5 || r.left < -0.5)) out.push(`${el.id || el.className || el.tagName} spans ${Math.round(r.left)}–${Math.round(r.right)}`);
        }
        for (const b of document.querySelectorAll(".tabs button")) {
          if (b.getBoundingClientRect().height > 44) out.push(`tab "${b.textContent}" wraps`);
        }
        // Inputs stay inside their card.
        for (const inp of document.querySelectorAll(".view:not([hidden]) .card input, .view:not([hidden]) .card select")) {
          const card = inp.closest(".card").getBoundingClientRect(), r = inp.getBoundingClientRect();
          if (r.width && r.right > card.right - 1) out.push(`${inp.id} sticks out of its card`);
        }
        for (const w of document.querySelectorAll(".view:not([hidden]) .table-wrap")) {
          if (w.scrollWidth > w.clientWidth + 1) out.push(`table needs side-scrolling (${w.scrollWidth}px in ${w.clientWidth}px)`);
        }
        const marks = [...document.querySelectorAll(".bmi-mark .lab")].map((m) => m.getBoundingClientRect());
        if (marks.length === 2 && !document.getElementById("view-plan").hidden) {
          const [a, b] = marks;
          if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) out.push("BMI labels overlap");
        }
        return out;
      });
      assert.deepStrictEqual(problems, [], tab + ": " + problems.join("; "));
    }
    // Text inputs are at least 16px so iPhone Safari doesn't zoom in on focus.
    const small = await page.evaluate(() => [...document.querySelectorAll("input:not([type=range]):not([type=file]), select")].filter((e) => parseFloat(getComputedStyle(e).fontSize) < 16).map((e) => e.id));
    assert.deepStrictEqual(small, [], "inputs under 16px: " + small.join(", "));
    assert.deepStrictEqual(errors, []);
    await ctx.close();
  });

  await test("web version: saves in the browser, hides updates, exports and imports", async () => {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 }, acceptDownloads: true });
    const page = await ctx.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const site = process.env.WMX_SITE_URL || PAGE;
    await page.goto(site);
    await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
    await set(page, "#w", 200);
    await page.click("#tab-log"); await page.fill("#log-date", "2026-11-03"); await page.fill("#log-w", "199"); await page.click("#log-add");
    await page.waitForTimeout(500);
    await page.reload();
    await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
    assert.strictEqual(await page.inputValue("#w"), "200", "form survives a reload");
    await page.click("#tab-log");
    assert.strictEqual(await page.locator(".log-row").count(), 1, "weigh-in survives a reload");
    await page.click("#tab-settings");
    assert.ok(await page.isHidden("#check-now"), "update controls hidden on the web");
    assert.ok(await page.isHidden("#open-folder"));
    assert.ok(await page.isVisible("#export-data"));
    assert.ok(await page.isHidden("#update-banner"));
    const [dl] = await Promise.all([page.waitForEvent("download"), page.click("#export-data")]);
    assert.match(dl.suggestedFilename(), /^weightmaxer-data-\d{4}-\d\d-\d\d\.json$/);
    const exported = JSON.parse(require("fs").readFileSync(await dl.path(), "utf8"));
    assert.strictEqual(exported.log.length, 1);
    exported.log.push({ d: "2026-11-10", kg: 89 });
    await page.setInputFiles("#import-file", { name: "w.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(exported)) });
    await page.waitForFunction(() => /Imported 2 weigh-ins/.test(document.getElementById("data-msg").textContent));
    await page.setInputFiles("#import-file", { name: "bad.json", mimeType: "application/json", buffer: Buffer.from("[1,2]") });
    await page.waitForFunction(() => /isn't a Weightmaxer export/.test(document.getElementById("data-msg").textContent));
    await page.click("#tab-log");
    assert.strictEqual(await page.locator(".log-row").count(), 2, "bad import leaves data alone");
    assert.deepStrictEqual(errors, []);
    await ctx.close();
  });

  await browser.close();
  if (failures) { console.log(`${failures} failed`); process.exit(1); }
  console.log("all browser tests passed");
})();
