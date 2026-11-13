// cairn: one chantier, in the Herdr app's right panel (next to Files and
// Changes). Written by cairn's install.sh --herdr-app; run it again to update it.
//
// The running cairn is the engine: it writes the selected chantier to
// detail.json (its card, sections and confirmations, the TUI's own) and runs
// what this panel asks with `cairn do`, the same code as its keys and clicks.
import { View, div } from "gpui-kit";
import { v_flex, h_flex, InputState, Input, TextView, Scrollbar } from "gpui-base";
import { agents, run, exec, readTextFile, watchFolder, theme, settings } from "herdr";

const CAIRN = "@CAIRN@";
const PATH = "@PATH@";
const DETAIL = "@DETAIL@";
const RANK = { blocked: 4, done: 3, working: 2, idle: 1 };
// Keys whose screen only the TUI has for now: they open it there.
const TUI_ONLY = new Set(["c", "i", "n", "s", "t"]);

const folder = (path) => path.slice(0, path.lastIndexOf("/"));
const cairn = (...args) => exec("/usr/bin/env", ["PATH=" + PATH, "CAIRN_MUX=herdr-app", CAIRN, ...args]);

export default class CairnDetail extends View {
  init(_props, cx) {
    this.d = null;
    this.picked = {}; // question index → option index, before sending
    this.freeQ = null; // question answered in your own words
    this.openStep = null; // manual step whose ✓ / ✗ are open
    // Input state can only be made here or in a handler: one box of each kind,
    // shown under the question or step it is open for, per language.
    this.boxes = {
      fr: { answer: InputState.new({ placeholder: "Ta réponse, ou une question pour l'agent…" }), report: InputState.new({ placeholder: "Ce que tu vois…" }) },
      en: { answer: InputState.new({ placeholder: "Your answer, or a question for the agent…" }), report: InputState.new({ placeholder: "What you see…" }) },
    };
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
        this.d = d;
      } catch {
        return;
      }
      cx.notify();
    });
  }

  fr() {
    return (this.d?.lang ?? "fr") !== "en";
  }

  ask(cx, ...args) {
    this.error = null;
    cx.spawn(async (cx) => {
      try {
        const r = await cairn("do", this.d.key, ...args);
        if (r.code !== 0) this.error = (r.stderr || r.stdout).trim();
      } catch (e) {
        this.error = String(e);
      }
      cx.notify();
    });
  }

  key(k, cx) {
    if (k === "o" && this.d.url) return exec("open", [this.d.url]);
    if (k === "e") return; // the questions are right here
    if (TUI_ONLY.has(k)) {
      cx.spawn(async () => {
        await cairn("do", this.d.key, "key", k);
        await cairn("open", this.d.key);
      });
      return;
    }
    this.ask(cx, "key", k);
  }

  input(kind) {
    return this.boxes[this.fr() ? "fr" : "en"][kind];
  }


  toneColor(tone, t) {
    return { warn: t.blocked, danger: t.ansi1, good: t.ansi2, accent: t.accent, dim: t.dim, muted: t.muted, line: t.muted }[tone] ?? t.muted;
  }

  md(id, text, t, color) {
    return div().min_w(0).text_color(color ?? t.foreground).child(TextView.markdown(id, text ?? "").selectable(true));
  }

  button(id, label, { primary, enabled = true, tone } = {}, onClick, t) {
    const bg = primary && enabled ? t.solid : t.raised;
    const fg = !enabled ? t.dim : primary ? t.solid_foreground : tone ? this.toneColor(tone, t) : t.foreground;
    return div()
      .id(id)
      .h(28)
      .px(10)
      .flex()
      .flex_none()
      .items_center()
      .rounded(7)
      .border_1()
      .border_color(primary && enabled ? bg : t.border)
      .bg(bg)
      .text_size(12)
      .text_color(fg)
      .when(enabled, (e) => e.cursor_pointer().hover((s) => s.opacity(0.88)).on_click((_e, cx) => onClick(cx)))
      .child(label);
  }

  items(id, list, t, indent = 0) {
    return v_flex()
      .gap(2)
      .children(
        (list ?? []).map((it, i) => {
          const pad = indent + (it.depth ?? 0) * 14;
          if (it.code) return div().pl(pad + 14).font_family(this.mono).text_size(11.5).text_color(t.muted).child(it.text);
          const bullet = it.plain ? "" : ["•", "◦", "·"][Math.min(it.depth ?? 0, 2)];
          return h_flex()
            .pl(pad)
            .gap(6)
            .items_start()
            .when(Boolean(bullet), (e) => e.child(div().w(8).flex_none().text_color(t.dim).child(bullet)))
            .child(div().flex_1().min_w(0).child(this.md(`${id}-${i}`, it.text, t, t.muted)));
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
      .child(
        h_flex()
          .gap(6)
          .items_start()
          .child(this.icon(task.state, t))
          .child(div().flex_1().min_w(0).child(this.md(`${id}-t`, task.text, t, task.state === "done" ? t.muted : t.foreground))),
      )
      .when((task.body ?? []).length > 0, (e) => e.child(this.items(`${id}-b`, task.body, t, 20)))
      .when(Boolean(task.reported), (e) => e.child(div().pl(20).text_size(12).text_color(t.ansi1).child((this.fr() ? "↳ signalé à l'agent : " : "↳ reported to the agent: ") + task.reported)))
      .when(Boolean(extra), (e) => e.child(extra));
  }

  question(q, t) {
    const fr = this.fr();
    const id = `q-${q.index}`;
    if (q.answered)
      return v_flex()
        .gap(3)
        .child(h_flex().gap(6).child(div().w(14).text_color(t.ansi2).child("✓")).child(div().flex_1().min_w(0).text_color(t.muted).child(this.md(`${id}-t`, q.text, t, t.muted))))
        .child(div().pl(20).text_size(12).text_color(t.ansi2).child((fr ? "↳ envoyé à l'agent : " : "↳ sent to the agent: ") + q.answered));
    const picked = this.picked[q.index] ?? (q.options ?? []).findIndex((o) => o.recommended);
    const writing = this.freeQ === q.index;
    const free = this.input("answer");
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
          .gap(6)
          .when((q.options ?? []).length > 0, (e) =>
            e.child(
              this.button(`${id}-send`, fr ? "Envoyer le choix" : "Send the choice", { primary: true, enabled: picked >= 0 }, (cx) => this.ask(cx, "answer", "--index", String(q.index), "--option", String(picked)), t),
            ),
          )
          .when(!writing, (e) =>
            e.child(
              div()
                .id(`${id}-write`)
                .text_size(12)
                .text_color(t.dim)
                .cursor_pointer()
                .hover((s) => s.text_color(t.foreground))
                .on_click((_e, cx) => {
                  this.freeQ = q.index;
                  free.set_value("");
                  cx.notify();
                })
                .child(fr ? "✎ Répondre dans tes mots" : "✎ Answer in your words"),
            ),
          ),
      )
      .when(writing, (e) =>
        e.child(
          h_flex()
            .ml(20)
            .gap(6)
            .child(div().flex_1().min_w(0).h(28).px(8).flex().items_center().rounded(7).border_1().border_color(t.accent).child(Input.new(free).h_full().text_size(12)))
            .child(
              this.button(`${id}-sendfree`, fr ? "Envoyer" : "Send", { primary: true }, (cx) => {
                const text = free.value().trim();
                if (!text) return;
                free.set_value("");
                this.freeQ = null;
                this.ask(cx, "answer", "--index", String(q.index), "--text", text);
              }, t),
            )
            .child(
              this.button(`${id}-cancelfree`, fr ? "Annuler" : "Cancel", {}, (cx) => {
                this.freeQ = null;
                cx.notify();
              }, t),
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
        const open = this.openStep === k.index && k.state !== "done";
        const report = this.input("report");
        const extra = open
          ? h_flex()
              .pl(20)
              .gap(6)
              .child(this.button(`ok-${k.index}`, fr ? "✓ ça marche" : "✓ it works", { tone: "good" }, (cx) => this.ask(cx, "manual-ok", "--index", String(k.index)), t))
              .child(div().flex_1().min_w(0).h(28).px(8).flex().items_center().rounded(7).border_1().border_color(t.border).child(Input.new(report).h_full().text_size(12)))
              .child(
                this.button(`ko-${k.index}`, fr ? "✗ ça ne marche pas" : "✗ it fails", { tone: "danger" }, (cx) => {
                  const text = report.value().trim();
                  if (!text) return;
                  report.set_value("");
                  this.ask(cx, "manual-ko", "--index", String(k.index), "--text", text);
                }, t),
              )
          : null;
        kids.push(
          this.task(`manual-${k.index}`, k, t, (cx) => {
            if (k.state === "done") return this.ask(cx, "manual-ok", "--index", String(k.index));
            this.openStep = this.openStep === k.index ? null : k.index;
            this.input("report").set_value("");
            cx.notify();
          }, extra),
        );
      }
    } else if (sec.tasks) kids.push(...sec.tasks.map((k) => this.task(`${sec.id}-${k.index}`, k, t)));
    else if (sec.id === "goal") kids.push(this.md("goal", sec.text, t));
    else if (sec.id === "approach") kids.push(this.items("approach", sec.items, t));
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
    } else if (sec.pr) {
      const p = sec.pr;
      const failing = (p.failed ?? []).length > 0;
      kids.push(
        h_flex()
          .gap(8)
          .child(div().id("pr-link").text_color(t.accent).cursor_pointer().hover((s) => s.opacity(0.8)).on_click(() => exec("open", [p.url])).child(`#${p.number}`))
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
          .when(failing, (e) => e.child(this.button("fix-ci", fr ? "Corriger la CI" : "Fix CI", { tone: "danger" }, (cx) => this.key("f", cx), t)))
          .child(this.button("comments", fr ? "Commentaires et images" : "Comments and images", {}, (cx) => this.key("c", cx), t)),
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
      .border_l(3)
      .child(h_flex().gap(8).items_center().child(div().flex_1().min_w(0).font_semibold().text_color(color).child(c.title)).when(Boolean(c.since), (e) => e.child(div().flex_none().text_size(11).text_color(t.dim).child(c.since))))
      .when(Boolean(c.sub), (e) => e.child(div().text_size(12).text_color(t.muted).child(c.sub)))
      .when((c.actions ?? []).length > 0, (e) =>
        e.child(
          h_flex()
            .pt(4)
            .gap(6)
            .flex_wrap()
            .children(c.actions.map((b, i) => this.button(`card-${b.key}`, b.label, { primary: c.primary && i === 0, enabled: b.enabled }, (cx) => this.key(b.key, cx), t))),
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

  render() {
    const t = theme();
    this.mono = settings().font_family ?? "JetBrains Mono";
    const d = this.d;
    const fr = this.fr();
    const shell = v_flex().id("cairn-detail").size_full().min_h(0).rounded(10).overflow_hidden().bg(t.background).border_1().border_color(t.border).text_color(t.foreground).text_size(13);
    if (!d || !d.key)
      return shell.child(
        div().p(16).text_color(t.dim).child(fr ? "Choisis un chantier dans la barre latérale." : "Pick a chantier in the sidebar."),
      );
    const live = new Map(agents().map((a) => [a.address, a.status]));
    const list = (d.agents ?? []).map((a) => ({ ...a, status: live.get(a.pane) ?? a.status }));
    return shell
      .child(
        h_flex()
          .h(42)
          .flex_none()
          .px(14)
          .gap(8)
          .items_center()
          .border_b(1)
          .border_color(t.border)
          .child(div().flex_none().font_family(this.mono).text_size(12).text_color(t.accent).child(d.key))
          .child(div().flex_1().min_w(0).truncate().font_semibold().child(d.title))
          .child(
            div()
              .id("open-tui")
              .flex_none()
              .text_size(11)
              .text_color(t.dim)
              .cursor_pointer()
              .hover((s) => s.text_color(t.foreground))
              .tooltip(fr ? "Ouvrir dans la TUI" : "Open in the TUI")
              .on_click(() => cairn("open", d.key))
              .child("TUI"),
          )
          .child(
            div()
              .id("close-panel")
              .flex_none()
              .text_size(13)
              .text_color(t.dim)
              .cursor_pointer()
              .hover((s) => s.text_color(t.foreground))
              .on_click(() => run("layout.hide", { id: "panel" }))
              .child("×"),
          ),
      )
      .child(
        div()
          .flex_1()
          .min_h(0)
          .relative()
          .child(
            v_flex()
              .id("cairn-detail-body")
              .absolute()
              .inset_0()
              .overflow_y_scroll()
              .px(14)
              .pb(20)
              .child(
                h_flex()
                  .pt(10)
                  .gap(8)
                  .items_center()
                  .when(Boolean(d.meta), (e) => e.child(div().flex_1().min_w(0).truncate().text_size(12).text_color(t.muted).child(d.meta)))
                  .when(Boolean(d.url), (e) => e.child(div().id("ticket").flex_none().text_size(11).text_color(t.dim).cursor_pointer().hover((s) => s.text_color(t.foreground)).on_click(() => exec("open", [d.url])).child(fr ? "Ticket ↗" : "Ticket ↗"))),
              )
              .when(Boolean(d.flash) || Boolean(this.error) || Boolean(d.busy), (e) =>
                e.child(
                  div()
                    .mt(10)
                    .px(10)
                    .py(6)
                    .rounded(7)
                    .bg(t.hover)
                    .text_size(12)
                    .text_color(this.error || d.flash?.error ? t.ansi1 : t.muted)
                    .child(this.error || d.busy || d.flash?.text),
                ),
              )
              .when(Boolean(d.overlay), (e) => e.child(this.overlay(d.overlay, t)))
              .when(Boolean(d.card), (e) => e.child(this.card(d.card, t)))
              .child(
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
              )
              .children(
                (d.status ?? []).map((r) =>
                  h_flex()
                    .pt(4)
                    .gap(8)
                    .items_start()
                    .text_size(12)
                    .child(div().w(42).flex_none().text_color(t.dim).child(r.label))
                    .child(div().flex_1().min_w(0).text_color(this.toneColor(r.tone, t)).child(r.note ? `${r.text} — ${r.note}` : r.text)),
                ),
              )
              .when(d.no_plan, (e) => e.child(div().pt(10).text_size(12).text_color(t.muted).child(fr ? "Pas de plan cairn pour ce chantier : seuls le code et la PR sont suivis." : "No cairn plan for this ticket: only the code and the PR are tracked.")))
              .children((d.sections ?? []).map((s) => this.section(s, t)))
              .when(list.length > 0, (e) =>
                e.child(this.header({ title: "Agents", hint: fr ? "clic pour y aller" : "click to go there" }, t)).children(
                  list.map((a) =>
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
                  ),
                ),
              )
              .when(Boolean(d.plan), (e) => e.child(div().pt(18).text_size(11).text_color(t.dim).truncate().child((fr ? "plan : " : "plan: ") + d.plan)))
              .when(d.lint > 0, (e) => e.child(div().text_size(11).text_color(t.blocked).child(fr ? `⚠ ${d.lint} point(s) de forme dans le plan` : `⚠ ${d.lint} formatting issue(s) in the plan`))),
          )
          .child(Scrollbar.vertical("cairn-detail-body").absolute().inset_0()),
      );
  }
}
