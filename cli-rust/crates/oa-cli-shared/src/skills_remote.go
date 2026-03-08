package infra

// skills_remote.go — 远程技能管理（全量实现）
// 对应 TS: src/infra/skills-remote.ts (361L)
//
// 管理远程节点注册表、缓存刷新和技能可用性判断。
// FIX-5: 扩展 NodeInfo + RecordNodeInfo + RefreshNodeBins + DescribeNode

import (
	"strings"
	"sync"
)

// ---------- 类型定义（全量 TS 字段）----------

// NodeInfo 远程节点信息。
type NodeInfo struct {
	NodeID       string            `json:"nodeId"`
	DisplayName  string            `json:"displayName,omitempty"`
	Platform     string            `json:"platform,omitempty"`
	DeviceFamily string            `json:"deviceFamily,omitempty"`
	Bins         map[string]string `json:"bins,omitempty"` // bin名 → 路径
	Caps         []string          `json:"caps,omitempty"` // 能力列表
	Commands     []string          `json:"commands,omitempty"`
	RemoteIP     string            `json:"remoteIp,omitempty"`
}

// RemoteSkillEligibility 远程技能可用性。
type RemoteSkillEligibility struct {
	Available bool       `json:"available"`
	Nodes     []NodeInfo `json:"nodes,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

// NodeRegistry 节点注册表接口。
type NodeRegistry interface {
	ListNodes() []NodeInfo
	GetNode(nodeID string) *NodeInfo
}

// ---------- 全局注册表 ----------

var (
	skillsRemoteMu       sync.RWMutex
	skillsRemoteRegistry NodeRegistry
	skillsRemoteCache    []NodeInfo
	remoteNodeRecords    map[string]*NodeInfo
)

func init() {
	remoteNodeRecords = make(map[string]*NodeInfo)
}

// SetSkillsRemoteRegistry 设置远程节点注册表。
func SetSkillsRemoteRegistry(registry NodeRegistry) {
	skillsRemoteMu.Lock()
	defer skillsRemoteMu.Unlock()
	skillsRemoteRegistry = registry
	skillsRemoteCache = nil
}

// PrimeRemoteSkillsCache 预热远程技能缓存。
func PrimeRemoteSkillsCache() {
	skillsRemoteMu.Lock()
	defer skillsRemoteMu.Unlock()
	if skillsRemoteRegistry == nil {
		return
	}
	skillsRemoteCache = skillsRemoteRegistry.ListNodes()
}

// GetRemoteSkillEligibility 获取远程技能可用性。
func GetRemoteSkillEligibility() *RemoteSkillEligibility {
	skillsRemoteMu.RLock()
	defer skillsRemoteMu.RUnlock()
	if skillsRemoteRegistry == nil {
		return &RemoteSkillEligibility{Reason: "no-registry"}
	}
	nodes := skillsRemoteCache
	if len(nodes) == 0 {
		nodes = skillsRemoteRegistry.ListNodes()
	}
	if len(nodes) == 0 {
		return &RemoteSkillEligibility{Reason: "no-nodes"}
	}
	var eligible []NodeInfo
	for _, n := range nodes {
		for _, cap := range n.Caps {
			if cap == "system.run" {
				eligible = append(eligible, n)
				break
			}
		}
	}
	if len(eligible) == 0 {
		return &RemoteSkillEligibility{Reason: "no-capable-nodes"}
	}
	return &RemoteSkillEligibility{
		Available: true,
		Nodes:     eligible,
	}
}

// ---------- 新增操作 (TS L73-210) ----------

// RecordNodeInfo 记录节点信息（TS L73-120）。
func RecordNodeInfo(nodeID string, info NodeInfo) {
	skillsRemoteMu.Lock()
	defer skillsRemoteMu.Unlock()
	info.NodeID = nodeID
	remoteNodeRecords[nodeID] = &info
}

// GetNodeInfo 获取已记录的节点信息。
func GetNodeInfo(nodeID string) *NodeInfo {
	skillsRemoteMu.RLock()
	defer skillsRemoteMu.RUnlock()
	return remoteNodeRecords[nodeID]
}

// DescribeNode 格式化节点描述（TS L24-30）。
func DescribeNode(nodeID string) string {
	skillsRemoteMu.RLock()
	record := remoteNodeRecords[nodeID]
	skillsRemoteMu.RUnlock()
	name := ""
	if record != nil {
		name = strings.TrimSpace(record.DisplayName)
	}
	var base string
	if name != "" && name != nodeID {
		base = name + " (" + nodeID + ")"
	} else {
		base = nodeID
	}
	ip := ""
	if record != nil {
		ip = strings.TrimSpace(record.RemoteIP)
	}
	if ip != "" {
		return base + " @ " + ip
	}
	return base
}

// RefreshNodeBins 探测远程节点二进制（TS L130-210）。
// 目前通过 NodeRegistry 获取，后续可集成 WS probe。
func RefreshNodeBins(requiredBins []string) map[string]map[string]string {
	skillsRemoteMu.RLock()
	defer skillsRemoteMu.RUnlock()
	result := make(map[string]map[string]string)
	for nodeID, info := range remoteNodeRecords {
		if info.Bins == nil {
			continue
		}
		matched := make(map[string]string)
		for _, bin := range requiredBins {
			if path, ok := info.Bins[bin]; ok {
				matched[bin] = path
			}
		}
		if len(matched) > 0 {
			result[nodeID] = matched
		}
	}
	return result
}
