---
id: greenfield-website-from-brief
description: |
  Greenfield single-page website triggered by a richly-worded user
  brief: design-tokens via `npx`, framework hint, "make analysis first"
  pull, Czech-only locale. Reproduces the real-world failure mode where
  rich context distracts the agent from routing through bootstrap and
  tempts /tmp scaffold + multi-section pre-platform prose. Tests
  whether the routing table + smells + discovery floor in
  claude_shared.md catch the drift early.
seed: empty
tags: [bootstrap, greenfield, single-page-website, brief-shape, prose-drift-trap, tmp-scaffold-trap, czech-locale]
area: bootstrap
retrospective:
  promptStyle: briefing-future-agent
notableFriction:
  - id: pre-platform-prose
    description: |
      Rich brief + "analyze first" pull tempts multi-section prose
      analysis (framework recommendation, IA proposal, copy questions,
      legal pages) before any zerops_* call. Surfaces whether the
      routing table and the "drafting multi-section prose analysis"
      smell in claude_shared.md catch the drift on turn 1.
  - id: tmp-scaffold-drift
    description: |
      `npx designdotmd add ...` reference tempts running it in /tmp/
      to inspect the template before any service exists. Surfaces
      whether the "/tmp or random scratch dirs for app code" smell
      deters or whether agent still scaffolds out-of-mount.
  - id: late-or-missing-bootstrap
    description: |
      Empty project; no service exists. Agent must reach
      zerops_workflow action="start" workflow="bootstrap" within the
      first turn or two. Surfaces whether the routing-table row
      "No service yet, or infra/topology change" + discovery floor
      classify "build a website" correctly when the prompt doesn't
      say the word "bootstrap" or "service".
---

I have a brand-style guide for a local sports club and want a small one-page informational website. I'd like to use the design tokens from `npx designdotmd add my-brand` (it ships colors, typography, and component rules). Czech-only content, no pricing page yet, the contact form mechanism can wait.

Make analysis what needs to be done first.
