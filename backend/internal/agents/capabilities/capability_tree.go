// capability_tree.go defines the hierarchical Capability Tree — the single source
// of truth for all tool capability metadata in Crab Claw.
//
// The tree replaces the flat []CapabilitySpec registry with a hierarchy carrying
// Runtime, Prompt, Routing, Permissions, SkillBinding, Display, and Policy metadata.
// All downstream consumers (prompt builder, intent router, tool policy, display config,
// skill binding, frontend mirrors) derive from this tree via derivation pipelines (D1-D9).
//
// Design doc: docs/codex/2026-03-09-能力树与自治能力管理系统架构设计-v2.md
// Tracking:   docs/claude/tracking/tracking-2026-03-09-capability-tree-implementation.md
package capabilities

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// P0-1: Node types and sub-structures
// ---------------------------------------------------------------------------

// NodeKind classifies the type of node in the capability tree.
type NodeKind string

const (
	NodeKindGroup    NodeKind = "group"    // Grouping node (e.g. "fs", "web", "sessions")
	NodeKindTool     NodeKind = "tool"     // Leaf tool node (e.g. "bash", "read_file")
	NodeKindSubagent NodeKind = "subagent" // Sub-agent entry (e.g. "spawn_coder_agent")
)

// CapabilityNode is a single node in the capability tree.
// Group nodes contain children; tool/subagent nodes are leaves carrying metadata.
type CapabilityNode struct {
	// -- Identity --
	ID       string   `json:"id"`       // Tree path: "fs/read_file", "media/send_media"
	Name     string   `json:"name"`     // Tool name: "read_file" or group name: "fs"
	Kind     NodeKind `json:"kind"`     // "group" | "tool" | "subagent"
	Parent   string   `json:"parent"`   // Parent node ID ("" for root children)
	Children []string `json:"children"` // Child node IDs (only for groups)

	// -- Sub-structures (7 dimensions) --
	Runtime *NodeRuntime      `json:"runtime,omitempty"`
	Prompt  *NodePrompt       `json:"prompt,omitempty"`
	Routing *NodeRouting      `json:"routing,omitempty"`
	Perms   *NodePermissions  `json:"perms,omitempty"`
	Skills  *NodeSkillBinding `json:"skills,omitempty"`
	Display *NodeDisplay      `json:"display,omitempty"`
	Policy  *NodePolicy       `json:"policy,omitempty"`
}

// NodeRuntime carries runtime metadata for a capability.
// For dynamic tool groups (Argus, Remote MCP, Local MCP), Dynamic=true and
// NamePrefix/DiscoverySource/ProviderID/ListMethod describe the discovery contract.
type NodeRuntime struct {
	Owner       string `json:"owner"`        // "attempt_runner", "argus_bridge", etc.
	EnabledWhen string `json:"enabled_when"` // "always", "MediaSender != nil", etc.
	Dynamic     bool   `json:"dynamic"`      // True for runtime-discovered tools

	// Dynamic tool discovery fields (P1-2: covers argus_/remote_/mcp_ prefix tools)
	NamePrefix      string `json:"name_prefix,omitempty"`      // "argus_", "remote_", "mcp_"
	DiscoverySource string `json:"discovery_source,omitempty"` // "ArgusBridge.AgentTools()"
	ProviderID      string `json:"provider_id,omitempty"`      // Bridge instance identifier
	ListMethod      string `json:"list_method,omitempty"`      // "AgentTools" / "AgentRemoteTools"
}

// NodePrompt carries capability-related prompt metadata only.
// Session state, operational principles, CLI compat, memory recall are NOT here.
type NodePrompt struct {
	Summary    string `json:"summary"`     // One-line summary for ## Tooling section
	SortOrder  int    `json:"sort_order"`  // Display ordering in prompt
	UsageGuide string `json:"usage_guide"` // When to use (tool/subagent only)
	Delegation string `json:"delegation"`  // Delegation strategy (subagent only)
	GroupIntro string `json:"group_intro"` // Group introduction (group only)
}

// NodeRouting carries intent routing rules for a capability.
type NodeRouting struct {
	MinTier        string         `json:"min_tier"`        // Minimum intent tier: "greeting"/"question"/"task_light"/...
	ExcludeFrom    []string       `json:"exclude_from"`    // Tiers to exclude from even if MinTier allows
	IntentKeywords IntentKeywords `json:"intent_keywords"` // Keywords for intent classification
	IntentPriority int            `json:"intent_priority"` // Priority when multiple tools match
}

// IntentKeywords holds locale-specific keywords for intent matching.
type IntentKeywords struct {
	ZH []string `json:"zh"` // Chinese keywords
	EN []string `json:"en"` // English keywords
}

// NodePermissions provides permission hints for the approval chain.
// These are hints for the planner and prompt — they don't replace the
// EscalationManager's runtime state machine.
type NodePermissions struct {
	MinSecurityLevel string `json:"min_security_level"` // "deny"/"allowlist"/"sandboxed"/"full"
	FileAccess       string `json:"file_access"`        // "global_read"/"scoped_read"/"scoped_write"/"none"
	ApprovalType     string `json:"approval_type"`      // "none"/"plan_confirm"/"exec_escalation"/"mount_access"/"data_export"
	ScopeCheck       string `json:"scope_check"`        // "none"/"workspace"/"scoped"/"mount_required"

	// P1-4 fix: escalation hints to drive real PendingEscalationRequest construction
	EscalationHints *EscalationHints `json:"escalation_hints,omitempty"`
}

// EscalationHints provides enough information for the planner to construct a
// PendingEscalationRequest. These are default-value hints; the actual request
// fields are determined by EscalationManager.RequestEscalation().
type EscalationHints struct {
	DefaultRequestedLevel string `json:"default_requested_level"` // "allowlist"/"sandboxed"/"full"
	DefaultTTLMinutes     int    `json:"default_ttl_minutes"`     // Default TTL (e.g. 30)
	DefaultMountMode      string `json:"default_mount_mode"`      // "ro" / "rw"
	NeedsOriginator       bool   `json:"needs_originator"`        // Needs originatorChatID/UserID
	NeedsRunSession       bool   `json:"needs_run_session"`       // Needs runID/sessionID association
}

// NodeSkillBinding manages tool<->skill binding relationships only.
// Skill installation/distribution/invocation-policy/store/VFS are managed by
// the skill lifecycle system independently (P2-3 boundary fix).
type NodeSkillBinding struct {
	Bindable    bool     `json:"bindable"`     // Whether external skills can bind to this tool
	BoundSkills []string `json:"bound_skills"` // Bound skill key list
	Guidance    bool     `json:"guidance"`     // Whether to inject skill guidance into tool description
}

// NodeDisplay carries tool display metadata for UI rendering.
// Derivation targets: tool-cards.ts, agents.ts, TUI view_tool.go (P2-1 fix).
type NodeDisplay struct {
	Icon       string `json:"icon"`        // Emoji or icon name
	Title      string `json:"title"`       // Display title
	Label      string `json:"label"`       // Short label
	Verb       string `json:"verb"`        // Action verb ("Execute"/"Read"/"Send")
	DetailKeys string `json:"detail_keys"` // Detail extraction key ("command"/"path")
}

// NodePolicy carries tool policy grouping metadata.
// Derivation targets: scope/tool_policy.go, ui/tool-policy.ts, wizard-v2 (P2-1 fix).
type NodePolicy struct {
	PolicyGroups []string `json:"policy_groups"` // "group:fs", "group:web", etc.
	Profiles     []string `json:"profiles"`      // Profile membership: "minimal"/"coding"/"messaging"/"full"
	WizardGroup  string   `json:"wizard_group"`  // wizard-v2 skill group: "fs"/"runtime"/"web"/"memory"
}

// ---------------------------------------------------------------------------
// P0-2: CapabilityTree and tree operations
// ---------------------------------------------------------------------------

// CapabilityTree is the top-level container holding all capability nodes.
// It provides lookup, traversal, and derivation methods.
type CapabilityTree struct {
	// Nodes is the flat index of all nodes keyed by ID.
	Nodes map[string]*CapabilityNode `json:"nodes"`

	// RootChildren are the top-level node IDs (direct children of the virtual root).
	RootChildren []string `json:"root_children"`
}

// NewCapabilityTree creates an empty tree.
func NewCapabilityTree() *CapabilityTree {
	return &CapabilityTree{
		Nodes:        make(map[string]*CapabilityNode),
		RootChildren: nil,
	}
}

// AddNode inserts a node into the tree, updating parent/child links.
func (t *CapabilityTree) AddNode(node *CapabilityNode) error {
	if node.ID == "" {
		return fmt.Errorf("node ID must not be empty")
	}
	if _, exists := t.Nodes[node.ID]; exists {
		return fmt.Errorf("duplicate node ID: %s", node.ID)
	}

	t.Nodes[node.ID] = node

	if node.Parent == "" {
		t.RootChildren = append(t.RootChildren, node.ID)
	} else {
		parent, ok := t.Nodes[node.Parent]
		if !ok {
			return fmt.Errorf("parent node %q not found for %q", node.Parent, node.ID)
		}
		parent.Children = append(parent.Children, node.ID)
	}
	return nil
}

// RemoveNode removes a node from the tree, updating parent/child links.
// If the node is a group, all descendants are recursively removed first.
// P7-5: Used by apply_patch for "remove" operations.
func (t *CapabilityTree) RemoveNode(id string) error {
	node, ok := t.Nodes[id]
	if !ok {
		return fmt.Errorf("node %q not found", id)
	}
	// Recursively remove children first to avoid orphans
	for _, childID := range append([]string(nil), node.Children...) {
		if err := t.RemoveNode(childID); err != nil {
			return fmt.Errorf("remove child %q of %q: %w", childID, id, err)
		}
	}
	if node.Parent != "" {
		parent := t.Nodes[node.Parent]
		if parent != nil {
			filtered := make([]string, 0, len(parent.Children))
			for _, c := range parent.Children {
				if c != id {
					filtered = append(filtered, c)
				}
			}
			parent.Children = filtered
		}
	} else {
		filtered := make([]string, 0, len(t.RootChildren))
		for _, c := range t.RootChildren {
			if c != id {
				filtered = append(filtered, c)
			}
		}
		t.RootChildren = filtered
	}
	delete(t.Nodes, id)
	return nil
}

// Lookup returns a node by its ID, or nil if not found.
func (t *CapabilityTree) Lookup(id string) *CapabilityNode {
	return t.Nodes[id]
}

// LookupByName returns the first node matching the given Name field.
func (t *CapabilityTree) LookupByName(name string) *CapabilityNode {
	for _, n := range t.Nodes {
		if n.Name == name {
			return n
		}
	}
	return nil
}

// LookupByToolHint returns a tool/subagent node whose Name matches toolHint.
// Used by GeneratePlanSteps to resolve IntentAction.ToolHint → node.
func (t *CapabilityTree) LookupByToolHint(toolHint string) *CapabilityNode {
	for _, n := range t.Nodes {
		if (n.Kind == NodeKindTool || n.Kind == NodeKindSubagent) && n.Name == toolHint {
			return n
		}
	}
	return nil
}

// Walk visits every node in the tree in deterministic order (sorted by ID).
// If fn returns false, traversal stops.
func (t *CapabilityTree) Walk(fn func(node *CapabilityNode) bool) {
	ids := make([]string, 0, len(t.Nodes))
	for id := range t.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !fn(t.Nodes[id]) {
			return
		}
	}
}

// WalkSubtree visits a node and all its descendants depth-first.
func (t *CapabilityTree) WalkSubtree(rootID string, fn func(node *CapabilityNode) bool) {
	node := t.Nodes[rootID]
	if node == nil {
		return
	}
	if !fn(node) {
		return
	}
	for _, childID := range node.Children {
		t.WalkSubtree(childID, fn)
	}
}

// AllStaticTools returns the names of all non-dynamic tool and subagent nodes.
func (t *CapabilityTree) AllStaticTools() []string {
	var names []string
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Runtime != nil && n.Runtime.Dynamic {
			continue
		}
		names = append(names, n.Name)
	}
	sort.Strings(names)
	return names
}

// AllTools returns all tool and subagent names (static + dynamic groups' own names).
func (t *CapabilityTree) AllTools() []string {
	var names []string
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		names = append(names, n.Name)
	}
	sort.Strings(names)
	return names
}

// DynamicGroups returns all group nodes with Runtime.Dynamic=true.
func (t *CapabilityTree) DynamicGroups() []*CapabilityNode {
	var groups []*CapabilityNode
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup && n.Runtime != nil && n.Runtime.Dynamic {
			groups = append(groups, n)
		}
	}
	return groups
}

// DynamicGroupPrefixes returns the set of NamePrefix values from dynamic groups.
func (t *CapabilityTree) DynamicGroupPrefixes() []string {
	var prefixes []string
	for _, g := range t.DynamicGroups() {
		if g.Runtime != nil && g.Runtime.NamePrefix != "" {
			prefixes = append(prefixes, g.Runtime.NamePrefix)
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

// validTiers lists all valid intent tier values in escalating order.
var validTiers = []string{
	"greeting",
	"question",
	"task_light",
	"task_write",
	"task_delete",
	"task_multimodal",
}

// TierIndex returns the position of a tier in the escalating order, or -1.
// Exported for use by intent_router.go dynamic group filtering.
func TierIndex(tier string) int {
	return tierIndex(tier)
}

// tierIndex returns the position of a tier in the escalating order, or -1.
func tierIndex(tier string) int {
	for i, t := range validTiers {
		if t == tier {
			return i
		}
	}
	return -1
}

// ToolsForTier returns all tool/subagent nodes whose MinTier <= the given tier.
// A node with no Routing or empty MinTier is treated as "task_multimodal" (most restrictive).
func (t *CapabilityTree) ToolsForTier(tier string) []*CapabilityNode {
	requestedIdx := tierIndex(tier)
	if requestedIdx < 0 {
		return nil
	}

	var result []*CapabilityNode
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		minTier := "task_multimodal"
		if n.Routing != nil && n.Routing.MinTier != "" {
			minTier = n.Routing.MinTier
		}
		nodeIdx := tierIndex(minTier)
		if nodeIdx < 0 {
			continue
		}
		if nodeIdx > requestedIdx {
			continue
		}
		// Check exclusions
		excluded := false
		if n.Routing != nil {
			for _, ex := range n.Routing.ExcludeFrom {
				if ex == tier {
					excluded = true
					break
				}
			}
		}
		if !excluded {
			result = append(result, n)
		}
	}
	return result
}

// AllowlistForTier returns the set of tool names allowed for a given intent tier.
func (t *CapabilityTree) AllowlistForTier(tier string) map[string]bool {
	nodes := t.ToolsForTier(tier)
	m := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		m[n.Name] = true
	}
	return m
}

// BindableTools returns the names of all tools with Skills.Bindable=true.
func (t *CapabilityTree) BindableTools() []string {
	var names []string
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Skills != nil && n.Skills.Bindable {
			names = append(names, n.Name)
		}
	}
	sort.Strings(names)
	return names
}

// ToolSummaries returns a map of tool name -> prompt summary.
// Used for D1 derivation (prompt ## Tooling section).
func (t *CapabilityTree) ToolSummaries() map[string]string {
	m := make(map[string]string)
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Prompt != nil && n.Prompt.Summary != "" {
			m[n.Name] = n.Prompt.Summary
		}
	}
	return m
}

// SortedToolSummaries returns tool summaries sorted by SortOrder for prompt generation.
func (t *CapabilityTree) SortedToolSummaries() []ToolSummaryEntry {
	var entries []ToolSummaryEntry
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Prompt != nil && n.Prompt.Summary != "" {
			order := n.Prompt.SortOrder
			entries = append(entries, ToolSummaryEntry{
				Name:      n.Name,
				Summary:   n.Prompt.Summary,
				SortOrder: order,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SortOrder != entries[j].SortOrder {
			return entries[i].SortOrder < entries[j].SortOrder
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// ToolSummaryEntry pairs a tool name with its summary and sort order.
type ToolSummaryEntry struct {
	Name      string
	Summary   string
	SortOrder int
}

// PolicyGroups returns the complete group -> members mapping derived from the tree.
// Used for D5 derivation (backend tool_policy.go).
func (t *CapabilityTree) PolicyGroups() map[string][]string {
	groups := make(map[string][]string)
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Policy == nil {
			continue
		}
		for _, pg := range n.Policy.PolicyGroups {
			groups[pg] = append(groups[pg], n.Name)
		}
	}
	// Sort members within each group for determinism
	for g := range groups {
		sort.Strings(groups[g])
	}
	return groups
}

// DisplaySpecs returns a map of tool name -> NodeDisplay for all tools with display metadata.
// Used for D7 derivation (tool-display.json, display.go).
func (t *CapabilityTree) DisplaySpecs() map[string]*NodeDisplay {
	m := make(map[string]*NodeDisplay)
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Display != nil {
			m[n.Name] = n.Display
		}
	}
	return m
}

// WizardGroups returns wizard_group -> tool names mapping.
// Used for D8 derivation (wizard-v2 skill groups).
func (t *CapabilityTree) WizardGroups() map[string][]string {
	groups := make(map[string][]string)
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Policy != nil && n.Policy.WizardGroup != "" {
			groups[n.Policy.WizardGroup] = append(groups[n.Policy.WizardGroup], n.Name)
		}
	}
	for g := range groups {
		sort.Strings(groups[g])
	}
	return groups
}

// IntentKeywordsForTier aggregates all intent keywords from tools available at a given tier.
// Used for D4 derivation (intent keyword aggregation).
func (t *CapabilityTree) IntentKeywordsForTier(tier string) IntentKeywords {
	nodes := t.ToolsForTier(tier)
	var zhAll, enAll []string
	seen := make(map[string]bool)
	for _, n := range nodes {
		if n.Routing == nil {
			continue
		}
		for _, kw := range n.Routing.IntentKeywords.ZH {
			if !seen["zh:"+kw] {
				zhAll = append(zhAll, kw)
				seen["zh:"+kw] = true
			}
		}
		for _, kw := range n.Routing.IntentKeywords.EN {
			if !seen["en:"+kw] {
				enAll = append(enAll, kw)
				seen["en:"+kw] = true
			}
		}
	}
	return IntentKeywords{ZH: zhAll, EN: enAll}
}

// intentPriorityToTier maps IntentPriority values to classification tiers.
// P3-6: nodes with IntentPriority > 0 have their IntentKeywords routed to the
// corresponding tier for intent classification, regardless of their MinTier.
var intentPriorityToTier = map[int]string{
	30: "task_delete",
	20: "task_multimodal",
	10: "task_write",
}

// ClassificationKeywords returns all intent keywords that classify user prompts
// into the given tier. Only nodes with explicit IntentPriority > 0 contribute.
// P3-6: D4 derivation — replaces hand-written deleteKeywords/multimodalKeywords/writeKeywords.
func (t *CapabilityTree) ClassificationKeywords(tier string) []string {
	var keywords []string
	seen := make(map[string]bool)
	for _, n := range t.Nodes {
		if n.Kind == NodeKindGroup || n.Routing == nil {
			continue
		}
		if n.Routing.IntentPriority <= 0 {
			continue
		}
		classifTier, ok := intentPriorityToTier[n.Routing.IntentPriority]
		if !ok || classifTier != tier {
			continue
		}
		for _, k := range n.Routing.IntentKeywords.ZH {
			if !seen[k] {
				keywords = append(keywords, k)
				seen[k] = true
			}
		}
		for _, k := range n.Routing.IntentKeywords.EN {
			if !seen[k] {
				keywords = append(keywords, k)
				seen[k] = true
			}
		}
	}
	return keywords
}

// TreeClassificationKeywords returns intent classification keywords for a tier from the tree.
// P3-6: D4 derivation — used by intent_router.go to replace hand-written keyword arrays.
func TreeClassificationKeywords(tier string) []string {
	return DefaultTree().ClassificationKeywords(tier)
}

// ToolIntentKeywords returns combined ZH+EN intent keywords for a specific tool.
// Stage 4 Phase B: used by intent_analysis.go to replace hardcoded keyword arrays
// in hasSendIntent/hasBrowseIntent with tree-derived keywords.
func ToolIntentKeywords(toolName string) []string {
	tree := DefaultTree()
	node := tree.LookupByToolHint(toolName)
	if node == nil || node.Routing == nil {
		return nil
	}
	var keywords []string
	keywords = append(keywords, node.Routing.IntentKeywords.ZH...)
	keywords = append(keywords, node.Routing.IntentKeywords.EN...)
	return keywords
}

// MatchesDynamicGroup checks if a tool name matches any dynamic group's NamePrefix.
// Returns the matching group node, or nil.
func (t *CapabilityTree) MatchesDynamicGroup(toolName string) *CapabilityNode {
	for _, g := range t.DynamicGroups() {
		if g.Runtime != nil && g.Runtime.NamePrefix != "" {
			if strings.HasPrefix(toolName, g.Runtime.NamePrefix) {
				return g
			}
		}
	}
	return nil
}

// NodeCount returns the total number of nodes in the tree.
func (t *CapabilityTree) NodeCount() int {
	return len(t.Nodes)
}

// ---------------------------------------------------------------------------
// Serialization
//
// NOTE: The runtime tree is built by GenerateTreeFromRegistry() in tree_migrate.go,
// NOT loaded from capability_tree.json. The JSON file is a reference snapshot for
// documentation and debugging. If you need the live tree, call DefaultTree().
// ---------------------------------------------------------------------------

// SaveJSON writes the tree to a JSON file.
func (t *CapabilityTree) SaveJSON(path string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal capability tree: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadTreeFromJSON reads a capability tree from a JSON file.
func LoadTreeFromJSON(path string) (*CapabilityTree, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read capability tree: %w", err)
	}
	tree := &CapabilityTree{}
	if err := json.Unmarshal(data, tree); err != nil {
		return nil, fmt.Errorf("unmarshal capability tree: %w", err)
	}
	if tree.Nodes == nil {
		tree.Nodes = make(map[string]*CapabilityNode)
	}
	return tree, nil
}

// ---------------------------------------------------------------------------
// Backward-compatible bridge: CapabilityTree -> []CapabilitySpec (D9)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Singleton: cached default tree
// ---------------------------------------------------------------------------

var (
	defaultTree     *CapabilityTree
	defaultTreeOnce sync.Once
)

// DefaultTree returns the lazily-initialized default capability tree.
// It is built once from GenerateTreeFromRegistry() and cached for the process lifetime.
// All downstream consumers (prompt, router, policy, skills, display) should use this.
func DefaultTree() *CapabilityTree {
	defaultTreeOnce.Do(func() {
		defaultTree = GenerateTreeFromRegistry()
	})
	return defaultTree
}

// treeMutationMu guards tree modifications during apply_patch (P7-5).
var treeMutationMu sync.Mutex

// ResetDefaultTreeForTesting rebuilds the default tree from scratch.
// Only for use in tests — ensures test isolation after apply_patch mutations.
func ResetDefaultTreeForTesting() {
	treeMutationMu.Lock()
	defer treeMutationMu.Unlock()
	defaultTree = GenerateTreeFromRegistry()
}

// TreeToolOrder returns tool names in the canonical display sort order from the tree.
// Used by prompt_sections.go to replace the hand-written toolOrder array (D1 derivation).
func TreeToolOrder() []string {
	entries := DefaultTree().SortedToolSummaries()
	order := make([]string, len(entries))
	for i, e := range entries {
		order[i] = e.Name
	}
	return order
}

// TreeToolSummaries returns tool summaries from the tree.
// Used by prompt_sections.go to replace the hand-written coreToolSummaries map (D1 derivation).
func TreeToolSummaries() map[string]string {
	return DefaultTree().ToolSummaries()
}

// TreePolicyGroups returns policy group -> members from the tree.
// Used by scope/tool_policy.go to replace hand-written ToolGroups (D5 derivation).
func TreePolicyGroups() map[string][]string {
	return DefaultTree().PolicyGroups()
}

// IsTreeBindable checks if a tool name is skill-bindable according to the tree.
// Also returns true for tools matching a dynamic group prefix.
func IsTreeBindable(toolName string) bool {
	tree := DefaultTree()
	node := tree.LookupByToolHint(toolName)
	if node != nil && node.Skills != nil {
		return node.Skills.Bindable
	}
	// Check dynamic groups — tools in dynamic groups inherit group bindability
	if g := tree.MatchesDynamicGroup(toolName); g != nil {
		return true // dynamic tools are bindable by default
	}
	return false
}

// IsInTreeOrDynamic checks if a tool name exists in the tree (static or dynamic group match).
func IsInTreeOrDynamic(toolName string) bool {
	tree := DefaultTree()
	if tree.LookupByToolHint(toolName) != nil {
		return true
	}
	return tree.MatchesDynamicGroup(toolName) != nil
}

// TreeSubagentDelegationEntries returns subagent nodes with their delegation guidance.
// Used by prompt_sections.go buildDelegationGuidanceSection (D2 derivation).
func TreeSubagentDelegationEntries() []SubagentEntry {
	tree := DefaultTree()
	var entries []SubagentEntry
	tree.Walk(func(n *CapabilityNode) bool {
		if n.Kind != NodeKindSubagent {
			return true
		}
		e := SubagentEntry{Name: n.Name}
		if n.Prompt != nil {
			e.Summary = n.Prompt.Summary
			e.Delegation = n.Prompt.Delegation
			e.UsageGuide = n.Prompt.UsageGuide
		}
		entries = append(entries, e)
		return true
	})
	return entries
}

// SubagentEntry holds delegation info for a sub-agent.
type SubagentEntry struct {
	Name       string
	Summary    string
	Delegation string
	UsageGuide string
}

// TreeIntentGroupSummaries returns a map from tier name to a compact string
// listing the group names and GroupIntro for groups that have tools at that tier.
// P1-8: D8 derivation — provides tree-derived group context for intentGuidanceText.
func TreeIntentGroupSummaries() map[string]string {
	tree := DefaultTree()
	result := make(map[string]string)
	for _, tier := range validTiers {
		allowed := tree.AllowlistForTier(tier)
		if len(allowed) == 0 {
			continue
		}
		// Collect groups that have at least one tool in this tier
		groupIntros := make(map[string]string) // groupName → GroupIntro
		for toolName := range allowed {
			node := tree.LookupByName(toolName)
			if node == nil {
				continue
			}
			parent := tree.Lookup(node.Parent)
			if parent != nil && parent.Kind == NodeKindGroup && parent.Prompt != nil && parent.Prompt.GroupIntro != "" {
				groupIntros[parent.Name] = parent.Prompt.GroupIntro
			}
		}
		if len(groupIntros) == 0 {
			continue
		}
		// Sort by group name for determinism
		names := make([]string, 0, len(groupIntros))
		for n := range groupIntros {
			names = append(names, n)
		}
		sort.Strings(names)
		var parts []string
		for _, n := range names {
			parts = append(parts, n+": "+groupIntros[n])
		}
		result[tier] = strings.Join(parts, "; ")
	}
	return result
}

// ---------------------------------------------------------------------------
// Backward-compatible bridge: CapabilityTree -> []CapabilitySpec (D9)
// ---------------------------------------------------------------------------

// ToRegistry converts the tree back to a flat []CapabilitySpec for backward compatibility.
// This is derivation target D9 — ensuring existing code that reads Registry still works.
func (t *CapabilityTree) ToRegistry() []CapabilitySpec {
	var specs []CapabilitySpec
	t.Walk(func(n *CapabilityNode) bool {
		if n.Kind == NodeKindGroup {
			return true
		}
		spec := CapabilitySpec{
			ID:       n.ID,
			ToolName: n.Name,
		}
		// Map NodeKind -> CapabilityKind
		switch n.Kind {
		case NodeKindSubagent:
			spec.Kind = KindSubagentEntry
		default:
			spec.Kind = KindTool
		}
		if n.Runtime != nil {
			spec.RuntimeOwner = n.Runtime.Owner
			spec.EnabledWhen = n.Runtime.EnabledWhen
		}
		if n.Prompt != nil {
			spec.PromptSummary = n.Prompt.Summary
		}
		if n.Policy != nil {
			spec.ToolGroups = n.Policy.PolicyGroups
		}
		if n.Skills != nil {
			spec.SkillBindable = n.Skills.Bindable
		}
		specs = append(specs, spec)
		return true
	})
	return specs
}
