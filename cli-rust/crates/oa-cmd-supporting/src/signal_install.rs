/// Signal CLI installation command.
///
/// Downloads and installs `signal-cli` from its GitHub releases. Supports
/// Linux and macOS platforms. Handles archive detection, download with
/// redirect following, extraction, and binary discovery.
///
/// Source: `src/commands/signal-install.ts`
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use tracing::info;

use oa_config::paths::resolve_state_dir;

/// Result of a signal-cli installation attempt.
///
/// Source: `src/commands/signal-install.ts` - `SignalInstallResult`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SignalInstallResult {
    /// Whether the installation succeeded.
    pub ok: bool,
    /// Path to the installed signal-cli binary.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cli_path: Option<String>,
    /// Installed version.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
    /// Error message if installation failed.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// A GitHub release asset.
///
/// Source: `src/commands/signal-install.ts` - `ReleaseAsset`
#[derive(Debug, Clone, Deserialize)]
struct ReleaseAsset {
    name: Option<String>,
    browser_download_url: Option<String>,
}

/// A GitHub release response.
///
/// Source: `src/commands/signal-install.ts` - `ReleaseResponse`
#[derive(Debug, Clone, Deserialize)]
struct ReleaseResponse {
    tag_name: Option<String>,
    assets: Option<Vec<ReleaseAsset>>,
}

/// Check if a filename looks like a compressed archive.
///
/// Source: `src/commands/signal-install.ts` - `looksLikeArchive`
fn looks_like_archive(name: &str) -> bool {
    name.ends_with(".tar.gz") || name.ends_with(".tgz") || name.ends_with(".zip")
}

/// Pick the best asset for the current platform.
///
/// Source: `src/commands/signal-install.ts` - `pickAsset`
fn pick_asset<'a>(assets: &'a [ReleaseAsset], platform: &str) -> Option<(&'a str, &'a str)> {
    let with_name: Vec<_> = assets
        .iter()
        .filter_map(|a| {
            let name = a.name.as_deref()?;
            let url = a.browser_download_url.as_deref()?;
            if name.is_empty() || url.is_empty() {
                return None;
            }
            Some((name, url))
        })
        .collect();

    let by_name = |pattern: &str| {
        with_name
            .iter()
            .find(|(name, _)| name.to_lowercase().contains(pattern))
            .copied()
    };

    match platform {
        "linux" => by_name("linux-native")
            .or_else(|| by_name("linux"))
            .or_else(|| {
                with_name
                    .iter()
                    .find(|(n, _)| looks_like_archive(&n.to_lowercase()))
                    .copied()
            }),
        "macos" | "darwin" => by_name("macos")
            .or_else(|| by_name("osx"))
            .or_else(|| by_name("darwin"))
            .or_else(|| {
                with_name
                    .iter()
                    .find(|(n, _)| looks_like_archive(&n.to_lowercase()))
                    .copied()
            }),
        "windows" | "win32" => by_name("windows").or_else(|| by_name("win")).or_else(|| {
            with_name
                .iter()
                .find(|(n, _)| looks_like_archive(&n.to_lowercase()))
                .copied()
        }),
        _ => with_name
            .iter()
            .find(|(n, _)| looks_like_archive(&n.to_lowercase()))
            .copied(),
    }
}

/// Search for the `signal-cli` binary in a directory tree.
///
/// Source: `src/commands/signal-install.ts` - `findSignalCliBinary`
async fn find_signal_cli_binary(root: &Path, max_depth: usize) -> Option<PathBuf> {
    find_binary_recursive(root, 0, max_depth).await
}

/// Recursive helper for binary search.
///
/// Uses `Box::pin` to allow recursion in async context.
fn find_binary_recursive(
    dir: &Path,
    depth: usize,
    max_depth: usize,
) -> std::pin::Pin<Box<dyn std::future::Future<Output = Option<PathBuf>> + Send + '_>> {
    Box::pin(async move {
        if depth > max_depth {
            return None;
        }

        let mut entries = match tokio::fs::read_dir(dir).await {
            Ok(e) => e,
            Err(_) => return None,
        };

        let mut subdirs = Vec::new();

        while let Ok(Some(entry)) = entries.next_entry().await {
            let file_type = match entry.file_type().await {
                Ok(ft) => ft,
                Err(_) => continue,
            };

            if file_type.is_file() && entry.file_name() == "signal-cli" {
                return Some(entry.path());
            }

            if file_type.is_dir() {
                subdirs.push(entry.path());
            }
        }

        for subdir in subdirs {
            if let Some(found) = find_binary_recursive(&subdir, depth + 1, max_depth).await {
                return Some(found);
            }
        }

        None
    })
}

/// Resolve the current platform string.
fn current_platform() -> &'static str {
    if cfg!(target_os = "macos") {
        "macos"
    } else if cfg!(target_os = "linux") {
        "linux"
    } else if cfg!(target_os = "windows") {
        "windows"
    } else {
        "unknown"
    }
}

/// Install signal-cli from GitHub releases.
///
/// Downloads the latest release, extracts it, and returns the path
/// to the installed binary.
///
/// Source: `src/commands/signal-install.ts` - `installSignalCli`
pub async fn install_signal_cli() -> SignalInstallResult {
    let platform = current_platform();
    if platform == "windows" {
        return SignalInstallResult {
            ok: false,
            cli_path: None,
            version: None,
            error: Some("Signal CLI auto-install is not supported on Windows yet.".to_owned()),
        };
    }

    let api_url = "https://api.github.com/repos/AsamK/signal-cli/releases/latest";

    let client = match reqwest::Client::builder().user_agent("openacosmi").build() {
        Ok(c) => c,
        Err(e) => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: None,
                error: Some(format!("Failed to create HTTP client: {e}")),
            };
        }
    };

    let response = match client
        .get(api_url)
        .header("Accept", "application/vnd.github+json")
        .send()
        .await
    {
        Ok(r) => r,
        Err(e) => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: None,
                error: Some(format!("Failed to fetch release info: {e}")),
            };
        }
    };

    if !response.status().is_success() {
        return SignalInstallResult {
            ok: false,
            cli_path: None,
            version: None,
            error: Some(format!(
                "Failed to fetch release info ({})",
                response.status()
            )),
        };
    }

    let payload: ReleaseResponse = match response.json().await {
        Ok(p) => p,
        Err(e) => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: None,
                error: Some(format!("Failed to parse release response: {e}")),
            };
        }
    };

    let version = payload
        .tag_name
        .as_deref()
        .map(|t| t.strip_prefix('v').unwrap_or(t))
        .unwrap_or("unknown")
        .to_owned();

    let assets = payload.assets.unwrap_or_default();
    let (asset_name, asset_url) = match pick_asset(&assets, platform) {
        Some((name, url)) => (name.to_owned(), url.to_owned()),
        None => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: Some(version),
                error: Some("No compatible release asset found for this platform.".to_owned()),
            };
        }
    };

    info!("Downloading signal-cli {version} ({asset_name})...");

    // Download the asset.
    let tmp_dir = match tempfile::tempdir() {
        Ok(d) => d,
        Err(e) => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: Some(version),
                error: Some(format!("Failed to create temp dir: {e}")),
            };
        }
    };

    let archive_path = tmp_dir.path().join(&asset_name);
    let download_result = client.get(&asset_url).send().await;
    let bytes = match download_result {
        Ok(resp) => match resp.bytes().await {
            Ok(b) => b,
            Err(e) => {
                return SignalInstallResult {
                    ok: false,
                    cli_path: None,
                    version: Some(version),
                    error: Some(format!("Failed to download asset: {e}")),
                };
            }
        },
        Err(e) => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: Some(version),
                error: Some(format!("Failed to download asset: {e}")),
            };
        }
    };

    if let Err(e) = tokio::fs::write(&archive_path, &bytes).await {
        return SignalInstallResult {
            ok: false,
            cli_path: None,
            version: Some(version),
            error: Some(format!("Failed to write archive: {e}")),
        };
    }

    // Extract archive.
    let state_dir = resolve_state_dir();
    let install_root = state_dir.join("tools").join("signal-cli").join(&version);
    if let Err(e) = tokio::fs::create_dir_all(&install_root).await {
        return SignalInstallResult {
            ok: false,
            cli_path: None,
            version: Some(version),
            error: Some(format!("Failed to create install dir: {e}")),
        };
    }

    let extract_result = if asset_name.ends_with(".zip") {
        tokio::process::Command::new("unzip")
            .args([
                "-q",
                &archive_path.display().to_string(),
                "-d",
                &install_root.display().to_string(),
            ])
            .output()
            .await
    } else if asset_name.ends_with(".tar.gz") || asset_name.ends_with(".tgz") {
        tokio::process::Command::new("tar")
            .args([
                "-xzf",
                &archive_path.display().to_string(),
                "-C",
                &install_root.display().to_string(),
            ])
            .output()
            .await
    } else {
        return SignalInstallResult {
            ok: false,
            cli_path: None,
            version: Some(version),
            error: Some(format!("Unsupported archive type: {asset_name}")),
        };
    };

    match extract_result {
        Ok(output) if !output.status.success() => {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: Some(version),
                error: Some(format!("Extraction failed: {stderr}")),
            };
        }
        Err(e) => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: Some(version),
                error: Some(format!("Extraction command failed: {e}")),
            };
        }
        _ => {}
    }

    // Find the binary.
    let cli_path = match find_signal_cli_binary(&install_root, 3).await {
        Some(p) => p,
        None => {
            return SignalInstallResult {
                ok: false,
                cli_path: None,
                version: Some(version),
                error: Some(format!(
                    "signal-cli binary not found after extracting {asset_name}"
                )),
            };
        }
    };

    // Make executable.
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let perms = std::fs::Permissions::from_mode(0o755);
        let _ = tokio::fs::set_permissions(&cli_path, perms).await;
    }

    let cli_path_str = cli_path.display().to_string();
    SignalInstallResult {
        ok: true,
        cli_path: Some(cli_path_str),
        version: Some(version),
        error: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn looks_like_archive_tar_gz() {
        assert!(looks_like_archive("signal-cli-0.13.3-linux-native.tar.gz"));
    }

    #[test]
    fn looks_like_archive_zip() {
        assert!(looks_like_archive("signal-cli-0.13.3.zip"));
    }

    #[test]
    fn looks_like_archive_tgz() {
        assert!(looks_like_archive("archive.tgz"));
    }

    #[test]
    fn not_archive() {
        assert!(!looks_like_archive("README.md"));
        assert!(!looks_like_archive("signal-cli"));
    }

    #[test]
    fn pick_asset_linux() {
        let assets = vec![
            ReleaseAsset {
                name: Some("signal-cli-0.13.3-linux-native.tar.gz".to_owned()),
                browser_download_url: Some("https://example.com/linux.tar.gz".to_owned()),
            },
            ReleaseAsset {
                name: Some("signal-cli-0.13.3-macos.tar.gz".to_owned()),
                browser_download_url: Some("https://example.com/macos.tar.gz".to_owned()),
            },
        ];
        let result = pick_asset(&assets, "linux");
        assert!(result.is_some());
        let (name, _) = result.expect("should find asset");
        assert!(name.contains("linux"));
    }

    #[test]
    fn pick_asset_macos() {
        let assets = vec![
            ReleaseAsset {
                name: Some("signal-cli-0.13.3-linux.tar.gz".to_owned()),
                browser_download_url: Some("https://example.com/linux.tar.gz".to_owned()),
            },
            ReleaseAsset {
                name: Some("signal-cli-0.13.3-macos.tar.gz".to_owned()),
                browser_download_url: Some("https://example.com/macos.tar.gz".to_owned()),
            },
        ];
        let result = pick_asset(&assets, "macos");
        assert!(result.is_some());
        let (name, _) = result.expect("should find asset");
        assert!(name.contains("macos"));
    }

    #[test]
    fn pick_asset_fallback_to_archive() {
        let assets = vec![ReleaseAsset {
            name: Some("signal-cli-0.13.3.tar.gz".to_owned()),
            browser_download_url: Some("https://example.com/generic.tar.gz".to_owned()),
        }];
        let result = pick_asset(&assets, "freebsd");
        assert!(result.is_some());
    }

    #[test]
    fn pick_asset_empty() {
        let result = pick_asset(&[], "linux");
        assert!(result.is_none());
    }

    #[test]
    fn pick_asset_skips_missing_fields() {
        let assets = vec![
            ReleaseAsset {
                name: None,
                browser_download_url: Some("https://example.com/x.tar.gz".to_owned()),
            },
            ReleaseAsset {
                name: Some("sig.tar.gz".to_owned()),
                browser_download_url: None,
            },
        ];
        let result = pick_asset(&assets, "linux");
        assert!(result.is_none());
    }

    #[test]
    fn signal_install_result_serialization() {
        let result = SignalInstallResult {
            ok: true,
            cli_path: Some("/opt/signal-cli".to_owned()),
            version: Some("0.13.3".to_owned()),
            error: None,
        };
        let json = serde_json::to_string(&result).expect("should serialize");
        assert!(json.contains("\"ok\":true"));
        assert!(json.contains("\"cliPath\""));
        assert!(!json.contains("\"error\""));
    }

    #[test]
    fn current_platform_is_known() {
        let platform = current_platform();
        assert!(
            ["macos", "linux", "windows", "unknown"].contains(&platform),
            "unexpected platform: {platform}"
        );
    }
}
