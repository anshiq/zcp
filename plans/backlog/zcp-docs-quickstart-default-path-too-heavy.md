# ZCP docs Quickstart — default path teaches topology before value

**Surfaced**: 2026-05-06, persona-pass review on the trimmed ZCP docs section. Codex named this as the 4th fundamental failure (alongside "explain-before-answer", "no persona pre-filter", and "boundaries stated as positives").

**Why deferred**: The persona filter added at the top of `quickstart.mdx` ("if you'd prefer a lighter starter — Node, Bun, Python — pick any recipe in the catalog with the AI Agent environment selected") addresses the symptom for readers who notice it. It does not fix the deeper choice: the canonical Quickstart still walks a vibecoder through a 7-service Laravel topology (`appdev` + `appstage` + `workerstage` + `db` + `redis` + `storage` + `zcp`) before they have a single line of code or a deployed URL. The vibecoder persona dropped at the service list — they wanted "JS notes app this weekend," not a platform topology tour.

The structural fix is bigger than this trim pass: either a recipe-chooser flow at the top of the Quickstart (pick the framework you actually want), or a separate lightweight Quickstart variant (single-runtime Node + nothing else, deploy in 5 minutes), or a pre-Quickstart "what shape should I pick?" page that owns the topology vocabulary the current Quickstart reveals as a sidebar gotcha.

**Trigger to promote**:
- A second persona-pass surfaces the same drop-off pattern.
- Public-preview feedback shows vibecoder readers consistently bouncing at the service list.
- A second canonical recipe (Node + minimal) is published and the docs need to reflect "pick your starting point."
- Quickstart conversion analytics (when available) show meaningful drop-off in the create-the-starter-project section.

## Sketch (rough, not committed)

Three options, ordered by scope:

1. **Add a recipe-chooser intro to the existing Quickstart.** Three buttons / cards: "Lightweight (Node + nothing)" / "Standard (one runtime + DB)" / "Showcase (Laravel + queue + Redis + storage)". The rest of the page text adapts to the choice via tabs or per-card paragraphs. ~+30 lines, real value for the vibecoder persona.

2. **Split into two Quickstart pages.** `/zcp/quickstart` becomes the lightweight one (Node + simple, no DB even). `/zcp/quickstart-fullstack` is the current Laravel walkthrough. Sidebar gets two entries. ~+80 lines, doubles the surface area.

3. **Pre-Quickstart shape-picker page.** New `/zcp/concept/pick-a-starter` page that owns the topology vocabulary (one runtime vs dev/stage; one app vs app + managed deps) and recommends a recipe per shape. Quickstart links to it instead of jumping straight to Laravel. ~+50 lines, adds a hop but addresses the vocabulary-too-late friction zCLI veteran also flagged.

The codex review of the consolidation explicitly said: *"the real fix is a lighter first-run path or a recipe chooser before the Laravel topology tour. The filter helps, but it does not fix the product-docs choice of 'maximal showcase as first success path.'"*

## Risks

- A recipe chooser at the top of Quickstart adds decision overhead even for readers who would have followed the canonical path happily. The current Laravel showcase is concrete and that has its own value.
- Splitting into two Quickstart pages dilutes the canonical "this is THE Quickstart" message that the warm-up rule established.
- A pre-Quickstart shape-picker page adds a hop and might not be read in the order intended.

## Refs

- `/tmp/zcp-docs-fundamental-codex-output.md` — codex consolidation review naming this as the 4th failure.
- `quickstart.mdx` filter line: "If you'd prefer a lighter starter — Node, Bun, Python — pick any recipe in the [catalog](https://app.zerops.io/recipes) with the **AI Agent** environment selected" — partial mitigation that ships in commit following c3d9f9d.
- Vibecoder persona report (transcript in `tasks/.../bt6vo73lf.output`): *"I wanted 'JavaScript notes app this weekend,' not a platform topology tour... I stopped around the starter project service list. It felt like I was adopting an architecture before I had made a note form."*
