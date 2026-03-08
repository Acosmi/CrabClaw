package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var extraMarkers = []string{"openacosmi", "clawdbot", "moltbot"}

// FindExtraGatewayServices 扫描系统中的其他 OpenAcosmi/Clawdbot/Moltbot 服务实例
// 对应 TS: inspect.ts findExtraGatewayServices
func FindExtraGatewayServices(env map[string]string, deep bool) []ExtraGatewayService {
	var results []ExtraGatewayService
	seen := make(map[string]bool)

	push := func(svc ExtraGatewayService) {
		key := svc.Platform + ":" + svc.Label + ":" + svc.Detail + ":" + svc.Scope
		if seen[key] {
			return
		}
		seen[key] = true
		results = append(results, svc)
	}

	switch runtime.GOOS {
	case "darwin":
		findExtraDarwinServices(env, deep, push)
	case "linux":
		findExtraLinuxServices(env, deep, push)
	case "windows":
		if deep {
			findExtraWindowsServices(push)
		}
	}

	return results
}

func findExtraDarwinServices(env map[string]string, deep bool, push func(ExtraGatewayService)) {
	home, err := ResolveHomeDir(env)
	if err != nil {
		return
	}

	userDir := filepath.Join(home, "Library", "LaunchAgents")
	scanLaunchdDir(userDir, "user", push)

	if deep {
		scanLaunchdDir("/Library/LaunchAgents", "system", push)
		scanLaunchdDir("/Library/LaunchDaemons", "system", push)
	}
}

func findExtraLinuxServices(env map[string]string, deep bool, push func(ExtraGatewayService)) {
	home, err := ResolveHomeDir(env)
	if err != nil {
		return
	}

	userDir := filepath.Join(home, ".config", "systemd", "user")
	scanSystemdDir(userDir, "user", push)

	if deep {
		for _, dir := range []string{
			"/etc/systemd/system",
			"/usr/lib/systemd/system",
			"/lib/systemd/system",
		} {
			scanSystemdDir(dir, "system", push)
		}
	}
}

func findExtraWindowsServices(push func(ExtraGatewayService)) {
	cmd := exec.Command("schtasks", "/Query", "/FO", "LIST", "/V")
	out, err := cmd.Output()
	if err != nil {
		return
	}

	tasks := parseSchtasksList(string(out))
	for _, task := range tasks {
		name := strings.TrimSpace(task.name)
		if name == "" {
			continue
		}
		if isOpenAcosmiGatewayTaskName(name) {
			continue
		}
		lowerName := strings.ToLower(name)
		lowerCmd := strings.ToLower(task.taskToRun)
		var marker string
		for _, candidate := range extraMarkers {
			if strings.Contains(lowerName, candidate) || strings.Contains(lowerCmd, candidate) {
				marker = candidate
				break
			}
		}
		if marker == "" {
			continue
		}
		detail := name
		if task.taskToRun != "" {
			detail = "task: " + name + ", run: " + task.taskToRun
		}
		push(ExtraGatewayService{
			Platform: "windows",
			Label:    name,
			Detail:   detail,
			Scope:    "system",
			Marker:   marker,
			Legacy:   marker != "openacosmi",
		})
	}
}

type schtaskEntry struct {
	name      string
	taskToRun string
}

func parseSchtasksList(output string) []schtaskEntry {
	var tasks []schtaskEntry
	var current *schtaskEntry

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimRight(rawLine, "\r"))
		if line == "" {
			if current != nil {
				tasks = append(tasks, *current)
				current = nil
			}
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])
		if value == "" {
			continue
		}
		if key == "taskname" {
			if current != nil {
				tasks = append(tasks, *current)
			}
			current = &schtaskEntry{name: value}
			continue
		}
		if current == nil {
			continue
		}
		if key == "task to run" {
			current.taskToRun = value
		}
	}
	if current != nil {
		tasks = append(tasks, *current)
	}
	return tasks
}

func scanLaunchdDir(dir, scope string, push func(ExtraGatewayService)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	currentLabel := ResolveGatewayLaunchAgentLabel("")
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".plist") {
			continue
		}
		labelFromName := strings.TrimSuffix(name, ".plist")
		if labelFromName == currentLabel {
			continue
		}

		fullPath := filepath.Join(dir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		contents := string(data)

		marker := detectMarker(contents)
		if marker == "" {
			if isLegacyLabel(labelFromName) {
				m := "clawdbot"
				if strings.Contains(strings.ToLower(labelFromName), "moltbot") {
					m = "moltbot"
				}
				push(ExtraGatewayService{
					Platform: "darwin",
					Label:    labelFromName,
					Detail:   "plist: " + fullPath,
					Scope:    scope,
					Marker:   m,
					Legacy:   true,
				})
			}
			continue
		}
		label := labelFromName
		// 尝试从 plist 内容中提取 label
		if extracted := tryExtractPlistLabelInline(contents); extracted != "" {
			label = extracted
		}
		if label == currentLabel {
			continue
		}
		if marker == "openacosmi" && isOpenAcosmiGatewayLaunchdService(label, contents) {
			continue
		}
		push(ExtraGatewayService{
			Platform: "darwin",
			Label:    label,
			Detail:   "plist: " + fullPath,
			Scope:    scope,
			Marker:   marker,
			Legacy:   marker != "openacosmi" || isLegacyLabel(label),
		})
	}
}

func scanSystemdDir(dir, scope string, push func(ExtraGatewayService)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	currentName := ResolveGatewaySystemdServiceName("")
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".service") {
			continue
		}
		svcName := strings.TrimSuffix(name, ".service")
		if svcName == currentName {
			continue
		}

		fullPath := filepath.Join(dir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		contents := string(data)

		marker := detectMarker(contents)
		if marker == "" {
			continue
		}
		if marker == "openacosmi" && isOpenAcosmiGatewaySystemdService(svcName, contents) {
			continue
		}
		push(ExtraGatewayService{
			Platform: "linux",
			Label:    name,
			Detail:   "unit: " + fullPath,
			Scope:    scope,
			Marker:   marker,
			Legacy:   marker != "openacosmi",
		})
	}
}

func detectMarker(content string) string {
	lower := strings.ToLower(content)
	for _, marker := range extraMarkers {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

func isLegacyLabel(label string) bool {
	lower := strings.ToLower(label)
	return strings.Contains(lower, "clawdbot") || strings.Contains(lower, "moltbot")
}

func hasGatewayServiceMarker(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "openacosmi_service_marker") &&
		strings.Contains(lower, "openacosmi_service_kind") &&
		strings.Contains(lower, strings.ToLower(GatewayServiceMarker)) &&
		strings.Contains(lower, strings.ToLower(GatewayServiceKind))
}

func isOpenAcosmiGatewayLaunchdService(label, contents string) bool {
	if hasGatewayServiceMarker(contents) {
		return true
	}
	if !strings.Contains(strings.ToLower(contents), "gateway") {
		return false
	}
	return isCompatibleGatewayLaunchAgentLabel(label)
}

func isOpenAcosmiGatewaySystemdService(name, contents string) bool {
	if hasGatewayServiceMarker(contents) {
		return true
	}
	if !isCompatibleGatewaySystemdServiceName(name) {
		return false
	}
	return strings.Contains(strings.ToLower(contents), "gateway")
}

func isOpenAcosmiGatewayTaskName(name string) bool {
	normalized := strings.TrimSpace(strings.ToLower(name))
	if normalized == "" {
		return false
	}
	for _, candidate := range ResolveCompatibleGatewayWindowsTaskNames("") {
		if normalized == strings.ToLower(candidate) {
			return true
		}
	}
	return strings.HasPrefix(normalized, "openacosmi gateway") ||
		strings.HasPrefix(normalized, "crab claw gateway")
}

func isCompatibleGatewayLaunchAgentLabel(label string) bool {
	normalized := strings.TrimSpace(strings.ToLower(label))
	return strings.HasPrefix(normalized, "ai.openacosmi.") ||
		strings.HasPrefix(normalized, "ai.crabclaw.")
}

func isCompatibleGatewaySystemdServiceName(name string) bool {
	normalized := strings.TrimSpace(strings.ToLower(name))
	return strings.HasPrefix(normalized, "openacosmi-gateway") ||
		strings.HasPrefix(normalized, "crabclaw-gateway")
}

func tryExtractPlistLabelInline(contents string) string {
	// 简单提取
	idx := strings.Index(contents, "<key>Label</key>")
	if idx < 0 {
		return ""
	}
	rest := contents[idx:]
	startTag := strings.Index(rest, "<string>")
	endTag := strings.Index(rest, "</string>")
	if startTag < 0 || endTag < 0 || endTag <= startTag {
		return ""
	}
	return strings.TrimSpace(rest[startTag+8 : endTag])
}

// RenderGatewayServiceCleanupHints 生成 gateway 服务清理命令提示
// 对应 TS: inspect.ts renderGatewayServiceCleanupHints
func RenderGatewayServiceCleanupHints(env map[string]string) []string {
	profile := env["OPENACOSMI_PROFILE"]
	switch runtime.GOOS {
	case "darwin":
		label := ResolveGatewayLaunchAgentLabel(profile)
		return []string{
			"launchctl bootout gui/$UID/" + label,
			"rm ~/Library/LaunchAgents/" + label + ".plist",
		}
	case "linux":
		unit := ResolveGatewaySystemdServiceName(profile)
		return []string{
			"systemctl --user disable --now " + unit + ".service",
			"rm ~/.config/systemd/user/" + unit + ".service",
		}
	case "windows":
		task := ResolveGatewayWindowsTaskName(profile)
		return []string{
			`schtasks /Delete /TN "` + task + `" /F`,
		}
	default:
		return nil
	}
}
