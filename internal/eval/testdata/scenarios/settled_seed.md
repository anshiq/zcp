---
id: test-settled
description: Settled seed scenario for parser tests
seed: settled
fixture: fixtures/python-broken.yaml
expect:
  mustCallTools:
    - zerops_workflow
  workflowCallsMin: 4
followUp:
  - "Did you read the runtime logs?"
---

# Task

A service in the project is failing. Diagnose and fix.
