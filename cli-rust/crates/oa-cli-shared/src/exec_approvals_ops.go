package infra

// exec_approvals_ops.go — Exec Approvals 补齐操作
// 对应 TS: exec-approvals.ts normalizeExecApprovals, addAllowlistEntry,
//   recordAllowlistUse, requiresExecApproval

import "time"

// NormalizeExecApprovals 填充默认值、确保 agents map 存在。
func NormalizeExecApprovals(file *ExecApprovalsFile) {
	if file == nil {
		return
	}
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Agents == nil {
		file.Agents = make(map[string]*ExecApprovalsAgent)
	}
	if file.Defaults == nil {
		file.Defaults = &ExecApprovalsDefaults{}
	}
}

// AddAllowlistEntry 为指定 agent 添加白名单条目。
func AddAllowlistEntry(file *ExecApprovalsFile, agentID, pattern string) {
	NormalizeExecApprovals(file)
	if agentID == "" {
		agentID = "main"
	}
	agent, ok := file.Agents[agentID]
	if !ok {
		agent = &ExecApprovalsAgent{}
		file.Agents[agentID] = agent
	}
	// 去重
	for _, e := range agent.Allowlist {
		if e.Pattern == pattern {
			return
		}
	}
	agent.Allowlist = append(agent.Allowlist, ExecAllowlistEntry{
		Pattern: pattern,
	})
}

// RecordAllowlistUse 记录白名单条目使用情况。
func RecordAllowlistUse(file *ExecApprovalsFile, agentID string, pattern, command, resolvedPath string) {
	NormalizeExecApprovals(file)
	if agentID == "" {
		agentID = "main"
	}
	agent, ok := file.Agents[agentID]
	if !ok {
		return
	}
	now := time.Now().UnixMilli()
	for i, e := range agent.Allowlist {
		if e.Pattern == pattern {
			agent.Allowlist[i].LastUsedAt = &now
			agent.Allowlist[i].LastUsedCommand = command
			agent.Allowlist[i].LastResolvedPath = resolvedPath
			return
		}
	}
}

// RequiresExecApproval 判断是否需要执行审批。
func RequiresExecApproval(ask ExecAsk, security ExecSecurity, analysisOk, allowlistSatisfied bool) bool {
	if security == ExecSecurityFull {
		return false
	}
	if security == ExecSecurityDeny {
		return true
	}
	// security == allowlist
	if allowlistSatisfied {
		return false
	}
	if ask == ExecAskAlways {
		return true
	}
	if ask == ExecAskOnMiss {
		return !analysisOk
	}
	// ask == off
	return !analysisOk
}
