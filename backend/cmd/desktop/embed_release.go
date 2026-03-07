//go:build desktopembed

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend/dist
var embeddedDesktopAssets embed.FS

func desktopEmbeddedControlUIFS() fs.FS {
	return embeddedDesktopAssets
}
