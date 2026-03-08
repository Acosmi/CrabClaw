// platform_clipboard.go — 剪贴板操作封装 (S2-7: HIDDEN-8)
//
// 封装 github.com/atotto/clipboard（已在 go.mod 中）。
// 提供跨平台剪贴板读写。

package infra

import "github.com/atotto/clipboard"

// ReadClipboard 从系统剪贴板读取文本。
// macOS: pbpaste, Linux: xclip/xsel, Windows: clipboard API
func ReadClipboard() (string, error) {
	return clipboard.ReadAll()
}

// WriteClipboard 写入文本到系统剪贴板。
func WriteClipboard(text string) error {
	return clipboard.WriteAll(text)
}

// IsClipboardAvailable 检查系统剪贴板是否可用。
// 在无头环境或 SSH 远程中剪贴板可能不可用。
func IsClipboardAvailable() bool {
	return !clipboard.Unsupported
}
