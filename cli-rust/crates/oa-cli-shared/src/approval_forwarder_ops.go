package infra

// approval_forwarder_ops.go — 转发器操作方法
// 对应 TS: exec-approval-forwarder.ts handleRequested/handleResolved/stop
// FIX-3: 全量实现含目标解析和投递

import (
	"log/slog"
	"time"
)

// HandleRequested 处理新的审批请求（TS L251-318）。
func (f *ApprovalForwarder) HandleRequested(req ExecApprovalRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped || f.cfg.Mode == ApprovalForwardModeOff {
		return
	}
	if !ShouldForwardApproval(&f.cfg, req) {
		return
	}

	nowMs := f.cfg.NowMs()
	if req.CreatedAtMs == 0 {
		req.CreatedAtMs = nowMs
	}

	// 解析转发目标 (TS L259-283)
	targets := f.resolveTargets(req)
	if len(targets) == 0 {
		return
	}

	// 超时处理
	expiresInMs := req.ExpiresAtMs - nowMs
	if expiresInMs <= 0 {
		expiresInMs = int64(f.cfg.TimeoutMs)
	}
	timer := time.AfterFunc(time.Duration(expiresInMs)*time.Millisecond, func() {
		f.handleExpired(req.ID)
	})

	p := &pendingApproval{
		request:   req,
		targets:   targets,
		timer:     timer,
		createdAt: time.Now(),
	}
	f.pending[req.ID] = p

	// 构建请求消息并投递
	text := BuildRequestMessage(req, nowMs)
	f.deliverToTargets(targets, text)

	if f.cfg.OnRequest != nil {
		go f.cfg.OnRequest(req)
	}
}

// HandleResolved 处理审批结果（TS L320-333）。
func (f *ApprovalForwarder) HandleResolved(res ExecApprovalResolved) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return
	}

	p, ok := f.pending[res.ID]
	if !ok {
		return
	}
	p.timer.Stop()
	targets := p.targets
	delete(f.pending, res.ID)

	// 构建结果消息并投递
	text := BuildResolvedMessage(res)
	f.deliverToTargets(targets, text)

	if f.cfg.OnResolved != nil {
		go f.cfg.OnResolved(res)
	}
}

// Stop 停止转发器（TS L335-342）。
func (f *ApprovalForwarder) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = true
	for id, p := range f.pending {
		p.timer.Stop()
		delete(f.pending, id)
	}
}

// PendingCount 返回待处理审批数。
func (f *ApprovalForwarder) PendingCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pending)
}

// handleExpired 超时清理并投递过期消息。
func (f *ApprovalForwarder) handleExpired(id string) {
	f.mu.Lock()
	p, ok := f.pending[id]
	if !ok {
		f.mu.Unlock()
		return
	}
	targets := p.targets
	req := p.request
	delete(f.pending, id)
	f.mu.Unlock()

	text := BuildExpiredMessage(req)
	f.deliverToTargets(targets, text)
}

// resolveTargets 解析转发目标（TS L259-283）。
func (f *ApprovalForwarder) resolveTargets(req ExecApprovalRequest) []ExecApprovalForwardTarget {
	mode := f.cfg.Mode
	if mode == "" {
		mode = ApprovalForwardModeSession
	}
	var targets []ExecApprovalForwardTarget
	seen := make(map[string]bool)

	if mode == ApprovalForwardModeSession || mode == ApprovalForwardModeBoth {
		if f.cfg.ResolveTarget != nil {
			t := f.cfg.ResolveTarget(req)
			if t != nil {
				key := t.Channel + ":" + t.To + ":" + t.AccountID + ":" + t.ThreadID
				if !seen[key] {
					seen[key] = true
					t.Source = "session"
					targets = append(targets, *t)
				}
			}
		}
	}

	if mode == ApprovalForwardModeTargets || mode == ApprovalForwardModeBoth {
		for _, t := range f.cfg.Targets {
			key := t.Channel + ":" + t.To + ":" + t.AccountID + ":" + t.ThreadID
			if seen[key] {
				continue
			}
			seen[key] = true
			t.Source = "target"
			targets = append(targets, t)
		}
	}
	return targets
}

// deliverToTargets 投递消息到所有目标。
func (f *ApprovalForwarder) deliverToTargets(targets []ExecApprovalForwardTarget, text string) {
	if f.cfg.DeliverFunc == nil {
		return
	}
	for _, t := range targets {
		if err := f.cfg.DeliverFunc(t, text); err != nil {
			slog.Error("approval forwarder: deliver failed",
				slog.String("channel", t.Channel),
				slog.String("to", t.To),
				slog.String("source", t.Source),
				slog.String("error", err.Error()),
			)
		}
	}
}
