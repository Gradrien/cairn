---
version: 1
slug: "site-index-html"
primary_target: "site/index.html"
related_targets: []
---

# Landing page (site/)

Scope: the public landing page of cairn. Mode: Persuade.
Audience: developers who run several coding agents in parallel and lose track of them. Action: copy the install command, star the repository. Proof: the product itself, demonstrated live with labelled example tickets; no invented users, stars or quotes.
Constraints: Astro, static build in `site/`; GitHub links assume the repository goes public; never publish the docs/ screenshots (Yalink data).

## Direction contract

THESIS: The developer-tool product page, played straight at the craft of Linear, Raycast and blume.codes. It refuses the screenshot-in-a-frame hero: the product is live in the first viewport and answers your keys.
OWN-WORLD: Near-black graphite ground with a faint cool bias; cairn's own TUI colours carry meaning: lavender for phase and action, amber for "waiting for you", green for go. One quiet radial light behind the product. Geist for text, Geist Mono for the product and code. Rendered keycaps with real depth for every shortcut. Hairline borders, 12px radii on windows only.
STORY: The visitor sees five agents become one list where the decision that needs them comes first; believes nothing ships without them, because the guard is shown blocking a real push; copies the install command.
FIRST VIEWPORT: Pill (new: Linear and no-tracker mode), headline "Know where every ticket stands.", one sentence, primary action = the install command with copy, secondary = GitHub. Below, at full container width, cairn's screen in a window, slightly tilted toward the visitor, lit from behind; a keycap row invites "press v". Pressing v, a, enter moves the example ticket through Plan, Build, Review, Shipped.
FORM: the category standard (canon), chosen by the user; first on no ordered list. seed:cbd24fc45eb98e7a
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Signature interaction: the live hero ticket driven by real cairn keys (v approve, a allow push, enter go to agent), each press advancing phase, tasks and checklist with one orchestrated motion.
Motion grammar: ease-out (0.23, 1, 0.32, 1), 160–320 ms, transform and opacity only; scroll reveals only where supported by view timelines, content visible at rest; reduced motion removes movement, keeps fades.

Unresolved: hosting, domain, license.
