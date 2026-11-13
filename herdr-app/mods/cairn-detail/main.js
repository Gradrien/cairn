// cairn: the chantier, in the Herdr app's right panel (next to Files and
// Changes). Written by cairn's install.sh --herdr-app; run it again to update it.
//
// cairn serve is the engine: it writes what to show to detail.json (the
// chantier's card and sections, the screen it is on and that screen's buttons,
// its confirmations) and runs what this panel asks with `cairn do`, the same
// code as the TUI's keys and clicks. Every key the TUI knows works here once
// the panel has the keyboard (a click on it).
import { View, div } from "gpui-kit";
import { v_flex, h_flex, InputState, Input, TextView, Scrollbar } from "gpui-base";
import { agents, run, exec, readTextFile, watchFolder, nextCommand, theme, settings } from "herdr";

const CAIRN = "@CAIRN@";
const PATH = "@PATH@";
const DETAIL = "@DETAIL@";
const KEYS = { escape: "esc", enter: "enter", up: "up", down: "down", left: "left", right: "right", pageup: "pgup", pagedown: "pgdown", space: " ", tab: "tab" };

const folder = (path) => path.slice(0, path.lastIndexOf("/"));
const cairn = (...args) => exec("/usr/bin/env", ["PATH=" + PATH, "CAIRN_MUX=herdr-app", CAIRN, ...args]);
const open = (target) => exec("open", [target]);

export default class CairnDetail extends View {
  init(_props, cx) {
    this.d = null;
    this.picked = {}; // question index → option index, before sending
    this.freeQ = null; // question answered in your own words
    this.openStep = null; // manual step whose ✓ / ✗ are open
    this.asking = false; // a question for an agent aside, in "how it works"
    this.typing = false;
    this.handle = cx.focus_handle();
    // Input state can only be made here or in a handler: one box of each kind
    // and language, shown under the question, step or view it is open for.
    const box = (placeholder) => {
      const s = InputState.new({ placeholder });
      s.on("focus", (_e, cx) => {
        this.typing = true;
        cx.notify();
      });
      s.on("blur", (_e, cx) => {
        this.typing = false;
        cx.notify();
      });
      return s;
    };
    this.boxes = {
      fr: { answer: box("Ta réponse, ou une question pour l'agent…"), report: box("Ce que tu vois…"), ask: box("Ta question sur ce ticket…") },
      en: { answer: box("Your answer, or a question for the agent…"), report: box("What you see…"), ask: box("Your question about this ticket…") },
    };
    for (const lang of ["fr", "en"]) {
      this.boxes[lang].answer.on("submit", (_e, cx) => this.sendFree(cx));
      this.boxes[lang].ask.on("submit", (_e, cx) => this.sendAsk(cx));
    }
    this.load(cx);
    cx.spawn(async (cx) => {
      for (;;) {
        try {
          await watchFolder(folder(DETAIL));
        } catch {
          return;
        }
        this.load(cx);
      }
    });
    cx.spawn(async (cx) => {
      for (;;) {
        const c = await nextCommand();
        if (c === "focus") this.handle.focus();
        cx.notify();
      }
    });
    cx.timer.every(15000, (cx) => this.load(cx));
  }

  load(cx) {
    cx.spawn(async (cx) => {
      const text = await readTextFile(DETAIL).catch(() => null);
      if (text === null) return;
      try {
        const d = JSON.parse(text);
        if (d.key !== this.d?.key) {
          this.picked = {};
          this.openStep = this.freeQ = null;
        }
        if (d.view !== "how") this.asking = false;
        this.d = d;
        this.pending = false;
      } catch {
        return;
      }
      cx.notify();
    });
  }

  fr() {
    return (this.d?.lang ?? "fr") !== "en";
  }

  box(kind) {
    return this.boxes[this.fr() ? "fr" : "en"][kind];
  }

  // ask sends a request to the engine: cairn do KEY ACTION ARGS.
  ask(cx, action, ...args) {
    this.error = null;
    this.pending = true;
    const key = this.d?.key || "-";
    cx.spawn(async (cx) => {
      try {
        const r = await cairn("do", key, action, ...args);
        if (r.code !== 0) this.error = (r.stderr || r.stdout).trim();
      } catch (e) {
        this.error = String(e);
      }
      if (this.error) this.pending = false;
      cx.notify();
    });
    cx.notify();
  }

  // key does what the TUI's key does on the screen the engine is on; the few
  // that the panel draws itself are handled here.
  key(k, cx) {
    const d = this.d;
    if (!d) return;
    const view = d.view || "";
    if (d.overlay && (k === "enter" || k === "esc" || d.overlay.buttons.some((b) => b.key === k))) return this.ask(cx, "overlay", k);
    if (k === "esc" && (this.freeQ !== null || this.asking || this.openStep !== null)) {
      this.freeQ = this.openStep = null;
      this.asking = false;
      cx.notify();
      return;
    }
    if (view === "" && k === "o" && d.url) return open(d.url);
    if (view === "" && k === "e") {
      const q = (d.sections ?? []).find((s) => s.id === "questions")?.questions?.find((q) => !q.answered);
      if (q) {
        this.freeQ = q.index;
        this.box("answer").set_value("");
        cx.notify();
      }
      return;
    }
    if (view === "how" && k === "e") {
      this.asking = d.how?.can_ask ?? false;
      this.box("ask").set_value("");
      cx.notify();
      return;
    }
    if (view === "comments" && k === "i") {
      for (const img of d.comments?.images ?? []) if (!img.broken && /^https?:/.test(img.url)) open(img.url);
      return;
    }
    this.ask(cx, "key", k);
  }

  sendFree(cx) {
    const box = this.box("answer");
    const text = box.value().trim();
    if (!text || this.freeQ === null) return;
    box.set_value("");
    const q = this.freeQ;
    this.freeQ = null;
    this.handle.focus();
    this.ask(cx, "answer", "--index", String(q), "--text", text);
  }

  sendAsk(cx) {
    const box = this.box("ask");
    const text = box.value().trim();
    if (!text) return;
    box.set_value("");
    this.asking = false;
    this.handle.focus();
    this.ask(cx, "ask", "--text", text);
  }

  onKey(e, cx) {
    if (this.typing) {
      if (e.key === "escape") {
        this.freeQ = this.openStep = null;
        this.asking = false;
        this.handle.focus();
        cx.stop_propagation();
        cx.notify();
      }
      return;
    }
    const m = e.modifiers ?? {};
    if (m.platform || m.alt) return; // the app's own shortcuts
    let k = KEYS[e.key] ?? (e.key_char && e.key_char.length === 1 ? e.key_char : e.key);
    if (m.control && e.key.length === 1) k = "ctrl+" + e.key;
    if (!k || (k.length > 1 && !Object.values(KEYS).includes(k) && !k.startsWith("ctrl+"))) return;
    cx.stop_propagation();
    this.key(k, cx);
  }

  toneColor(tone, t) {
    return { warn: t.blocked, danger: t.ansi1, good: t.ansi2, accent: t.accent, dim: t.dim, muted: t.muted, line: t.muted }[tone] ?? t.muted;
  }

  md(id, text, t, color) {
    return div()
      .min_w(0)
      .text_color(color ?? t.foreground)
      .child(
        TextView.markdown(id, text ?? "")
          .selectable(true)
          .on_link_click((url) => open(url)),
      );
  }

  button(id, label, { primary, enabled = true, tone, hint } = {}, onClick, t) {
    const bg = primary && enabled ? t.solid : t.raised;
    const fg = !enabled ? t.dim : primary ? t.solid_foreground : tone ? this.toneColor(tone, t) : t.foreground;
    return h_flex()
      .id(id)
      .h(28)
      .px(10)
      .gap(6)
      .flex_none()
      .items_center()
      .rounded(7)
      .border_1()
      .border_color(primary && enabled ? bg : t.border)
      .bg(bg)
      .text_size(12)
      .text_color(fg)
      .when(enabled, (e) => e.cursor_pointer().hover((s) => s.opacity(0.88)).on_click((_e, cx) => onClick(cx)))
      .when(Boolean(hint), (e) => e.child(div().font_family(this.mono).text_size(10.5).text_color(primary && enabled ? fg : t.dim).child(hint)))
      .child(label);
  }

  link(id, label, onClick, t, color) {
    return div()
      .id(id)
      .flex_none()
      .text_size(12)
      .text_color(color ?? t.accent)
      .cursor_pointer()
      .hover((s) => s.opacity(0.8))
      .on_click((_e, cx) => onClick(cx))
      .child(label);
  }

  items(id, list, t, indent = 0) {
    return v_flex()
      .gap(2)
      .children(
        (list ?? []).map((it, i) => {
          const pad = indent + (it.depth ?? 0) * 14;
          if (it.code) return div().pl(pad + 14).font_family(this.mono).text_size(11.5).text_color(t.muted).child(it.text);
          const bullet = it.image ? "▣" : it.plain ? "" : ["•", "◦", "·"][Math.min(it.depth ?? 0, 2)];
          const row = h_flex()
            .id(`${id}-${i}`)
            .pl(pad)
            .gap(6)
            .items_start()
            .when(Boolean(bullet), (e) => e.child(div().w(10).flex_none().text_color(it.image ? t.accent : t.dim).child(bullet)))
            .child(div().flex_1().min_w(0).child(this.md(`${id}-${i}-t`, it.text, t, it.image ? t.accent : t.muted)));
          return it.image ? row.rounded(6).cursor_pointer().hover((s) => s.bg(t.hover)).on_click(() => open(it.image)) : row;
        }),
      );
  }

  header(sec, t) {
    return h_flex()
      .pt(18)
      .pb(6)
      .gap(8)
      .items_center()
      .child(div().text_size(11.5).font_semibold().text_color(t.dim).child(sec.title))
      .when(Boolean(sec.count), (e) => e.child(div().text_size(11).text_color(this.toneColor(sec.tone, t)).child(sec.count)))
      .child(div().flex_1())
      .when(Boolean(sec.hint), (e) => e.child(div().text_size(11).text_color(t.dim).truncate().child(sec.hint)));
  }

  icon(state, t) {
    const [glyph, color] = { done: ["✓", t.ansi2], doing: ["◐", t.working], reported: ["✗", t.ansi1] }[state] ?? ["○", t.dim];
    return div().w(14).flex_none().text_color(color).child(glyph);
  }

  task(id, task, t, onClick, extra) {
    return v_flex()
      .id(id)
      .px(6)
      .py(3)
      .gap(3)
      .rounded(6)
      .when(Boolean(onClick), (e) => e.cursor_pointer().hover((s) => s.bg(t.hover)).on_click((_e, cx) => onClick(cx)))
      .child(h_flex().gap(6).items_start().child(this.icon(task.state, t)).child(div().flex_1().min_w(0).child(this.md(`${id}-t`, task.text, t, task.state === "done" ? t.muted : t.foreground))))
      .when((task.body ?? []).length > 0, (e) => e.child(this.items(`${id}-b`, task.body, t, 20)))
      .when(Boolean(task.reported), (e) => e.child(div().pl(20).text_size(12).text_color(t.ansi1).child((this.fr() ? "↳ signalé à l'agent : " : "↳ reported to the agent: ") + task.reported)))
      .when(Boolean(extra), (e) => e.child(extra));
  }

  inputRow(id, box, t, send, cancel) {
    const fr = this.fr();
    return h_flex()
      .gap(6)
      .child(div().flex_1().min_w(0).h(28).px(8).flex().items_center().rounded(7).border_1().border_color(t.accent).child(Input.new(box).h_full().text_size(12)))
      .child(this.button(`${id}-send`, fr ? "Envoyer" : "Send", { primary: true }, send, t))
      .child(this.button(`${id}-cancel`, fr ? "Annuler" : "Cancel", {}, cancel, t));
  }

  question(q, t) {
    const fr = this.fr();
    const id = `q-${q.index}`;
    if (q.answered)
      return v_flex()
        .gap(3)
        .child(h_flex().gap(6).child(div().w(14).text_color(t.ansi2).child("✓")).child(div().flex_1().min_w(0).child(this.md(`${id}-t`, q.text, t, t.muted))))
        .child(div().pl(20).text_size(12).text_color(t.ansi2).child((fr ? "↳ envoyé à l'agent : " : "↳ sent to the agent: ") + q.answered));
    const picked = this.picked[q.index] ?? (q.options ?? []).findIndex((o) => o.recommended);
    const writing = this.freeQ === q.index;
    return v_flex()
      .gap(6)
      .child(h_flex().gap(6).items_start().child(div().w(14).flex_none().text_color(t.blocked).font_bold().child("?")).child(div().flex_1().min_w(0).child(this.md(`${id}-t`, q.text, t))))
      .when((q.body ?? []).length > 0, (e) => e.child(this.items(`${id}-b`, q.body, t, 20)))
      .children(
        (q.options ?? []).map((o, j) =>
          h_flex()
            .id(`${id}-o${j}`)
            .ml(20)
            .px(8)
            .py(5)
            .gap(8)
            .items_start()
            .rounded(7)
            .border_1()
            .border_color(picked === j ? t.accent : t.border)
            .when(picked === j, (e) => e.bg(t.selected))
            .cursor_pointer()
            .hover((s) => s.bg(t.hover))
            .on_click((_e, cx) => {
              this.picked[q.index] = j;
              cx.notify();
            })
            .child(div().flex_none().font_family(this.mono).text_size(11.5).text_color(t.dim).child(`(${o.key})`))
            .child(div().flex_1().min_w(0).child(this.md(`${id}-o${j}-t`, o.text, t)))
            .when(o.recommended, (e) => e.child(div().flex_none().text_size(11).text_color(t.ansi2).child(fr ? "★ recommandé" : "★ recommended"))),
        ),
      )
      .when(Boolean(q.recommendation), (e) => e.child(div().pl(20).child(this.md(`${id}-r`, "★ " + q.recommendation, t, t.muted))))
      .child(
        h_flex()
          .ml(20)
          .gap(10)
          .items_center()
          .when((q.options ?? []).length > 0, (e) =>
            e.child(this.button(`${id}-send`, fr ? "Envoyer le choix" : "Send the choice", { primary: true, enabled: picked >= 0 }, (cx) => this.ask(cx, "answer", "--index", String(q.index), "--option", String(picked)), t)),
          )
          .when(!writing, (e) =>
            e.child(
              this.link(`${id}-write`, fr ? "✎ Répondre dans tes mots" : "✎ Answer in your words", (cx) => {
                this.freeQ = q.index;
                this.box("answer").set_value("");
                cx.notify();
              }, t, t.dim),
            ),
          ),
      )
      .when(writing, (e) =>
        e.child(
          div().ml(20).child(
            this.inputRow(`${id}-free`, this.box("answer"), t, (cx) => this.sendFree(cx), (cx) => {
              this.freeQ = null;
              cx.notify();
            }),
          ),
        ),
      );
  }

  section(sec, t) {
    const fr = this.fr();
    const kids = [];
    if (sec.questions) kids.push(...sec.questions.map((q) => this.question(q, t)));
    else if (sec.id === "side") kids.push(...(sec.tasks ?? []).map((k) => this.task(`side-${k.index}`, k, t, (cx) => this.ask(cx, "side", "--index", String(k.index)))));
    else if (sec.id === "manual") {
      kids.push(...(sec.setup ?? []).map((l, i) => h_flex().gap(6).child(div().w(14).text_color(t.dim).child("·")).child(div().flex_1().min_w(0).child(this.md(`setup-${i}`, l, t, t.muted)))));
      for (const k of sec.tasks ?? []) {
        const isOpen = this.openStep === k.index && k.state !== "done";
        const report = this.box("report");
        const extra = isOpen
          ? v_flex()
              .pl(20)
              .gap(6)
              .child(h_flex().child(this.button(`ok-${k.index}`, fr ? "✓ ça marche" : "✓ it works", { tone: "good" }, (cx) => this.ask(cx, "manual-ok", "--index", String(k.index)), t)))
              .child(
                this.inputRow(`ko-${k.index}`, report, t, (cx) => {
                  const text = report.value().trim();
                  if (!text) return;
                  report.set_value("");
                  this.openStep = null;
                  this.ask(cx, "manual-ko", "--index", String(k.index), "--text", text);
                }, (cx) => {
                  this.openStep = null;
                  cx.notify();
                }),
              )
          : null;
        kids.push(
          this.task(`manual-${k.index}`, k, t, (cx) => {
            if (k.state === "done") return this.ask(cx, "manual-ok", "--index", String(k.index));
            this.openStep = this.openStep === k.index ? null : k.index;
            this.box("report").set_value("");
            cx.notify();
          }, extra),
        );
      }
    } else if (sec.tasks) kids.push(...sec.tasks.map((k) => this.task(`${sec.id}-${k.index}`, k, t)));
    else if (sec.id === "goal") kids.push(this.md("goal", sec.text, t));
    else if (sec.id === "how") {
      if (sec.text) {
        const intro = sec.text.split(/\n#+ /)[0];
        const mechs = [...sec.text.matchAll(/^#+\s+(.+)$/gm)].map((m) => m[1]);
        kids.push(this.md("how-intro", intro, t, t.muted));
        if (mechs.length) kids.push(div().text_size(12).text_color(t.dim).child(mechs.join(" · ")));
      } else kids.push(div().text_size(12).text_color(t.muted).child((sec.lines ?? []).join(" ")));
      kids.push(h_flex().pt(2).child(this.link("how-open", sec.text ? (fr ? "› Tout lire" : "› Read it all") : fr ? "› Demander l'explication" : "› Ask for it", (cx) => this.key("i", cx), t)));
    } else if (sec.id === "approach") kids.push(this.items("approach", sec.items, t));
    else if (sec.id === "proofs") kids.push(this.items("proofs", sec.items, t));
    else if (sec.code) {
      const c = sec.code;
      const summary = c.reading
        ? fr ? "lecture…" : "reading…"
        : c.error || (c.ahead > 0 ? `${c.ahead} commit${c.ahead > 1 ? "s" : ""} · ${c.files} ${fr ? "fichier" : "file"}${c.files > 1 ? "s" : ""}` : fr ? "aucun commit sur la branche" : "no commit on the branch");
      kids.push(
        h_flex()
          .gap(6)
          .child(div().child(summary))
          .when(c.ahead > 0, (e) => e.child(div().text_color(t.ansi2).child(`+${c.added}`)).child(div().text_color(t.ansi1).child(`−${c.removed}`)))
          .when(c.uncommitted > 0 && !c.main, (e) => e.child(div().text_color(t.blocked).child(`· ${c.uncommitted} ${fr ? "non commité(s)" : "uncommitted"}`))),
      );
      if (c.last) kids.push(div().text_color(t.muted).truncate().child((fr ? "dernier : " : "last: ") + c.last));
      kids.push(div().font_family(this.mono).text_size(11.5).text_color(t.dim).truncate().child(c.branch));
      kids.push(div().font_family(this.mono).text_size(11.5).text_color(t.dim).truncate().child(c.folder));
      kids.push(h_flex().pt(2).child(this.link("diff", fr ? "› Relire le diff" : "› Review the diff", (cx) => this.key("d", cx), t)));
    } else if (sec.pr) {
      const p = sec.pr;
      const failing = (p.failed ?? []).length > 0;
      kids.push(
        h_flex()
          .gap(8)
          .child(this.link("pr-link", `#${p.number} ↗`, () => open(p.url), t))
          .child(div().flex_1().min_w(0).truncate().text_color(t.muted).child(p.line || p.error || p.state)),
      );
      if ((p.checks ?? []).length > 0) {
        kids.push(div().pt(2).text_size(11.5).text_color(p.ready ? t.ansi2 : t.dim).child(p.ready ? (fr ? "Prêt à merger" : "Ready to merge") : fr ? "Pas encore prêt à merger" : "Not ready to merge yet"));
        for (const k of p.checks) {
          const broken = !k.ok && k.label === "CI" && failing;
          kids.push(
            h_flex()
              .gap(6)
              .child(div().w(14).text_color(k.ok ? t.ansi2 : broken ? t.ansi1 : t.dim).child(k.ok ? "✓" : broken ? "✗" : "○"))
              .child(div().text_color(k.ok ? t.foreground : t.muted).child(broken ? `${k.label} · ${p.failed.join(", ")}` : k.label)),
          );
        }
      }
      kids.push(
        h_flex()
          .pt(4)
          .gap(6)
          .when(failing, (e) => e.child(this.button("fix-ci", fr ? "Corriger la CI" : "Fix CI", { tone: "danger", hint: "f" }, (cx) => this.key("f", cx), t)))
          .child(this.button("comments", fr ? "Commentaires et images" : "Comments and images", { hint: "c" }, (cx) => this.key("c", cx), t)),
      );
    } else if (sec.lines) kids.push(v_flex().font_family(this.mono).text_size(11.5).text_color(t.muted).children(sec.lines.map((l) => div().child(l || " "))));
    else if (sec.hint) return div().pt(14).text_size(12).text_color(this.toneColor(sec.tone, t)).child(`${sec.title} : ${sec.hint}`);
    return v_flex()
      .child(sec.title ? this.header(sec, t) : div().pt(14))
      .child(v_flex().gap(4).children(kids));
  }

  card(c, t) {
    const color = this.toneColor(c.tone, t);
    return v_flex()
      .mt(12)
      .p(12)
      .gap(6)
      .rounded(9)
      .bg(t.raised)
      .border_1()
      .border_color(t.border)
      .child(h_flex().gap(8).items_center().child(div().flex_1().min_w(0).font_semibold().text_color(color).child(c.title)).when(Boolean(c.since), (e) => e.child(div().flex_none().text_size(11).text_color(t.dim).child(c.since))))
      .when(Boolean(c.sub), (e) => e.child(div().text_size(12).text_color(t.muted).child(c.sub)))
      .when((c.actions ?? []).length > 0, (e) =>
        e.child(
          h_flex()
            .pt(4)
            .gap(6)
            .flex_wrap()
            .children(c.actions.map((b, i) => this.button(`card-${b.key}`, b.label, { primary: c.primary && i === 0, enabled: b.enabled, hint: b.key === "enter" ? "↵" : b.key }, (cx) => this.key(b.key, cx), t))),
        ),
      );
  }

  overlay(o, t) {
    return v_flex()
      .mt(12)
      .p(12)
      .gap(3)
      .rounded(9)
      .bg(t.popover)
      .border_1()
      .border_color(t.accent)
      .children(o.lines.map((l, i) => div().text_size(12).text_color(i === 0 ? t.foreground : t.muted).when(i === 0, (e) => e.font_semibold()).child(l || " ")))
      .child(
        h_flex()
          .pt(6)
          .gap(6)
          .flex_wrap()
          .children(o.buttons.map((b, i) => this.button(`ov-${b.key}`, b.label, { primary: i === 0 }, (cx) => this.ask(cx, "overlay", b.key), t))),
      );
  }

  // The chantier: what it waits for, where it stands, its sections.
  home(d, t) {
    const fr = this.fr();
    if (d.other) return this.other(d.other, t);
    if (!d.key) return [div().pt(16).text_color(t.dim).child(fr ? "Choisis un chantier dans la barre latérale." : "Pick a chantier in the sidebar.")];
    const kids = [];
    kids.push(
      h_flex()
        .pt(10)
        .gap(8)
        .items_center()
        .when(Boolean(d.meta), (e) => e.child(div().flex_1().min_w(0).truncate().text_size(12).text_color(t.muted).child(d.meta)))
        .when(Boolean(d.url), (e) => e.child(this.link("ticket", "Ticket ↗", () => open(d.url), t, t.dim))),
    );
    if (d.overlay) kids.push(this.overlay(d.overlay, t));
    if (d.card) kids.push(this.card(d.card, t));
    kids.push(
      h_flex()
        .pt(14)
        .gap(6)
        .items_center()
        .flex_wrap()
        .children(
          (d.flow ?? []).flatMap((s, i) => {
            const color = s.state === "done" ? t.ansi2 : s.state === "current" ? t.accent : t.dim;
            const label = s.state === "done" ? `${s.label} ✓` : s.state === "current" ? `${s.label} ●` : s.label;
            const pill = div().text_size(12).text_color(color).when(s.state === "current", (e) => e.font_semibold()).child(label);
            return i ? [div().text_size(12).text_color(t.dim).child("→"), pill] : [pill];
          }),
        ),
    );
    for (const r of d.status ?? [])
      kids.push(
        h_flex()
          .pt(4)
          .gap(8)
          .items_start()
          .text_size(12)
          .child(div().w(42).flex_none().text_color(t.dim).child(r.label))
          .child(div().flex_1().min_w(0).text_color(this.toneColor(r.tone, t)).child(r.note ? `${r.text} — ${r.note}` : r.text)),
      );
    if (d.no_plan) kids.push(div().pt(10).text_size(12).text_color(t.muted).child(fr ? "Pas de plan cairn pour ce chantier : seuls le code et la PR sont suivis." : "No cairn plan for this ticket: only the code and the PR are tracked."));
    for (const s of d.sections ?? []) kids.push(this.section(s, t));
    const live = new Map(agents().map((a) => [a.address, a.status]));
    const list = (d.agents ?? []).map((a) => ({ ...a, status: live.get(a.pane) ?? a.status }));
    if (list.length) {
      kids.push(this.header({ title: "Agents", hint: fr ? "clic pour y aller" : "click to go there" }, t));
      for (const a of list)
        kids.push(
          h_flex()
            .id(`agent-${a.pane}`)
            .px(6)
            .h(28)
            .gap(8)
            .items_center()
            .rounded(6)
            .cursor_pointer()
            .hover((s) => s.bg(t.hover))
            .on_click(() => run("pane.focus", { terminal: a.pane }))
            .child(div().size(7).rounded(4).bg({ working: t.working, blocked: t.blocked, done: t.done }[a.status] ?? t.idle))
            .child(div().font_semibold().child(a.name))
            .child(div().text_color(t.muted).child(a.status)),
        );
    }
    if (d.plan) kids.push(div().pt(18).text_size(11).text_color(t.dim).truncate().child((fr ? "plan : " : "plan: ") + d.plan));
    if (d.lint > 0) kids.push(div().text_size(11).text_color(t.blocked).child(fr ? `⚠ ${d.lint} point(s) de forme dans le plan` : `⚠ ${d.lint} formatting issue(s) in the plan`));
    return kids;
  }

  // An agent on no known ticket: attach it to one.
  other(o, t) {
    const fr = this.fr();
    return [
      h_flex().pt(12).gap(8).items_center().child(div().font_semibold().child(o.name)).child(div().text_color(t.muted).child(o.status)),
      div().pt(4).text_size(12).text_color(t.muted).child(o.title),
      div().text_size(11.5).font_family(this.mono).text_color(t.dim).child(o.dir),
      div().pt(8).text_size(12).text_color(t.muted).child(fr ? "Cet agent ne travaille sur aucun ticket connu." : "This agent works on no known ticket."),
      h_flex().pt(6).child(this.button("go-other", fr ? "Aller à l'agent" : "Go to the agent", { hint: "↵" }, () => run("pane.focus", { terminal: o.pane }), t)),
      this.header({ title: fr ? "Rattacher à un chantier" : "Attach to a ticket", hint: fr ? "l'agent prend le nom du ticket" : "the agent takes the ticket's name" }, t),
      ...o.targets.map((k) => h_flex().child(this.link(`attach-${k}`, `› ${k}`, (cx) => this.ask(cx, "attach", k), t))),
    ];
  }

  how(d, t) {
    const h = d.how;
    const fr = this.fr();
    if (!h) return [div().pt(16).text_color(t.dim).child(fr ? "Pas de plan cairn pour ce chantier." : "No cairn plan for this ticket.")];
    const kids = [];
    if (h.note) kids.push(div().pt(12).text_size(12).text_color(t.muted).child(h.note));
    if (this.asking)
      kids.push(
        v_flex()
          .pt(12)
          .gap(6)
          .child(div().text_size(12).text_color(t.muted).child(fr ? "Un agent à part lit le code et te répond, sans toucher au chantier." : "An agent aside reads the code and answers, without touching the ticket."))
          .child(
            this.inputRow("ask", this.box("ask"), t, (cx) => this.sendAsk(cx), (cx) => {
              this.asking = false;
              cx.notify();
            }),
          ),
      );
    if (h.markdown) kids.push(div().pt(12).child(this.md(`how-${h.key}`, h.markdown, t)));
    return kids;
  }

  comments(d, t) {
    const c = d.comments;
    const fr = this.fr();
    if (!c || c.loading) return [div().pt(16).text_color(t.dim).child(fr ? "Chargement de la conversation…" : "Loading the conversation…")];
    if (c.error) return [div().pt(16).text_color(t.ansi1).child(c.error)];
    const images = new Map(c.images.map((i) => [i.n, i]));
    const withLinks = (body) =>
      body.replace(/\[image (\d+)\]/g, (_s, n) => {
        const img = images.get(Number(n));
        if (!img || img.broken || !/^https?:/.test(img.url)) return fr ? `[image ${n} : lien cassé]` : `[image ${n}: broken link]`;
        return `[🖼 image ${n}](${img.url})`;
      });
    const kids = [];
    for (const e of c.entries) {
      let body = withLinks(e.body);
      if (e.bot && !c.show_bots) {
        const lines = body.split("\n");
        if (lines.length > 8) body = lines.slice(0, 8).join("\n") + `\n\n*… ${lines.length - 8} ${fr ? "lignes de bot masquées" : "bot lines hidden"}*`;
      }
      const kind =
        e.state === "APPROVED"
          ? [fr ? "review approuvée" : "review approved", t.ansi2]
          : e.state === "CHANGES_REQUESTED"
            ? [fr ? "changements demandés" : "changes requested", t.ansi1]
            : e.kind === "inline"
              ? [e.where, t.working]
              : [e.kind, t.dim];
      kids.push(
        v_flex()
          .mt(12)
          .p(10)
          .gap(6)
          .rounded(9)
          .border_1()
          .border_color(e.new ? t.accent : t.border)
          .child(
            h_flex()
              .gap(8)
              .items_center()
              .text_size(12)
              .when(e.new, (x) => x.child(div().text_color(t.accent).child("●")))
              .child(div().font_semibold().text_color(e.bot ? t.muted : t.foreground).child(e.bot ? `${e.author} (bot)` : e.author))
              .child(div().text_color(kind[1]).truncate().child(kind[0]))
              .child(div().text_color(t.dim).child(e.ago))
              .child(div().flex_1())
              .when(e.kind !== "description", (x) => x.child(this.link(`send-${e.index}`, "→ agent", (cx) => this.ask(cx, "comment", "--index", String(e.index)), t))),
          )
          .child(this.md(`entry-${e.index}`, body, t, e.bot ? t.muted : t.foreground)),
      );
    }
    return kids;
  }

  render() {
    const t = theme();
    this.mono = settings().font_family ?? "JetBrains Mono";
    const d = this.d;
    const fr = this.fr();
    const view = d?.view || "";
    let title = d?.title ?? "cairn";
    let body;
    if (!d) body = [div().pt(16).text_color(t.dim).child(fr ? "cairn démarre…" : "cairn is starting…")];
    else if (view === "how") {
      title = (fr ? "Comment ça marche · " : "How it works · ") + (d.how?.title ?? "");
      body = this.how(d, t);
    } else if (view === "comments") {
      title = `PR #${d.comments?.number ?? ""} · ${d.title ?? ""}`;
      body = this.comments(d, t);
    } else if (view !== "") body = [div().pt(16).text_color(t.muted).child(fr ? "Cet écran arrive bientôt en natif. esc pour revenir." : "This screen comes natively soon. esc to go back.")];
    else body = this.home(d, t);
    const status = this.error || d?.busy || (this.pending ? "…" : d?.flash?.text);
    const buttons = d?.buttons ?? [];
    return v_flex()
      .id("cairn-detail")
      .track_focus(this.handle)
      .on_key_down((e, cx) => this.onKey(e, cx))
      .on_mouse_down("left", () => {
        if (!this.typing) this.handle.focus();
      })
      .size_full()
      .min_h(0)
      .rounded(10)
      .overflow_hidden()
      .bg(t.background)
      .border_1()
      .border_color(t.border)
      .text_color(t.foreground)
      .text_size(13)
      .child(
        h_flex()
          .h(42)
          .flex_none()
          .px(14)
          .gap(8)
          .items_center()
          .border_b(1)
          .border_color(t.border)
          .when(view !== "", (e) => e.child(this.link("back", fr ? "← Retour" : "← Back", (cx) => this.key("esc", cx), t, t.dim)))
          .when(view === "" && Boolean(d?.key), (e) => e.child(div().flex_none().font_family(this.mono).text_size(12).text_color(t.accent).child(d.key)))
          .child(div().flex_1().min_w(0).truncate().font_semibold().child(title))
          .child(
            div()
              .id("close-panel")
              .flex_none()
              .text_size(14)
              .text_color(t.dim)
              .cursor_pointer()
              .hover((s) => s.text_color(t.foreground))
              .on_click(() => run("layout.hide", { id: "panel" }))
              .child("×"),
          ),
      )
      .when(Boolean(status), (e) =>
        e.child(
          div()
            .flex_none()
            .px(14)
            .py(6)
            .border_b(1)
            .border_color(t.border)
            .text_size(12)
            .text_color(this.error || d?.flash?.error ? t.ansi1 : t.muted)
            .child(status),
        ),
      )
      .child(
        div()
          .flex_1()
          .min_h(0)
          .relative()
          .child(v_flex().id("cairn-detail-body").absolute().inset_0().overflow_y_scroll().px(14).pb(20).children(body))
          .child(Scrollbar.vertical("cairn-detail-body").absolute().inset_0()),
      )
      .when(buttons.length > 0, (e) =>
        e.child(
          h_flex()
            .flex_none()
            .px(10)
            .py(8)
            .gap(6)
            .flex_wrap()
            .border_t(1)
            .border_color(t.border)
            .children(
              buttons.map((b, i) =>
                this.button(`foot-${b.key}`, b.label, { primary: d.primary && i === 0, enabled: b.enabled, hint: b.key === "enter" ? "↵" : b.key }, (cx) => this.key(b.key, cx), t),
              ),
            ),
        ),
      );
  }
}
