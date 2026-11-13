// cairn: where each chantier stands, in the Herdr app's sidebar between Spaces
// and Agents. Written by cairn's install.sh --herdr-app; run it again to update it.
//
// The TUI writes its list to board.json; this section draws it and keeps each
// agent's status live. A click opens the chantier: in the right panel with
// "ui": "native" in cairn's config, else in the TUI (cairn open KEY). The arrow
// on hover goes to its agent; the header opens cairn as it was.
import { View, div, svg } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { agents, spaces, run, exec, readTextFile, watchFolder, theme, settings, layout, openOverlay } from "herdr";

const CAIRN = "@CAIRN@";
const PATH = "@PATH@";
const BOARD = "@BOARD@";
const DETAIL = "@DETAIL@";
const GROUPS = ["wait", "work", "toplan", "kept", "shipped", "others"];
const RANK = { blocked: 4, done: 3, working: 2, idle: 1 };

function find(node, id) {
  if (node?.id === id) return node;
  for (const child of node?.children ?? []) {
    const found = find(child, id);
    if (found) return found;
  }
  return null;
}

// The sidebar's rail goes by its width, as in the shipped Agents section.
const narrow = () => (find(layout(), "sidebar")?.size?.px ?? Infinity) < 160;
const folder = (path) => path.slice(0, path.lastIndexOf("/"));
const cairn = (...args) => exec("/usr/bin/env", ["PATH=" + PATH, "CAIRN_MUX=herdr-app", CAIRN, ...args]);

// bar draws the tasks' progress as the TUI does (█ done, ▓ under way, ░ left):
// the filled parts slide when a task moves. Nothing loops: a looping
// animation redraws the whole window at every tick, the plan's long markdown
// included, and kept Herdr at 100 % CPU.
function bar(id, done, doing, total, width, t) {
  const d = total > 0 ? Math.round((done * width) / total) : 0;
  const g = total > 0 ? Math.min(width - d, Math.ceil((doing * width) / total)) : 0;
  const slide = { duration: 450, easing: "ease-out" };
  return h_flex()
    .id(id)
    .flex_none()
    .w(width)
    .h(4)
    .rounded(2)
    .overflow_hidden()
    .bg(t.border)
    .child(div().id(`${id}-done`).h_full().w(d).bg(t.ansi2).transition("width", slide))
    .child(div().id(`${id}-doing`).h_full().w(g).bg(t.working).transition("width", slide));
}

export default class Cairn extends View {
  init(_props, cx) {
    this.board = null;
    this.folded = new Set(["kept", "shipped"]);
    // cairn's engine (native) or its TUI's space comes back with the app.
    cx.spawn(async (cx) => {
      await cairn("ensure").catch(() => null);
      this.load(cx);
    });
    this.load(cx);
    cx.spawn(async (cx) => {
      for (;;) {
        try {
          await watchFolder(folder(BOARD));
        } catch {
          return; // the timer below still refreshes
        }
        this.load(cx);
      }
    });
    cx.timer.every(15000, (cx) => this.load(cx));
  }

  load(cx) {
    cx.spawn(async (cx) => {
      const detail = await readTextFile(DETAIL).catch(() => null);
      try {
        this.shown = detail ? JSON.parse(detail).key : null;
      } catch {}
      const text = await readTextFile(BOARD).catch(() => null);
      if (text === null) this.board = null;
      else {
        try {
          this.board = JSON.parse(text);
        } catch {
          return; // a write in progress: the next change brings it whole
        }
      }
      cx.notify();
    });
  }

  fr() {
    return (this.board?.lang ?? "fr") !== "en";
  }

  // The chantier in the right panel, drawn natively; the running cairn selects it.
  show(cx, key) {
    this.shown = key;
    run("layout.show", { id: "panel" });
    run("layout.select", { stack: "panel-body", child: "cairn-detail" });
    run("cairn-detail.focus");
    cx.spawn(async (cx) => {
      const r = await cairn("do", key, "select").catch((e) => ({ code: 1, stderr: String(e) }));
      this.error = r.code !== 0 ? (r.stderr || r.stdout).trim() : null;
      cx.notify();
    });
    cx.notify();
  }

  // cairn's own space, on the chantier key when given: the TUI selects it.
  open(cx, key) {
    this.error = null;
    cx.spawn(async (cx) => {
      try {
        const home = this.board?.home_workspace;
        if (!key && this.board?.ui === "native") {
          run("layout.show", { id: "panel" });
          run("layout.select", { stack: "panel-body", child: "cairn-detail" });
          run("cairn-detail.focus");
        } else if (!key && home && spaces().some((s) => s.id === home)) run("space.select", { id: home });
        else {
          const r = await cairn("open", ...(key ? [key] : []));
          if (r.code !== 0) this.error = (r.stderr || r.stdout).trim();
        }
      } catch (e) {
        this.error = String(e);
      }
      this.load(cx);
    });
  }

  liveAgents(c) {
    const live = new Map(agents().map((a) => [a.address, a.status]));
    return (c.agents ?? []).map((a) => ({ ...a, status: live.get(a.pane) ?? a.status }));
  }

  // The chantier's agent, the one that needs you first; else its space.
  jump(c, list) {
    const target = [...list].sort((a, b) => (RANK[b.status] ?? 0) - (RANK[a.status] ?? 0))[0];
    if (target) run("pane.focus", { terminal: target.pane });
    else if (c.workspace) run("space.select", { id: c.workspace });
  }

  dot(t, status, urgent) {
    if (urgent) return div().size(14).flex_none().rounded(4).bg(t.blocked + "26").text_size(10.5).font_bold().text_color(t.blocked).flex().items_center().justify_center().child("!");
    const color = { working: t.working, blocked: t.blocked, done: t.done }[status];
    if (!status || status === "idle") return div().size(7).mx(3).flex_none().rounded(4).border(1.5).border_color(status ? t.idle ?? t.dim : t.border);
    return div().size(7).mx(3).flex_none().rounded(4).bg(color ?? t.dim);
  }

  tip(c, list) {
    const fr = this.fr();
    const lines = [`${c.key} · ${c.title}`];
    if (c.waiting) lines.push(`⚠ ${c.waiting}`);
    if (c.ci_failed) lines.push(fr ? "⚠ CI en échec" : "⚠ CI failing");
    if (c.pr) lines.push(`PR #${c.pr.number} · ${c.pr.state}`);
    for (const a of list) lines.push(`${a.name} · ${a.status}`);
    return lines.join("\n");
  }

  row(c, t, mono, activeSpace, collapsed, cx) {
    const list = this.liveAgents(c);
    const urgent = Boolean(c.waiting || c.ci_failed);
    const top = list.reduce((best, a) => ((RANK[a.status] ?? 0) > (RANK[best] ?? 0) ? a.status : best), null);
    const native = this.board?.ui === "native";
    const here = native ? this.shown === c.key : Boolean(c.workspace) && c.workspace === activeSpace;
    const progress =
      c.tasks > 0
        ? h_flex()
            .flex_none()
            .gap(5)
            .items_center()
            .child(c.phase)
            .child(bar(`cairn-bar-${c.key}`, c.tasks_done, c.tasks_doing ?? 0, c.tasks, 28, t))
            .child(`${c.tasks_done}/${c.tasks}`)
        : c.phase;
    const el = h_flex()
      .id(`cairn-${c.key}`)
      .h(30)
      .flex_none()
      .rounded(8)
      .items_center()
      .when(here, (e) => e.bg(t.selected))
      .cursor_pointer()
      .hover((s) => s.bg(t.hover))
      .tooltip(this.tip(c, list))
      .on_hover((on, cx) => {
        if (on) this.hovered = c.key;
        else if (this.hovered === c.key) this.hovered = null;
        cx.notify();
      })
      .on_click((_e, cx) => (native ? this.show(cx, c.key) : this.open(cx, c.key)));
    if (collapsed) return el.justify_center().child(this.dot(t, top, urgent));
    const canJump = list.length > 0 || Boolean(c.workspace);
    return el
      .px(8)
      .gap(9)
      .child(this.dot(t, top, urgent))
      .child(
        h_flex()
          .flex_1()
          .min_w(0)
          .gap(6)
          .items_center()
          .child(div().flex_none().font_family(mono).text_size(11).text_color(urgent ? t.blocked : t.dim).child(c.key))
          .child(div().flex_1().min_w(0).truncate().text_color(top && top !== "idle" ? t.foreground : t.muted).child(c.title || "")),
      )
      .when(this.hovered !== c.key || !canJump, (e) => e.child(div().flex_none().text_size(11).text_color(t.dim).child(progress)))
      .when(this.hovered === c.key && canJump, (e) =>
        e.child(
          div()
            .id(`cairn-jump-${c.key}`)
            .flex_none()
            .px(5)
            .rounded(4)
            .text_size(12)
            .text_color(t.dim)
            .hover((s) => s.text_color(t.foreground).bg(t.raised))
            .tooltip(this.fr() ? (list.length ? "Aller à l'agent" : "Aller à l'espace") : list.length ? "Go to the agent" : "Go to the space")
            .on_click((_e, cx) => {
              cx.stop_propagation();
              this.jump(c, list);
            })
            .child("→"),
        ),
      );
  }

  header(g, count, t, cx) {
    const open = !this.folded.has(g);
    return h_flex()
      .id(`cairn-g-${g}`)
      .px(8)
      .pt(8)
      .h(30)
      .flex_none()
      .gap(6)
      .items_center()
      .text_size(11)
      .font_medium()
      .cursor_pointer()
      .hover((s) => s.text_color(t.foreground))
      .on_click((_e, cx) => {
        open ? this.folded.add(g) : this.folded.delete(g);
        cx.notify();
      })
      .child(div().truncate().text_color(g === "wait" ? t.blocked : t.dim).child(this.label(g)))
      .child(div().text_color(t.dim).child(String(count)))
      .child(div().flex_1())
      .when(!open, (el) => el.child(div().text_color(t.dim).child("›")));
  }

  iconButton(id, path, tip, onClick, t) {
    return div()
      .id(id)
      .flex_none()
      .p(2)
      .rounded(5)
      .text_color(t.dim)
      .cursor_pointer()
      .hover((s) => s.text_color(t.foreground).bg(t.hover))
      .tooltip(tip)
      .on_click((_e, cx) => onClick(cx))
      .child(svg(path).size(14));
  }

  // The settings: natively the panel's page; else cairn, where s opens them.
  openSettings(cx) {
    if (this.board?.ui !== "native") return this.open(cx);
    run("layout.show", { id: "panel" });
    run("layout.select", { stack: "panel-body", child: "cairn-detail" });
    run("cairn-detail.focus");
    cx.spawn(async () => {
      await cairn("do", "-", "key", "s").catch(() => null);
    });
  }

  label(g) {
    const fr = this.fr();
    if (g === "shipped") return fr ? "Livrés" : "Shipped";
    if (g === "others") return fr ? "Hors chantier" : "On no ticket";
    return this.board.groups?.[g] ?? g;
  }

  // A shipped chantier, or an agent on no known one: a plain row.
  plain(id, label, sub, t, onClick, dot) {
    return h_flex()
      .id(id)
      .h(30)
      .flex_none()
      .px(8)
      .gap(9)
      .rounded(8)
      .items_center()
      .cursor_pointer()
      .hover((s) => s.bg(t.hover))
      .on_click((_e, cx) => onClick(cx))
      .child(dot ?? this.dot(t, null, false))
      .child(div().flex_none().font_family(settings().font_family ?? "JetBrains Mono").text_size(11).text_color(t.dim).child(label))
      .child(div().flex_1().min_w(0).truncate().text_color(t.muted).child(sub));
  }

  // An agent on no known chantier: shown natively, to attach it to one.
  showOther(cx, a) {
    if (this.board?.ui !== "native") return run("pane.focus", { terminal: a.pane });
    run("layout.show", { id: "panel" });
    run("layout.select", { stack: "panel-body", child: "cairn-detail" });
    run("cairn-detail.focus");
    cx.spawn(async () => {
      await cairn("do", "-", "select-other", a.pane).catch(() => null);
    });
  }

  render(cx) {
    const t = theme();
    const mono = settings().font_family ?? "JetBrains Mono";
    const collapsed = narrow();
    const fr = this.fr();
    const b = this.board;
    const chantiers = b?.chantiers ?? [];
    const waiting = chantiers.filter((c) => c.group === "wait").length;
    const activeSpace = spaces().find((s) => s.active)?.id;

    const rows = [];
    if (collapsed) {
      for (const c of chantiers) rows.push(this.row(c, t, mono, activeSpace, true, cx));
    } else {
      for (const g of GROUPS) {
        if (g === "shipped" || g === "others") {
          const list = g === "shipped" ? b?.shipped_list ?? [] : b?.others ?? [];
          if (!list.length) continue;
          rows.push(this.header(g, list.length, t, cx));
          if (this.folded.has(g)) continue;
          for (const it of list)
            rows.push(
              g === "shipped"
                ? this.plain(`shipped-${it.key}`, it.key, it.title, t, (cx) => (this.board?.ui === "native" ? this.show(cx, it.key) : this.open(cx, it.key)), div().w(14).flex_none().text_color(t.ansi2).child("✓"))
                : this.plain(`other-${it.pane}`, it.name, it.status, t, (cx) => this.showOther(cx, it), this.dot(t, it.status, false)),
            );
          continue;
        }
        const list = chantiers.filter((c) => c.group === g);
        if (!list.length) continue;
        rows.push(this.header(g, list.length, t, cx));
        if (!this.folded.has(g)) for (const c of list) rows.push(this.row(c, t, mono, activeSpace, false, cx));
      }
    }

    const link = (id, label, onClick) =>
      div().id(id).text_size(11).text_color(t.dim).cursor_pointer().hover((s) => s.text_color(t.foreground)).on_click(onClick).child(label);

    return v_flex()
      .size_full()
      .min_h(0)
      .bg(t.chrome)
      .border_t(1)
      .border_color(t.border)
      .text_size(13)
      .when(!collapsed, (el) =>
        el.child(
          h_flex()
            .px(16)
            .h(41)
            .pt(14)
            .pb(5)
            .flex_none()
            .gap(6)
            .items_center()
            .child(div().text_size(11.5).font_medium().text_color(t.dim).child("cairn"))
            .when(waiting > 0, (el) => el.child(div().text_size(11).font_medium().text_color(t.blocked).child(String(waiting))))
            .child(div().flex_1())
            .child(link("cairn-open", fr ? "Ouvrir" : "Open", (_e, cx) => this.open(cx)))
            .child(this.iconButton("cairn-help", "mods/cairn/help.svg", fr ? "Raccourcis" : "Keys", () => openOverlay({ mod: "cairn-detail", kind: "dialog", props: { help: true }, width: 560, height: 620 }), t))
            .child(this.iconButton("cairn-settings", "mods/cairn/settings.svg", fr ? "Réglages" : "Settings", (cx) => this.openSettings(cx), t)),
        ),
      )
      .when(b === null && !collapsed, (el) =>
        el.child(
          v_flex()
            .px(16)
            .gap(6)
            .text_size(12)
            .text_color(t.dim)
            .child(fr ? "cairn n'a pas encore écrit son tableau." : "cairn has not written its board yet.")
            .child(link("cairn-start", fr ? "Lancer cairn" : "Start cairn", (_e, cx) => this.open(cx))),
        ),
      )
      .when(b !== null && !chantiers.length && !collapsed, (el) =>
        el.child(div().px(16).text_size(12).text_color(t.dim).child(fr ? "Aucun chantier en cours." : "No chantier in progress.")),
      )
      .when(Boolean(this.error) && !collapsed, (el) => el.child(div().px(16).pt(4).text_size(11).text_color(t.ansi1).child(this.error)))
      .child(v_flex().id("cairn-list").flex_1().min_h(0).px(collapsed ? 0 : 8).overflow_y_scrollbar().children(rows));
  }
}
