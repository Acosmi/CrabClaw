package gateway

// remote_approval_dingtalk.go — P4 钉钉远程审批 Provider
// 通过钉钉自定义机器人 Webhook 发送 ActionCard 消息。
//
// 实现方式：标准库 HTTP 调用
// 优势：自定义机器人 Webhook 无需 App 认证，配置最简单。
//
// API 参考：
//   - 自定义机器人: POST https://oapi.dingtalk.com/robot/send?access_token=xxx
//   - 消息类型: ActionCard（整体跳转 + 独立跳转按钮）

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// dingtalkProvider 钉钉远程审批 Provider。
type dingtalkProvider struct {
	config *DingTalkProviderConfig
	client *http.Client
}

// newDingTalkProvider 创建钉钉 Provider。
func newDingTalkProvider(cfg *DingTalkProviderConfig) *dingtalkProvider {
	return &dingtalkProvider{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *dingtalkProvider) Name() string { return "dingtalk" }

func (p *dingtalkProvider) ValidateConfig() error {
	if p.config.WebhookURL == "" {
		return fmt.Errorf("钉钉 Webhook URL 不能为空")
	}
	return nil
}

func (p *dingtalkProvider) SendApprovalRequest(ctx context.Context, req ApprovalCardRequest) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	// 构建 ActionCard 消息
	card := p.buildActionCard(req)
	body, err := json.Marshal(card)
	if err != nil {
		return err
	}

	// 构建签名 URL
	webhookURL, err := p.buildSignedURL()
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
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
		return fmt.Errorf("钉钉响应解析失败: %s", string(respBody))
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("钉钉消息发送失败: errcode=%d, errmsg=%s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// buildActionCard 构建钉钉 ActionCard 消息。
func (p *dingtalkProvider) buildActionCard(req ApprovalCardRequest) map[string]interface{} {
	levelLabel := map[string]string{
		"full":      "🔴 L3 — 完全权限",
		"sandboxed": "🟠 L2 — 沙盒执行",
		"allowlist": "🟡 L1 — 受限执行",
	}[req.RequestedLevel]
	if levelLabel == "" {
		levelLabel = req.RequestedLevel
	}

	approveURL := fmt.Sprintf("%s?action=approve&id=%s", req.CallbackURL, req.EscalationID)
	if !isPermanentApprovalLevel(req.RequestedLevel) {
		approveURL = fmt.Sprintf("%s&ttl=%d", approveURL, req.TTLMinutes)
	}
	denyURL := fmt.Sprintf("%s?action=deny&id=%s", req.CallbackURL, req.EscalationID)

	durationLine := fmt.Sprintf("**授权时长**: %d 分钟", req.TTLMinutes)
	if isPermanentApprovalLevel(req.RequestedLevel) {
		durationLine = "**授权模式**: 永久授权"
	}
	markdown := fmt.Sprintf("## 🔐 权限提升审批\n\n"+
		"**请求级别**: %s\n\n"+
		"**原因**: %s\n\n"+
		"%s\n\n"+
		"**请求时间**: %s\n\n"+
		"---\n\n"+
		"ID: `%s`",
		levelLabel,
		req.Reason,
		durationLine,
		req.RequestedAt.Format("2006-01-02 15:04:05"),
		req.EscalationID,
	)

	return map[string]interface{}{
		"msgtype": "actionCard",
		"actionCard": map[string]interface{}{
			"title":          "🔐 权限提升审批",
			"text":           markdown,
			"btnOrientation": "1", // 横向排列
			"btns": []interface{}{
				map[string]interface{}{
					"title":     "✅ 批准",
					"actionURL": approveURL,
				},
				map[string]interface{}{
					"title":     "❌ 拒绝",
					"actionURL": denyURL,
				},
			},
		},
	}
}

// buildSignedURL 构建带签名的 Webhook URL（如果配置了签名密钥）。
func (p *dingtalkProvider) buildSignedURL() (string, error) {
	if p.config.WebhookSecret == "" {
		return p.config.WebhookURL, nil
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	stringToSign := timestamp + "\n" + p.config.WebhookSecret

	mac := hmac.New(sha256.New, []byte(p.config.WebhookSecret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	u, err := url.Parse(p.config.WebhookURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("timestamp", timestamp)
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ---------- Phase 8: 审批结果通知 ----------

// SendResultNotification 实现 ResultNotifier 接口，推送审批结果 ActionCard。
func (p *dingtalkProvider) SendResultNotification(ctx context.Context, result ApprovalResultNotification) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	card := p.buildResultCard(result)
	body, err := json.Marshal(card)
	if err != nil {
		return err
	}

	webhookURL, err := p.buildSignedURL()
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
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
	var apiResult struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &apiResult); err != nil {
		return fmt.Errorf("钉钉响应解析失败: %s", string(respBody))
	}
	if apiResult.ErrCode != 0 {
		return fmt.Errorf("钉钉结果通知发送失败: errcode=%d, errmsg=%s", apiResult.ErrCode, apiResult.ErrMsg)
	}
	return nil
}

// buildResultCard 构建审批结果 ActionCard。
func (p *dingtalkProvider) buildResultCard(result ApprovalResultNotification) map[string]interface{} {
	var title, markdown string

	if result.Approved {
		title = "✅ 权限已生效"
		if isPermanentApprovalLevel(result.RequestedLevel) {
			markdown = fmt.Sprintf("## ✅ 权限已生效\n\n"+
				"**授权级别**: %s\n\n"+
				"**授权模式**: 永久授权\n\n"+
				"该权限将持续生效，直到手动改回。\n\n"+
				"---\n\nID: `%s`",
				result.RequestedLevel, result.EscalationID)
		} else {
			markdown = fmt.Sprintf("## ✅ 权限已生效\n\n"+
				"**授权级别**: %s\n\n"+
				"**有效时长**: %d 分钟\n\n"+
				"权限到期后将自动降级。\n\n"+
				"---\n\nID: `%s`",
				result.RequestedLevel, result.TTLMinutes, result.EscalationID)
		}
	} else {
		title = "❌ 权限请求已拒绝"
		reason := result.Reason
		if reason == "" {
			reason = "管理员拒绝"
		}
		markdown = fmt.Sprintf("## ❌ 权限请求已拒绝\n\n"+
			"**拒绝原因**: %s\n\n"+
			"相关任务已暂停执行。如需继续，请重新发起权限申请。\n\n"+
			"---\n\nID: `%s`",
			reason, result.EscalationID)
	}

	return map[string]interface{}{
		"msgtype": "actionCard",
		"actionCard": map[string]interface{}{
			"title":          title,
			"text":           markdown,
			"btnOrientation": "0",
			"singleTitle":    "已处理",
			"singleURL":      "dingtalk://dingtalkclient/page/link",
		},
	}
}
