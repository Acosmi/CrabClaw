package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/cmd/desktop/internal/release"
)

type artifactList []string

func (l *artifactList) String() string {
	return strings.Join(*l, ",")
}

func (l *artifactList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("artifact value cannot be empty")
	}
	*l = append(*l, value)
	return nil
}

func main() {
	var artifacts artifactList
	var opts release.GenerateOptions
	var outputPath string
	var checksumsPath string

	flag.StringVar(&opts.Version, "version", "", "release version to publish")
	flag.StringVar(&opts.Channel, "channel", "stable", "release channel")
	flag.StringVar(&opts.BaseURL, "base-url", "", "base URL prefix for published artifacts")
	flag.StringVar(&opts.ArtifactsDir, "artifacts-dir", ".", "directory to scan for release artifacts")
	flag.StringVar(&opts.PublishedAt, "published-at", "", "RFC3339 publish timestamp (defaults to now)")
	flag.StringVar(&opts.Notes, "notes", "", "release notes summary")
	flag.StringVar(&opts.MinimumSupportedVersion, "minimum-supported-version", "", "oldest supported client version")
	flag.IntVar(&opts.Rollout, "rollout", 0, "rollout percentage (0 omits the field)")
	flag.StringVar(&outputPath, "output", "", "path to write update.json")
	flag.StringVar(&checksumsPath, "checksums", "", "optional path to write SHA256SUMS")
	flag.Var(&artifacts, "artifact", "explicit platform mapping in the form key=/path/to/artifact")
	flag.Parse()

	explicitArtifacts, err := parseExplicitArtifacts(artifacts)
	if err != nil {
		exitf("%v", err)
	}
	opts.ExplicitArtifacts = explicitArtifacts

	manifest, checksums, err := release.GenerateManifest(opts)
	if err != nil {
		exitf("generate manifest: %v", err)
	}

	if outputPath != "" {
		raw, err := release.MarshalManifest(manifest)
		if err != nil {
			exitf("marshal manifest: %v", err)
		}
		if err := writeFile(outputPath, raw); err != nil {
			exitf("write manifest: %v", err)
		}
	}
	if checksumsPath != "" {
		if err := writeFile(checksumsPath, release.MarshalChecksums(checksums)); err != nil {
			exitf("write checksums: %v", err)
		}
	}

	if outputPath == "" {
		raw, err := release.MarshalManifest(manifest)
		if err != nil {
			exitf("marshal manifest: %v", err)
		}
		_, _ = os.Stdout.Write(raw)
	}
}

func parseExplicitArtifacts(values []string) ([]release.ExplicitArtifact, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]release.ExplicitArtifact, 0, len(values))
	for _, value := range values {
		key, pathValue, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("invalid -artifact value %q: expected key=/path/to/file", value)
		}
		key = strings.TrimSpace(key)
		pathValue = strings.TrimSpace(pathValue)
		if key == "" || pathValue == "" {
			return nil, fmt.Errorf("invalid -artifact value %q: key and path are required", value)
		}
		result = append(result, release.ExplicitArtifact{
			PlatformKey: key,
			Path:        pathValue,
		})
	}
	return result, nil
}

func writeFile(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func exitf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
