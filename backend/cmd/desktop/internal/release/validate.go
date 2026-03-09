package release

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type ValidateOptions struct {
	ManifestPath    string
	ChecksumsPath   string
	ArtifactsDir    string
	ExpectedVersion string
	ExpectedChannel string
}

func ValidateBundle(opts ValidateOptions) error {
	manifest, err := readManifestFile(opts.ManifestPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.ExpectedVersion) != "" && manifest.Version != strings.TrimSpace(opts.ExpectedVersion) {
		return fmt.Errorf("release manifest version mismatch: got %q want %q", manifest.Version, strings.TrimSpace(opts.ExpectedVersion))
	}
	if strings.TrimSpace(opts.ExpectedChannel) != "" && manifest.Channel != strings.TrimSpace(opts.ExpectedChannel) {
		return fmt.Errorf("release manifest channel mismatch: got %q want %q", manifest.Channel, strings.TrimSpace(opts.ExpectedChannel))
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("release manifest missing version")
	}
	if len(manifest.Platforms) == 0 {
		return fmt.Errorf("release manifest has no platforms")
	}

	entries, err := readChecksumsFile(opts.ChecksumsPath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("release checksums file has no entries")
	}

	entryByPath := make(map[string]ChecksumEntry, len(entries))
	entryByBase := make(map[string]ChecksumEntry, len(entries))
	for _, entry := range entries {
		entryByPath[entry.Path] = entry
		base := path.Base(entry.Path)
		if existing, ok := entryByBase[base]; ok && existing.Path != entry.Path {
			return fmt.Errorf("release checksums contain duplicate basenames %q: %s and %s", base, existing.Path, entry.Path)
		}
		entryByBase[base] = entry
	}

	used := make(map[string]bool, len(entries))
	keys := make([]string, 0, len(manifest.Platforms))
	for key := range manifest.Platforms {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, platformKey := range keys {
		platform := manifest.Platforms[platformKey]
		if err := validatePlatform(platformKey, platform, opts.ArtifactsDir, entryByPath, entryByBase, used); err != nil {
			return err
		}
	}

	for _, entry := range entries {
		if !used[entry.Path] {
			return fmt.Errorf("release checksums entry %s is not referenced by update.json", entry.Path)
		}
	}
	return nil
}

func readManifestFile(manifestPath string) (UpdateManifest, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return UpdateManifest{}, fmt.Errorf("manifest path is required")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return UpdateManifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	var manifest UpdateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return UpdateManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	return manifest, nil
}

func readChecksumsFile(checksumsPath string) ([]ChecksumEntry, error) {
	if strings.TrimSpace(checksumsPath) == "" {
		return nil, fmt.Errorf("checksums path is required")
	}
	raw, err := os.ReadFile(checksumsPath)
	if err != nil {
		return nil, fmt.Errorf("read release checksums: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	entries := make([]ChecksumEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid checksums line %q", line)
		}
		entries = append(entries, ChecksumEntry{
			SHA256: parts[0],
			Path:   filepath.ToSlash(parts[1]),
		})
	}
	return entries, nil
}

func validatePlatform(
	platformKey string,
	platform ManifestPlatform,
	artifactsDir string,
	entryByPath map[string]ChecksumEntry,
	entryByBase map[string]ChecksumEntry,
	used map[string]bool,
) error {
	if strings.TrimSpace(platform.URL) == "" {
		return fmt.Errorf("release manifest platform %s missing URL", platformKey)
	}
	if strings.TrimSpace(platform.SHA256) == "" {
		return fmt.Errorf("release manifest platform %s missing sha256", platformKey)
	}
	if platform.Size <= 0 {
		return fmt.Errorf("release manifest platform %s missing size", platformKey)
	}
	if strings.TrimSpace(platform.Name) == "" {
		return fmt.Errorf("release manifest platform %s missing name", platformKey)
	}

	parsedURL, err := url.Parse(platform.URL)
	if err != nil {
		return fmt.Errorf("release manifest platform %s invalid URL: %w", platformKey, err)
	}
	urlBase := path.Base(parsedURL.Path)
	if urlBase == "." || urlBase == "/" || urlBase == "" {
		return fmt.Errorf("release manifest platform %s URL has no artifact name", platformKey)
	}
	if urlBase != platform.Name {
		return fmt.Errorf("release manifest platform %s name/url mismatch: %q vs %q", platformKey, platform.Name, urlBase)
	}

	entry, ok := entryByPath[platform.Name]
	if !ok {
		entry, ok = entryByBase[platform.Name]
	}
	if !ok {
		return fmt.Errorf("release checksums missing entry for platform %s artifact %s", platformKey, platform.Name)
	}
	used[entry.Path] = true

	if !strings.EqualFold(strings.TrimSpace(platform.SHA256), strings.TrimSpace(entry.SHA256)) {
		return fmt.Errorf("release checksum mismatch for %s: manifest=%s checksums=%s", platformKey, platform.SHA256, entry.SHA256)
	}

	artifactPath, err := resolveArtifactPath(artifactsDir, entry)
	if err != nil {
		return fmt.Errorf("resolve artifact for %s: %w", platformKey, err)
	}
	shaHex, size, err := fileSHA256(artifactPath)
	if err != nil {
		return fmt.Errorf("hash artifact for %s: %w", platformKey, err)
	}
	if !strings.EqualFold(shaHex, entry.SHA256) {
		return fmt.Errorf("artifact sha256 mismatch for %s: file=%s expected=%s", platformKey, shaHex, entry.SHA256)
	}
	if size != platform.Size {
		return fmt.Errorf("artifact size mismatch for %s: file=%d expected=%d", platformKey, size, platform.Size)
	}
	return nil
}

func resolveArtifactPath(artifactsDir string, entry ChecksumEntry) (string, error) {
	if strings.TrimSpace(artifactsDir) == "" {
		return "", fmt.Errorf("artifacts dir is required")
	}
	rootAbs, err := filepath.Abs(artifactsDir)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(entry.Path))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}

	var matches []string
	err = filepath.WalkDir(rootAbs, func(entryPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(entryPath) == filepath.Base(entry.Path) {
			matches = append(matches, entryPath)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("artifact %s not found under %s", entry.Path, rootAbs)
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("artifact %s matched multiple files: %s", entry.Path, strings.Join(matches, ", "))
	}
}
