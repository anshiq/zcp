# Showcase scenario specification

A `tier=showcase` recipe MUST produce an SPA (in the frontend codebase)
that visibly demonstrates EVERY managed-service category the recipe
provisions. Read the SPA as an extended health dashboard: one **card**
per managed-service category plus a leading Status strip, each card
showing the service's live state AND housing the interactive demo
that proves the category. The porter clicks the published recipe and
sees real numbers — row counts, hit/miss ratios, queue depth, object
count, indexed documents — before they touch anything.

This spec is engine-injected, framework-agnostic. A NestJS recipe, a
Laravel recipe, and a Rails recipe all implement the same dashboard
shape — only the framework idioms underneath the cards change.

## Mandate: one card per managed-service category

The frontend codebase MUST render these cards:

| Card | Proves | Live-state | Mandatory observable |
|------|--------|------------|----------------------|
| **Status strip** | per-service liveness | Row per managed service (api, db, cache, broker, search, storage); dot+label `ok`/`down` via `--zerops-success`/`-error`. Mandatory when any managed service is provisioned. | The strip is itself the demonstration. |
| **Items / DB** | crud through DB | Row count badge from `SELECT COUNT(*)` (or framework equivalent). | Create form + list; row count survives container restart; counter increments on create. |
| **Cache** | cache-demo (read-through) | `X-Cache: HIT/MISS` colored badge (success on HIT, warning on MISS) AND a hit-counter + miss-counter pair (both required, not either/or). | Trigger fires the demo endpoint; first call shows `MISS`, second `HIT`; the badge value is the load-bearing proof — counters are supplementary. |
| **Queue / Broker** | queue-demo via worker | Pending + processed counters + chip list of last 3 events. | Publish trigger; processed counter increments AND the indexed document appears in the Search card within seconds. The two-card integration is required. |
| **Storage** | storage-upload | Object count + chip list of recent uploads (filename + size). | Upload affordance; on success the file appears in the chip list AND object count increments. Browser-walk MUST observe the click handler firing — curl alone is insufficient. |
| **Search** | search-items (full-text) | Indexed-doc count badge. | Search box + ranked results; result count matches rendered list length. |

Scope the cards to the managed-service categories the recipe actually
provisions. A recipe without a queue/broker doesn't render a Queue
card; a recipe without object-storage doesn't render a Storage card.

The Status strip is the leading element of the dashboard — it answers
"is anything wired?" before the porter touches a CRUD form.

## Card anatomy

Every category card has the same top-to-bottom shape: header (category
name in `headline-md`, optional service-type subtitle in muted
`body-sm`, status dot mirroring the Status strip), live-state element
(counter in `headline-lg` + `body-sm` label, badge in status tokens,
or both — the Cache card requires both, the Queue card requires both
counters and a chip list), demo trigger (button or inline form, never
a modal or separate route), result display (chips, plain `<ul>`, or
ranked list). All cards share width, padding, and radius via
`var(--zerops-radius-card)` and `card-bg-light/dark` from the
feature brief's design-tokens table — never hardcode dimensions or
redefine tokens locally.

## Visualization vocabulary (closed list)

The dashboard framing is a magnet for chart libraries. It is wrong for
this brief. Allowed: plain text counters (`headline-lg` + `body-sm`
label), colored badges using `--zerops-success`/`-warning`/`-error`/
`-primary`, bullet/chip lists capped at 3-5 items (no virtualization,
no infinite scroll), 8px status dots next to labels.

Forbidden: chart dependencies (recharts, visx, chart.js, d3, victory,
apexcharts, plotly, nivo, lightweight-charts, observable-plot);
generated chart components (`<LineChart>`/`<BarChart>`/`<Sparkline>`/
`<Gauge>`/`<Donut>`); hand-rolled SVG/canvas/CSS sparklines (gradient
bars, rotated divs, `<polyline>` paths, `stroke-dasharray` ring/arc
progress); HTML viz primitives standing in for charts (`<progress>`,
`<meter>`, `aria-valuenow` divs styled as bars) — the verifier reads
`data-test` text content, none of these are text; any `npm install`/
`yarn add`/`composer require` for a visualization package; animated
counters that interpolate on transition; CSS animations on counter or
badge elements (`animate-pulse`, `animate-bounce`, `transition` on
the value); emoji used as a status indicator (use the colored dot +
text label, not 🟢/🔴). Render numbers directly; the verifier reads
the DOM at click+wait, not mid-tween.

## Live-state pattern (no websockets, no fake state)

"Live state" means counters and badges reflect the actual managed
service state AT THE TIME OF FETCH — not a real-time stream. On
mount, each card fetches once via whatever the backend feature pass
already exposes — the existing collection endpoint (`.length` for
the count) or a dedicated `*-state` route if shipped. Do not invent
endpoints. After demo trigger: re-fetch the live-state value
immediately on trigger success — without the re-fetch the counter
goes stale and the verifier sees no change. Async convergence (queue → search): the
publishing card polls the search card's state endpoint at 500ms
intervals up to a 5s ceiling, then surfaces `processed` once the
indexed document appears; no background polling outside that bounded
window. Forbidden: websockets, server-sent events, unbounded
setInterval timers, client-only optimistic state reconciled silently,
client-only fake counters that increment without a backend
round-trip. The recipe does not ship websocket infra; do not invent
it.

## Design priorities

- **Demonstration-first content.** Effort goes on what each card
  demonstrates. No hero sections, marketing copy, decorative
  iconography, or chrome that exceeds the viewport.
- **Design tokens, not custom systems.** Use the Zerops design tokens
  via the Tailwind utility shapes from the feature brief; do not
  author a custom design system or add a second CSS framework.
- **Real data.** Cards exercise the actual deployed managed services
  — real rows, real worker output, real index hits. No mocks, no
  client-only fixtures.
- **Card uniformity over decoration.** Same card shape with
  category-specific content reads as polish; per-card custom styling
  reads as chaos.

### Stable selectors for browser-walk verification

Per-snapshot DOM refs go stale across `zerops_browser` calls (silent
no-op clicks). Use stable attribute selectors. Add `data-feature` to
interactive elements (`publish`, `upload`, `search`, `create-item`,
`cache-fetch`). Add `data-test` to every counter and badge — the
canonical set: `[data-test="items-count"]`,
`[data-test="cache-hits"]`, `[data-test="cache-misses"]`,
`[data-test="cache-state-badge"]` (carrying the literal `HIT` or
`MISS` text), `[data-test="queue-processed"]`,
`[data-test="storage-objects"]`, `[data-test="search-indexed"]`,
`[data-test="status-<service>"]`. Browser-walk targets by attribute,
not per-snapshot ref.

## Layout: card grid sized to the headless viewport

`zerops_browser` runs headless Chrome at a small default viewport;
treat sizing as conservative-minimum and do not assume a specific
pixel value. Click events dispatch at element-center coordinates
without auto-scrolling; elements below the fold receive clicks at
out-of-bounds coordinates. Single-column multi-panel scrolls have
failed verification before; the layout below avoids that by
construction.

Canonical layout: a **two-column card grid under a full-width Status
strip**. Status strip on top; row 1: Items / Cache; row 2: Queue /
Storage; row 3: Search (full-width). The Status strip + row 1 MUST
fit within the viewport without scrolling — verify on first render
that the Items card's CTA is reachable. Card bodies stay compact
when collapsed (live-state + trigger visible; result display may
expand on demand). When a card grows on interaction, re-target by
`data-feature` selector after expansion, not by coordinates.

Fallback — single-column accordion when row 2 + row 3 still push the
active panel below the fold: cards collapse to header + live-state +
chevron; clicking a header expands and collapses the prior one;
default-open is Items; Status strip stays uncollapsed. The accordion
keeps the active panel above the fold by construction — choose it
when in doubt. Avoid a multi-card single-column scroll: the verifier
dispatches clicks out-of-bounds and verification takes 2-3× longer
with partial fact coverage.

## Per-card browser-verification

After implementing the cards, run `zerops_browser` against the SPA and
exercise EACH card. For each one, record one fact:

```
zerops_recipe action=record-fact slug=<slug>
  topic=<frontend-cb>-<category>-browser
  symptom="<what you saw + whether the demonstration signal was visible AND the counter delta held>"
  mechanism="zerops_browser"
  surfaceHint=browser-verification
  citation=none
  scope=<frontend-cb>/<category>
  extra.console=<digest>
  extra.screenshot=<path or none-snapshot-only>
```

Mandatory facts, one per card — `<frontend-cb>-<category>-browser`:
status (every provisioned service `ok`, any `down` is a wiring
regression); items (create works + `[data-test="items-count"]`
increments by 1, AND row count survives a `zerops dev restart` —
read counter, restart container, browser-walk again, counter holds);
cache (`[data-test="cache-state-badge"]` reads `MISS` first call,
`HIT` second call; `[data-test="cache-hits"]` and
`[data-test="cache-misses"]` both increment to reflect the two
calls); queue (publish fires + `[data-test="queue-processed"]`
increments + indexed doc appears in `[data-test="search-indexed"]`
within 5s); storage (upload click fires + `[data-test=
"storage-objects"]` increments + file appears in the chip list —
curl alone is insufficient); search (query returns ranked hits,
result count matches rendered list length).

Verification protocol for any card with a counter: read the counter
selector → click `[data-feature="<name>"]` → wait for response (up
to 5s on async) → re-read the same counter selector → assert delta
(typically +1). The counter delta is the canonical
click-caused-state-change signal but does NOT replace the
category-specific mandatory observable (X-Cache `HIT`/`MISS` badge
text, ranked results, queue→search integration, upload-handler-fires,
row-count-survives-restart). Both must hold; the load-bearing proof
on the Cache card is the badge text, not the counters.

Any browser walk producing console errors is a regression — fix
before close. The feature_kinds taxonomy names the backend endpoints;
the cards are the frontend's responsibility — a queue-demo backend
that's never visualized fails this scenario spec even if curl proves
round-trip. The dashboard is the deliverable.
