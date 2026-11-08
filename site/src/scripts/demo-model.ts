// The hero's live ticket: one example chantier walked through cairn's real
// phases with cairn's real keys. Rendered on the server for the first frame,
// then re-rendered in the browser at each step. Example data only.

export type Mark = " " | "~" | "x";

export interface DemoState {
  step: "plan" | "build" | "push" | "review" | "shipped";
  tasks: Mark[];
  sides: boolean[];
  answered: boolean;
  proofs: number;
  ci: "none" | "run" | "ok";
  review: "none" | "wait" | "ok";
  merged: boolean;
}

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

export const initial = (): DemoState => ({
  step: "plan",
  tasks: [" ", " ", " ", " "],
  sides: [false, false],
  answered: false,
  proofs: 0,
  ci: "none",
  review: "none",
  merged: false,
});

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const k = (key: string, label: string, primary = false) =>
  `<button type="button" class="k${primary ? " p" : ""}" data-key="${key}"><b>${esc(key === "enter" ? "↵" : key)}</b> ${esc(label)}</button>`;
const line = (html: string, cls = "") => `<div class="l${cls ? " " + cls : ""}">${html}</div>`;
const sec = (title: string, count: string) => `<div class="sec">── ${title} <span class="t-mut">${count}</span></div>`;

function mark(m: Mark) {
  if (m === "x") return `<span class="t-good">✓</span>`;
  if (m === "~") return `<span class="t-warn">◐</span>`;
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
  return line(`<span class="t-mut">Phase    </span>${parts.join(' <span class="t-dim">→</span> ')}`);
}

function card(s: DemoState) {
  switch (s.step) {
    case "plan":
      return `<div class="card warn"><div class="row"><span class="t-warn">⚠ Your call: approve the plan</span><span class="t-mut">for 12 min</span></div><div class="l">${k("v", "Approve", true)}${k("e", "Answer")}${k("d", "Diff")}</div></div>`;
    case "build":
      return `<div class="card"><div class="row"><span class="t-acc">◐ eng-412 is building</span><span class="t-mut">just now</span></div><div class="l">${k("enter", "Go to agent")}${k("d", "Diff")}</div></div>`;
    case "push":
      return `<div class="card warn"><div class="row"><span class="t-warn">⚠ Your call: ready to push · 3 commits, 7 files</span><span class="t-mut">now</span></div><div class="l">${k("a", "Allow push", true)}${k("d", "Diff")}</div></div>`;
    case "review":
      return s.review === "ok" && s.ci === "ok"
        ? `<div class="card good"><div class="row"><span class="t-good">✓ Ready to merge</span><span class="t-mut">merging stays yours</span></div><div class="l">${k("c", "Comments")}${k("o", "Ticket")}</div></div>`
        : `<div class="card"><div class="row"><span class="t-acc">◐ In review · PR #418</span><span class="t-mut">push allowed for 4f2a91c</span></div><div class="l">${k("c", "Comments")}${k("f", "Fix CI")}</div></div>`;
    case "shipped":
      return `<div class="card good"><div class="row"><span class="t-good">✓ Shipped · worktree removed, plan archived</span><span class="t-mut">merged</span></div><div class="l">${k("r", "Replay", true)}</div></div>`;
  }
}

export function renderDetail(s: DemoState): string {
  const out: string[] = [];
  out.push(line(`<span class="t-b">ENG-412</span>  <span class="t-b">Rate-limit the public API</span>`));
  const status = { plan: "Todo", build: "In Progress", push: "In Progress", review: "In Review", shipped: "Done" }[s.step];
  out.push(line(`Linear · ${status} · High · 3 pts`, "t-mut"));
  out.push(card(s));
  out.push(phaseLine(s));
  out.push(line("&nbsp;"));

  if (s.step === "review" || s.step === "shipped") {
    const ok = (b: boolean, label: string, extra = "") => line(`  ${b ? '<span class="t-good">✓</span>' : '<span class="t-mut">○</span>'} ${label}${extra}`);
    const ready = s.ci === "ok" && s.review === "ok";
    out.push(`<div class="sec">── Ready to merge? <span class="${ready ? "t-good" : "t-mut"}">${ready ? "✓ ready" : "not yet"}</span></div>`);
    out.push(ok(true, "tasks"));
    out.push(ok(true, "side effects"));
    out.push(line(`  ${s.ci === "ok" ? '<span class="t-good">✓</span>' : '<span class="t-warn">◐</span>'} CI${s.ci === "ok" ? ' <span class="t-mut">· 6 checks</span>' : ' <span class="t-mut">· running 4/6</span>'}`));
    out.push(line(`  ${s.review === "ok" ? '<span class="t-good">✓</span>' : '<span class="t-mut">○</span>'} review${s.review === "ok" ? ' <span class="t-mut">· approved by maya</span>' : ' <span class="t-mut">· waiting</span>'}`));
    out.push(ok(true, "no conflicts"));
    out.push(ok(true, "not a draft"));
    out.push(sec("PR #418", s.merged ? '<span class="t-good">merged</span>' : "open"));
    out.push(line(`  CI ${s.ci === "ok" ? '<span class="t-good">✓ 6</span>' : '<span class="t-warn">◐ 4/6</span>'} <span class="t-mut">·</span> ${s.review === "ok" ? '<span class="t-good">approved</span>' : "review waiting"} <span class="t-mut">· mergeable · 3 comments</span>`));
    return out.join("");
  }

  if (s.step === "plan" || !s.answered) {
    out.push(sec("Open questions", '<span class="t-warn">2</span>'));
    out.push(line(`  <span class="t-warn">?</span> Per API key or per IP?`));
    out.push(line(`  <span class="t-warn">?</span> Keep the 429 body as JSON?`));
  }
  const done = s.tasks.filter((m) => m === "x").length;
  out.push(sec("Tasks", `${done}/${TASKS.length}`));
  TASKS.forEach((t, i) => out.push(line(`  ${mark(s.tasks[i])} ${s.tasks[i] === "x" ? `<span class="t-mut">${esc(t)}</span>` : esc(t)}`)));
  const sdone = s.sides.filter(Boolean).length;
  out.push(sec("Side effects", `${sdone}/${SIDES.length}`));
  SIDES.forEach((t, i) => out.push(line(`  ${s.sides[i] ? '<span class="t-good">✓</span>' : '<span class="t-mut">○</span>'} ${esc(t)}`)));
  if (s.proofs > 0) {
    out.push(sec("Proofs", String(s.proofs)));
    PROOFS.slice(0, s.proofs).forEach((p) => out.push(line(`  <span class="t-mut">•</span> ${esc(p)}`)));
  }
  return out.join("");
}

// The left pane: where ENG-412 sits in the list.
export function renderList(s: DemoState): string {
  const waiting = s.step === "plan" || s.step === "push";
  const what = s.step === "plan" ? "Approve the plan · 12 min" : "Ready to push · 3 commits";
  const ours = waiting
    ? line(`⚠ <span class="t-b">ENG-412</span> Rate-limit the public API`, "sel") + line(`   ⚠ ${what}`, "sel t-warn")
    : "";
  const oursBusy =
    s.step === "build"
      ? line(`  <span class="t-b">ENG-412</span> Rate-limit the public API`, "sel") + line(`     <span class="t-acc">Build</span> <span class="t-mut">${s.tasks.filter((m) => m === "x").length}/4 ·</span> <span class="t-warn">◐</span>`, "sel")
      : s.step === "review"
        ? line(`  <span class="t-b">ENG-412</span> Rate-limit the public API`, "sel") + line(`     <span class="t-acc">Review</span> <span class="t-mut">· PR #418 ·</span> ${s.ci === "ok" ? '<span class="t-good">CI ✓</span>' : '<span class="t-warn">CI ◐</span>'}`, "sel")
        : "";
  return [
    line("/ filter · ctrl+k search everything", "t-dim"),
    line("Waiting for you", "t-warn"),
    ours,
    line(`<span class="t-warn">⚠</span> EIDO-7 Export invoices as PDF`),
    line("   ⚠ Answer 1 question", "t-warn"),
    line("&nbsp;"),
    line("In progress", "t-mut"),
    oursBusy,
    line("  ENG-398 Retry failed webhooks"),
    line(`     <span class="t-acc">Build</span> <span class="t-mut">4/6 ·</span> <span class="t-warn">◐</span>`),
    line("  EIDO-5 Dark mode for the editor"),
    line(`     <span class="t-acc">Review</span> <span class="t-mut">· PR #31 ·</span> <span class="t-good">CI ✓</span>`),
    line("&nbsp;"),
    line(s.step === "shipped" ? "› 3 shipped: show" : "› 2 shipped: show", "t-mut"),
    line("&nbsp;"),
    line("Not on a ticket", "t-mut"),
    line(`<span class="t-mut">○</span> Bump Go to 1.27`),
  ].join("");
}

// What the keycaps under the window invite you to do.
export function hint(s: DemoState): { key: string; text: string } {
  switch (s.step) {
    case "plan": return { key: "v", text: "to approve the plan" };
    case "build": return { key: "", text: "The agent builds. Tasks tick, proofs land." };
    case "push": return { key: "a", text: "to allow the push" };
    case "review": return { key: "", text: "CI and review report in. Merging stays yours." };
    case "shipped": return { key: "r", text: "to replay" };
  }
}
