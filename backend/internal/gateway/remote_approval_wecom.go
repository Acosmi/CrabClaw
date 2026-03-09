package gateway

// remote_approval_wecom.go — P4 企业微信远程审批 Provider
// 通过企业微信应用消息 API 发送文本卡片消息。
//
// 实现方式：标准库 HTTP 调用（企业微信无官方 Go SDK）
//
// API 参考：
//   - 获取 access_token: GET https://qyapi.weixin.qq.com/cgi-bin/gettoken
//   - 发送应用消息: POST https://qyapi.weixin.qq.com/cgi-bin/message/send

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	wecomTokenURL   = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	wecomMessageURL = "https://qyapi.weixin.qq.com/cgi-bin/message/send"
)

// wecomProvider 企业微信远程审批 Provider。
type wecomProvider struct {
	config *WeComProviderConfig
	client *http.Client
}

// newWeComProvider 创建企业微信 Provider。
func newWeComProvider(cfg *WeComProviderConfig) *wecomProvider {
	return &wecomProvider{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *wecomProvider) Name() string { return "wecom" }

func (p *wecomProvider) ValidateConfig() error {
	if p.config.CorpID == "" {
		return fmt.Errorf("企业微信 Corp ID 不能为空")
	}
	if p.config.Secret == "" {
		return fmt.Errorf("企业微信 Secret 不能为空")
	}
	if p.config.AgentID <= 0 {
		return fmt.Errorf("企业微信 Agent ID 无效")
	}
	if p.config.ToUser == "" && p.config.ToParty == "" {
		return fmt.Errorf("企业微信需要指定接收用户或部门")
	}
	return nil
}

func (p *wecomProvider) SendApprovalRequest(ctx context.Context, req ApprovalCardRequest) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	// 1. 获取 access_token
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取企业微信 access_token 失败: %w", err)
	}

	// 2. 构建文本卡片消息
	msg := p.buildTextCard(req)

	// 3. 发送消息
	return p.sendMessage(ctx, token, msg)
}

// getAccessToken 获取企业微信 access_token。
func (p *wecomProvider) getAccessToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s?corpid=%s&corpsecret=%s", wecomTokenURL, p.config.CorpID, p.config.Secret)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("企业微信 token API 错误: errcode=%d, errmsg=%s", result.ErrCode, result.ErrMsg)
	}
	return result.AccessToken, nil
}

// buildTextCard 构建企业微信文本卡片消息。
func (p *wecomProvider) buildTextCard(req ApprovalCardRequest) map[string]interface{} {
	levelLabel := map[string]string{
		"full":      "🔴 L3 完全权限",
		"sandboxed": "🟠 L2 沙盒执行",
		"allowlist": "🟡 L1 受限执行",
	}[req.RequestedLevel]
	if levelLabel == "" {
		levelLabel = req.RequestedLevel
	}

	durationLine := fmt.Sprintf("授权时长: %d 分钟", req.TTLMinutes)
	if isPermanentApprovalLevel(req.RequestedLevel) {
		durationLine = "授权模式: 永久授权"
	}
	description := fmt.Sprintf(
		"请求级别: %s\n原因: %s\n%s\n请求时间: %s\nID: %s",
		levelLabel,
		req.Reason,
		durationLine,
		req.RequestedAt.Format("2006-01-02 15:04:05"),
		req.EscalationID,
	)

	// 企业微信文本卡片仅支持一个跳转 URL
	approveURL := fmt.Sprintf("%s?action=approve&id=%s", req.CallbackURL, req.EscalationID)
	if !isPermanentApprovalLevel(req.RequestedLevel) {
		approveURL = fmt.Sprintf("%s&ttl=%d", approveURL, req.TTLMinutes)
	}

	msg := map[string]interface{}{
		"msgtype": "textcard",
		"agentid": p.config.AgentID,
		"textcard": map[string]interface{}{
			"title":       "🔐 权限提升审批",
			"description": description,
			"url":         approveURL,
			"btntxt":      "审批操作",
		},
	}

	if p.config.ToUser != "" {
		msg["touser"] = p.config.ToUser
	}
	if p.config.ToParty != "" {
		msg["toparty"] = p.config.ToParty
	}

	return msg
}

// sendMessage 发送企业微信消息。
func (p *wecomProvider) sendMessage(ctx context.Context, token string, msg map[string]interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s?access_token=%s", wecomMessageURL, token)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("企业微信响应解析失败: %s", string(respBody))
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("企业微信消息发送失败: errcode=%d, errmsg=%s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// ---------- Phase 8: 审批结果通知 ----------

// SendResultNotification 实现 ResultNotifier 接口，推送审批结果文本卡片。
func (p *wecomProvider) SendResultNotification(ctx context.Context, result ApprovalResultNotification) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取企业微信 access_token 失败: %w", err)
	}

	msg := p.buildResultTextCard(result)
	return p.sendMessage(ctx, token, msg)
}

// buildResultTextCard 构建审批结果文本卡片。
func (p *wecomProvider) buildResultTextCard(result ApprovalResultNotification) map[string]interface{} {
	var title, description string

	if result.Approved {
		title = "✅ 权限已生效"
		if isPermanentApprovalLevel(result.RequestedLevel) {
			description = fmt.Sprintf(
				"授权级别: %s\n授权模式: 永久授权\n\n该权限将持续生效，直到手动改回。\nID: %s",
				result.RequestedLevel, result.EscalationID)
		} else {
			description = fmt.Sprintf(
				"授权级别: %s\n有效时长: %d 分钟\n\n权限到期后将自动降级。\nID: %s",
				result.RequestedLevel, result.TTLMinutes, result.EscalationID)
		}
	} else {
		reason := result.Reason
		if reason == "" {
			reason = "管理员拒绝"
		}
		title = "❌ 权限请求已拒绝"
		description = fmt.Sprintf(
			"拒绝原因: %s\n\n相关任务已暂停执行。如需继续，请重新发起权限申请。\nID: %s",
			reason, result.EscalationID)
	}

	msg := map[string]interface{}{
		"msgtype": "textcard",
		"agentid": p.config.AgentID,
		"textcard": map[string]interface{}{
			"title":       title,
			"description": description,
			"url":         "https://open.work.weixin.qq.com",
			"btntxt":      "已处理",
		},
	}

	if p.config.ToUser != "" {
		msg["touser"] = p.config.ToUser
	}
	if p.config.ToParty != "" {
		msg["toparty"] = p.config.ToParty
	}

	return msg
}
