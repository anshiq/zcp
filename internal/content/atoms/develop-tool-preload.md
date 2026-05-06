---
id: develop-tool-preload
priority: 1
phases: [develop-active]
environments: [container]
title: "Pre-load deferred tool schemas in one ToolSearch call"
---

### Pre-load tool schemas in one batch

`zerops_*` tools are deferred — schemas load via `ToolSearch`. If you
arrived in develop fresh (compaction recovery, or develop without prior
bootstrap), batch-load before iterating:

```
ToolSearch query="select:zerops_workflow,zerops_deploy,zerops_verify,zerops_logs,zerops_events,zerops_manage,zerops_env,zerops_discover"
```

`select:` accepts a comma-separated list and returns all matching
schemas in one round-trip. Loading sequentially defeats the point.
