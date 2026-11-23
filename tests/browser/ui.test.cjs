// Browser tests for web/index.html with the Windows host replaced by stubs.
// Run: npm test --prefix tests   (or node tests/browser/ui.test.cjs)
const path = require("path");
const assert = require("assert");
const { chromium } = require("playwright");

const PAGE = "file://" + path.resolve(__dirname, "..", "..", "web", "index.html").replace(/\\/g, "/");
let failures = 0;

async function withPage(browser, opts, fn) {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: opts.dark ? "dark" : "light", locale: "en-US", timezoneId: opts.timezone || "UTC" });
  if (opts.now) await page.clock.setFixedTime(new Date(opts.now));
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  if (opts.web) {
    await page.addInitScript((saved) => {
      if (!localStorage.getItem("weightmaxer-data")) localStorage.setItem("weightmaxer-data", saved);
    }, opts.saved || "{}");
  } else await page.addInitScript((saved) => {
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
  await page.goto(opts.web ? (process.env.WMX_SITE_URL || PAGE) : PAGE);
  await page.waitForFunction((web) => web ? document.getElementById("head").textContent.length > 0 : window.__calls.includes("ready"), !!opts.web);
  try {
    await fn(page);
    assert.deepStrictEqual(errors, [], "page errors");
  } finally {
    await page.close();
  }
}

async function test(name, fn) {
  try { await fn(); console.log("ok   " + name); }
  catch (e) { failures++; console.log("FAIL " + name + "\n     " + (e.stack || e).toString().split("\n").slice(0, 4).join("\n     ")); }
}

const set = async (page, sel, value) => { await page.fill(sel, String(value)); await page.dispatchEvent(sel, "input"); };
const update = (page, u) => page.evaluate((u) => window.wmxUpdate(u), u);
const base = { current: "1.0.0", latest: "1.1.0", dev: false, check: { kind: "available", at: new Date().toISOString() }, notes: ["Faster charts"], more: false, progress: { done: 0, total: 0 } };


// Fictional profile with a steady, unconstrained 0.5 kg/week plan. Keep the
// calendar fixed so eligibility and deadline tests do not age out in CI.
const forecastProfile = { sex: "m", age: "40", hcm: "180", w: "90", g: "80", act: "1.55", rate: "0.5", start: "2026-11-01", date: "2027-03-21", macro: "balanced" };
const forecastDoc = (profile = {}, log = [], mode = "pace") => JSON.stringify({ schema: 1, units: "met", mode, profile: { ...forecastProfile, ...profile }, log });
const forecastOpts = (doc = forecastDoc(), opts = {}) => ({ saved: doc, now: "2026-11-23T17:00:00Z", ...opts });
const weigh = async (page, date, weight) => {
  await page.click("#tab-log");
  await page.fill("#log-date", date);
  await page.fill("#log-w", String(weight));
  await page.click("#log-add");
};
const forecast = (page) => page.evaluate(() => {
  const chart = document.getElementById("chart")._chart;
  return {
    head: document.getElementById("head").textContent,
    sub: document.getElementById("subhead").textContent,
    goal: document.getElementById("schedule-goal").textContent,
    rows: [...document.querySelectorAll("#table tbody tr")].map((r) => [...r.cells].map((c) => c.textContent.trim())),
    stats: [...document.querySelectorAll("#stats .stat")].map((s) => ({ label: s.querySelector(".lbl").textContent, value: s.querySelector(".v").textContent })),
    macros: document.getElementById("macros").textContent,
    points: chart ? chart.plan.map((p) => ({ x: p.x, y: p.y })) : [],
    fields: Object.fromEntries(["w", "g", "start", "rate", "date"].map((id) => [id, document.getElementById(id).value])),
  };
});
const rowWeight = (snapshot) => Number.parseFloat(snapshot.rows[0][2]);
const endDay = (snapshot) => Date.parse(snapshot.rows[snapshot.rows.length - 1][1] + " 12:00:00 UTC");
const statValue = (snapshot, label) => Number(snapshot.stats.find((s) => s.label === label).value.replace(/,/g, ""));

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

  await test("weigh-ins immediately revise the loss forecast without rewriting the plan", () => withPage(browser, forecastOpts(), async (page) => {
    const original = await forecast(page);
    await weigh(page, "2026-11-15", 88);
    // Read the hidden Plan results before navigating back. Merely refreshing
    // the log chart or recalculating only when the Plan tab opens is not enough.
    const faster = await forecast(page);
    assert.match(faster.head, /^Lose 8\.0 kg by /);
    assert.strictEqual(rowWeight(faster), 88);
    assert.strictEqual(faster.rows[0][0], "2");
    assert.strictEqual(faster.rows[0][1], "Nov 15, 2026");
    assert.ok(endDay(faster) < endDay(original), "faster progress moves the estimated finish earlier");
    assert.ok(statValue(faster, "Eat now") < statValue(original, "Eat now"), "daily calories use the latest weight");
    assert.notStrictEqual(faster.macros, original.macros, "macros use the revised calorie target");
    assert.deepStrictEqual(faster.fields, original.fields, "starting weight, goal, start date and choices stay intact");
    assert.deepStrictEqual(faster.points[0], { x: 2, y: 88 }, "forecast begins at the recorded date and weight");
    assert.match(await page.textContent("#log-sub"), /ahead of (?:the )?(?:original )?plan/i, "progress compares with the original plan");
    const saved = JSON.parse(await page.evaluate(() => window.__saved.at(-1)));
    assert.strictEqual(saved.profile.w, "90");
    assert.strictEqual(saved.profile.start, "2026-11-01");
    assert.strictEqual(saved.profile.g, "80");

    await weigh(page, "2026-11-15", 89.5);
    const slower = await forecast(page);
    assert.strictEqual(await page.locator(".log-row").count(), 1, "same-date entry is replaced");
    assert.strictEqual(rowWeight(slower), 89.5);
    assert.ok(endDay(slower) > endDay(original), "slower progress moves the finish later");
    assert.match(await page.textContent("#log-sub"), /behind (?:the )?(?:original )?plan/i);

    await weigh(page, "2026-11-08", 89);
    assert.deepStrictEqual(await forecast(page), slower, "adding an older entry does not replace the newest forecast anchor");
    await page.click('.del[data-d="2026-11-15"]');
    const previous = await forecast(page);
    assert.strictEqual(rowWeight(previous), 89);
    assert.strictEqual(previous.rows[0][1], "Nov 8, 2026");
    await page.click('.del[data-d="2026-11-08"]');
    assert.deepStrictEqual(await forecast(page), original, "removing every weigh-in restores the starting forecast");
  }));

  await test("gain forecasts update and goal attainment never reverses the plan", async () => {
    await withPage(browser, forecastOpts(forecastDoc({ w: "70", g: "75" })), async (page) => {
      const original = await forecast(page);
      await weigh(page, "2026-11-15", 72);
      const revised = await forecast(page);
      assert.match(revised.head, /^Gain 3\.0 kg by /);
      assert.strictEqual(rowWeight(revised), 72);
      assert.ok(endDay(revised) < endDay(original), "gain progress shortens the remaining plan");
      assert.ok(statValue(revised, "Eat now") > statValue(original, "Eat now"));
      for (const weight of [75, 76]) {
        await weigh(page, "2026-11-15", weight);
        const reached = await forecast(page);
        assert.match(reached.head, /goal.*reach|reach.*goal/i);
        assert.ok(!/^Lose /.test(reached.head), "a successful gain must not become a loss plan");
        assert.ok(reached.stats.some((s) => /maintenance/i.test(s.label)), "goal attainment shows maintenance");
        assert.strictEqual(reached.fields.g, "75");
      }
    });
    await withPage(browser, forecastOpts(), async (page) => {
      for (const weight of [80, 79]) {
        await weigh(page, "2026-11-15", weight);
        const reached = await forecast(page);
        assert.match(reached.head, /goal.*reach|reach.*goal/i);
        assert.ok(!/^Gain /.test(reached.head), "a successful loss must not become a gain plan");
        assert.ok(reached.stats.some((s) => /maintenance/i.test(s.label)));
        assert.strictEqual(reached.fields.g, "80");
      }
      await page.click('.del[data-d="2026-11-15"]');
      assert.match((await forecast(page)).head, /^Lose 10\.0 kg by /, "removing the completed weigh-in restores the active plan");
    });
  });

  await test("target-date plans keep the deadline and recompute the remaining pace", () => withPage(browser, forecastOpts(forecastDoc({}, [], "date")), async (page) => {
    const original = await forecast(page);
    await weigh(page, "2026-11-15", 88);
    const revised = await forecast(page);
    const remainingDays = (Date.UTC(2027, 2, 21) - Date.UTC(2026, 10, 15)) / 864e5;
    const maintenance = (10 * 88 + 6.25 * 180 - 5 * 40 + 5) * 1.55;
    const expectedCalories = Math.round(maintenance - (8 * 7700 / remainingDays));
    assert.strictEqual(revised.fields.date, "2027-03-21", "chosen deadline is preserved");
    assert.strictEqual(rowWeight(revised), 88);
    assert.ok(Math.abs(statValue(revised, "Eat now") - expectedCalories) <= 1, "intake uses remaining weight divided by remaining days");
    assert.ok(statValue(revised, "Eat now") > statValue(original, "Eat now"), "ahead of schedule needs a smaller daily deficit");
    assert.ok(Math.abs(endDay(revised) - Date.UTC(2027, 2, 21, 12)) <= 864e5, "unconstrained forecast finishes by the target within daily simulation precision");
    await page.click("#tab-plan");
    await page.focus("#chart");
    await page.keyboard.press("End");
    assert.ok(await page.isVisible("#view-plan .chart-tip"));
    // Keep the chart focused while an invalid date triggers a re-render. A
    // normal click in the date field would hide the readout independently.
    await page.evaluate(() => {
      const date = document.getElementById("date");
      date.value = "2026-11-10";
      date.dispatchEvent(new Event("input", { bubbles: true }));
    });
    const expired = await forecast(page);
    assert.match(expired.head + " " + expired.sub, /later|passed|after.*weigh|future/i, "expired deadline explains how to resume");
    assert.deepStrictEqual(expired.rows, [], "stale future schedule is removed");
    assert.deepStrictEqual(expired.points, [], "stale chart state is removed");
    assert.ok(await page.isHidden("#macro-card"), "stale macro targets are removed");
    assert.ok(await page.isHidden("#view-plan .chart-tip"), "stale readout is removed");
  }));

  await test("loaded forecasts use date order and ignore entries outside the current plan", () => withPage(browser, forecastOpts(forecastDoc({}, [
    { d: "2026-11-15", kg: 88 },
    { d: "2026-10-31", kg: 94 },
    { d: "2026-11-08", kg: 89 },
    { d: "2026-11-24", kg: 85 },
    { d: "2026-11-31", kg: 84 },
  ])), async (page) => {
    const loaded = await forecast(page);
    assert.strictEqual(rowWeight(loaded), 88, "newest eligible date wins, not array order");
    assert.strictEqual(loaded.rows[0][1], "Nov 15, 2026");
    await page.click("#tab-log");
    assert.strictEqual(await page.locator(".log-row").count(), 4, "valid out-of-range entries remain in history; impossible calendar dates do not");
    await page.click('.del[data-d="2026-11-15"]');
    assert.strictEqual(rowWeight(await forecast(page)), 89);
    await page.click('.del[data-d="2026-11-08"]');
    const baseline = await forecast(page);
    assert.strictEqual(rowWeight(baseline), 90, "only pre-plan/future entries falls back to the starting plan");
    assert.strictEqual(baseline.rows[0][1], "Nov 1, 2026");
  }));

  await test("forecast chart dates and schedule agree across daylight saving", () => withPage(browser, forecastOpts(forecastDoc({ start: "2026-10-25" }, [{ d: "2026-11-08", kg: 88 }]), { timezone: "America/New_York" }), async (page) => {
    const revised = await forecast(page);
    assert.deepStrictEqual(revised.points[0], { x: 2, y: 88 }, "14 calendar days is exactly two weeks across the fall DST change");
    assert.strictEqual(revised.rows[0][0], "2");
    assert.strictEqual(revised.rows[0][1], "Nov 8, 2026");
    await page.focus("#chart");
    const tip = page.locator("#view-plan .chart-tip");
    assert.ok(!/Plan88\.0 kg/.test(await tip.textContent()), "history before the forecast anchor is not reported as the first forecast weight");
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("ArrowRight");
    assert.match(await tip.textContent(), /Nov 8, 2026/);
    assert.match(await tip.textContent(), /Plan88\.0 kg/);
    await page.click("#tab-log");
    const point = await page.evaluate(() => document.getElementById("log-chart")._chart.actual.find((p) => p.d === "2026-11-08"));
    assert.strictEqual(point.x, 2, "actual and forecast points share the same calendar-day offset");
    await weigh(page, "2026-11-09", 87.8);
    assert.strictEqual((await forecast(page)).points[0].x, 15 / 7, "midweek weigh-ins retain their exact date offset");
    await page.setViewportSize({ width: 390, height: 844 });
    await page.click("#tab-plan");
    const overflow = await page.evaluate(() => {
      const table = document.querySelector(".table-wrap");
      return document.documentElement.scrollWidth > innerWidth + 1 || table.scrollWidth > table.clientWidth + 1;
    });
    assert.strictEqual(overflow, false, "revised midweek schedule fits a phone without side scrolling");
  }));

  await test("weight regain beyond the starting weight stays inside the chart", () => withPage(browser, forecastOpts(forecastDoc({}, [{ d: "2026-11-15", kg: 94 }])), async (page) => {
    const revised = await forecast(page);
    assert.match(revised.head, /^Lose 14\.0 kg by /);
    assert.strictEqual(rowWeight(revised), 94);
    const positions = await page.evaluate(() => {
      const c = document.getElementById("chart")._chart;
      return c.plan.map((p) => ({ y: c.y(p.y), top: c.m.t, bottom: c.H - c.m.b }));
    });
    assert.ok(positions.every((p) => p.y >= p.top && p.y <= p.bottom), "forecast weights fit within the chart's vertical scale");
  }));

  await test("five-year forecast cutoff is not reported as goal completion", () => withPage(browser, forecastOpts(forecastDoc({ w: "300", rate: "0.1" }, [{ d: "2026-11-15", kg: 299 }])), async (page) => {
    const limited = await forecast(page);
    assert.match(limited.head, /beyond the five-year forecast/i);
    assert.ok(!/^Lose .* by /.test(limited.head), "cutoff date must not be promised as a completion date");
    assert.match(limited.goal, /before reaching your goal/i);
    assert.ok(!/goal reached/i.test(limited.goal));
    assert.ok(Number.parseFloat(limited.rows.at(-1)[2]) > 80, "the projection stops before reaching the chosen goal weight");
    assert.strictEqual(await page.locator("#table .goal-row").count(), 0, "the cutoff is not styled as goal completion");
    assert.strictEqual(statValue(limited, "After reaching goal"), 2682, "goal maintenance is based on 80 kg, not the much heavier cutoff weight");
    await page.click("#tab-log");
    const progress = await page.textContent("#log-sub");
    assert.match(progress, /beyond the five-year forecast/i);
    assert.ok(!/estimated goal|goal reached/i.test(progress), "log summary does not label the cutoff as the goal date");
  }));

  await test("web forecasts survive reload and recalculate after compatible import", () => withPage(browser, forecastOpts(forecastDoc(), { web: true }), async (page) => {
    assert.strictEqual(await page.title(), "WeightMaxxer");
    await weigh(page, "2026-11-15", 88);
    const revised = await forecast(page);
    await page.reload();
    await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
    assert.deepStrictEqual(await forecast(page), revised, "stored weigh-ins rebuild the same forecast on reload");
    const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("weightmaxer-data")));
    assert.strictEqual(stored.profile.w, "90", "legacy browser storage and original profile are retained");
    assert.strictEqual(stored.log[0].kg, 88);
    await page.click("#tab-settings");
    const imported = { ...stored, log: [{ d: "2026-11-22", kg: 87.5 }, ...stored.log] };
    await page.setInputFiles("#import-file", { name: "weightmaxer-data.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(imported)) });
    await page.waitForFunction(() => /Imported 2 weigh-ins/.test(document.getElementById("data-msg").textContent));
    const afterImport = await forecast(page);
    assert.strictEqual(rowWeight(afterImport), 87.5);
    assert.strictEqual(afterImport.rows[0][1], "Nov 22, 2026");
    assert.strictEqual(afterImport.fields.w, "90");
    assert.strictEqual(afterImport.fields.g, "80");
  }));

  await test("update banner: versions, notes, Read more only when there is more", () => withPage(browser, {}, async (page) => {
    await update(page, { ...base, phase: "available" });
    const text = await page.textContent("#update-banner");
    assert.match(text, /WeightMaxxer 1\.1\.0 is available\. You have 1\.0\.0\./);
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
    await update(page, { ...base, latest: "", phase: "idle", notice: { ok: true, title: "WeightMaxxer was updated to 1.1.0." } });
    assert.match(await page.textContent("#update-banner"), /updated to 1\.1\.0/);
    await update(page, { ...base, latest: "", phase: "idle", notice: { ok: false, title: "The update to 1.1.0 didn't finish.", detail: "WeightMaxxer went back to version 1.0.0." } });
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
        const marks = [...document.querySelectorAll(".bmi-marks .lab")].map((m) => m.getBoundingClientRect());
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

  await test("BMI labels never overlap or leave the card", async () => {
    for (const width of [390, 1280]) {
      const ctx = await browser.newContext({ viewport: { width, height: 900 } });
      const page = await ctx.newPage();
      await page.goto(PAGE);
      await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
      await set(page, "#hft", 5); await set(page, "#hin", 6);
      // Pairs of start/goal weights (lb) covering both ends of the scale, close and far apart,
      // gain and loss. At 5 ft 6 in, 90 lb is BMI 14.5 and 360 lb is BMI 58.
      const pairs = [[176, 169], [100, 104], [95, 92], [104, 120], [340, 330], [360, 350], [300, 360], [150, 151], [150, 150.5], [118, 125], [200, 120], [96, 300]];
      for (const axis of ["weeks", "dates"]) for (const [w, g] of pairs) {
        await page.click(`#view-plan .axis-toggle [data-axis=${axis}]`);
        await set(page, "#w", w); await set(page, "#g", g);
        const bad = await page.evaluate(() => {
          const card = document.getElementById("bmi-marks").closest(".card").getBoundingClientRect();
          const labs = [...document.querySelectorAll(".bmi-marks .lab")].map((l) => l.getBoundingClientRect());
          const out = [];
          for (const r of labs) if (r.left < card.left + 1 || r.right > card.right - 1) out.push("outside card");
          if (labs.length === 2) {
            const [a, b] = labs;
            if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) out.push("overlap");
          }
          const legend = [...document.querySelectorAll(".bmi-legend span")].map((l) => l.getBoundingClientRect());
          for (let i = 0; i < legend.length; i++) for (let j = i + 1; j < legend.length; j++) {
            const a = legend[i], b = legend[j];
            if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) out.push("legend overlap");
          }
          // Chart text (axis labels, start and goal labels) doesn't collide either.
          const texts = [...document.querySelectorAll("#chart text")].map((t) => [t.textContent, t.getBoundingClientRect()]);
          for (let i = 0; i < texts.length; i++) for (let j = i + 1; j < texts.length; j++) {
            const [ta, a] = texts[i], [tb, b] = texts[j];
            if (a.left < b.right - 1 && b.left < a.right - 1 && a.top < b.bottom - 1 && b.top < a.bottom - 1) out.push(`chart "${ta}" overlaps "${tb}"`);
          }
          // Lower value's label comes first.
          const marks = [...document.querySelectorAll(".bmi-marks .lab")];
          const vals = marks.map((m) => parseFloat(m.textContent.split(" ")[1]));
          if (labs.length === 2 && labs[0].top === labs[1].top && (vals[0] - vals[1]) * (labs[0].left - labs[1].left) < 0) out.push("labels out of order");
          return out;
        });
        assert.deepStrictEqual(bad, [], `${width}px, ${axis}, ${w} → ${g} lb: ${bad.join("; ")}`);
      }
      await ctx.close();
    }
  });

  await test("charts show weeks or dates, in both charts, and remember the choice", () => withPage(browser, {}, async (page) => {
    const labels = (id) => page.$$eval(`#${id} text`, (ts) => ts.map((t) => t.textContent));
    assert.ok((await labels("chart")).includes("start"));
    assert.ok((await labels("chart")).some((l) => /^wk \d+$/.test(l)));
    await page.click("#view-plan .axis-toggle [data-axis=dates]");
    const dated = await labels("chart");
    assert.ok(!dated.some((l) => /^wk /.test(l)), dated.join(","));
    assert.ok(dated.some((l) => /^[A-Z][a-z]{2}( \d{1,2}| '?\d{2})?$/.test(l)), dated.join(","));
    await page.click("#tab-log");
    assert.strictEqual(await page.getAttribute("#view-log .axis-toggle [data-axis=dates]", "aria-pressed"), "true");
    assert.ok(!(await labels("log-chart")).some((l) => /^wk /.test(l)));
    await page.waitForFunction(() => window.__saved.some((d) => JSON.parse(d).axis === "dates"));
    // A long plan (2 years) uses month labels.
    await page.click("#tab-plan");
    await set(page, "#w", 260); await set(page, "#g", 160); await set(page, "#rate", 0.75);
    const long = await labels("chart");
    assert.ok(long.filter((l) => /^[A-Z][a-z]{2}/.test(l)).length >= 3, long.join(","));
  }));

  await test("pointing at a chart shows the date and weight there", async () => {
    for (const vp of [{ width: 1280, height: 900 }, { width: 390, height: 844 }]) {
      const ctx = await browser.newContext({ viewport: vp, hasTouch: vp.width < 500 });
      const page = await ctx.newPage();
      await page.addInitScript(() => {
        window.wmxLoad = () => JSON.stringify({ units: "imp", profile: { sex: "f", age: "38", hft: "5", hin: "6", w: "196", g: "165", act: "1.375", rate: "1", start: "2026-09-07" },
          log: [{ d: "2026-10-05", kg: 192.0 * 0.45359237 }] });
        window.wmxSave = () => {}; window.wmxReady = () => {};
      });
      await page.goto(PAGE);
      await page.waitForFunction(() => document.getElementById("chart").innerHTML.length > 0);
      // Point at week 4 (day 28): the readout shows Oct 5, 2026 and the schedule's week-4 weight.
      const pointAt = (id, day) => page.evaluate(([id, day]) => {
        const svg = document.getElementById(id), c = svg._chart;
        svg.scrollIntoView({ block: "center" });
        const r = svg.getBoundingClientRect();
        return { x: r.left + (c.x(day / 7) / c.W) * r.width, y: r.top + r.height / 2 };
      }, [id, day]);
      let pt = await pointAt("chart", 28);
      await page.mouse.move(pt.x, pt.y);
      const tip = page.locator("#view-plan .chart-tip");
      await tip.waitFor({ state: "visible" });
      const text = await tip.textContent();
      assert.match(text, /Oct 5, 2026/, text);
      assert.match(text, /Week 4/, text);
      const week4 = await page.$$eval("#table tbody tr", (rows) => rows.find((r) => r.cells[0].textContent === "4").cells[2].textContent);
      assert.ok(text.includes(week4.trim()), `${text} should include ${week4}`);
      // The readout stays inside the chart, also at the far right.
      const lastDay = await page.evaluate(() => Math.round(document.getElementById("chart")._chart.maxX * 7));
      for (const day of [0, 28, lastDay]) {
        pt = await pointAt("chart", day);
        if (vp.width < 500) await page.touchscreen.tap(pt.x - (day === lastDay ? 1 : 0), pt.y);
        else await page.mouse.move(pt.x - (day === lastDay ? 1 : 0), pt.y);
        await tip.waitFor({ state: "visible" });
        const [b, t] = await page.evaluate(() => [document.querySelector("#view-plan .chart-box").getBoundingClientRect().toJSON(), document.querySelector("#view-plan .chart-tip").getBoundingClientRect().toJSON()]);
        assert.ok(t.left >= b.left - 0.5 && t.right <= b.right + 0.5, `readout outside the chart at day ${day}`);
      }
      // Leaving the chart hides it (mouse).
      await page.mouse.move(2, 2);
      if (vp.width >= 500) assert.ok(await tip.isHidden(), "hidden after leaving");
      // Weigh-ins chart: the weigh-in near the pointer is listed.
      await page.click("#tab-log");
      await page.waitForFunction(() => document.getElementById("log-chart")._chart);
      pt = await pointAt("log-chart", 28);
      if (vp.width < 500) await page.touchscreen.tap(pt.x, pt.y); else await page.mouse.move(pt.x, pt.y);
      const logTip = page.locator("#view-log .chart-tip");
      await logTip.waitFor({ state: "visible" });
      assert.match(await logTip.textContent(), /Weighed192\.0 lb/);
      await ctx.close();
    }
  });

  await test("chart readout works with the keyboard", () => withPage(browser, {}, async (page) => {
    await page.focus("#chart");
    const tip = page.locator("#view-plan .chart-tip");
    await tip.waitFor({ state: "visible" });
    assert.match(await tip.textContent(), /Plan start/);
    await page.keyboard.press("ArrowRight");
    assert.match(await tip.textContent(), /Week 1/);
    await page.keyboard.press("Shift+ArrowRight");
    assert.match(await tip.textContent(), /Week 1\.1/);
    await page.keyboard.press("End");
    const end = await tip.textContent();
    assert.ok(/Plan165\.0 lb/.test(end), end);
    await page.keyboard.press("Escape");
    assert.ok(await tip.isHidden());
  }));

  await test("light and dark mode: top-bar button, Settings choice, remembered", async () => {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: "light" });
    const page = await ctx.newPage();
    await page.goto(process.env.WMX_SITE_URL || PAGE);
    await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
    const bg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
    const light = await bg();
    assert.strictEqual(await page.getAttribute("#theme-btn", "aria-label"), "Switch to dark mode");
    await page.click("#theme-btn");
    assert.strictEqual(await page.getAttribute("html", "data-theme"), "dark");
    assert.notStrictEqual(await bg(), light, "dark background applied");
    assert.strictEqual(await page.getAttribute("#theme-btn", "aria-label"), "Switch to light mode");
    await page.reload();
    await page.waitForFunction(() => document.getElementById("head").textContent.length > 0);
    assert.strictEqual(await page.getAttribute("html", "data-theme"), "dark", "choice remembered");
    await page.click("#tab-settings");
    assert.strictEqual(await page.getAttribute("[data-theme-choice=dark]", "aria-pressed"), "true");
    await page.click("[data-theme-choice=system]");
    assert.strictEqual(await page.getAttribute("html", "data-theme"), null);
    assert.strictEqual(await bg(), light, "system follows the light device setting");
    // System follows the device when it switches to dark.
    await page.emulateMedia({ colorScheme: "dark" });
    assert.notStrictEqual(await bg(), light);
    await page.waitForFunction(() => document.getElementById("theme-btn").getAttribute("aria-label") === "Switch to light mode");
    await page.click("[data-theme-choice=light]");
    assert.strictEqual(await bg(), light, "Light overrides a dark device");
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
    assert.match(dl.suggestedFilename(), /^weightmaxxer-data-\d{4}-\d\d-\d\d\.json$/);
    const exported = JSON.parse(require("fs").readFileSync(await dl.path(), "utf8"));
    assert.strictEqual(exported.log.length, 1);
    exported.log.push({ d: "2026-11-10", kg: 89 });
    await page.setInputFiles("#import-file", { name: "w.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(exported)) });
    await page.waitForFunction(() => /Imported 2 weigh-ins/.test(document.getElementById("data-msg").textContent));
    await page.setInputFiles("#import-file", { name: "bad.json", mimeType: "application/json", buffer: Buffer.from("[1,2]") });
    await page.waitForFunction(() => /isn't a WeightMaxxer export/.test(document.getElementById("data-msg").textContent));
    await page.click("#tab-log");
    assert.strictEqual(await page.locator(".log-row").count(), 2, "bad import leaves data alone");
    assert.deepStrictEqual(errors, []);
    await ctx.close();
  });

  await browser.close();
  if (failures) { console.log(`${failures} failed`); process.exit(1); }
  console.log("all browser tests passed");
})();
