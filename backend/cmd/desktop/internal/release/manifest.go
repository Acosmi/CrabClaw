package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type UpdateManifest struct {
	Version                 string                      `json:"version,omitempty"`
	Channel                 string                      `json:"channel,omitempty"`
	PublishedAt             string                      `json:"publishedAt,omitempty"`
	Notes                   string                      `json:"notes,omitempty"`
	MinimumSupportedVersion string                      `json:"minimumSupportedVersion,omitempty"`
	Rollout                 int                         `json:"rollout,omitempty"`
	Platforms               map[string]ManifestPlatform `json:"platforms,omitempty"`
}

type ManifestPlatform struct {
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Name   string `json:"name,omitempty"`
}

type ChecksumEntry struct {
	Path   string
	SHA256 string
}

type ExplicitArtifact struct {
	PlatformKey string
	Path        string
}

type GenerateOptions struct {
	Version                 string
	Channel                 string
	BaseURL                 string
	ArtifactsDir            string
	PublishedAt             string
	Notes                   string
	MinimumSupportedVersion string
	Rollout                 int
	ExplicitArtifacts       []ExplicitArtifact
}

func GenerateManifest(opts GenerateOptions) (UpdateManifest, []ChecksumEntry, error) {
	if strings.TrimSpace(opts.Version) == "" {
		return UpdateManifest{}, nil, fmt.Errorf("version is required")
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		return UpdateManifest{}, nil, fmt.Errorf("baseURL is required")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(opts.BaseURL)); err != nil {
		return UpdateManifest{}, nil, fmt.Errorf("invalid baseURL: %w", err)
	}

	channel := strings.TrimSpace(opts.Channel)
	if channel == "" {
		channel = "stable"
	}
	publishedAt := strings.TrimSpace(opts.PublishedAt)
	if publishedAt == "" {
		publishedAt = time.Now().UTC().Format(time.RFC3339)
	}

	artifacts, err := collectArtifacts(opts)
	if err != nil {
		return UpdateManifest{}, nil, err
	}
	if len(artifacts) == 0 {
		return UpdateManifest{}, nil, fmt.Errorf("no release artifacts found")
	}

	manifest := UpdateManifest{
		Version:                 strings.TrimSpace(opts.Version),
		Channel:                 channel,
		PublishedAt:             publishedAt,
		Notes:                   strings.TrimSpace(opts.Notes),
		MinimumSupportedVersion: strings.TrimSpace(opts.MinimumSupportedVersion),
		Rollout:                 opts.Rollout,
		Platforms:               make(map[string]ManifestPlatform, len(artifacts)),
	}
	checksums := make([]ChecksumEntry, 0, len(artifacts))
	for _, artifact := range artifacts {
		shaHex, size, err := fileSHA256(artifact.Path)
		if err != nil {
			return UpdateManifest{}, nil, err
		}
		manifest.Platforms[artifact.PlatformKey] = ManifestPlatform{
			URL:    joinBaseURL(opts.BaseURL, artifact.RelPath),
			SHA256: shaHex,
			Size:   size,
			Name:   filepath.Base(artifact.Path),
		}
		checksums = append(checksums, ChecksumEntry{
			Path:   artifact.RelPath,
			SHA256: shaHex,
		})
	}
	sort.Slice(checksums, func(i, j int) bool {
		return checksums[i].Path < checksums[j].Path
	})
	return manifest, checksums, nil
}

func MarshalManifest(manifest UpdateManifest) ([]byte, error) {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func MarshalChecksums(entries []ChecksumEntry) []byte {
	var b strings.Builder
	for _, entry := range entries {
		b.WriteString(entry.SHA256)
		b.WriteString("  ")
		b.WriteString(entry.Path)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

type collectedArtifact struct {
	PlatformKey string
	Path        string
	RelPath     string
}

func collectArtifacts(opts GenerateOptions) ([]collectedArtifact, error) {
	root := strings.TrimSpace(opts.ArtifactsDir)
	if root == "" {
		root = "."
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var artifacts []collectedArtifact
	index := map[string]collectedArtifact{}

	addArtifact := func(platformKey string, artifactPath string) error {
		platformKey = strings.TrimSpace(platformKey)
		if platformKey == "" {
			return fmt.Errorf("release artifact platform key is required")
		}
		artifactAbs, err := filepath.Abs(strings.TrimSpace(artifactPath))
		if err != nil {
			return err
		}
		info, err := os.Stat(artifactAbs)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("release artifact must be a file: %s", artifactAbs)
		}
		relPath := relativeArtifactPath(rootAbs, artifactAbs)
		artifact := collectedArtifact{
			PlatformKey: platformKey,
			Path:        artifactAbs,
			RelPath:     relPath,
		}
		if existing, ok := index[platformKey]; ok {
			if existing.Path == artifact.Path {
				return nil
			}
			return fmt.Errorf("duplicate release artifact key %s for %s and %s", platformKey, existing.Path, artifact.Path)
		}
		index[platformKey] = artifact
		artifacts = append(artifacts, artifact)
		return nil
	}

	for _, explicit := range opts.ExplicitArtifacts {
		if err := addArtifact(explicit.PlatformKey, explicit.Path); err != nil {
			return nil, fmt.Errorf("add explicit artifact: %w", err)
		}
	}

	rootInfo, err := os.Stat(rootAbs)
	switch {
	case err == nil && !rootInfo.IsDir():
		return nil, fmt.Errorf("release artifacts dir must be a directory: %s", rootAbs)
	case err != nil && !os.IsNotExist(err):
		return nil, err
	case os.IsNotExist(err) && len(opts.ExplicitArtifacts) > 0:
		sort.Slice(artifacts, func(i, j int) bool {
			return artifacts[i].PlatformKey < artifacts[j].PlatformKey
		})
		return artifacts, nil
	case os.IsNotExist(err):
		return nil, err
	}

	walkErr := filepath.WalkDir(rootAbs, func(entryPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		key, ok := DetectPlatformKey(filepath.Base(entryPath))
		if !ok {
			return nil
		}
		if err := addArtifact(key, entryPath); err != nil {
			return err
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].PlatformKey < artifacts[j].PlatformKey
	})
	return artifacts, nil
}

func DetectPlatformKey(filename string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(filename))
	if name == "" {
		return "", false
	}
	arch, ok := detectArtifactArch(name)
	if !ok {
		return "", false
	}

	switch {
	case strings.HasSuffix(name, ".appimage"):
		return "linux-appimage-" + arch, true
	case strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".msi"):
		if containsAny(name, "installer", "setup", "nsis") {
			return "windows-nsis-" + arch, true
		}
	case strings.HasSuffix(name, ".zip"),
		strings.HasSuffix(name, ".dmg"),
		strings.HasSuffix(name, ".pkg"),
		strings.HasSuffix(name, ".app"):
		if containsAny(name, "macos", "darwin") {
			return "macos-wails-" + arch, true
		}
	}

	return "", false
}

func detectArtifactArch(name string) (string, bool) {
	switch {
	case containsAny(name, "arm64", "aarch64"):
		return "arm64", true
	case containsAny(name, "amd64", "x86_64", "x64"):
		return "amd64", true
	default:
		return "", false
	}
}

func containsAny(value string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func relativeArtifactPath(rootAbs string, artifactAbs string) string {
	rel, err := filepath.Rel(rootAbs, artifactAbs)
	if err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.Base(artifactAbs)
}

func joinBaseURL(baseURL string, relPath string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	relPath = strings.TrimLeft(filepath.ToSlash(strings.TrimSpace(relPath)), "/")
	if relPath == "" {
		return baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/" + relPath
	}
	parsed.Path = path.Join(parsed.Path, relPath)
	return parsed.String()
}

func fileSHA256(artifactPath string) (string, int64, error) {
	file, err := os.Open(artifactPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hasher := sha256.New()
	written, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), written, nil
}
