/// Update availability resolution and formatting.
///
/// Source: `src/commands/status.update.ts`
use serde::{Deserialize, Serialize};

use oa_cli_shared::command_format::format_cli_command;

/// Result of an update check.
///
/// Source: `src/infra/update-check.ts` - `UpdateCheckResult`
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateCheckResult {
    /// Install kind ("git", "npm", "pkg", "unknown").
    pub install_kind: String,
    /// Package manager identifier.
    pub package_manager: String,
    /// Git information (if install kind is "git").
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub git: Option<GitInfo>,
    /// Registry information.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub registry: Option<RegistryInfo>,
    /// Dependency status.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub deps: Option<DepsStatus>,
}

/// Git repository information.
///
/// Source: `src/infra/update-check.ts` - `UpdateCheckResult.git`
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitInfo {
    /// Current branch.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub branch: Option<String>,
    /// Current tag.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tag: Option<String>,
    /// Current SHA.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sha: Option<String>,
    /// Upstream remote name.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub upstream: Option<String>,
    /// Whether the working tree is dirty.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub dirty: Option<bool>,
    /// Commits behind upstream.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub behind: Option<u64>,
    /// Commits ahead of upstream.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ahead: Option<u64>,
    /// Whether git fetch succeeded.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub fetch_ok: Option<bool>,
}

/// Registry version information.
///
/// Source: `src/infra/update-check.ts` - `UpdateCheckResult.registry`
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RegistryInfo {
    /// Latest version available.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub latest_version: Option<String>,
    /// Registry error.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// Dependency status.
///
/// Source: `src/infra/update-check.ts` - `UpdateCheckResult.deps`
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DepsStatus {
    /// Status: "ok", "missing", "stale".
    pub status: String,
}

/// Update availability summary.
///
/// Source: `src/commands/status.update.ts` - `UpdateAvailability`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateAvailability {
    /// Whether any update is available.
    pub available: bool,
    /// Whether a git update is available.
    pub has_git_update: bool,
    /// Whether a registry update is available.
    pub has_registry_update: bool,
    /// Latest version from registry (if newer).
    pub latest_version: Option<String>,
    /// Number of commits behind upstream.
    pub git_behind: Option<u64>,
}

/// Compare two semver strings.
///
/// Returns `-1` if `a < b`, `0` if equal, `1` if `a > b`, or `None` on parse error.
///
/// Source: `src/infra/update-check.ts` - `compareSemverStrings`
fn compare_semver(a: &str, b: &str) -> Option<i32> {
    let parse = |s: &str| -> Option<(u64, u64, u64)> {
        let s = s.strip_prefix('v').unwrap_or(s);
        let parts: Vec<&str> = s.split('.').collect();
        if parts.len() < 3 {
            return None;
        }
        let major = parts[0].parse::<u64>().ok()?;
        let minor = parts[1].parse::<u64>().ok()?;
        // Strip pre-release suffixes from patch.
        let patch_str = parts[2].split('-').next().unwrap_or(parts[2]);
        let patch = patch_str.parse::<u64>().ok()?;
        Some((major, minor, patch))
    };
    let av = parse(a)?;
    let bv = parse(b)?;
    Some(av.cmp(&bv) as i32)
}

/// Resolve the update availability from an update check result.
///
/// Source: `src/commands/status.update.ts` - `resolveUpdateAvailability`
#[must_use]
pub fn resolve_update_availability(
    update: &UpdateCheckResult,
    current_version: &str,
) -> UpdateAvailability {
    let latest_version = update
        .registry
        .as_ref()
        .and_then(|r| r.latest_version.as_deref());

    let registry_cmp = latest_version.and_then(|lv| compare_semver(current_version, lv));
    let has_registry_update = registry_cmp.is_some_and(|c| c < 0);

    let git_behind = if update.install_kind == "git" {
        update.git.as_ref().and_then(|g| g.behind)
    } else {
        None
    };
    let has_git_update = git_behind.is_some_and(|b| b > 0);

    UpdateAvailability {
        available: has_git_update || has_registry_update,
        has_git_update,
        has_registry_update,
        latest_version: if has_registry_update {
            latest_version.map(String::from)
        } else {
            None
        },
        git_behind,
    }
}

/// Format the update available hint for display.
///
/// Source: `src/commands/status.update.ts` - `formatUpdateAvailableHint`
#[must_use]
pub fn format_update_available_hint(
    update: &UpdateCheckResult,
    current_version: &str,
) -> Option<String> {
    let availability = resolve_update_availability(update, current_version);
    if !availability.available {
        return None;
    }

    let mut details: Vec<String> = Vec::new();
    if availability.has_git_update {
        if let Some(behind) = availability.git_behind {
            details.push(format!("git behind {behind}"));
        }
    }
    if availability.has_registry_update {
        if let Some(ref version) = availability.latest_version {
            details.push(format!("npm {version}"));
        }
    }
    let suffix = if details.is_empty() {
        String::new()
    } else {
        format!(" ({})", details.join(" \u{00b7} "))
    };
    Some(format!(
        "Update available{suffix}. Run: {}",
        format_cli_command("crabclaw update")
    ))
}

/// Format a one-line update summary.
///
/// Source: `src/commands/status.update.ts` - `formatUpdateOneLiner`
#[must_use]
pub fn format_update_one_liner(update: &UpdateCheckResult, current_version: &str) -> String {
    let mut parts: Vec<String> = Vec::new();

    if update.install_kind == "git" {
        if let Some(ref git) = update.git {
            let branch = git
                .branch
                .as_deref()
                .map_or("git".to_string(), |b| format!("git {b}"));
            parts.push(branch);
            if let Some(ref upstream) = git.upstream {
                parts.push(format!("\u{2194} {upstream}"));
            }
            if git.dirty == Some(true) {
                parts.push("dirty".to_string());
            }
            if let (Some(behind), Some(ahead)) = (git.behind, git.ahead) {
                match (behind, ahead) {
                    (0, 0) => parts.push("up to date".to_string()),
                    (b, 0) if b > 0 => parts.push(format!("behind {b}")),
                    (0, a) if a > 0 => parts.push(format!("ahead {a}")),
                    (b, a) => parts.push(format!("diverged (ahead {a}, behind {b})")),
                }
            }
            if git.fetch_ok == Some(false) {
                parts.push("fetch failed".to_string());
            }
        }
    } else {
        let pkg = if update.package_manager != "unknown" {
            update.package_manager.clone()
        } else {
            "pkg".to_string()
        };
        parts.push(pkg);
    }

    // Registry info.
    if let Some(ref registry) = update.registry {
        if let Some(ref latest) = registry.latest_version {
            let cmp = compare_semver(current_version, latest);
            match cmp {
                Some(0) => parts.push(format!("npm latest {latest}")),
                Some(c) if c < 0 => parts.push(format!("npm update {latest}")),
                Some(_) => parts.push(format!("npm latest {latest} (local newer)")),
                None => parts.push(format!("npm latest {latest}")),
            }
        } else if registry.error.is_some() {
            parts.push("npm latest unknown".to_string());
        }
    }

    // Deps info.
    if let Some(ref deps) = update.deps {
        match deps.status.as_str() {
            "ok" => parts.push("deps ok".to_string()),
            "missing" => parts.push("deps missing".to_string()),
            "stale" => parts.push("deps stale".to_string()),
            _ => {}
        }
    }

    format!("Update: {}", parts.join(" \u{00b7} "))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn compare_semver_equal() {
        assert_eq!(compare_semver("1.2.3", "1.2.3"), Some(0));
    }

    #[test]
    fn compare_semver_less() {
        assert_eq!(compare_semver("1.2.3", "1.3.0"), Some(-1));
    }

    #[test]
    fn compare_semver_greater() {
        assert_eq!(compare_semver("2.0.0", "1.9.9"), Some(1));
    }

    #[test]
    fn compare_semver_v_prefix() {
        assert_eq!(compare_semver("v1.2.3", "1.2.3"), Some(0));
    }

    #[test]
    fn compare_semver_invalid() {
        assert!(compare_semver("invalid", "1.2.3").is_none());
    }

    #[test]
    fn resolve_availability_no_update() {
        let update = UpdateCheckResult {
            install_kind: "npm".to_string(),
            package_manager: "npm".to_string(),
            registry: Some(RegistryInfo {
                latest_version: Some("1.0.0".to_string()),
                error: None,
            }),
            ..Default::default()
        };
        let result = resolve_update_availability(&update, "1.0.0");
        assert!(!result.available);
        assert!(!result.has_registry_update);
    }

    #[test]
    fn resolve_availability_registry_update() {
        let update = UpdateCheckResult {
            install_kind: "npm".to_string(),
            package_manager: "npm".to_string(),
            registry: Some(RegistryInfo {
                latest_version: Some("2.0.0".to_string()),
                error: None,
            }),
            ..Default::default()
        };
        let result = resolve_update_availability(&update, "1.0.0");
        assert!(result.available);
        assert!(result.has_registry_update);
        assert_eq!(result.latest_version.as_deref(), Some("2.0.0"));
    }

    #[test]
    fn resolve_availability_git_update() {
        let update = UpdateCheckResult {
            install_kind: "git".to_string(),
            package_manager: "unknown".to_string(),
            git: Some(GitInfo {
                behind: Some(5),
                ahead: Some(0),
                ..Default::default()
            }),
            ..Default::default()
        };
        let result = resolve_update_availability(&update, "1.0.0");
        assert!(result.available);
        assert!(result.has_git_update);
        assert_eq!(result.git_behind, Some(5));
    }

    #[test]
    fn format_hint_no_update() {
        let update = UpdateCheckResult {
            install_kind: "npm".to_string(),
            package_manager: "npm".to_string(),
            registry: Some(RegistryInfo {
                latest_version: Some("1.0.0".to_string()),
                error: None,
            }),
            ..Default::default()
        };
        assert!(format_update_available_hint(&update, "1.0.0").is_none());
    }

    #[test]
    fn format_hint_with_update() {
        let update = UpdateCheckResult {
            install_kind: "npm".to_string(),
            package_manager: "npm".to_string(),
            registry: Some(RegistryInfo {
                latest_version: Some("2.0.0".to_string()),
                error: None,
            }),
            ..Default::default()
        };
        let hint = format_update_available_hint(&update, "1.0.0");
        assert!(hint.is_some());
        let text = hint.unwrap_or_default();
        assert!(text.contains("Update available"));
        assert!(text.contains("npm 2.0.0"));
    }

    #[test]
    fn format_one_liner_npm() {
        let update = UpdateCheckResult {
            install_kind: "npm".to_string(),
            package_manager: "npm".to_string(),
            registry: Some(RegistryInfo {
                latest_version: Some("1.0.0".to_string()),
                error: None,
            }),
            deps: Some(DepsStatus {
                status: "ok".to_string(),
            }),
            ..Default::default()
        };
        let line = format_update_one_liner(&update, "1.0.0");
        assert!(line.starts_with("Update:"));
        assert!(line.contains("npm"));
        assert!(line.contains("deps ok"));
    }

    #[test]
    fn format_one_liner_git() {
        let update = UpdateCheckResult {
            install_kind: "git".to_string(),
            package_manager: "unknown".to_string(),
            git: Some(GitInfo {
                branch: Some("main".to_string()),
                upstream: Some("origin/main".to_string()),
                behind: Some(0),
                ahead: Some(0),
                ..Default::default()
            }),
            ..Default::default()
        };
        let line = format_update_one_liner(&update, "1.0.0");
        assert!(line.contains("git main"));
        assert!(line.contains("up to date"));
    }
}
