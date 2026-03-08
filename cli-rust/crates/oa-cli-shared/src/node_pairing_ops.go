package infra

// node_pairing_ops.go — 节点配对操作（全量字段）
// 对应 TS: node-pairing.ts 核心操作函数
// FIX-4: struct 参数 + UpdatePairedNodeMetadata

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// RequestNodePairing 发起配对请求（接收完整 struct）。
func RequestNodePairing(req NodePairingPendingRequest) (*NodePairingPendingRequest, bool) {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	// DY-P01: 规范化 nodeId（对齐 TS normalizeNodeId）
	req.NodeID = strings.TrimSpace(req.NodeID)
	s := loadPairingState()

	// 检查是否已有 pending
	for _, p := range s.Pending {
		if p.NodeID == req.NodeID {
			return &p, false
		}
	}

	// W-008: 自动检测 IsRepair（对齐 TS L216: isRepair = Boolean(state.pairedByNodeId[nodeId])）
	for _, paired := range s.Paired {
		if paired.NodeID == req.NodeID {
			req.IsRepair = true
			break
		}
	}

	if req.RequestID == "" {
		req.RequestID = generatePairingID()
	}
	if req.Ts == 0 {
		req.Ts = time.Now().UnixMilli()
	}
	s.Pending = append(s.Pending, req)
	if err := savePairingState(s); err != nil {
		slog.Error("node pairing: failed to save state after RequestNodePairing", slog.String("error", err.Error()))
	}
	return &req, true
}

// ApproveNodePairing 审批配对请求（复制全量字段）。
func ApproveNodePairing(requestID string) (*NodePairingPairedNode, error) {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	s := loadPairingState()

	var found *NodePairingPendingRequest
	remaining := make([]NodePairingPendingRequest, 0, len(s.Pending))
	for i := range s.Pending {
		if s.Pending[i].RequestID == requestID {
			found = &s.Pending[i]
		} else {
			remaining = append(remaining, s.Pending[i])
		}
	}
	if found == nil {
		return nil, fmt.Errorf("pairing request %s not found", requestID)
	}
	s.Pending = remaining

	// W-007: 保留原始 CreatedAtMs（对齐 TS L249: createdAtMs: existing?.createdAtMs ?? now）
	now := time.Now().UnixMilli()
	createdAtMs := now
	for _, existing := range s.Paired {
		if existing.NodeID == found.NodeID {
			createdAtMs = existing.CreatedAtMs
			break
		}
	}

	node := NodePairingPairedNode{
		NodeID:          found.NodeID,
		Token:           generatePairingToken(),
		DisplayName:     found.DisplayName,
		Platform:        found.Platform,
		Version:         found.Version,
		CoreVersion:     found.CoreVersion,
		UIVersion:       found.UIVersion,
		DeviceFamily:    found.DeviceFamily,
		ModelIdentifier: found.ModelIdentifier,
		Caps:            found.Caps,
		Commands:        found.Commands,
		Permissions:     found.Permissions,
		Hostname:        found.Hostname,
		RemoteIP:        found.RemoteIP,
		CreatedAtMs:     createdAtMs,
		ApprovedAtMs:    now,
	}

	// 移除旧的已配对条目（对齐 TS map key 覆盖语义，防止重复）
	newPaired := make([]NodePairingPairedNode, 0, len(s.Paired))
	for _, p := range s.Paired {
		if p.NodeID != found.NodeID {
			newPaired = append(newPaired, p)
		}
	}
	s.Paired = append(newPaired, node)
	if err := savePairingState(s); err != nil {
		slog.Error("node pairing: failed to save state after ApproveNodePairing", slog.String("error", err.Error()))
	}
	return &node, nil
}

// RejectNodePairing 拒绝配对请求。
func RejectNodePairing(requestID string) bool {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	s := loadPairingState()
	found := false
	remaining := make([]NodePairingPendingRequest, 0, len(s.Pending))
	for _, p := range s.Pending {
		if p.RequestID == requestID {
			found = true
		} else {
			remaining = append(remaining, p)
		}
	}
	if !found {
		return false
	}
	s.Pending = remaining
	if err := savePairingState(s); err != nil {
		slog.Error("node pairing: failed to save state after RejectNodePairing", slog.String("error", err.Error()))
	}
	return true
}

// VerifyNodeToken 验证节点 token，成功时返回匹配的节点。
// DY-P01: 规范化 nodeId。DY-P04: 返回 node 对象（对齐 TS { ok, node }）。
func VerifyNodeToken(nodeID, token string) (*NodePairingPairedNode, bool) {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	s := loadPairingState()
	normalized := strings.TrimSpace(nodeID)
	for i := range s.Paired {
		if s.Paired[i].NodeID == normalized && s.Paired[i].Token == token {
			result := s.Paired[i]
			return &result, true
		}
	}
	return nil, false
}

// ListNodePairingStatus 列出所有配对状态（排序后返回）。
// DY-P03: 对齐 TS listNodePairing — pending 按 ts 降序，paired 按 approvedAtMs 降序。
func ListNodePairingStatus() *NodePairingState {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	s := loadPairingState()
	slices.SortFunc(s.Pending, func(a, b NodePairingPendingRequest) int {
		if b.Ts > a.Ts {
			return 1
		}
		if b.Ts < a.Ts {
			return -1
		}
		return 0
	})
	slices.SortFunc(s.Paired, func(a, b NodePairingPairedNode) int {
		if b.ApprovedAtMs > a.ApprovedAtMs {
			return 1
		}
		if b.ApprovedAtMs < a.ApprovedAtMs {
			return -1
		}
		return 0
	})
	return s
}

// UpdatePairedNodeMetadata 部分更新已配对节点元数据（TS L290-337）。
func UpdatePairedNodeMetadata(nodeID string, patch func(*NodePairingPairedNode)) bool {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	// DY-P01: 规范化 nodeId（对齐 TS normalizeNodeId）
	normalized := strings.TrimSpace(nodeID)
	s := loadPairingState()
	found := false
	for i := range s.Paired {
		if s.Paired[i].NodeID == normalized {
			patch(&s.Paired[i])
			found = true
			break
		}
	}
	if found {
		if err := savePairingState(s); err != nil {
			slog.Error("node pairing: failed to save state after UpdatePairedNodeMetadata", slog.String("error", err.Error()))
		}
	}
	return found
}

// GetPairedNode 按 nodeID 查询已配对节点。
// W-005: 对齐 TS getPairedNode() (node-pairing.ts L197-200)。
func GetPairedNode(nodeID string) *NodePairingPairedNode {
	pairingMu.Lock()
	defer pairingMu.Unlock()
	s := loadPairingState()
	normalized := strings.TrimSpace(nodeID)
	for i := range s.Paired {
		if s.Paired[i].NodeID == normalized {
			result := s.Paired[i]
			return &result
		}
	}
	return nil
}

// RenamePairedNode 重命名已配对节点。
// W-006: 对齐 TS renamePairedNode() (node-pairing.ts L319-337)。
func RenamePairedNode(nodeID, displayName string) (*NodePairingPairedNode, error) {
	trimmed := strings.TrimSpace(displayName)
	if trimmed == "" {
		return nil, fmt.Errorf("displayName required")
	}
	pairingMu.Lock()
	defer pairingMu.Unlock()
	s := loadPairingState()
	normalized := strings.TrimSpace(nodeID)
	for i := range s.Paired {
		if s.Paired[i].NodeID == normalized {
			s.Paired[i].DisplayName = trimmed
			if err := savePairingState(s); err != nil {
				slog.Error("node pairing: failed to save state after RenamePairedNode", slog.String("error", err.Error()))
			}
			result := s.Paired[i]
			return &result, nil
		}
	}
	return nil, nil
}

func generatePairingID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("pair_%s", base64.RawURLEncoding.EncodeToString(buf))
}

func generatePairingToken() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
