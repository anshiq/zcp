---
id: bootstrap-tool-preload
priority: 1
phases: [bootstrap-active]
environments: [container]
title: "Pre-load deferred tool schemas in one ToolSearch call"
---

### Pre-load tool schemas in one batch

`zerops_*` tools are deferred — schemas load via `ToolSearch`. Loading
them sequentially burns 2-3 round-trips before the first real action.
On the first turn, batch-load:

```
ToolSearch query="select:zerops_workflow,zerops_discover,zerops_import,zerops_deploy,zerops_verify,zerops_logs,zerops_events,zerops_dev_server"
```

`select:` accepts a comma-separated list and returns all matching
schemas in one round-trip. Loading sequentially defeats the point.
