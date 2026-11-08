# Product

<!-- impeccable:product-schema 1 -->

## Platform

web (the landing page). The product itself is a terminal UI (Go, Bubble Tea) that runs inside herdr.

## Stack

Landing page: Astro, static build, in `site/`. Hosting not decided yet; the build must deploy anywhere static (GitHub Pages, Netlify, Cloudflare Pages).

## Users

Developers who already run several coding agents in parallel (Claude Code, Codex…), one per ticket, ideally inside herdr. With five agents on five tickets they lose track: they open pane after pane to learn whether each agent is planning, building, stuck on a question, or done and waiting for them.

## Product Purpose

cairn is a mouse-driven TUI that sits next to herdr. herdr says which agent is working, waiting or asleep; cairn says where the work is: each ticket's phase, plan, proofs, pull request, and what is waiting for the human's decision. The tickets that need you come first, with the exact thing to decide and the button that decides it. Success for the landing page: a visitor who runs agents in parallel installs cairn (copies the install command, stars the repository).

## Positioning

- A ticket is a **chantier** (French for a work site): ticket + git worktree + herdr workspace + agents + plan file + pull request, moving through Plan → Build → Review → Shipped.
- **Nothing ships without you, enforced**: a Claude Code hook (`cairn guard`) blocks `git push` and `gh pr create/ready/merge` on a chantier's branch until the human allows the exact commit they reviewed. Once the PR is open, fixes pass freely; merging stays human.
- **Proofs, not claims**: agents record the commands they really ran, with results and dates.
- **Tool-agnostic**: several repositories on one screen, each with its tracker: Linear, Sprintosaur (an in-house tracker), or none (tickets born in cairn from a title: EIDO-1, EIDO-2…). PRs are found on GitHub by branch.

## Operating Context

- Runs in a terminal pane in its own herdr workspace (`prefix+a` toggles it; a herdr plugin and sidebar button).
- Agents keep `~/.cairn/chantiers/<KEY>.md` true through the `cairn` skill: front matter (`phase`, `attente`, `push`), tasks `[ ]` `[~]` `[x]`, `## Questions`, `## Effets de bord` (side effects), `## Preuves` (proofs).
- GitHub through `gh`: PRs, CI, reviews, private images (rendered inline with the Kitty graphics protocol in Ghostty, Kitty, WezTerm).
- Install: `git clone https://github.com/Gradrien/cairn && cd cairn && ./install.sh --herdr`; then `cairn add` in each repository (`--tracker linear --key ENG` for Linear). Needs Go, `gh` logged in, herdr.

## Capabilities and Constraints

- Keys: `enter` go to agent, `p` plan, `n` new ticket, `v` approve, `e` answer questions, `a` allow push, `x` close shipped, `c` PR comments, `f` fix CI (sends failing logs to the agent), `d` diff, `o` open ticket, `s` settings, `t` board, `ctrl+k` palette, `?` help, `l` language.
- Merge checklist: tasks done, side effects checked, CI green, review approved, no conflicts, not a draft.
- "New since your visit" markers; herdr notifications (waiting for you, CI broken, review verdict, shipped).
- Shipped chantiers clean up after themselves (worktree, workspace, plan archived) only when nothing can be lost.
- Interface in French and English, switched live.
- The repository will be made public (decision taken for the landing page CTA); until then GitHub links 404 for visitors.
- Open: license, hosting, domain.

## Brand Commitments

- Name: **cairn**, lowercase. A cairn is a pile of stones that marks a trail.
- The French term **chantier** is part of the product vocabulary.
- Section rhythm of the README (eyebrow + short headline + short paragraph) was inspired by blume.codes; the user named blume.codes as a reference for the landing page.
- Landing page: the user chose the category standard (the developer-tool product page) on purpose, over themed worlds. Its craft bar is Linear, Raycast and blume.codes.

## Evidence on Hand

- Screenshots in `docs/` (overview, plan, pr, comments, board): real cairn screens, but they show internal Yalink tickets (YAK-…, Yak GPT, Superforest). Do not publish them; recreate screens with example data instead.
- No testimonials, users counts, stars, benchmarks or press. Do not invent any.

## Product Principles

1. The human decides; cairn makes the decision obvious and one key away.
2. Show where the work is, with evidence, not what the agent says about it.
3. Stay out of the way: one command to remember, everything else is a button.
4. Work with whatever tools a team already has, or none.
