/**
 * Tool policy utilities — constants derived from capability tree (D6 derivation).
 * Source: backend/internal/agents/capabilities/gen_frontend.go
 *
 * TOOL_GROUPS and TOOL_PROFILES are generated from the capability tree via
 * `go generate ./internal/agents/capabilities/...`
 *
 * Public API (hand-maintained):
 *   - normalizeToolName()
 *   - expandToolGroups()
 *   - resolveToolProfilePolicy()
 */

// ---------------------------------------------------------------------------
// Constants — generated from capability tree (Phase 2 D6 derivation)
// ---------------------------------------------------------------------------

const TOOL_NAME_ALIASES: Record<string, string> = {
  bash: "exec",
  "apply-patch": "apply_patch",
};

// D6: TOOL_GROUPS derived from CapabilityTree.PolicyGroups()
const TOOL_GROUPS: Record<string, string[]> = {
  "group:ai": ["image"],
  "group:automation": ["cron", "gateway"],
  "group:fs": ["apply_patch", "list_dir", "read", "write"],
  "group:memory": ["memory_get", "memory_search"],
  "group:messaging": ["message"],
  "group:nodes": ["nodes"],
  "group:openacosmi": [
    "agents_list",
    "browser",
    "canvas",
    "cron",
    "gateway",
    "image",
    "memory_get",
    "memory_search",
    "message",
    "nodes",
    "session_status",
    "sessions_history",
    "sessions_list",
    "sessions_send",
    "sessions_spawn",
    "web_fetch",
    "web_search",
  ],
  "group:runtime": ["exec"],
  "group:sessions": [
    "agents_list",
    "session_status",
    "sessions_history",
    "sessions_list",
    "sessions_send",
    "sessions_spawn",
  ],
  "group:system": ["cron", "gateway", "nodes"],
  "group:ui": ["canvas"],
  "group:web": ["browser", "web_fetch", "web_search"],
};

type ToolProfileId = "minimal" | "coding" | "messaging" | "full";

type ToolProfilePolicy = {
  allow?: string[];
  deny?: string[];
};

// D6: TOOL_PROFILES derived from CapabilityTree node Policy.Profiles
const TOOL_PROFILES: Record<ToolProfileId, ToolProfilePolicy> = {
  minimal: {
    allow: ["session_status"],
  },
  coding: {
    allow: ["group:fs", "group:runtime", "group:sessions", "group:memory", "image"],
  },
  messaging: {
    allow: [
      "group:messaging",
      "sessions_list",
      "sessions_history",
      "sessions_send",
      "session_status",
    ],
  },
  full: {},
};

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function normalizeToolName(name: string): string {
  const normalized = name.trim().toLowerCase();
  return TOOL_NAME_ALIASES[normalized] ?? normalized;
}

function normalizeToolList(list?: string[]): string[] {
  if (!list) {
    return [];
  }
  return list.map(normalizeToolName).filter(Boolean);
}

export function expandToolGroups(list?: string[]): string[] {
  const normalized = normalizeToolList(list);
  const expanded: string[] = [];
  for (const value of normalized) {
    const group = TOOL_GROUPS[value];
    if (group) {
      expanded.push(...group);
      continue;
    }
    expanded.push(value);
  }
  return Array.from(new Set(expanded));
}

export function resolveToolProfilePolicy(profile?: string): ToolProfilePolicy | undefined {
  if (!profile) {
    return undefined;
  }
  const resolved = TOOL_PROFILES[profile as ToolProfileId];
  if (!resolved) {
    return undefined;
  }
  if (!resolved.allow && !resolved.deny) {
    return undefined;
  }
  return {
    allow: resolved.allow ? [...resolved.allow] : undefined,
    deny: resolved.deny ? [...resolved.deny] : undefined,
  };
}
