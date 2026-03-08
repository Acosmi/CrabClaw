package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// 对应 TS src/commands/doctor*.ts（20+ 文件）

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "System health diagnostics",
		Long:  "Run diagnostic checks on the Crab Claw（蟹爪） installation — auth, config, services, sandbox, and security.",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")
			minimal, _ := cmd.Flags().GetBool("minimal")
			autoFix, _ := cmd.Flags().GetBool("yes")
			_ = autoFix // reserved for future auto-fix support

			type checkResult struct {
				Name   string `json:"name"`
				Status string `json:"status"` // "ok", "warn", "fail"
				Detail string `json:"detail,omitempty"`
			}
			var results []checkResult

			add := func(name, status, detail string) {
				results = append(results, checkResult{Name: name, Status: status, Detail: detail})
			}

			stateDir := config.ResolveStateDir()

			// ---------- 检查 1: 配置文件 ----------
			cfgLoader := config.NewConfigLoader()
			snapshot, err := cfgLoader.ReadConfigFileSnapshot()
			if err != nil {
				add("Config", "fail", fmt.Sprintf("无法读取: %v", err))
			} else if !snapshot.Valid {
				detail := "配置无效"
				if len(snapshot.Issues) > 0 {
					detail = snapshot.Issues[0].Message
				}
				add("Config", "fail", detail)
			} else {
				add("Config", "ok", fmt.Sprintf("路径: %s", snapshot.Path))
			}

			// ---------- 检查 2: Gateway 端口 ----------
			port := config.ResolveGatewayPort(nil)
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			conn, dialErr := net.DialTimeout("tcp", addr, 2*time.Second)
			if dialErr != nil {
				add("Gateway", "warn", fmt.Sprintf("端口 %d 未响应（gateway 可能未启动）", port))
			} else {
				conn.Close()
				add("Gateway", "ok", fmt.Sprintf("端口 %d 可达", port))
			}

			// ---------- 检查 3: Auth Profiles ----------
			authProfilesPath := filepath.Join(stateDir, "auth-profiles.json")
			if _, statErr := os.Stat(authProfilesPath); os.IsNotExist(statErr) {
				add("AuthProfiles", "warn", "未找到（可能未登录，运行 'crabclaw setup' 初始化）")
			} else {
				raw, readErr := os.ReadFile(authProfilesPath)
				if readErr != nil {
					add("AuthProfiles", "fail", "无法读取文件")
				} else {
					var profiles map[string]any
					if jsonErr := json.Unmarshal(raw, &profiles); jsonErr != nil {
						add("AuthProfiles", "fail", "JSON 格式错误")
					} else {
						badCount := 0
						for k := range profiles {
							if !strings.Contains(k, ":") {
								badCount++
							}
						}
						if badCount > 0 {
							add("AuthProfiles", "warn", fmt.Sprintf("%d 个 profile，其中 %d 个 ID 格式有误（应为 provider:name）", len(profiles), badCount))
						} else {
							add("AuthProfiles", "ok", fmt.Sprintf("%d 个 profile", len(profiles)))
						}
					}
				}
			}

			// ---------- 检查 4: Device Identity ----------
			devicePath := filepath.Join(stateDir, "identity", "device.json")
			if _, statErr := os.Stat(devicePath); os.IsNotExist(statErr) {
				add("DeviceIdentity", "warn", "未初始化（首次启动 gateway 将自动创建）")
			} else {
				raw, readErr := os.ReadFile(devicePath)
				if readErr != nil {
					add("DeviceIdentity", "fail", "无法读取 device.json")
				} else {
					var id struct {
						Version  int    `json:"version"`
						DeviceID string `json:"deviceId"`
					}
					if jsonErr := json.Unmarshal(raw, &id); jsonErr != nil || id.DeviceID == "" {
						add("DeviceIdentity", "fail", "device.json 格式无效")
					} else {
						short := id.DeviceID
						if len(short) > 12 {
							short = short[:12] + "…"
						}
						add("DeviceIdentity", "ok", fmt.Sprintf("ID: %s", short))
					}
				}
			}

			// ---------- 检查 5: Device Auth ----------
			deviceAuthPath := filepath.Join(stateDir, "identity", "device-auth.json")
			if _, statErr := os.Stat(deviceAuthPath); os.IsNotExist(statErr) {
				add("DeviceAuth", "warn", "未认证（运行 'crabclaw setup' 完成认证）")
			} else {
				add("DeviceAuth", "ok", "认证文件存在")
			}

			// ---------- 检查 6: Gateway Lock ----------
			lockPath := filepath.Join(stateDir, "gateway.lock")
			if raw, readErr := os.ReadFile(lockPath); readErr == nil {
				var lock struct {
					PID int `json:"pid"`
				}
				if json.Unmarshal(raw, &lock) == nil && lock.PID > 0 {
					add("GatewayLock", "ok", fmt.Sprintf("PID %d 持有锁", lock.PID))
				} else {
					add("GatewayLock", "warn", "锁文件存在但格式异常")
				}
			}
			// 无锁文件=gateway 未运行，由 Gateway 端口检查覆盖

			// ---------- 检查 7: TLS 证书 ----------
			tlsCertPath := filepath.Join(stateDir, "tls", "cert.pem")
			tlsKeyPath := filepath.Join(stateDir, "tls", "key.pem")
			if _, statErr := os.Stat(tlsCertPath); os.IsNotExist(statErr) {
				add("TLSCert", "warn", "未找到（启动 gateway 时将自动生成）")
			} else if _, statErr := os.Stat(tlsKeyPath); os.IsNotExist(statErr) {
				add("TLSCert", "fail", "证书存在但私钥缺失")
			} else {
				add("TLSCert", "ok", "证书和私钥均存在")
			}

			// ---------- 检查 8: StateDir 可写性 ----------
			if mkdirErr := os.MkdirAll(stateDir, 0o700); mkdirErr != nil {
				add("StateDirWrite", "fail", fmt.Sprintf("无法创建状态目录: %v", mkdirErr))
			} else {
				tmpFile := filepath.Join(stateDir, fmt.Sprintf(".write_test_%d", time.Now().UnixNano()))
				if writeErr := os.WriteFile(tmpFile, []byte("test"), 0o600); writeErr != nil {
					add("StateDirWrite", "fail", "状态目录不可写")
				} else {
					os.Remove(tmpFile)
					add("StateDirWrite", "ok", stateDir)
				}
			}

			// ---------- 检查 9: 网络连通性 ----------
			netStart := time.Now()
			_, netErr := net.LookupHost("api.anthropic.com")
			netMs := time.Since(netStart).Milliseconds()
			if netErr != nil {
				add("Network", "warn", "DNS 解析 api.anthropic.com 失败（检查网络连接）")
			} else {
				add("Network", "ok", fmt.Sprintf("api.anthropic.com 可达（%dms）", netMs))
			}

			// ---------- 检查 10: Memory DB ----------
			memDBPath := filepath.Join(stateDir, "memory", "memory.db")
			if info, statErr := os.Stat(memDBPath); os.IsNotExist(statErr) {
				add("MemoryDB", "warn", "未初始化（首次运行将自动创建）")
			} else if statErr == nil {
				add("MemoryDB", "ok", fmt.Sprintf("%.1f KB", float64(info.Size())/1024))
			} else {
				add("MemoryDB", "fail", fmt.Sprintf("无法访问: %v", statErr))
			}

			if !minimal {
				// ---------- 检查 11: 可选媒体工具 ----------
				for _, t := range []struct{ Name, Cmd string }{
					{"ffmpeg", "ffmpeg"},
					{"ffprobe", "ffprobe"},
				} {
					if _, lookErr := exec.LookPath(t.Cmd); lookErr != nil {
						add(t.Name, "warn", "未找到（可选，用于音视频处理）")
					} else {
						add(t.Name, "ok", "")
					}
				}

				// ---------- 检查 12: 可选系统工具 ----------
				for _, t := range []struct{ Name, Cmd, Desc string }{
					{"ssh", "ssh", "SSH 隧道"},
					{"git", "git", "Git 操作"},
					{"curl", "curl", "HTTP 请求"},
				} {
					if _, lookErr := exec.LookPath(t.Cmd); lookErr != nil {
						add(t.Name, "warn", fmt.Sprintf("未找到（可选，用于%s）", t.Desc))
					} else {
						add(t.Name, "ok", "")
					}
				}

				// ---------- 检查 13: Hooks 目录 ----------
				hooksDir := filepath.Join(stateDir, "hooks")
				if _, statErr := os.Stat(hooksDir); os.IsNotExist(statErr) {
					add("HooksDir", "ok", "未配置（可选）")
				} else {
					entries, _ := os.ReadDir(hooksDir)
					add("HooksDir", "ok", fmt.Sprintf("%d 个文件", len(entries)))
				}

				// ---------- 检查 14: Skills 目录 ----------
				skillsDir := filepath.Join(stateDir, "skills")
				if _, statErr := os.Stat(skillsDir); os.IsNotExist(statErr) {
					add("SkillsDir", "ok", "未配置（可选）")
				} else {
					entries, _ := os.ReadDir(skillsDir)
					add("SkillsDir", "ok", fmt.Sprintf("%d 个文件", len(entries)))
				}

				// ---------- 检查 15: OAuth 凭证 ----------
				oauthPath := config.ResolveOAuthPath()
				if _, statErr := os.Stat(oauthPath); os.IsNotExist(statErr) {
					add("OAuth", "warn", "未找到（OAuth 未配置）")
				} else {
					add("OAuth", "ok", "凭证文件存在")
				}

				// ---------- 检查 16: 二进制版本 ----------
				versionStr := "(开发版)"
				if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
					versionStr = info.Main.Version
				}
				add("Version", "ok", versionStr)
			}

			// ---------- 输出结果 ----------
			if jsonFlag {
				data, _ := json.MarshalIndent(results, "", "  ")
				cmd.Println(string(data))
			} else {
				cmd.Println("🩺 Crab Claw（蟹爪）诊断报告")
				cmd.Println()
				failCount, warnCount := 0, 0
				for _, r := range results {
					icon := "✅"
					switch r.Status {
					case "warn":
						icon = "⚠️ "
						warnCount++
					case "fail":
						icon = "❌"
						failCount++
					}
					if r.Detail != "" {
						cmd.Printf("  %s %-18s %s\n", icon, r.Name+":", r.Detail)
					} else {
						cmd.Printf("  %s %s: OK\n", icon, r.Name)
					}
				}
				cmd.Println()
				switch {
				case failCount > 0:
					cmd.Printf("  总计: %d 项失败, %d 项警告\n", failCount, warnCount)
				case warnCount > 0:
					cmd.Printf("  总计: %d 项警告（非致命，功能可能受限）\n", warnCount)
				default:
					cmd.Println("  ✅ 所有检查通过")
				}
			}

			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "Auto-fix issues without prompting")
	cmd.Flags().Bool("minimal", false, "Run minimal checks only")
	cmd.Flags().Bool("json", false, "Output in JSON format")
	return cmd
}
