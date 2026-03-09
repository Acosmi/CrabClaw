---
name: gateway
description: "Restart, apply config, run updates, and manage system health with safe change procedures"
tools: gateway
metadata:
  tree_id: "system/gateway"
  tree_group: "system"
  min_tier: "task_multimodal"
  approval_type: "exec_escalation"
---

# Gateway — System Configuration & Health

## Config Change Procedure

1. `config.get` — record current state (hash + key fields)
2. `config.schema` — validate types and options
3. `config.patch` — incremental edit (one theme block per call)
4. Only use `config.set` for full replacement
5. `config.apply` — immediate effect + verify restart/results
6. On failure: rollback last patch first

## Rules

- Avoid touching multiple high-risk blocks (auth, models, security) in one change
- Minimal diff for sensitive items
- Requires **exec_escalation** approval

## System Observability

| Operation | Purpose |
|-----------|---------|
| `health` + `status` | Coarse baseline (not full root cause) |
| `logs.tail` | Error window analysis (watch for truncation signals) |
| `system.reset.preview` | Always preview before any reset |
| `system.backup.restore` | Preferred for config rollback |
| `system.reset` | Only after confirmed root cause |

## Critical Boundaries

- `health`/`status` give coarse baseline only, not full root cause
- `logs.tail` truncation/rotation may itself be signal
- `system.reset` L1 only deletes `exec-approvals.json`, truncates audit log, resets UHMS + escalation — NOT full system clear
