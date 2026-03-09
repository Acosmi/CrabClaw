package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Acosmi/ClawAcosmi/cmd/desktop/internal/release"
)

func main() {
	var opts release.ValidateOptions

	flag.StringVar(&opts.ManifestPath, "manifest", "", "path to update.json")
	flag.StringVar(&opts.ChecksumsPath, "checksums", "", "path to SHA256SUMS")
	flag.StringVar(&opts.ArtifactsDir, "artifacts-dir", ".", "directory containing release artifacts")
	flag.StringVar(&opts.ExpectedVersion, "version", "", "expected release version")
	flag.StringVar(&opts.ExpectedChannel, "channel", "", "expected release channel")
	flag.Parse()

	if err := release.ValidateBundle(opts); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "validate release bundle: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintln(os.Stdout, "release bundle validated")
}
