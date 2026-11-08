// The hero's live ticket: one example ticket walked through cairn's real
// phases with cairn's real keys. Rendered on the server for the first frame,
// then re-rendered in the browser at each step. Example data only.
//
// Every line that can change carries a data-id, so the browser can light up
// exactly the lines that changed between two frames.

export type Mark = " " | "~" | "x";
export type Step = "plan" | "build" | "push" | "review" | "shipped";

export interface DemoState {
  step: Step;
  tasks: Mark[];
  sides: boolean[];
  answered: boolean;
  proofs: number;
  ci: number; // checks passed, out of CI_TOTAL; -1 before the PR exists
  review: "none" | "wait" | "ok";
  merged: boolean;
  other: number; // ENG-398's task count, ticking on its own in the list
}

export const STEPS: Array<{ id: Step; label: string }> = [
  { id: "plan", label: "Plan" },
  { id: "build", label: "Build" },
  { id: "push", label: "Push" },
  { id: "review", label: "Review" },
  { id: "shipped", label: "Shipped" },
];

export const CI_TOTAL = 6;
export const TASKS = [
  "Token bucket in middleware/ratelimit.go",
  "Limits per plan, read from billing",
  "Retry-After header on 429",
  "Load test: 500 req/s for 60 s",
];
export const SIDES = ["Webhooks share the router: exempt them", "Status page polls /health every 5 s"];
export const PROOFS = [
  "go test ./middleware/... ✓ 38 tests (14:20)",
  "curl -i /v1/events → 429, Retry-After: 12 (14:31)",
  "k6 run load.js ✓ p99 41 ms at 500 req/s (14:44)",
];
const QUESTIONS: Array<[string, string]> = [
  ["Per API key or per IP?", "Per API key"],
  ["Keep the 429 body as JSON?", "Yes, with retry_after"],
];

export const initial = (): DemoState => ({
  step: "plan",
  tasks: [" ", " ", " ", " "],
  sides: [false, false],
  answered: false,
  proofs: 0,
  ci: -1,
  review: "none",
  merged: false,
  other: 4,
});

// The settled state at the start of each step, for the step rail.
export function stateAt(step: Step): DemoState {
  const s = initial();
  if (step === "plan") return s;
  const built = { ...s, answered: true, tasks: ["x", "x", "x", "x"] as Mark[], sides: [true, true], proofs: 3, other: 5 };
  if (step === "build") return { ...s, step, answered: true, tasks: ["~", " ", " ", " "] };
  if (step === "push") return { ...built, step };
  if (step === "review") return { ...built, step, ci: 2, review: "wait" };
  return { ...built, step, ci: CI_TOTAL, review: "ok", merged: true, other: 6 };
}

// What the other keys do in cairn, said in the footer when you press them.
export const NOTES: Record<string, string> = {
  d: "d opens the ticket's diff in your diff viewer",
  enter: "↵ jumps to the agent's pane in herdr",
  c: "c opens the PR's comments: send one to the agent in a click",
  f: "f hands the failing CI logs to the agent",
  o: "o opens the ticket in Linear",
  e: "e answers the agent's questions",
  n: "n starts a new ticket from a title or a ticket number",
  s: "s opens the settings: projects, trackers, keys",
  t: "t shows every ticket on a board",
};

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const k = (key: string, label: string, primary = false) =>
  `<button type="button" class="k${primary ? " p" : ""}" data-key="${key}"><b>${esc(key === "enter" ? "↵" : key)}</b> ${esc(label)}</button>`;
const line = (html: string, cls = "", id = "") =>
  `<div class="l${cls ? " " + cls : ""}"${id ? ` data-id="${id}"` : ""}>${html}</div>`;
const sec = (title: string, count: string, id = "") =>
  `<div class="sec"${id ? ` data-id="${id}"` : ""}>── ${title} <span class="t-mut">${count}</span></div>`;
// A spinning ◐, drawn by CSS so it turns between repaints too.
const spin = `<span class="t-warn spin" aria-hidden="true"></span>`;

function mark(m: Mark) {
  if (m === "x") return `<span class="t-good">✓</span>`;
  if (m === "~") return spin;
  return `<span class="t-mut">○</span>`;
}

const phases = ["Plan", "Build", "Review", "Shipped"];
function phaseLine(s: DemoState) {
  const at = { plan: 0, build: 1, push: 1, review: 2, shipped: 3 }[s.step];
  const parts = phases.map((p, i) => {
    if (i < at) return `<span class="t-good">${p} ✓</span>`;
    if (i === at) return s.step === "shipped" ? `<span class="t-good">${p} ✓</span>` : `<span class="t-acc">${p} ●</span>`;
    return `<span class="t-dim">${p}</span>`;
  });
  return line(`<span class="t-mut">Phase    </span>${parts.join(' <span class="t-dim">→</span> ')}`, "", "phase");
}

const ciDone = (s: DemoState) => s.ci >= CI_TOTAL;

function card(s: DemoState) {
  const c = (cls: string, head: string, when: string, keys: string) =>
    `<div class="card${cls ? " " + cls : ""}" data-card="${s.step}${s.step === "review" ? (ciDone(s) && s.review === "ok" ? "-ok" : "") : ""}${s.step === "plan" && s.answered ? "-a" : ""}"><div class="row">${head}<span class="t-mut">${when}</span></div><div class="l">${keys}</div></div>`;
  switch (s.step) {
    case "plan":
      return s.answered
        ? c("warn", `<span class="t-warn">⚠ Your call: approve the plan · questions answered</span>`, "for 12 min", k("v", "Approve", true) + k("d", "Diff"))
        : c("warn", `<span class="t-warn">⚠ Your call: approve the plan</span>`, "for 12 min", k("v", "Approve", true) + k("e", "Answer") + k("d", "Diff"));
    case "build":
      return c("", `<span class="t-acc">${spin} ENG-412 is building</span>`, "just now", k("enter", "Go to agent") + k("d", "Diff"));
    case "push":
      return c("warn", `<span class="t-warn">⚠ Your call: ready to push · 3 commits, 7 files</span>`, "now", k("a", "Allow push", true) + k("d", "Diff"));
    case "review":
      return ciDone(s) && s.review === "ok"
        ? c("good", `<span class="t-good">✓ Ready to merge</span>`, "merging stays yours", k("c", "Comments") + k("o", "Ticket"))
        : c("", `<span class="t-acc">${spin} In review · PR #418</span>`, "push allowed for 4f2a91c", k("c", "Comments") + k("f", "Fix CI"));
    case "shipped":
      return c("good", `<span class="t-good">✓ Shipped · worktree removed, plan archived</span>`, "merged", k("r", "Replay", true));
  }
}

export function renderDetail(s: DemoState): string {
  const out: string[] = [];
  out.push(line(`<span class="t-b">ENG-412</span>  <span class="t-b">Rate-limit the public API</span>`));
  const status = { plan: "Todo", build: "In Progress", push: "In Progress", review: "In Review", shipped: "Done" }[s.step];
  out.push(line(`Linear · ${status} · High · 3 pts`, "t-mut", "status"));
  out.push(card(s));
  out.push(phaseLine(s));
  out.push(line("&nbsp;"));

  if (s.step === "review" || s.step === "shipped") {
    const ok = (b: boolean, label: string, extra = "", id = "") =>
      line(`  ${b ? '<span class="t-good">✓</span>' : '<span class="t-mut">○</span>'} ${label}${extra}`, "", id);
    const ready = ciDone(s) && s.review === "ok";
    out.push(`<div class="sec" data-id="ready">── Ready to merge? <span class="${ready ? "t-good" : "t-mut"}">${ready ? "✓ ready" : "not yet"}</span></div>`);
    out.push(ok(true, "tasks"));
    out.push(ok(true, "side effects"));
    out.push(line(`  ${ciDone(s) ? '<span class="t-good">✓</span>' : spin} CI <span class="t-mut">· ${ciDone(s) ? `${CI_TOTAL} checks` : `running ${s.ci}/${CI_TOTAL}`}</span>`, "", "ci"));
    out.push(line(`  ${s.review === "ok" ? '<span class="t-good">✓</span>' : '<span class="t-mut">○</span>'} review <span class="t-mut">· ${s.review === "ok" ? "approved by maya" : "waiting"}</span>`, "", "rev"));
    out.push(ok(true, "no conflicts"));
    out.push(ok(true, "not a draft"));
    out.push(sec("PR #418", s.merged ? '<span class="t-good">merged</span>' : "open", "pr"));
    out.push(line(`  CI ${ciDone(s) ? `<span class="t-good">✓ ${CI_TOTAL}</span>` : `<span class="t-warn">◐ ${s.ci}/${CI_TOTAL}</span>`} <span class="t-mut">·</span> ${s.review === "ok" ? '<span class="t-good">approved</span>' : "review waiting"} <span class="t-mut">· mergeable · 3 comments</span>`, "", "prl"));
    return out.join("");
  }

  if (s.step === "plan") {
    out.push(sec("Open questions", s.answered ? '<span class="t-good">answered</span>' : '<span class="t-warn">2</span>', "qs"));
    QUESTIONS.forEach(([q, a], i) =>
      out.push(line(s.answered ? `  <span class="t-good">✓</span> <span class="t-mut">${esc(q)}</span> ${esc(a)}` : `  <span class="t-warn">?</span> ${esc(q)}`, "", `q${i}`)),
    );
  }
  const done = s.tasks.filter((m) => m === "x").length;
  out.push(sec("Tasks", `${done}/${TASKS.length}`));
  TASKS.forEach((t, i) => out.push(line(`  ${mark(s.tasks[i])} ${s.tasks[i] === "x" ? `<span class="t-mut">${esc(t)}</span>` : esc(t)}`, "", `t${i}`)));
  const sdone = s.sides.filter(Boolean).length;
  out.push(sec("Side effects", `${sdone}/${SIDES.length}`));
  SIDES.forEach((t, i) => out.push(line(`  ${s.sides[i] ? '<span class="t-good">✓</span>' : '<span class="t-mut">○</span>'} ${esc(t)}`, "", `s${i}`)));
  if (s.proofs > 0) {
    out.push(sec("Proofs", String(s.proofs)));
    PROOFS.slice(0, s.proofs).forEach((p, i) => out.push(line(`  <span class="t-mut">•</span> ${esc(p)}`, "", `p${i}`)));
  }
  return out.join("");
}

// The left pane: where ENG-412 sits in the list, among the other tickets.
export function renderList(s: DemoState): string {
  const waiting = s.step === "plan" || s.step === "push";
  const what = s.step === "plan" ? "Approve the plan · 12 min" : "Ready to push · 3 commits";
  const ours = waiting
    ? line(`⚠ <span class="t-b">ENG-412</span> Rate-limit the public API`, "sel", "o1") + line(`   ⚠ ${what}`, "sel t-warn", "o2")
    : "";
  const oursBusy =
    s.step === "build"
      ? line(`  <span class="t-b">ENG-412</span> Rate-limit the public API`, "sel", "o1") +
        line(`     <span class="t-acc">Build</span> <span class="t-mut">${s.tasks.filter((m) => m === "x").length}/4 ·</span> ${spin}`, "sel", "o2")
      : s.step === "review"
        ? line(`  <span class="t-b">ENG-412</span> Rate-limit the public API`, "sel", "o1") +
          line(`     <span class="t-acc">Review</span> <span class="t-mut">· PR #418 ·</span> ${ciDone(s) ? '<span class="t-good">CI ✓</span>' : `<span class="t-warn">CI ${s.ci}/${CI_TOTAL}</span>`}`, "sel", "o2")
        : "";
  const otherDone = s.other >= 6;
  return [
    line("/ filter · ctrl+k search everything", "t-dim"),
    line("Waiting for you", "t-warn"),
    ours,
    line(`<span class="t-warn">⚠</span> WEB-7 Export invoices as PDF`),
    line("   ⚠ Answer 1 question", "t-warn"),
    line("&nbsp;"),
    line("In progress", "t-mut"),
    oursBusy,
    line("  ENG-398 Retry failed webhooks"),
    line(
      otherDone
        ? `     <span class="t-acc">Build</span> <span class="t-mut">6/6 ·</span> <span class="t-good">✓</span>`
        : `     <span class="t-acc">Build</span> <span class="t-mut">${s.other}/6 ·</span> ${spin}`,
      "",
      "other",
    ),
    line("  WEB-5 Dark mode for the editor"),
    line(`     <span class="t-acc">Review</span> <span class="t-mut">· PR #31 ·</span> <span class="t-good">CI ✓</span>`),
    line("&nbsp;"),
    line(s.step === "shipped" ? "› 3 shipped: show" : "› 2 shipped: show", "t-mut", "shipped"),
    line("&nbsp;"),
    line("Not on a ticket", "t-mut"),
    line(`<span class="t-mut">○</span> Bump Go to 1.27`),
  ].join("");
}

// What the line above the window invites you to do, in plain words. {key}
// marks where the keycap goes.
export function hint(s: DemoState): { key: string; text: string } {
  switch (s.step) {
    case "plan":
      return s.answered
        ? { key: "v", text: "Questions answered. Press {key} to approve the plan." }
        : { key: "v", text: "The agent wrote a plan and waits for you. Press {key} to approve it." };
    case "build": return { key: "", text: "The agent builds. Tasks tick off, proofs come in." };
    case "push": return { key: "a", text: "Work done. Nothing leaves your machine: press {key} to allow the push." };
    case "review":
      return { key: "", text: ciDone(s) && s.review === "ok" ? "Tests pass, the review is in. Merging stays yours." : "Pull request open. Tests and the reviewer report back." };
    case "shipped": return { key: "r", text: "Merged, and cairn cleaned up behind it. Press {key} to replay." };
  }
}

export function renderHint(s: DemoState): string {
  const h = hint(s);
  const cap = `<button type="button" class="kbd big" data-key="${h.key}" aria-label="Press ${h.key.toUpperCase()}">${h.key.toUpperCase()}</button>`;
  if (!h.key) return `<span>${h.text}</span>`;
  // The sentence with the key stays on one line: "Press [V] to approve it."
  const [a, b = ""] = h.text.split("{key}");
  const cut = a.lastIndexOf(". ") + 1;
  const lead = a.slice(0, cut).trim();
  return `${lead ? `<span>${lead}</span>` : ""}<span class="hk">${a.slice(cut).trim()} ${cap} ${b.trim()}</span>`;
}
