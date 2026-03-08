package infra

import (
	"fmt"
	"net"
	"testing"
)

func TestEnsurePortAvailable_Free(t *testing.T) {
	// 找一个空闲端口
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// 端口应该可用
	if err := EnsurePortAvailable(port); err != nil {
		t.Errorf("expected port %d to be available, got: %v", port, err)
	}
}

func TestEnsurePortAvailable_Busy(t *testing.T) {
	// 占用一个端口
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	// 端口应该不可用
	err = EnsurePortAvailable(port)
	if err == nil {
		t.Error("expected error for busy port")
	}
	portErr, ok := err.(*PortInUseError)
	if !ok {
		t.Fatalf("expected PortInUseError, got %T", err)
	}
	if portErr.Port != port {
		t.Errorf("PortInUseError.Port = %d, want %d", portErr.Port, port)
	}
}

func TestParseLsofFieldOutput(t *testing.T) {
	// lsof -Fpcn 格式模拟
	output := `p1234
cnode
n127.0.0.1:19001
p5678
cpython
n*:3000
`
	listeners := parseLsofFieldOutput(output)
	if len(listeners) != 2 {
		t.Fatalf("expected 2 listeners, got %d", len(listeners))
	}

	if listeners[0].PID != 1234 {
		t.Errorf("listener[0].PID = %d, want 1234", listeners[0].PID)
	}
	if listeners[0].Command != "node" {
		t.Errorf("listener[0].Command = %q, want %q", listeners[0].Command, "node")
	}
	if listeners[0].Address != "127.0.0.1:19001" {
		t.Errorf("listener[0].Address = %q, want %q", listeners[0].Address, "127.0.0.1:19001")
	}

	if listeners[1].PID != 5678 {
		t.Errorf("listener[1].PID = %d, want 5678", listeners[1].PID)
	}
	if listeners[1].Command != "python" {
		t.Errorf("listener[1].Command = %q, want %q", listeners[1].Command, "python")
	}
}

func TestParseNetstatListeners(t *testing.T) {
	output := `Active Internet connections (only servers)
Proto    Local Address    Foreign Address    State    PID
TCP      0.0.0.0:19001   0.0.0.0:0          LISTEN   1234
TCP      0.0.0.0:3000    0.0.0.0:0          LISTEN   5678
`
	listeners := parseNetstatListeners(output, 19001)
	if len(listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(listeners))
	}
	if listeners[0].PID != 1234 {
		t.Errorf("listener[0].PID = %d, want 1234", listeners[0].PID)
	}
}

func TestClassifyPortListener(t *testing.T) {
	tests := []struct {
		listener PortListener
		want     PortListenerKind
	}{
		{PortListener{Command: "node", CommandLine: "/usr/bin/openacosmi start"}, PortKindGateway},
		{PortListener{Command: "ssh", CommandLine: "ssh -L 8080:localhost:80"}, PortKindSSH},
		{PortListener{Command: "python"}, PortKindUnknown},
	}
	for _, tt := range tests {
		got := ClassifyPortListener(tt.listener, 19001)
		if got != tt.want {
			t.Errorf("ClassifyPortListener(%+v) = %q, want %q", tt.listener, got, tt.want)
		}
	}
}

func TestBuildPortHints(t *testing.T) {
	// 空监听者
	hints := BuildPortHints(nil, 19001)
	if len(hints) != 0 {
		t.Errorf("expected no hints for empty listeners, got %d", len(hints))
	}

	// gateway 监听者
	listeners := []PortListener{{Command: "openacosmi"}}
	hints = BuildPortHints(listeners, 19001)
	if len(hints) == 0 {
		t.Error("expected hints for gateway listener")
	}
	found := false
	for _, h := range hints {
		if contains(h, "Gateway") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected gateway hint")
	}
}

func TestFormatPortListener(t *testing.T) {
	l := PortListener{
		PID:         1234,
		User:        "root",
		CommandLine: "/usr/bin/node server.js",
		Address:     "0.0.0.0:19001",
	}
	s := FormatPortListener(l)
	if !contains(s, "pid 1234") {
		t.Errorf("expected 'pid 1234' in %q", s)
	}
	if !contains(s, "root") {
		t.Errorf("expected 'root' in %q", s)
	}
	if !contains(s, "node server.js") {
		t.Errorf("expected 'node server.js' in %q", s)
	}
}

func TestFormatPortDiagnostics(t *testing.T) {
	// Free port
	usage := PortUsage{Port: 8080, Status: PortFree}
	lines := FormatPortDiagnostics(usage)
	if len(lines) != 1 || !contains(lines[0], "free") {
		t.Errorf("expected free message, got %v", lines)
	}

	// Busy port
	usage = PortUsage{
		Port:      19001,
		Status:    PortBusy,
		Listeners: []PortListener{{PID: 1234, Command: "node"}},
		Hints:     []string{"stop the process"},
	}
	lines = FormatPortDiagnostics(usage)
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines, got %d", len(lines))
	}
	if !contains(lines[0], "already in use") {
		t.Errorf("expected 'already in use' in %q", lines[0])
	}
}

func TestPortInUseError(t *testing.T) {
	err := &PortInUseError{Port: 19001, Details: "used by node"}
	msg := err.Error()
	if !contains(msg, "19001") || !contains(msg, "used by node") {
		t.Errorf("unexpected error message: %q", msg)
	}
}

func TestCheckPortInUse(t *testing.T) {
	// 占用一个端口
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	status := checkPortInUse(port)
	if status != PortBusy {
		t.Errorf("expected busy for occupied port, got %q", status)
	}
}

func TestTryListen_Free(t *testing.T) {
	// 找一个空闲端口
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	status := tryListen(port, "127.0.0.1")
	if status != PortFree {
		t.Errorf("expected free, got %q", status)
	}
}

func TestResolveLsofCommand(t *testing.T) {
	cmd := resolveLsofCommand()
	if cmd == "" {
		t.Error("resolveLsofCommand returned empty string")
	}
	// 在 macOS/Linux 上应该返回一个路径
	fmt.Printf("lsof resolved to: %s\n", cmd)
}

// contains 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
