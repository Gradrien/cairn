---
name: cairn
description: Know where every ticket stands. A developer-tool product page where cairn's own terminal is the hero.
colors:
  graphite-ground: "#08090c"
  terminal-slate: "#15161d"
  hairline: "rgba(232, 236, 255, 0.07)"
  hairline-strong: "rgba(232, 236, 255, 0.12)"
  hairline-hover: "rgba(232, 236, 255, 0.2)"
  paper-white: "#f1f2f5"
  ash-grey: "#a8adb9"
  slate-grey: "#7e8492"
  terminal-text: "#e6e6ef"
  phase-lavender: "#bd93f9"
  lilac-glow: "#d8c4ff"
  waiting-amber: "#ffb86c"
  go-green: "#50fa7b"
  block-red: "#ff6e6e"
  tui-muted: "#8b93c0"
  tui-dim: "#7a89bd"
  tui-rule: "#3a3d50"
  keycap-top: "#2c2f39"
  keycap-base: "#23252e"
typography:
  display:
    fontFamily: "Geist Variable, ui-sans-serif, system-ui, -apple-system, Segoe UI, sans-serif"
    fontSize: "clamp(3rem, 7.4vw, 6rem)"
    fontWeight: 600
    lineHeight: 0.98
    letterSpacing: "-0.045em"
    fontFeature: "\"ss01\", \"cv11\""
  headline:
    fontFamily: "Geist Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(2.25rem, 4.6vw, 3.5rem)"
    fontWeight: 600
    lineHeight: 1.02
    letterSpacing: "-0.035em"
  title:
    fontFamily: "Geist Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "-0.015em"
  lede:
    fontFamily: "Geist Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(1.125rem, 1.6vw, 1.3rem)"
    fontWeight: 400
    lineHeight: 1.5
  body:
    fontFamily: "Geist Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.6
  label:
    fontFamily: "Geist Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.88rem"
    fontWeight: 400
    lineHeight: 1.6
  terminal:
    fontFamily: "Geist Mono Variable, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "0.84rem"
    fontWeight: 400
    lineHeight: 1.6
  keycap:
    fontFamily: "Geist Mono Variable, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "0.82em"
    fontWeight: 500
    lineHeight: 1
rounded:
  keycap: "0.38em"
  term-control: "6px"
  inset: "8px"
  control: "0.75rem"
  panel: "12px"
  window: "14px"
spacing:
  gutter-sm: "20px"
  gutter-lg: "40px"
  page: "74rem"
  section: "clamp(5rem, 11vw, 9rem)"
  nav-height: "4rem"
components:
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.paper-white}"
    rounded: "{rounded.control}"
    padding: "0 1.15rem"
    height: "3rem"
  copy-command:
    backgroundColor: "#17191f"
    textColor: "{colors.paper-white}"
    typography: "{typography.terminal}"
    rounded: "0.8rem"
    height: "3rem"
  keycap:
    backgroundColor: "{colors.keycap-base}"
    textColor: "{colors.paper-white}"
    typography: "{typography.keycap}"
    rounded: "{rounded.keycap}"
    padding: "0 0.45em"
    height: "1.7em"
  keycap-large:
    backgroundColor: "{colors.keycap-base}"
    textColor: "{colors.lilac-glow}"
    rounded: "0.7rem"
    padding: "0 0.8rem"
    height: "3.4rem"
  terminal-window:
    backgroundColor: "{colors.terminal-slate}"
    textColor: "{colors.terminal-text}"
    typography: "{typography.terminal}"
    rounded: "{rounded.window}"
    padding: "1.4rem 1.5rem"
  terminal-panel:
    backgroundColor: "{colors.terminal-slate}"
    textColor: "{colors.terminal-text}"
    typography: "{typography.terminal}"
    rounded: "{rounded.panel}"
    padding: "1rem 1.15rem 1.15rem"
  term-action:
    backgroundColor: "{colors.phase-lavender}"
    textColor: "{colors.terminal-slate}"
    rounded: "{rounded.term-control}"
    padding: "0 0.9rem"
    height: "2.1rem"
  term-action-done:
    backgroundColor: "{colors.go-green}"
    textColor: "{colors.terminal-slate}"
---

# Design System: cairn

## Overview

**Creative North Star: "The Lit Terminal"**

A near-black graphite page with a faint cool bias, played straight as a developer-tool product page in the class of Linear and Raycast. The page itself is quiet: a fine grain over the black, hairlines that fade at their ends, generous section rhythm, two greys of text. All colour belongs to cairn's own terminal, and it means something there: lavender is the phase and the action, amber is "waiting for you", green is go, red is a block. The product is not pictured; it is rendered, live, in mono type inside dark panels made of one material: an edge that catches light from above and a layered lift beneath. Light gathers in the hero, falling on the headline from above, glowing behind the product, and running along the window's edge under the pointer.

Shortcuts are physical. Every key the product uses appears as a rendered keycap with real depth, because the product is operated by keys. Motion is one grammar throughout: a fast ease-out, transform, opacity and blur only, one orchestrated entrance, and scroll-driven assembly only where view timelines exist, with content fully visible at rest.

The system is dark-only (`color-scheme: dark`). Recreated product surfaces are always labelled as example data; the one photograph carries its credit on the page.

**Key Characteristics:**
- Graphite ground, hairline structure, colour only where cairn's TUI would put it.
- Live product surfaces in Geist Mono inside 14px dark windows; marketing copy in Geist.
- Rendered keycaps for every shortcut, at three sizes.
- One surface material for every product panel: lit edge plus layered lift.
- Light lives in the hero; everywhere else it only catches panel edges.
- A fixed fine grain (5%, overlay) gives the black a material.
- Ease-out cubic-bezier(0.23, 1, 0.32, 1), 120 to 320 ms for interaction, longer only for the entrance.

## Colors

A graphite page carrying a terminal palette: the page is neutral, the product's semantic colours do all the speaking.

### Primary
- **Phase Lavender** (phase-lavender): cairn's accent. The current phase, the primary in-terminal action button, focus rings, caret, selection tint (at 32% alpha), step numbers in Install, the logo's top stone, and the one hero light. Never a page-level fill.
- **Lilac Glow** (lilac-glow): the lifted lavender. Legends on large keycaps, keycaps inside terminal frames, hover state of the lavender action.

### Secondary (semantic, terminal only)
- **Waiting Amber** (waiting-amber): "waiting for you". Group headings and reasons for tickets that need a decision, the warn card border, in-progress task glyphs.
- **Go Green** (go-green): go and done. CI passed, push accepted, a sent action, the shipped card border, the "done" pane dot.
- **Block Red** (block-red): the guard refusing a push; the "blocked" pane dot. Rare by design.

### Neutral
- **Graphite Ground** (graphite-ground): the page, the nav's 72% translucent backdrop, the photo's dissolve, `theme-color`.
- **Terminal Slate** (terminal-slate): every terminal window and product panel; dark text on lavender and green buttons.
- **Paper White** (paper-white): headings, strong text, ghost-button labels.
- **Ash Grey** (ash-grey): body copy, ledes, nav links, the hero hint.
- **Slate Grey** (slate-grey): secondary phrases in two-tone headings, small print, the copy prompt, footer text.
- **Terminal Text** (terminal-text): default text inside terminal frames.
- **TUI Muted** (tui-muted) and **TUI Dim** (tui-dim): cairn's own muted and dim text; metadata, keymap footers, `example data` labels, chrome titles.
- **TUI Rule** (tui-rule): section rules and card borders inside the terminal, unpressed in-terminal key chips.
- **Hairlines** (hairline 7%, hairline-strong 12%, hairline-hover 20%, all of #e8ecff): section rules (hairline-strong, fading to transparent over the outer 20% on both sides), row dividers inside sections, ghost-button rings and their hover, the left separators of the integrations strip. The surface edge is a vertical ramp of the same tint: 18% at the top, 4 to 6% through the middle, 9% at the bottom.

### Named Rules
**The Meaning-Only Colour Rule.** Lavender, amber, green and red appear only where cairn's TUI would show them, with the same meaning. The page chrome stays graphite and grey.

**The Two-Tone Heading Rule.** A headline may split into a statement in Paper White and a turn in Slate Grey ("Five agents. Five panes. / One list."). Grey, never an accent.

## Typography

**Display Font:** Geist Variable (via @fontsource-variable/geist, with ui-sans-serif, system-ui fallback)
**Body Font:** Geist Variable
**Label/Mono Font:** Geist Mono Variable (via @fontsource-variable/geist-mono, with ui-monospace, SFMono-Regular, Menlo fallback)

**Character:** Geist is tight and engineered at display sizes (negative tracking, 600 weight) and neutral in running text; Geist Mono is the product's own voice, used for anything the terminal would print and for keycap legends. Stylistic sets `ss01` and `cv11` are on for Geist; slashed zero is off in mono.

### Hierarchy
- **Display** (600, clamp(3rem, 7.4vw, 6rem), 0.98, -0.045em): the hero headline only, set as stacked lines, balanced.
- **Headline** (600, clamp(2.25rem, 4.6vw, 3.5rem), 1.02, -0.035em): one per section, balanced. The closing section enlarges it to clamp(2.75rem, 6vw, 5rem).
- **Title** (600, 1.25rem, 1.3, -0.015em): step names, keymap groups, tracker columns.
- **Lede** (400, clamp(1.125rem, 1.6vw, 1.3rem), 1.5, max 36ch): one per hero or closing.
- **Body** (400, 1rem, 1.6, max 60ch, pretty-wrapped): section copy in Ash Grey; `strong` lifts to Paper White at 500.
- **Label** (400, 0.88rem): small print under CTAs, footer, captions.
- **Terminal** (400 mono, 0.76 to 0.86rem, 1.5 to 1.7): product panels; the hero window scales clamp(10.5px, 1.05vw, 13.5px).

### Named Rules
**The Mono-Means-Product Rule.** Mono type is for what cairn or the shell prints, commands, file paths, ticket keys and keycaps. Marketing sentences are never set in mono.

## Layout

A single centred column of 74rem plus gutters (20px under 760px, 40px above, each widened by safe-area insets). Sections are separated by a 1px rule that fades out toward both edges of the viewport and padded clamp(5rem, 11vw, 9rem) top and bottom; the hero is tighter (clamp(2.75rem, 5.5vw, 4.5rem) top) so the product enters the first viewport. Content sections pair copy with a product surface in a two-column grid from about 1000px (ratios 0.8/1.2 to 1.1/0.9, alternating side), collapsing to one column below. Lists of rules (Guard facts, keymap groups, install steps) are hairline-ruled rows with a fixed term column (15 to 16rem) and a fluid consequence column. An integrations strip (name over its role, each cell opened by a left hairline) runs 2 columns, then 4 from 900px. The footer sits under the same fading rule: brand and one sentence, then two link columns (Product, Get started), stacked under 760px and 1.6/0.7/0.7 above. The sticky 4rem nav gains its blurred translucent backdrop over content. Breakpoints in use: 720/760px (nav links, gutter), 820px (two-pane hero terminal), 860 to 1000px (section grids), 900px (closing photo layout).

## Elevation & Depth

Hybrid. The page is flat under a fixed fine-grain overlay (fractal noise, 5% opacity, overlay blend); depth belongs to objects the visitor would touch or that represent the product. Every product panel is made of the surface material: a transparent 1px border painted with the vertical edge gradient (`--edge`, border-box) under the panel fill (padding-box), and the layered lift shadow (`--lift`). The agent mosaic uses the edge on a darker #0b0c10 fill without the lift. The hero window additionally sits in perspective (rotateX 5deg at rest, flattening to -5deg as it scrolls through, where view timelines exist) over a lavender backlight and a smaller pool of light beneath it, under a wide pale top light that falls on the headline; on fine pointers a lilac light runs along its edge and a faint sheen follows inside. Keycaps carry the strongest physical depth: a top highlight, a darker inner bottom lip, a short solid skirt and a soft drop.

### Shadow Vocabulary
- **Lift** (`inset 0 1px 0 rgba(255,255,255,0.06), 0 1px 2px rgba(0,0,0,0.5), 0 24px 48px -16px rgba(0,0,0,0.7), 0 60px 120px -40px rgba(30,16,70,0.55)`): every product panel (hero window, Guard transcript, Review PR, cairn list), paired with the edge border.
- **Edge** (`linear-gradient(180deg, rgba(232,236,255,0.18) 0%, rgba(232,236,255,0.06) 28%, rgba(232,236,255,0.04) 70%, rgba(232,236,255,0.09) 100%)` on a transparent 1px border-box): the light-catching rim of panels and command wells.
- **Pointer edge light** (`radial-gradient(var(--gr) circle at var(--mx) var(--my), rgba(225,210,255,0.85), transparent)` as an extra border-box layer, radius 0 to 380px): the hero window only.
- **Keycap** (`inset 0 1px 0 rgba(255,255,255,0.09), inset 0 -2px 0 rgba(0,0,0,0.35), 0 1px 2px rgba(0,0,0,0.6)`): inline keycaps.
- **Keycap large** (`inset 0 1px 0 rgba(255,255,255,0.1), inset 0 -4px 0 rgba(0,0,0,0.38), 0 2px 0 #0a0b0e, 0 8px 18px rgba(0,0,0,0.45)`): keymap and hero hint caps; pressed state drops 2px and collapses the skirt.
- **Command well** (`inset 0 1px 0 rgba(255,255,255,0.05), 0 12px 32px -12px rgba(0,0,0,0.7)` on the edge border): copy-command boxes; hover adds a 4px halo of 7% lavender.

### Named Rules
**The Hero Holds the Light Rule.** Light sources live in the hero only: the pale top light, the lavender backlight and pool under the window, and the pointer light on its edge. Elsewhere, light only catches panel edges through the surface material; no glows, washes or coloured section backgrounds.

**The One Material Rule.** Every product panel uses the same material, edge border plus lift. No plain hairline rings or bespoke shadow stacks on new panels.

**The Keys Are Physical Rule.** The solid 2px skirt under a keycap is the key's own wall and lives on keycaps only; no card, button or panel gets a zero-blur offset shadow.

## Shapes

Soft, small radii scaled to the object: keycaps 0.38em (inline) to 0.7rem (large), in-terminal controls 6px, inset blocks 8px, buttons 0.75rem, copy wells 0.8rem, panels 12px, terminal windows 14px. Status dots and avatars are circles. Window chrome uses three 11px grey dots. Panel borders are the lit edge on a transparent 1px border; buttons use hairline rings (`box-shadow: inset 0 0 0 1px`); section rules fade at their ends; never heavy strokes; inside the terminal, cards use cairn's 1.5px rule-coloured border, swapped to amber or green for state.

## Components

### Buttons
- **Shape:** gently rounded (0.75rem), 3rem tall; the nav variant is 2.25rem at 0.6rem.
- **Ghost (the only page-level button):** transparent with a 12% hairline ring, Paper White label, SVG icon at 1.1rem.
- **Hover / Focus:** hover (fine pointers only) adds 4% white fill and lifts the ring to 20%; `:active` scales to 0.97 over 160ms ease-out; focus is a 2px lavender outline offset 3px.
- **Terminal action:** inside product frames the primary action is a lavender chip with dark mono label (600, 0.82rem, 6px radius); on success it turns green and its label swaps with a blur-and-slide crossfade.

### Copy Command (signature)
The primary call to action is the install command itself, in a dark command well (a #17191f to #111318 fill on the lit edge) with a grey `$` prompt and a 2.5rem icon button; on fine-pointer hover a soft 4px lavender halo appears over 240ms. Commands wrap only at spaces: each word is unbreakable, and URLs may break only after a slash past the host, so what you read is exactly what you copy. A hanging indent keeps wrapped lines aligned after the prompt. On copy, the copy icon blurs out and a green check scales in for 1.6s, the button's label becomes "Copied"; if the clipboard is unavailable the command text is selected instead. Two sizes: lg (3rem, hero and closing) and md (2.75rem, 0.92rem, inline in sections).

### Keycaps (signature)
Rendered keys in Geist Mono 500 on a top-lit gradient (keycap-top to keycap-base). Three sizes: inline (1.7em, in running text and inside terminal frames, where the legend turns Lilac Glow), hint (2.6rem, the hero's "Press V" invitation, with a faint lavender under-glow and a slow 2.4s press loop that stops once pressed), and keymap (3.4rem, Lilac Glow legend, wide caps for multi-character legends). Large caps press down 2px on hover or real keydown. Every shortcut shown on the page is a keycap, never plain text.

### Terminal Window (signature)
Terminal Slate panels in the surface material (14px windows, 12px panels), in Geist Mono, that recreate cairn's screens with its exact semantic colours: a header row, section rules drawn as `── label ───` with a TUI Rule line, a selected-row band (#2c2e3c), bordered status cards, and a key footer. The hero version adds window chrome and is live: v approves, a allows the push, r replays; each step repaints the detail pane with one 260ms settle from a 3px blur. On fine pointers without reduced motion, a light follows the pointer along the hero window's edge: its radius (a registered `<length>` property) grows from 0 to 380px over 500ms on enter, and its position eases toward the pointer at 0.14 per frame so it trails the hand. The list pane is hidden under 820px and the detail pane carries the story alone.

### Navigation
Sticky, 4rem, brand (logo plus lowercase "cairn", 600, -0.02em) left; links in Ash Grey at 0.93rem, Paper White on hover; a compact ghost GitHub button right. Section links hide under 720px, the GitHub label under 420px.

### Footer
Under a fading rule: the brand (20px logo) with one Slate Grey sentence, then two link columns headed in Paper White 500 (Product, Get started) with Slate Grey links that turn Paper White on hover.

### Integrations Strip
The tools cairn builds on, as a row of cells: name in Ash Grey (600, 1.15rem, -0.02em) over its role in Slate Grey (0.85rem), each cell opened by a 12% left hairline. Names are set as type, not logos.

### Example-Data Labels
Every recreated product surface carries a small right-aligned label in TUI Dim, inside its frame: `example data`, or `example session` for shell transcripts. Example tickets use invented keys (ENG-412, EIDO-7) and never real customer data.

### Photo and Credit
The single photograph (a cairn on the Plateau d'Emparis) is full-bleed behind the closing section, dissolved into Graphite Ground on every side the text needs, and lazy-loaded at responsive widths. Where view timelines exist and motion is allowed, it drifts from -4% to 4% vertically at 1.08 scale as the section passes, slower than the page. Its credit sits on the page in Label size, Slate Grey: place, author, licence link, a note that it was colour-graded, and a source link.

### Motion
- **Easing:** ease-out cubic-bezier(0.23, 1, 0.32, 1) for everything that moves; plain `ease` for colour and opacity fades.
- **Entrance:** the hero headline lines, lede and CTA rise 14px out of an 8px blur over 900ms, staggered 90ms; the product window then settles from rotateX 14deg, 28px down and 97% scale to its 5deg rest over 1100ms after 380ms.
- **Pointer light:** the hero window's edge light grows over 500ms ease-out and trails the pointer with a 0.14 per-frame lerp; fine pointers only, never under reduced motion.
- **Scroll assembly:** in Panes, where `animation-timeline: view()` exists, the five agent panes quiet to 45% opacity while the cairn list assembles row by row, decisions first (each row slides 28px from the left out of a 4px blur, offset 7% of range per row). The closing photo drifts on its own view timeline. Without view timelines everything is static and fully visible.
- **Interaction:** 120 to 320ms, transform, opacity and filter only.
- **Reduced motion:** a global rule collapses every animation and transition to 0.01ms, the demo's timers fire immediately and its settle is skipped, the pointer light is never attached, and the Panes assembly and photo drift are gated behind `prefers-reduced-motion: no-preference`.

## Do's and Don'ts

### Do:
- **Do** keep the page graphite and grey, and put lavender, amber, green and red only where cairn's TUI would, with the same meaning.
- **Do** show the product live in mono inside a Terminal Slate panel rather than as a screenshot.
- **Do** build every product panel from the surface material: transparent 1px border painted with the edge gradient, the panel fill on padding-box, and the lift shadow.
- **Do** render every keyboard shortcut as a keycap.
- **Do** separate sections with the 1px rule that fades to transparent at both ends, not a full-width border.
- **Do** label every recreated product surface `example data` (or `example session`) in TUI Dim inside its frame.
- **Do** credit any photograph on the page: author, licence link, source, and whether it was graded.
- **Do** wrap commands only at spaces and keep the copied text identical to the visible text.
- **Do** use ease-out cubic-bezier(0.23, 1, 0.32, 1), animate transform, opacity and blur only, and leave content visible at rest when scroll timelines are unsupported.
- **Do** use text glyphs (✓ ✗ ◐ ● →) inside terminal frames, where cairn itself prints them; use the 1.6px-stroke SVG icon set everywhere else.

### Don't:
- **Don't** add light sources outside the hero, gradient washes or coloured section backgrounds; beyond the hero, light only catches panel edges.
- **Don't** put an eyebrow, kicker or pill above a headline.
- **Don't** give cards, buttons or panels a zero-blur offset shadow; that skirt belongs to keycaps.
- **Don't** set marketing sentences in mono, or product output in Geist.
- **Don't** invent users, stars, quotes or metrics, and never show real ticket data in a product recreation.
- **Don't** truncate or ellipsize a command the visitor is meant to copy.
