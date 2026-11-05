// Weightmaxer user interface. The Windows host (internal/host) provides the
// wmx* functions; without them (a plain browser) the page still works with
// data kept in memory, which the browser tests rely on.
(() => {
  "use strict";
  const LB = 0.45359237, KCAL_PER_KG = 7700;
  const $ = (id) => document.getElementById(id);
  const host = {
    has: (n) => typeof window[n] === "function",
    call(n, ...a) { return this.has(n) ? Promise.resolve(window[n](...a)) : Promise.resolve(null); },
  };

  // The Windows app provides wmxLoad; anywhere else (GitHub Pages, a local
  // file) this is the web version, which saves to the browser's storage.
  const desktop = host.has("wmxLoad");
  const STORE_KEY = "weightmaxer-data";
  const browserStore = {
    load() { try { return localStorage.getItem(STORE_KEY); } catch (e) { return null; } },
    save(doc) { try { localStorage.setItem(STORE_KEY, doc); return true; } catch (e) { return false; } },
  };

  // ---------- state ----------
  const FIELDS = ["sex", "age", "hft", "hin", "hcm", "w", "g", "act", "floor", "rate", "date", "start", "macro"];
  const MIN_FLOOR = 800, MAX_FLOOR = 4000;
  const standardFloor = (sex) => (sex === "m" ? 1500 : 1200);
  // The lowest daily intake the plan allows: the user's choice (800 or more) or the standard minimum.
  function floorFor(sex) {
    const v = parseFloat($("floor").value);
    return v >= MIN_FLOOR && v <= MAX_FLOOR ? Math.round(v) : standardFloor(sex);
  }
  const data = { schema: 1, units: "imp", mode: "pace", profile: {}, log: [] };
  let view = "plan";
  let showAllLog = false;
  let lastSim = null;
  let update = null;
  let bannerHidden = "";

  // ---------- helpers ----------
  const num = (id) => parseFloat($(id).value);
  const toKg = (v) => (data.units === "imp" ? v * LB : v);
  const fromKg = (v) => (data.units === "imp" ? v / LB : v);
  const wUnit = () => (data.units === "imp" ? "lb" : "kg");
  const fmtW = (kg) => `${fromKg(kg).toFixed(1)} ${wUnit()}`;
  const fmtN = (n) => Math.round(n).toLocaleString();
  const fmtDate = (d) => d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  const addDays = (d, n) => { const x = new Date(d); x.setDate(x.getDate() + n); return x; };
  const today = () => { const d = new Date(); d.setHours(0, 0, 0, 0); return d; };
  const iso = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  const parseISO = (s) => { const m = /^(\d{4})-(\d\d)-(\d\d)$/.exec(s || ""); return m ? new Date(+m[1], m[2] - 1, +m[3]) : null; };
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const heightCm = () => (data.units === "imp" ? (num("hft") * 12 + (num("hin") || 0)) * 2.54 : num("hcm"));
  const bmr = (kg, cm, age, sex) => 10 * kg + 6.25 * cm - 5 * age + (sex === "m" ? 5 : -161);
  const bmiOf = (kg, cm) => kg / Math.pow(cm / 100, 2);
  const bmiClass = (b) => (b < 18.5 ? "underweight" : b < 25 ? "healthy" : b < 30 ? "overweight" : "obese");
  const startDate = () => parseISO($("start").value) || today();

  // ---------- persistence ----------
  let saveTimer = 0;
  function collect() {
    FIELDS.forEach((f) => (data.profile[f] = $(f).value));
  }
  function scheduleSave() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(saveNow, 400);
  }
  function saveNow() {
    clearTimeout(saveTimer);
    collect();
    const doc = JSON.stringify(data);
    if (!desktop) {
      if (!browserStore.save(doc)) console.error("save failed: browser storage is unavailable");
      return Promise.resolve();
    }
    return host.call("wmxSave", doc).catch((e) => console.error("save failed", e));
  }
  async function load() {
    let doc = null;
    try { doc = JSON.parse((desktop ? await host.call("wmxLoad") : browserStore.load()) || "null"); } catch (e) { doc = null; }
    applyDoc(doc);
  }
  function applyDoc(doc) {
    if (doc && typeof doc === "object" && !Array.isArray(doc)) {
      if (doc.units === "met" || doc.units === "imp") setUnits(doc.units, false);
      if (doc.mode === "date" || doc.mode === "pace") setMode(doc.mode);
      if (doc.profile) FIELDS.forEach((f) => { if (doc.profile[f] != null && doc.profile[f] !== "") $(f).value = doc.profile[f]; });
      if (Array.isArray(doc.log)) data.log = doc.log.filter((e) => e && parseISO(e.d) && e.kg > 20 && e.kg < 400);
    }
  }

  // ---------- units / mode ----------
  function setUnits(u, convert = true) {
    if (u === data.units) return;
    let convertedRate = null;
    if (convert) {
      const toImp = u === "imp";
      ["w", "g", "log-w"].forEach((f) => { const v = num(f); if (!isNaN(v)) $(f).value = (toImp ? v / LB : v * LB).toFixed(1); });
      if (toImp) {
        const tin = num("hcm") / 2.54;
        $("hft").value = Math.floor(tin / 12);
        $("hin").value = Math.round((tin % 12) * 2) / 2;
      } else {
        $("hcm").value = Math.round((num("hft") * 12 + (num("hin") || 0)) * 2.54);
      }
      convertedRate = toImp ? Math.min(2, Math.round((num("rate") / LB) * 4) / 4) : Math.max(0.1, Math.round(num("rate") * LB * 20) / 20);
    }
    data.units = u;
    $("u-imp").setAttribute("aria-pressed", u === "imp");
    $("u-met").setAttribute("aria-pressed", u === "met");
    $("h-imp").hidden = u !== "imp";
    $("h-met").hidden = u !== "met";
    document.querySelectorAll(".wu").forEach((e) => (e.textContent = wUnit()));
    const r = $("rate");
    if (u === "imp") { r.min = 0.25; r.max = 2; r.step = 0.25; } else { r.min = 0.1; r.max = 1; r.step = 0.05; }
    // Set the value after the new range so the slider doesn't snap it to the old steps.
    if (convertedRate != null) r.value = convertedRate;
  }
  function setMode(m) {
    data.mode = m;
    $("m-pace").setAttribute("aria-pressed", m === "pace");
    $("m-date").setAttribute("aria-pressed", m === "date");
    $("pace-f").hidden = m !== "pace";
    $("date-f").hidden = m !== "date";
  }

  // ---------- simulation ----------
  // Each day: intake = maintenance at the current weight -/+ the planned
  // difference, never below the minimum for losing. Weight changes by the
  // daily difference / 7,700 kcal per kg.
  function simulate(p) {
    const gaining = p.goal > p.kg;
    const floor = p.floor;
    let kg = p.kg, day = 0, floored = false;
    const weeks = [];
    while ((gaining ? kg < p.goal : kg > p.goal) && day < 365 * 5) {
      const tdee = bmr(kg, p.cm, p.age, p.sex) * p.act;
      let intake = gaining ? tdee + p.daily : tdee - p.daily;
      if (!gaining && intake < floor) { intake = floor; floored = true; }
      const change = (intake - tdee) / KCAL_PER_KG;
      if (day % 7 === 0) weeks.push({ week: day / 7, kg, intake, tdee });
      if (Math.abs(change) < 0.0001) return { stuck: true, weeks, floor };
      kg += change; day++;
    }
    const endKg = gaining ? Math.min(kg, p.goal) : Math.max(kg, p.goal);
    const tdeeEnd = bmr(endKg, p.cm, p.age, p.sex) * p.act;
    weeks.push({ week: day / 7, kg: endKg, intake: null, tdee: tdeeEnd, end: true });
    return { days: day, weeks, floored, floor, tdeeEnd, gaining, done: day < 365 * 5 };
  }

  // ---------- plan view ----------
  function clearPlan(head, sub) {
    $("head").textContent = head;
    $("subhead").textContent = sub || "";
    ["stats", "alerts", "table"].forEach((i) => ($(i).innerHTML = ""));
    $("chart").innerHTML = "";
    lastSim = null;
    $("macro-card").hidden = true;
  }

  function renderPlan() {
    const sex = $("sex").value, age = num("age"), cm = heightCm(), act = num("act");
    const kg = toKg(num("w")), goal = toKg(num("g"));
    const gaining = goal > kg;
    $("rate-verb").textContent = gaining ? "Weekly gain" : "Weekly loss";
    $("floor-f").hidden = gaining;
    $("floor").placeholder = String(standardFloor(sex));
    const fv = $("floor").value.trim(), fn = parseFloat(fv);
    $("floor-hint").textContent = fv === "" ? `Leave empty to use the standard minimum of ${fmtN(standardFloor(sex))} kcal.`
      : !(fn >= MIN_FLOOR && fn <= MAX_FLOOR) ? `Enter ${MIN_FLOOR}–${MAX_FLOOR} kcal. Using the standard ${fmtN(standardFloor(sex))} kcal for now.`
      : fn < standardFloor(sex) ? `Below the standard ${fmtN(standardFloor(sex))} kcal. Get medical advice before eating this little.`
      : `The plan won't go below ${fmtN(fn)} kcal a day.`;
    $("rate-out").textContent = `${(+$("rate").value).toFixed(2).replace(/0$/, "")} ${wUnit()}/week`;
    $("rate-hint").textContent = gaining
      ? "Gaining 0.25–0.5% of body weight per week keeps most of the gain lean."
      : "0.5–1% of body weight per week is the usual safe range.";
    $("schedule-hint").textContent = gaining
      ? "Your body burns more as you get heavier, so the daily target rises over time. Recheck it every few weeks."
      : "Your body burns less as you get lighter, so the daily target drops over time. Recheck it every few weeks.";

    const bad = !(age >= 15 && age <= 100) || !(cm > 100 && cm < 250) || !(kg > 30 && kg < 350) || !(goal > 30 && goal < 350);
    if (bad) return clearPlan("Fill in every field to see your plan.", "Age 15–100, a realistic height, and both weights are needed.");
    const maint = bmr(kg, cm, age, sex) * act;
    const bmiNow = bmiOf(kg, cm), bmiGoal = bmiOf(goal, cm);
    renderBmi(bmiNow, bmiGoal);
    if (Math.abs(goal - kg) < 0.05) {
      clearPlan("Your goal matches your starting weight.", `To stay at ${fmtW(kg)}, eat about ${fmtN(maint)} kcal a day.`);
      $("stats").innerHTML = stat("Maintenance", fmtN(maint), "kcal / day") + stat("BMI", bmiNow.toFixed(1), bmiClass(bmiNow));
      return;
    }

    let rateKg;
    if (data.mode === "pace") {
      rateKg = toKg(num("rate"));
    } else {
      const d = parseISO($("date").value);
      const weeks = d ? (d - startDate()) / (7 * 864e5) : NaN;
      if (!(weeks > 0)) return clearPlan("Pick a target date after the plan starts.");
      rateKg = Math.abs(kg - goal) / weeks;
    }
    const floor = floorFor(sex);
    const sim = simulate({ kg, goal, cm, age, sex, act, floor, daily: (rateKg * KCAL_PER_KG) / 7 });
    const alerts = [];
    if (!gaining && floor < standardFloor(sex)) alerts.push(["bad", `Your lowest daily calories (${fmtN(floor)} kcal) is below the usual minimum of ${fmtN(standardFloor(sex))} kcal. Eating this little can leave you short of nutrients and muscle; do it only with a doctor's supervision.`]);
    if (!gaining && rateKg > kg * 0.01 * 1.0001) alerts.push(["bad", `Losing ${fmtW(rateKg)} a week is more than 1% of your body weight (${fmtW(kg * 0.01)}). Faster loss tends to cost muscle and is hard to keep up.`]);
    if (gaining && rateKg > kg * 0.005 * 1.0001) alerts.push(["", `Gaining ${fmtW(rateKg)} a week is more than 0.5% of your body weight. More of it is likely to be fat rather than muscle.`]);
    if (gaining && bmiGoal >= 30) alerts.push(["bad", `A goal of ${fmtW(goal)} gives a BMI of ${bmiGoal.toFixed(1)}, which is in the obese range for your height. Consider a lower goal.`]);
    if (bmiGoal < 18.5) alerts.push(["bad", `A goal of ${fmtW(goal)} gives a BMI of ${bmiGoal.toFixed(1)}, which is underweight. A healthy BMI for your height starts at ${fmtW(18.5 * Math.pow(cm / 100, 2))}.`]);
    if (sim.floored) alerts.push(["", `To keep eating at least ${fmtN(sim.floor)} kcal a day, the plan slows down, so it takes longer than the pace you picked.`]);
    if (sim.stuck) {
      clearPlan("This goal can't be reached by diet alone at your activity level.", `Your maintenance is about ${fmtN(maint)} kcal, close to your lowest daily calories of ${fmtN(sim.floor)} kcal. More activity, or a lower minimum, would make room for a deficit.`);
      $("stats").innerHTML = stat("Maintenance", fmtN(maint), "kcal / day");
      renderAlerts(alerts);
      return;
    }
    if (!sim.done) alerts.push(["bad", "At this pace the goal is more than five years away. Pick a faster pace or a closer goal."]);

    lastSim = { sim, kg, goal, start: startDate() };
    const endDate = addDays(startDate(), sim.days);
    const weeksTotal = sim.days / 7;
    const change = Math.abs(kg - goal);
    const first = sim.weeks[0];
    $("head").innerHTML = `${gaining ? "Gain" : "Lose"} <em>${fmtW(change)}</em> by <em>${fmtDate(endDate)}</em>`;
    $("subhead").textContent = `About ${weeksTotal < 9 ? Math.round(weeksTotal) + " weeks" : (weeksTotal / 4.345).toFixed(1) + " months"} at an average of ${fmtW(change / weeksTotal)} per week.`;
    $("stats").innerHTML =
      stat("Eat now", fmtN(first.intake), "kcal / day") +
      stat("Maintenance", fmtN(maint), "kcal / day at the start") +
      stat(gaining ? "Daily surplus" : "Daily deficit", fmtN(Math.abs(first.tdee - first.intake)), "kcal") +
      stat("After reaching goal", fmtN(sim.tdeeEnd), "kcal / day to hold");
    if (!alerts.length) alerts.push(["ok", gaining ? "This pace is within the usual range for a lean gain." : "This pace is within the commonly recommended range of up to 1% of body weight per week."]);
    renderAlerts(alerts);
    drawChart($("chart"), sim.weeks.map((w) => ({ x: w.week, y: fromKg(w.kg) })), [], fromKg(kg), fromKg(goal), sim.weeks[sim.weeks.length - 1].week);
    renderTable(sim);
    renderMacros(first.intake, kg, goal, gaining, sex);
  }

  // Macro targets for a daily intake. Protein follows body weight (goal weight
  // when losing, which avoids over-counting fat mass), fat is a share of
  // calories with a floor of 0.6 g/kg, and carbohydrate fills the rest.
  const MACRO_STYLES = { balanced: { fat: 0.27, label: "Balanced" }, lowcarb: { fat: 0.4, label: "Lower carb" }, lowfat: { fat: 0.2, label: "Lower fat" } };
  function macrosFor(kcal, kg, goal, gaining, style) {
    const ref = gaining ? kg : Math.min(kg, goal);
    const perKg = gaining ? 1.8 : 2.0;
    let protein = ref * perKg;
    const fatShare = (MACRO_STYLES[style] || MACRO_STYLES.balanced).fat;
    let fat = Math.max((kcal * fatShare) / 9, 0.6 * kg);
    // Keep protein and fat inside the calorie budget on very low intakes.
    if (protein * 4 + fat * 9 > kcal * 0.9) {
      fat = Math.max(0.6 * kg, Math.min(fat, (kcal * 0.9 - protein * 4) / 9));
      if (protein * 4 + fat * 9 > kcal * 0.9) protein = Math.max(1.2 * ref, (kcal * 0.9 - fat * 9) / 4);
    }
    const carbs = Math.max(0, (kcal - protein * 4 - fat * 9) / 4);
    return { protein, fat, carbs, fibre: (kcal / 1000) * 14, kcal, perKg: protein / ref };
  }
  function renderMacros(kcal, kg, goal, gaining, sex) {
    const m = macrosFor(kcal, kg, goal, gaining, $("macro").value);
    const parts = [["protein", "Protein", m.protein, 4], ["fat", "Fat", m.fat, 9], ["carbs", "Carbs", m.carbs, 4]];
    const total = parts.reduce((t, p) => t + p[2] * p[3], 0) || 1;
    $("macro-sub").textContent = `For your current target of ${fmtN(kcal)} kcal a day. Recalculate as the target changes.`;
    $("macro-bar").innerHTML = parts.map(([c, , g, k]) => `<span class="m-${c}" style="width:${((g * k) / total) * 100}%"></span>`).join("");
    $("macros").innerHTML = parts.map(([c, name, g, k]) =>
      `<div class="macro"><span class="lbl"><span class="dot m-${c}"></span>${name}</span><span class="g">${fmtN(g)} g</span><span class="k">${fmtN(g * k)} kcal · ${Math.round(((g * k) / total) * 100)}%</span></div>`).join("");
    let note = `Protein is about ${m.perKg.toFixed(1)} g per kg of ${gaining || kg <= goal ? "body" : "goal"} weight. Aim for at least ${fmtN(m.fibre)} g of fibre.`;
    if (m.protein * 4 + m.fat * 9 > kcal + 1) note += ` At ${fmtN(kcal)} kcal, minimum protein and fat already use up the day's calories, so these targets go slightly over.`;
    else if (m.carbs < 50) note += " Carbohydrate is very low at this intake; that's fine if it suits you, but add vegetables for fibre.";
    $("macro-note").textContent = note;
    $("macro-card").hidden = false;
  }
  const stat = (k, v, sub) => `<div class="stat"><span class="lbl">${k}</span><span class="v">${v}</span><span class="k">${sub}</span></div>`;
  const renderAlerts = (list) => ($("alerts").innerHTML = list.map(([c, t]) => `<div class="alert ${c}">${t}</div>`).join(""));

  function renderTable(sim) {
    const w = sim.weeks;
    const step = w.length > 40 ? 8 : w.length > 16 ? 4 : w.length > 8 ? 2 : 1;
    const rows = w.filter((r, i) => i % step === 0 && !r.end);
    const last = w[w.length - 1];
    const s = startDate();
    let h = `<thead><tr><th>Week</th><th>Date</th><th class="n">Weight</th><th class="n">Eat</th><th class="n">Burn</th></tr></thead><tbody>`;
    rows.forEach((r) => {
      h += `<tr><td>${r.week}</td><td>${fmtDate(addDays(s, r.week * 7))}</td><td class="n">${fmtW(r.kg)}</td><td class="n">${fmtN(r.intake)} kcal</td><td class="n">${fmtN(r.tdee)} kcal</td></tr>`;
    });
    h += `<tr><td>${last.week.toFixed(1)}</td><td>${fmtDate(addDays(s, Math.round(last.week * 7)))}</td><td class="n">${fmtW(last.kg)}</td><td class="n" colspan="2">Goal reached · maintain at ${fmtN(last.tdee)} kcal</td></tr>`;
    $("table").innerHTML = h + "</tbody>";
  }

  function niceStep(raw) {
    const p = Math.pow(10, Math.floor(Math.log10(raw)));
    const n = raw / p;
    return (n < 1.5 ? 1 : n < 3 ? 2 : n < 7 ? 5 : 10) * p;
  }

  // drawChart plots the plan line (and optional weigh-in points) on one scale.
  function drawChart(svg, plan, actual, startY, goalY, maxX) {
    const W = svg.clientWidth || 700, H = 260;
    const m = { l: 52, r: 18, t: 16, b: 30 };
    svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
    const ys = [startY, goalY, ...actual.map((p) => p.y)];
    const hi = Math.max(...ys), lo = Math.min(...ys);
    const pad = Math.max(1, (hi - lo) * 0.12);
    const yMax = hi + pad, yMin = lo - pad;
    maxX = Math.max(maxX, ...actual.map((p) => p.x), 1);
    const x = (v) => m.l + (v / maxX) * (W - m.l - m.r);
    const y = (v) => m.t + (1 - (v - yMin) / (yMax - yMin)) * (H - m.t - m.b);
    const yStep = niceStep((yMax - yMin) / 4);
    const xStep = Math.max(1, niceStep(maxX / Math.max(2, Math.floor((W - m.l - m.r) / 80))));
    let s = "";
    for (let v = Math.ceil(yMin / yStep) * yStep; v <= yMax; v += yStep) {
      s += `<line x1="${m.l}" x2="${W - m.r}" y1="${y(v)}" y2="${y(v)}" stroke="var(--line)"/>`;
      s += `<text x="${m.l - 8}" y="${y(v) + 4}" text-anchor="end">${+v.toFixed(1)}</text>`;
    }
    for (let v = 0; v <= maxX + 1e-9; v += xStep) s += `<text x="${x(v)}" y="${H - 8}" text-anchor="middle">${v === 0 ? "start" : "wk " + v}</text>`;
    s += `<line x1="${m.l}" x2="${W - m.r}" y1="${y(goalY)}" y2="${y(goalY)}" stroke="var(--good)" stroke-dasharray="4 4"/>`;
    s += `<text x="${m.l + 8}" y="${y(goalY) + (goalY < startY ? 16 : -6)}" style="fill:var(--good)">goal ${goalY.toFixed(1)} ${wUnit()}</text>`;
    if (plan.length) {
      const line = plan.map((p, i) => `${i ? "L" : "M"}${x(p.x).toFixed(1)},${y(p.y).toFixed(1)}`).join("");
      const base = goalY < startY ? H - m.b : m.t;
      s += `<path d="${line}L${x(plan[plan.length - 1].x)},${base}L${x(0)},${base}Z" fill="var(--accent)" fill-opacity="${actual.length ? 0.06 : 0.12}"/>`;
      s += `<path d="${line}" fill="none" stroke="var(--accent)" stroke-width="2.5" stroke-opacity="${actual.length ? 0.55 : 1}" stroke-linejoin="round"/>`;
      s += `<circle cx="${x(0)}" cy="${y(startY)}" r="4" fill="var(--accent)"/>`;
      s += `<text x="${x(0) + 8}" y="${y(startY) + (goalY < startY ? -6 : 14)}" style="fill:var(--fg)">${startY.toFixed(1)} ${wUnit()}</text>`;
    }
    if (actual.length) {
      const line = actual.map((p, i) => `${i ? "L" : "M"}${x(p.x).toFixed(1)},${y(p.y).toFixed(1)}`).join("");
      s += `<path d="${line}" fill="none" stroke="var(--fg)" stroke-width="2" stroke-linejoin="round"/>`;
      actual.forEach((p) => (s += `<circle cx="${x(p.x)}" cy="${y(p.y)}" r="3.5" fill="var(--surface)" stroke="var(--fg)" stroke-width="2"/>`));
    }
    svg.innerHTML = s;
  }

  function renderBmi(now, goal) {
    const min = 15, max = 40;
    const pos = (b) => Math.min(100, Math.max(0, ((b - min) / (max - min)) * 100));
    const bands = [[15, 18.5, "var(--warn)"], [18.5, 25, "var(--good)"], [25, 30, "var(--warn)"], [30, 40, "var(--bad)"]];
    $("bmi-bar").innerHTML = bands.map(([a, b, c]) => `<span style="width:${pos(b) - pos(a)}%;background:${c}"></span>`).join("");
    $("bmi-marks").innerHTML =
      `<div class="bmi-mark" style="left:${pos(now)}%">Start ${now.toFixed(1)}</div>` +
      `<div class="bmi-mark" style="left:${pos(goal)}%;color:var(--accent)">Goal ${goal.toFixed(1)}</div>`;
  }

  // ---------- weigh-ins ----------
  function renderLog() {
    const entries = [...data.log].sort((a, b) => (a.d < b.d ? -1 : 1));
    const s = startDate();
    const plan = lastSim ? lastSim.sim.weeks.map((w) => ({ x: w.week, y: fromKg(w.kg) })) : [];
    const actual = entries.map((e) => ({ x: Math.max(0, (parseISO(e.d) - s) / (7 * 864e5)), y: fromKg(e.kg) }));
    if (!entries.length) {
      $("log-head").textContent = "No weigh-ins yet";
      $("log-sub").textContent = "Add your first one on the left to compare your progress with the plan.";
    } else {
      const latest = entries[entries.length - 1];
      const startKg = toKg(num("w"));
      const diff = latest.kg - startKg;
      $("log-head").innerHTML = `${fmtW(latest.kg)} <span class="sub">on ${fmtDate(parseISO(latest.d))}</span>`;
      let sub = `${diff <= 0 ? "Down" : "Up"} ${fmtW(Math.abs(diff))} since the plan started.`;
      if (lastSim) {
        const wk = (parseISO(latest.d) - s) / (7 * 864e5);
        const pw = lastSim.sim.weeks.reduce((best, w) => (Math.abs(w.week - wk) < Math.abs(best.week - wk) ? w : best), lastSim.sim.weeks[0]);
        const ahead = lastSim.sim.gaining ? latest.kg - pw.kg : pw.kg - latest.kg;
        sub += Math.abs(ahead) < 0.25 ? " Right on plan." : ahead > 0 ? ` ${fmtW(ahead)} ahead of plan.` : ` ${fmtW(-ahead)} behind plan.`;
      }
      $("log-sub").textContent = sub;
    }
    const goalY = fromKg(toKg(num("g"))), startY = fromKg(toKg(num("w")));
    const maxX = plan.length ? plan[plan.length - 1].x : 12;
    if (isFinite(goalY) && isFinite(startY)) drawChart($("log-chart"), plan, actual, startY, goalY, maxX);

    const rows = [...entries].reverse();
    const visible = showAllLog ? rows : rows.slice(0, 10);
    $("log-list").innerHTML = rows.length
      ? visible.map((e) => {
          const i = entries.indexOf(e);
          const prev = entries[i - 1];
          const delta = prev ? e.kg - prev.kg : null;
          return `<div class="log-row"><span>${fmtDate(parseISO(e.d))}</span><span>${fmtW(e.kg)}</span><span class="delta">${delta == null ? "" : (delta > 0 ? "+" : "−") + Math.abs(fromKg(delta)).toFixed(1)}</span><button type="button" class="del" data-d="${esc(e.d)}" aria-label="Delete weigh-in on ${esc(e.d)}">Delete</button></div>`;
        }).join("")
      : `<div class="empty">Your weigh-ins will appear here.</div>`;
    $("log-more").hidden = rows.length <= 10;
    $("log-more").textContent = showAllLog ? "Show fewer" : `Show all ${rows.length}`;
  }

  function addWeighIn(ev) {
    ev.preventDefault();
    const d = $("log-date").value, v = num("log-w");
    const msg = $("log-msg");
    if (!parseISO(d)) { msg.textContent = "Choose a date."; return; }
    if (!(toKg(v) > 20 && toKg(v) < 400)) { msg.textContent = "Enter your weight."; return; }
    const existing = data.log.find((e) => e.d === d);
    if (existing) existing.kg = toKg(v); else data.log.push({ d, kg: toKg(v) });
    msg.textContent = existing ? `Updated ${fmtDate(parseISO(d))}.` : `Added ${fmtDate(parseISO(d))}.`;
    $("log-w").value = "";
    saveNow();
    renderLog();
  }

  // Work that would be lost if Weightmaxer restarted now.
  function unsavedWork() {
    return $("log-w").value.trim() !== "" ? "You started a weigh-in that isn't added yet. Add it or clear it first." : "";
  }

  // ---------- export / import (web version) ----------
  function exportData() {
    collect();
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `weightmaxer-data-${iso(today())}.json`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    $("data-msg").textContent = "Exported.";
  }
  function importData(file) {
    const reader = new FileReader();
    reader.onload = () => {
      let doc = null;
      try { doc = JSON.parse(reader.result); } catch (e) { doc = null; }
      if (!doc || typeof doc !== "object" || Array.isArray(doc) || !(doc.profile || doc.log)) {
        $("data-msg").textContent = "That file isn't a Weightmaxer export.";
        return;
      }
      data.log = [];
      applyDoc(doc);
      saveNow();
      render();
      $("data-msg").textContent = `Imported ${data.log.length} weigh-in${data.log.length === 1 ? "" : "s"}.`;
    };
    reader.onerror = () => ($("data-msg").textContent = "Couldn't read that file.");
    reader.readAsText(file);
  }

  // ---------- update banner and settings ----------
  function ago(iso) {
    if (!iso) return "";
    const s = Math.max(0, (Date.now() - new Date(iso)) / 1000);
    if (s < 60) return "just now";
    if (s < 3600) return `${Math.round(s / 60)} min ago`;
    if (s < 86400) return `${Math.round(s / 3600)} h ago`;
    return new Date(iso).toLocaleDateString();
  }

  function statusText(u) {
    const c = u.check || {};
    if (u.dev) return "Updates are off in development builds.";
    if (c.checking) return "Checking for updates…";
    switch (c.kind) {
      case "up-to-date": return `Up to date · checked ${ago(c.at)}`;
      case "available": return `Version ${esc(u.latest)} is available`;
      case "no-release": return `No release has been published yet · checked ${ago(c.at)}`;
      case "offline": case "rate-limited": case "error":
        return `Couldn't check for updates. ${esc(c.message || "")}${c.lastSuccess ? ` Last successful check ${ago(c.lastSuccess)}.` : ""}`;
      default: return "Not checked yet";
    }
  }

  function renderUpdate() {
    const u = update;
    if (!u) return;
    $("set-version").textContent = u.current;
    $("set-status").innerHTML = statusText(u);
    $("check-now").disabled = !!u.dev || (u.check && u.check.checking);
    const b = $("update-banner");
    let html = "", cls = "banner";
    const notes = (u.notes || []).length ? `<ul>${u.notes.map((n) => `<li>${esc(n)}</li>`).join("")}</ul>` : "";
    const more = u.more ? ` <button type="button" class="linkbtn" data-act="notes">Read more</button>` : "";
    switch (u.phase) {
      case "available":
        html = `<div><span class="b-title">Weightmaxer ${esc(u.latest)} is available.</span> <span class="b-sub">You have ${esc(u.current)}.</span></div>` +
          (notes ? `<div class="b-notes"><strong>What's new</strong>${notes}${more}</div>` : more) +
          `<div class="b-row"><button type="button" class="btn primary" data-act="download">Download</button><button type="button" class="btn" data-act="later">Later</button></div>`;
        break;
      case "downloading": {
        const p = u.progress && u.progress.total ? Math.floor((u.progress.done / u.progress.total) * 100) : 0;
        html = `<div class="b-title">Downloading Weightmaxer ${esc(u.latest)}… ${p}%</div><div class="progress" role="progressbar" aria-valuenow="${p}" aria-valuemin="0" aria-valuemax="100"><span style="width:${p}%"></span></div>`;
        break;
      }
      case "ready":
        html = `<div class="b-title">Weightmaxer ${esc(u.latest)} is ready to install</div>` +
          `<div class="b-sub">Weightmaxer will close and reopen. Your plan and weigh-ins are already saved.</div>` +
          `<div class="b-row"><button type="button" class="btn primary" data-act="install">Install and restart</button><button type="button" class="btn" data-act="later">Later</button><span class="hint" id="install-msg" role="status"></span></div>`;
        break;
      case "installing":
        html = `<div class="b-title">Installing Weightmaxer ${esc(u.latest)}…</div><div class="b-sub">Weightmaxer will reopen in a moment.</div>`;
        break;
      case "error":
        cls += " bad";
        html = `<div class="b-title">The update didn't finish</div><div class="b-sub">${esc(u.error || "")}</div>` +
          `<div class="b-row">${u.latest ? `<button type="button" class="btn" data-act="download">Try again</button>` : ""}<button type="button" class="btn" data-act="later">Dismiss</button></div>`;
        break;
    }
    if (!html && u.notice) {
      cls += u.notice.ok ? " good" : " bad";
      html = `<div class="b-title">${esc(u.notice.title)}</div>${u.notice.detail ? `<div class="b-sub">${esc(u.notice.detail)}</div>` : ""}` +
        `<div class="b-row"><button type="button" class="btn" data-act="dismiss-notice">OK</button></div>`;
    }
    const key = `${u.phase}|${u.latest}`;
    const hide = !html || (bannerHidden === key && u.phase !== "downloading" && u.phase !== "installing");
    b.className = cls;
    b.hidden = hide;
    if (!hide) b.innerHTML = html;
  }

  async function onBannerClick(ev) {
    const act = ev.target && ev.target.dataset && ev.target.dataset.act;
    if (!act) return;
    if (act === "later") { bannerHidden = `${update.phase}|${update.latest}`; renderUpdate(); }
    if (act === "dismiss-notice") { update.notice = null; renderUpdate(); host.call("wmxDismissNotice"); }
    if (act === "notes") host.call("wmxOpenNotes");
    if (act === "download") host.call("wmxDownload");
    if (act === "install") {
      const why = unsavedWork();
      if (why) { $("install-msg").textContent = why; return; }
      await saveNow();
      const err = await host.call("wmxInstall");
      if (err && $("install-msg")) $("install-msg").textContent = err;
    }
  }

  // Called by the host whenever the update state changes.
  window.wmxUpdate = (u) => { update = u; renderUpdate(); };

  // ---------- navigation ----------
  function show(v) {
    view = v;
    ["plan", "log", "settings"].forEach((n) => {
      $("view-" + n).hidden = n !== v;
      $("tab-" + n).setAttribute("aria-selected", n === v);
    });
    if (v === "log") renderLog();
    if (v === "plan") renderPlan();
    try { localStorage.setItem("wmx-view", v); } catch (e) {}
  }

  function render() {
    renderPlan();
    if (view === "log") renderLog();
  }

  // ---------- startup ----------
  async function start() {
    $("start").value = iso(today());
    $("date").value = iso(addDays(today(), 7 * 26));
    $("log-date").value = iso(today());
    await load();
    $("u-imp").onclick = () => { setUnits("imp"); render(); scheduleSave(); };
    $("u-met").onclick = () => { setUnits("met"); render(); scheduleSave(); };
    $("m-pace").onclick = () => { setMode("pace"); render(); scheduleSave(); };
    $("m-date").onclick = () => { setMode("date"); render(); scheduleSave(); };
    $("form").addEventListener("input", () => { render(); scheduleSave(); });
    $("form").addEventListener("submit", (e) => e.preventDefault());
    $("macro").addEventListener("change", () => { render(); scheduleSave(); });
    $("log-form").addEventListener("submit", addWeighIn);
    $("log-list").addEventListener("click", (e) => {
      const d = e.target.dataset && e.target.dataset.d;
      if (!d) return;
      data.log = data.log.filter((x) => x.d !== d);
      saveNow();
      renderLog();
    });
    $("log-more").onclick = () => { showAllLog = !showAllLog; renderLog(); };
    document.querySelectorAll(".tabs button").forEach((b) => (b.onclick = () => show(b.dataset.view)));
    $("update-banner").addEventListener("click", onBannerClick);
    $("check-now").onclick = async () => {
      const r = await host.call("wmxCheckNow");
      $("check-msg").textContent = r && r.message ? r.message : "";
    };
    $("open-folder").onclick = () => host.call("wmxOpenDataFolder");
    let rt = 0;
    window.addEventListener("resize", () => { clearTimeout(rt); rt = setTimeout(render, 120); });
    setInterval(() => update && renderUpdate(), 30000); // keep "checked N min ago" current

    if (!desktop) {
      document.querySelectorAll(".desktop-only").forEach((e) => (e.hidden = true));
      document.querySelectorAll(".web-only").forEach((e) => (e.hidden = false));
      $("export-data").onclick = exportData;
      $("import-file").onchange = (e) => { if (e.target.files[0]) importData(e.target.files[0]); e.target.value = ""; };
      // Save the starting form so a first visit is remembered too.
      if (!browserStore.load()) saveNow();
    }
    const info = (await host.call("wmxInfo")) || { version: "web", dataFolder: "This browser, on this device only" };
    $("set-version").textContent = info.version;
    $("set-folder").textContent = info.dataFolder;
    let saved = "plan";
    try { saved = localStorage.getItem("wmx-view") || "plan"; } catch (e) {}
    renderPlan(); // the weigh-in chart needs the plan even when another tab opens first
    show(["plan", "log", "settings"].includes(saved) ? saved : "plan");
    window.wmxUnsavedWork = unsavedWork;
    host.call("wmxReady");
  }
  start();
})();
