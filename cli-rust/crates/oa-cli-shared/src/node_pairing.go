package infra

// node_pairing.go — 节点配对管理（全量字段）
// 对应 TS: src/infra/node-pairing.ts (337L)
//
// 管理远程节点配对请求、审批和 token 验证。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ---------- 类型定义（全量 TS 字段）----------

// NodePairingPendingRequest 待审批配对请求。
type NodePairingPendingRequest struct {
	RequestID       string          `json:"requestId"`
	NodeID          string          `json:"nodeId"`
	DisplayName     string          `json:"displayName,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	Version         string          `json:"version,omitempty"`
	CoreVersion     string          `json:"coreVersion,omitempty"`
	UIVersion       string          `json:"uiVersion,omitempty"`
	DeviceFamily    string          `json:"deviceFamily,omitempty"`
	ModelIdentifier string          `json:"modelIdentifier,omitempty"`
	Caps            []string        `json:"caps,omitempty"`
	Commands        []string        `json:"commands,omitempty"`
	Permissions     map[string]bool `json:"permissions,omitempty"`
	RemoteIP        string          `json:"remoteIp,omitempty"`
	Silent          bool            `json:"silent,omitempty"`
	IsRepair        bool            `json:"isRepair,omitempty"`
	Hostname        string          `json:"hostname,omitempty"`
	Ts              int64           `json:"ts"`
}

// NodePairingPairedNode 已配对节点。
type NodePairingPairedNode struct {
	NodeID            string          `json:"nodeId"`
	Token             string          `json:"token"`
	DisplayName       string          `json:"displayName,omitempty"`
	Platform          string          `json:"platform,omitempty"`
	Version           string          `json:"version,omitempty"`
	CoreVersion       string          `json:"coreVersion,omitempty"`
	UIVersion         string          `json:"uiVersion,omitempty"`
	DeviceFamily      string          `json:"deviceFamily,omitempty"`
	ModelIdentifier   string          `json:"modelIdentifier,omitempty"`
	Caps              []string        `json:"caps,omitempty"`
	Commands          []string        `json:"commands,omitempty"`
	Bins              []string        `json:"bins,omitempty"`
	Permissions       map[string]bool `json:"permissions,omitempty"`
	Hostname          string          `json:"hostname,omitempty"`
	RemoteIP          string          `json:"remoteIp,omitempty"`
	CreatedAtMs       int64           `json:"createdAtMs"`
	ApprovedAtMs      int64           `json:"approvedAtMs"`
	LastConnectedAtMs int64           `json:"lastConnectedAtMs,omitempty"`
}

// NodePairingState 持久化配对状态。
type NodePairingState struct {
	Pending []NodePairingPendingRequest `json:"pending,omitempty"`
	Paired  []NodePairingPairedNode     `json:"paired,omitempty"`
}

// ---------- 配对管理器 ----------

var (
	pairingMu    sync.Mutex
	pairingState *NodePairingState
)

const pairingFile = "node-pairing.json"

// pendingTTLMs 是 pending 请求的过期时间（对齐 TS PENDING_TTL_MS = 5 * 60 * 1000）。
const pendingTTLMs int64 = 5 * 60 * 1000

func pairingFilePath() string {
	return filepath.Join(resolveOpenAcosmiDir(), pairingFile)
}

func loadPairingState() *NodePairingState {
	data, err := os.ReadFile(pairingFilePath())
	if err != nil {
		return &NodePairingState{}
	}
	var s NodePairingState
	if err := json.Unmarshal(data, &s); err != nil {
		return &NodePairingState{}
	}
	// DY-P02: 清理过期 pending 请求（对齐 TS loadState → pruneExpiredPending）
	pruneExpiredPending(&s)
	return &s
}

// pruneExpiredPending 移除超过 pendingTTLMs 的 pending 请求。
func pruneExpiredPending(s *NodePairingState) {
	if len(s.Pending) == 0 {
		return
	}
	nowMs := time.Now().UnixMilli()
	kept := make([]NodePairingPendingRequest, 0, len(s.Pending))
	for _, p := range s.Pending {
		if nowMs-p.Ts <= pendingTTLMs {
			kept = append(kept, p)
		}
	}
	s.Pending = kept
}

func savePairingState(s *NodePairingState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(pairingFilePath())
	_ = os.MkdirAll(dir, 0o755)
	return os.WriteFile(pairingFilePath(), append(data, '\n'), 0o600)
}
