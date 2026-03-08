package infra

// exec_host.go — 执行宿主 Socket IPC 客户端
// 对应 TS: src/infra/exec-host.ts (121L)
//
// 通过 Unix domain socket 向执行宿主发送命令执行请求。
// 请求经 HMAC-SHA256 签名验证。

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
)

// ─── 类型定义 ───

// ExecHostRequest 执行宿主请求参数。
type ExecHostRequest struct {
	Command              []string          `json:"command"`
	RawCommand           string            `json:"rawCommand,omitempty"`
	Cwd                  string            `json:"cwd,omitempty"`
	Env                  map[string]string `json:"env,omitempty"`
	TimeoutMs            int               `json:"timeoutMs,omitempty"`
	NeedsScreenRecording bool              `json:"needsScreenRecording,omitempty"`
	AgentID              string            `json:"agentId,omitempty"`
	SessionKey           string            `json:"sessionKey,omitempty"`
	ApprovalDecision     string            `json:"approvalDecision,omitempty"` // "allow-once" | "allow-always"
}

// ExecHostRunResult 执行结果。
type ExecHostRunResult struct {
	ExitCode int    `json:"exitCode,omitempty"`
	TimedOut bool   `json:"timedOut"`
	Success  bool   `json:"success"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// ExecHostError 执行错误。
type ExecHostError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

// ExecHostResponse 执行宿主响应（Ok + Payload 或 Error）。
type ExecHostResponse struct {
	Ok      bool               `json:"ok"`
	Payload *ExecHostRunResult `json:"payload,omitempty"`
	Error   *ExecHostError     `json:"error,omitempty"`
}

// ─── IPC 传输 ───

// execHostEnvelope IPC 传输封包。
type execHostEnvelope struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Nonce       string `json:"nonce"`
	Ts          int64  `json:"ts"`
	HMAC        string `json:"hmac"`
	RequestJSON string `json:"requestJson"`
}

// execHostReply IPC 响应解析。
type execHostReply struct {
	Type    string           `json:"type"`
	Ok      *bool            `json:"ok,omitempty"`
	Payload *json.RawMessage `json:"payload,omitempty"`
	Error   *json.RawMessage `json:"error,omitempty"`
}

// RequestExecHostViaSocket 通过 Unix domain socket 向执行宿主发送请求。
// 对应 TS: requestExecHostViaSocket(params)
//
// 返回 nil 表示连接失败或超时（非致命错误）。
func RequestExecHostViaSocket(ctx context.Context, socketPath, token string, request ExecHostRequest, timeoutMs int) *ExecHostResponse {
	if socketPath == "" || token == "" {
		return nil
	}
	if timeoutMs <= 0 {
		timeoutMs = 20_000
	}

	// 带超时的 context
	deadline := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	// 连接 Unix domain socket
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil
	}
	defer conn.Close()

	// 序列化请求
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil
	}

	// 生成 nonce + HMAC 签名
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil
	}
	nonce := hex.EncodeToString(nonceBytes)
	ts := time.Now().UnixMilli()

	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(fmt.Sprintf("%s:%d:%s", nonce, ts, string(requestJSON))))
	hmacHex := hex.EncodeToString(mac.Sum(nil))

	envelope := execHostEnvelope{
		Type:        "exec",
		ID:          uuid.NewString(),
		Nonce:       nonce,
		Ts:          ts,
		HMAC:        hmacHex,
		RequestJSON: string(requestJSON),
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil
	}

	// 发送请求
	payload = append(payload, '\n')
	if _, err := conn.Write(payload); err != nil {
		return nil
	}

	// 读取响应
	resultCh := make(chan *ExecHostResponse, 1)
	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var reply execHostReply
			if err := json.Unmarshal([]byte(line), &reply); err != nil {
				continue
			}
			if reply.Type != "exec-res" {
				continue
			}
			if reply.Ok != nil && *reply.Ok && reply.Payload != nil {
				var result ExecHostRunResult
				if err := json.Unmarshal(*reply.Payload, &result); err == nil {
					resultCh <- &ExecHostResponse{Ok: true, Payload: &result}
					return
				}
			}
			if reply.Ok != nil && !*reply.Ok && reply.Error != nil {
				var errResult ExecHostError
				if err := json.Unmarshal(*reply.Error, &errResult); err == nil {
					resultCh <- &ExecHostResponse{Ok: false, Error: &errResult}
					return
				}
			}
			resultCh <- nil
			return
		}
		resultCh <- nil
	}()

	select {
	case result := <-resultCh:
		return result
	case <-ctx.Done():
		return nil
	}
}
