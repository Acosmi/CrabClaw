//go:build linux

package daemon

import (
	"fmt"
	"sort"
	"strings"
)

// SystemdUnitArgs 对应 TS: systemd-unit.ts buildSystemdUnit 参数类型
type SystemdUnitArgs struct {
	Description      string            // [Unit] Description=
	ProgramArguments []string          // ExecStart 命令行参数
	WorkingDirectory string            // WorkingDirectory=（可选）
	Environment      map[string]string // Environment= 行（可选）
}

// systemdEscapeArg 对包含空白、引号或反斜杠的参数加双引号并转义。
// 对应 TS: systemd-unit.ts systemdEscapeArg
func systemdEscapeArg(value string) string {
	needsQuote := strings.ContainsAny(value, " \t\n\"\\")
	if !needsQuote {
		return value
	}
	// 先转义反斜杠，再转义双引号
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`"%s"`, escaped)
}

// renderEnvLines 将环境变量 map 转换为 systemd Environment= 行列表。
// 对应 TS: systemd-unit.ts renderEnvLines
// 为保证输出确定性，按 key 字母顺序排列。
func renderEnvLines(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}

	// 收集非空 value 的条目
	type kv struct{ k, v string }
	var entries []kv
	for k, v := range env {
		if strings.TrimSpace(v) != "" {
			entries = append(entries, kv{k, strings.TrimSpace(v)})
		}
	}
	if len(entries) == 0 {
		return nil
	}

	// 按 key 排序，保证输出确定
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].k < entries[j].k
	})

	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		assignment := fmt.Sprintf("%s=%s", e.k, e.v)
		lines = append(lines, "Environment="+systemdEscapeArg(assignment))
	}
	return lines
}

// buildSystemdUnitContent 生成 systemd service unit 文件的完整内容。
// 对应 TS: systemd-unit.ts buildSystemdUnit
//
// 生成格式示例：
//
//	[Unit]
//	Description=Crab Claw Gateway
//	After=network-online.target
//	Wants=network-online.target
//
//	[Service]
//	ExecStart=/usr/bin/openacosmi --flag
//	Restart=always
//	RestartSec=5
//	KillMode=process
//	WorkingDirectory=/home/user
//	Environment="KEY=VALUE"
//
//	[Install]
//	WantedBy=default.target
func buildSystemdUnitContent(args SystemdUnitArgs) string {
	// [Unit] 段
	description := args.Description
	if strings.TrimSpace(description) == "" {
		description = GatewayDisplayName
	}

	// ExecStart：逐参数 escape 后空格拼接
	var escapedArgs []string
	for _, a := range args.ProgramArguments {
		escapedArgs = append(escapedArgs, systemdEscapeArg(a))
	}
	execStart := strings.Join(escapedArgs, " ")

	// WorkingDirectory 行（可选）
	var workingDirLine string
	if args.WorkingDirectory != "" {
		workingDirLine = "WorkingDirectory=" + systemdEscapeArg(args.WorkingDirectory)
	}

	// Environment= 行
	envLines := renderEnvLines(args.Environment)

	// 按照 TS 版本 filter(line => line !== null).join("\n") 的逻辑组装
	var parts []string
	parts = append(parts,
		"[Unit]",
		"Description="+description,
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"ExecStart="+execStart,
		"Restart=always",
		"RestartSec=5",
		// KillMode=process 确保 systemd 只等待主进程退出，
		// 避免 podman 的 conmon 子进程阻塞关机（与 TS 注释一致）
		"KillMode=process",
	)
	if workingDirLine != "" {
		parts = append(parts, workingDirLine)
	}
	parts = append(parts, envLines...)
	parts = append(parts,
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	)

	return strings.Join(parts, "\n")
}

// parseSystemdExecStart 将 systemd ExecStart 值解析回参数列表。
// 对应 TS: systemd-unit.ts parseSystemdExecStart
func parseSystemdExecStart(value string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false
	escapeNext := false

	for _, ch := range value {
		if escapeNext {
			current.WriteRune(ch)
			escapeNext = false
			continue
		}
		if ch == '\\' {
			escapeNext = true
			continue
		}
		if ch == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(ch)
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// parseSystemdEnvAssignment 解析 systemd Environment= 行的赋值内容（去掉 "Environment=" 前缀后的部分）。
// 返回 key/value 对，解析失败返回 false。
// 对应 TS: systemd-unit.ts parseSystemdEnvAssignment
func parseSystemdEnvAssignment(raw string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}

	// 如果整体被双引号包裹，先去引号并处理转义
	unquoted := trimmed
	if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		inner := trimmed[1 : len(trimmed)-1]
		var out strings.Builder
		escNext := false
		for _, ch := range inner {
			if escNext {
				out.WriteRune(ch)
				escNext = false
				continue
			}
			if ch == '\\' {
				escNext = true
				continue
			}
			out.WriteRune(ch)
		}
		unquoted = out.String()
	}

	eq := strings.Index(unquoted, "=")
	if eq <= 0 {
		return "", "", false
	}
	k := strings.TrimSpace(unquoted[:eq])
	if k == "" {
		return "", "", false
	}
	v := unquoted[eq+1:]
	return k, v, true
}
