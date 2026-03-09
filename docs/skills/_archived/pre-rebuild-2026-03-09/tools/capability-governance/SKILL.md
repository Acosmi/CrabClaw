---
name: capability-governance
description: "Inspect, diagnose, and manage the capability tree. Phase A: read-only (tree, inspect, validate, diagnose, generate_prompt, generate_allowlist). Phase B: write paths (propose_register, propose_update, propose_routing, propose_binding, apply_patch)."
tools: capability_manage
---

## capability_manage (Phase A + Phase B)

The `capability_manage` tool provides self-inspection, diagnostic, and controlled write capabilities for the capability tree — the single source of truth for all tool metadata.

### Phase A: Read-Only Actions

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `tree` | View the full tree or a subtree | `node_id` (optional), `depth` (optional) |
| `inspect` | View single node with all 7 dimensions | `node_id` (required) |
| `validate` | Three-level validation (L1/L2/L3) | `level` (optional, 0=all) |
| `diagnose` | Drift diagnosis across 9 derivation targets | — |
| `generate_prompt` | Generate prompt section for a tier | `tier` (required) |
| `generate_allowlist` | Generate tool allowlist for a tier | `tier` (required) |

### Phase B: Write Actions (Patch-Based)

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `propose_register` | Propose registering a new tool node | `node_spec` (required) |
| `propose_update` | Propose updating node fields | `node_id` + `updates` (required) |
| `propose_routing` | Propose routing rule changes | `node_id` + `updates` (required) |
| `propose_binding` | Propose skill binding changes | `node_id` + `updates` (required) |
| `apply_patch` | Apply an approved patch to the tree | `patch_id` + `approved` (required) |

---

### action=tree

View the capability tree structure. Supports subtree filtering and depth control.

```json
{"action": "tree"}
{"action": "tree", "node_id": "fs"}
{"action": "tree", "node_id": "fs", "depth": 1}
```

Returns: tree nodes with id, name, kind, min_tier, summary, children.

### action=inspect

View a single node's complete metadata (all 7 dimensions: Runtime, Prompt, Routing, Perms, Skills, Display, Policy).

```json
{"action": "inspect", "node_id": "fs/read_file"}
{"action": "inspect", "node_id": "bash"}
```

Accepts both tree path IDs (`fs/read_file`) and tool names (`bash`).

### action=validate

Run three-level validation on the tree:

- **L1 (Node)**: Each tool/subagent has Summary, valid MinTier, non-empty Owner
- **L2 (Cross-Node)**: Child MinTier >= parent, dynamic prefixes unique
- **L3 (System)**: Registry/tree alignment, allowlist monotonicity, summary coverage, policy naming

```json
{"action": "validate"}
{"action": "validate", "level": 1}
{"action": "validate", "level": 3}
```

### action=diagnose

Generate a drift diagnosis report checking all 9 derivation targets (D1-D9).

```json
{"action": "diagnose"}
```

Returns: inventory (static/dynamic/group counts), checks per derivation target, registry alignment.

### action=generate_prompt

Generate the `## Tooling` prompt section for a specific intent tier.

```json
{"action": "generate_prompt", "tier": "task_light"}
{"action": "generate_prompt", "tier": "task_write"}
```

Valid tiers: greeting, question, task_light, task_write, task_delete, task_multimodal.

### action=generate_allowlist

Generate the tool allowlist for a specific intent tier.

```json
{"action": "generate_allowlist", "tier": "task_write"}
```

Returns: tier, count, sorted tool names.

---

### action=propose_register

Propose registering a new tool/subagent node. Generates a patch diff without modifying the tree.

```json
{
  "action": "propose_register",
  "node_spec": {
    "name": "new_tool",
    "parent": "runtime",
    "kind": "tool",
    "prompt": {"summary": "A new tool", "sort_order": 50},
    "routing": {"min_tier": "task_write"}
  }
}
```

Missing dimensions are filled with safe defaults (Owner=attempt_runner, MinTier=task_write, etc.).
Returns: `TreePatch` with status "proposed" and a single "add" operation.

### action=propose_update

Propose updating one or more dimensions on an existing node.

```json
{
  "action": "propose_update",
  "node_id": "runtime/bash",
  "updates": {
    "prompt": {"summary": "Execute commands in workspace", "sort_order": 1}
  }
}
```

Returns: `TreePatch` with "replace" operations, each capturing old and new values.

### action=propose_routing

Propose routing rule changes. Returns patch diff plus `intent_router_suggestion` text.

```json
{
  "action": "propose_routing",
  "node_id": "runtime/bash",
  "updates": {
    "min_tier": "task_delete",
    "intent_keywords": {"zh": ["删除"], "en": ["delete"]},
    "intent_priority": 30
  }
}
```

The suggestion confirms that IntentKeywords changes are automatically picked up via `TreeClassificationKeywords()` — no manual intent_router.go edits needed.

### action=propose_binding

Propose skill binding changes for a tool.

```json
{
  "action": "propose_binding",
  "node_id": "runtime/bash",
  "updates": {
    "bindable": true,
    "bound_skills": ["skill:automation"],
    "guidance": true
  }
}
```

### action=apply_patch

Apply a previously proposed patch to the in-memory tree. **Requires exec_escalation approval** — set `approved=true` only after obtaining approval.

```json
{
  "action": "apply_patch",
  "patch_id": "patch-1741510800000000000",
  "approved": true
}
```

After successful application:
1. **Auto-validate**: L1-L3 validation runs automatically (P7-7)
2. **Derivation info**: Reports which derivation targets (D1-D9) are affected (P7-8)
3. Patch status changes from "proposed" to "applied"
4. Double-apply is rejected

---

## Patch Diff Format (P7-6)

All `propose_*` actions produce a `TreePatch` containing one or more `PatchOperation`:

```json
{
  "id": "patch-1741510800000000000",
  "action": "register",
  "description": "Register new tool \"new_tool\" under \"runtime\"",
  "operations": [
    {
      "op": "add",
      "path": "runtime/new_tool",
      "value": { "...full node JSON..." }
    }
  ],
  "created_at": "2026-03-09T12:00:00Z",
  "status": "proposed"
}
```

Supported operations:
- `add`: Add a new node (path = node ID, value = full CapabilityNode)
- `replace`: Update a node field (path = node ID, field = dimension name, value = new, old = previous)
- `remove`: Remove a node (path = node ID)

---

## When to Use

- Use `validate` to check tree health after changes
- Use `diagnose` to detect drift between tree and consumers
- Use `inspect` to understand a tool's full metadata before modifying
- Use `generate_prompt`/`generate_allowlist` to preview what the agent sees at each tier
- Use `tree` to navigate the capability hierarchy
- Use `propose_*` to generate patch diffs for review before applying
- Use `apply_patch` to apply approved changes to the live tree

## Safety Model

- **propose_* actions** are safe — they only generate diffs, never modify the tree
- **apply_patch** requires explicit `approved=true` flag (exec_escalation)
- Post-apply validation catches any introduced inconsistencies
- All changes are to the in-memory tree; run `go generate` to persist to frontend artifacts
