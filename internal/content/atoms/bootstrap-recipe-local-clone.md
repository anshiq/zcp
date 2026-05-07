---
id: bootstrap-recipe-local-clone
priority: 1
phases: [bootstrap-active]
routes: [recipe]
environments: [local]
steps: [discover]
title: "Recipe local — clone repo into CWD"
coverageExempt: "recipe+local+discover — covered by recipe-local-flow design (Theme 1) + flow-eval-local scenarios"
---

### Local mode replaces the dev runtime with your CWD

In local mode there is no SSH-in dev workspace. Your CWD becomes the source-of-truth checkout.

```
1. Verify CWD is empty (or contains only ZCP state):
     ls -A
   If it has unrelated files, stop and ask the user — never clone over their work.

2. Clone the recipe app repo:
     git clone {{recipe.repo}} .
```

The upstream remote stays connected. To use your own remote later:

```
git remote set-url origin <your-repo-url>
```

`zerops.yaml` arrives in the cloned tree as-is from the recipe. ZCP transforms the project import.yml separately at provision (drops `appdev`, strips `buildFromGit`); your local `zerops.yaml` is untouched.
