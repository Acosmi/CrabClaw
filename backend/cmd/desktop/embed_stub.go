//go:build !desktopembed

package main

import "io/fs"

func desktopEmbeddedControlUIFS() fs.FS {
	return nil
}
