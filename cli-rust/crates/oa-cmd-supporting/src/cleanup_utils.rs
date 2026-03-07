/// Cleanup utilities for reset and uninstall commands.
///
/// Provides safe path removal (with dry-run support), workspace directory
/// collection, agent session directory listing, and path containment checks.
///
/// Source: `src/commands/cleanup-utils.ts`
use std::path::{Path, PathBuf};

use tracing::{error, info};

use oa_types::config::OpenAcosmiConfig;

/// Result of a path removal operation.
///
/// Source: `src/commands/cleanup-utils.ts` - `RemovalResult`
#[derive(Debug, Clone)]
pub struct RemovalResult {
    /// Whether the removal succeeded.
    pub ok: bool,
    /// Whether the removal was skipped (dry-run or already gone).
    pub skipped: bool,
}

/// Collect all workspace directories from the configuration.
///
/// Gathers the default workspace and any per-agent workspace overrides.
/// Falls back to the default workspace directory if none are configured.
///
/// Source: `src/commands/cleanup-utils.ts` - `collectWorkspaceDirs`
pub fn collect_workspace_dirs(cfg: Option<&OpenAcosmiConfig>) -> Vec<PathBuf> {
    let mut dirs = Vec::new();
    let mut seen = std::collections::HashSet::new();

    if let Some(config) = cfg {
        // Default workspace.
        if let Some(ref agents) = config.agents {
            if let Some(ref defaults) = agents.defaults {
                if let Some(ref ws) = defaults.workspace {
                    let trimmed = ws.trim();
                    if !trimmed.is_empty() {
                        let path = resolve_user_path(trimmed);
                        if seen.insert(path.clone()) {
                            dirs.push(path);
                        }
                    }
                }
            }

            // Per-agent workspaces.
            if let Some(ref list) = agents.list {
                for agent in list {
                    if let Some(ref ws) = agent.workspace {
                        let trimmed = ws.trim();
                        if !trimmed.is_empty() {
                            let path = resolve_user_path(trimmed);
                            if seen.insert(path.clone()) {
                                dirs.push(path);
                            }
                        }
                    }
                }
            }
        }
    }

    if dirs.is_empty() {
        dirs.push(resolve_default_workspace_dir());
    }

    dirs
}

/// Check whether `child` is within or equal to `parent`.
///
/// Source: `src/commands/cleanup-utils.ts` - `isPathWithin`
pub fn is_path_within(child: &Path, parent: &Path) -> bool {
    match child.strip_prefix(parent) {
        Ok(relative) => relative == Path::new("") || !relative.starts_with(".."),
        Err(_) => false,
    }
}

/// Check whether a removal target is unsafe (root dir, home dir, empty).
///
/// Source: `src/commands/cleanup-utils.ts` - `isUnsafeRemovalTarget`
fn is_unsafe_removal_target(target: &Path) -> bool {
    let resolved = match target.canonicalize() {
        Ok(p) => p,
        Err(_) => target.to_path_buf(),
    };
    let resolved_str = resolved.to_string_lossy();

    // Empty path.
    if resolved_str.trim().is_empty() {
        return true;
    }

    // Root directory.
    if resolved == Path::new("/") || resolved_str == "/" {
        return true;
    }

    // Home directory.
    if let Some(home) = dirs::home_dir() {
        if let Ok(canonical_home) = home.canonicalize() {
            if resolved == canonical_home {
                return true;
            }
        }
        if resolved == home {
            return true;
        }
    }

    false
}

/// Shorten a path by replacing the home directory with `~`.
///
/// Source: `src/utils.ts` - `shortenHomeInString`
pub fn shorten_home_path(path: &str) -> String {
    if let Some(home) = dirs::home_dir() {
        let home_str = home.to_string_lossy();
        if let Some(rest) = path.strip_prefix(home_str.as_ref()) {
            return format!("~{rest}");
        }
    }
    path.to_owned()
}

/// Remove a path (file or directory) with safety checks.
///
/// Refuses to remove root, home, or empty paths. Supports dry-run mode.
///
/// Source: `src/commands/cleanup-utils.ts` - `removePath`
pub async fn remove_path(target: &str, dry_run: bool, label: Option<&str>) -> RemovalResult {
    let trimmed = target.trim();
    if trimmed.is_empty() {
        return RemovalResult {
            ok: false,
            skipped: true,
        };
    }

    let resolved = Path::new(trimmed);
    let display_label = shorten_home_path(label.unwrap_or(trimmed));

    if is_unsafe_removal_target(resolved) {
        error!("Refusing to remove unsafe path: {display_label}");
        return RemovalResult {
            ok: false,
            skipped: false,
        };
    }

    if dry_run {
        info!("[dry-run] remove {display_label}");
        return RemovalResult {
            ok: true,
            skipped: true,
        };
    }

    match tokio::fs::remove_dir_all(resolved).await {
        Ok(()) => {
            info!("Removed {display_label}");
            RemovalResult {
                ok: true,
                skipped: false,
            }
        }
        Err(ref e) if e.kind() == std::io::ErrorKind::NotFound => {
            // Also try removing as a file.
            match tokio::fs::remove_file(resolved).await {
                Ok(()) => {
                    info!("Removed {display_label}");
                    RemovalResult {
                        ok: true,
                        skipped: false,
                    }
                }
                Err(ref e2) if e2.kind() == std::io::ErrorKind::NotFound => RemovalResult {
                    ok: true,
                    skipped: true,
                },
                Err(e2) => {
                    error!("Failed to remove {display_label}: {e2}");
                    RemovalResult {
                        ok: false,
                        skipped: false,
                    }
                }
            }
        }
        Err(e) => {
            error!("Failed to remove {display_label}: {e}");
            RemovalResult {
                ok: false,
                skipped: false,
            }
        }
    }
}

/// List agent session directories under the state directory.
///
/// Returns paths like `<state_dir>/agents/<id>/sessions` for each agent.
///
/// Source: `src/commands/cleanup-utils.ts` - `listAgentSessionDirs`
pub async fn list_agent_session_dirs(state_dir: &Path) -> Vec<PathBuf> {
    let root = state_dir.join("agents");
    let mut result = Vec::new();

    let entries = match tokio::fs::read_dir(&root).await {
        Ok(e) => e,
        Err(_) => return result,
    };

    let mut stream = entries;
    while let Ok(Some(entry)) = stream.next_entry().await {
        if let Ok(file_type) = entry.file_type().await {
            if file_type.is_dir() {
                result.push(entry.path().join("sessions"));
            }
        }
    }

    result
}

/// Resolve the default workspace directory.
fn resolve_default_workspace_dir() -> PathBuf {
    let state = oa_config::paths::resolve_state_dir();
    state.join("workspace")
}

/// Expand `~` in a path to the user's home directory.
fn resolve_user_path(input: &str) -> PathBuf {
    let trimmed = input.trim();
    if let Some(rest) = trimmed.strip_prefix('~') {
        let home = dirs::home_dir().unwrap_or_else(|| PathBuf::from("."));
        let rest = rest.strip_prefix('/').unwrap_or(rest);
        home.join(rest)
    } else {
        PathBuf::from(trimmed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn is_path_within_true() {
        assert!(is_path_within(
            Path::new("/home/user/.openacosmi/config"),
            Path::new("/home/user/.openacosmi")
        ));
    }

    #[test]
    fn is_path_within_same() {
        assert!(is_path_within(
            Path::new("/home/user/.openacosmi"),
            Path::new("/home/user/.openacosmi")
        ));
    }

    #[test]
    fn is_path_within_false() {
        assert!(!is_path_within(
            Path::new("/etc/hosts"),
            Path::new("/home/user/.openacosmi")
        ));
    }

    #[test]
    fn unsafe_removal_root() {
        assert!(is_unsafe_removal_target(Path::new("/")));
    }

    #[test]
    fn unsafe_removal_empty() {
        assert!(is_unsafe_removal_target(Path::new("")));
    }

    #[test]
    fn safe_removal_normal_path() {
        assert!(!is_unsafe_removal_target(Path::new("/tmp/test-openacosmi")));
    }

    #[test]
    fn collect_workspace_dirs_empty_config() {
        let dirs = collect_workspace_dirs(None);
        assert_eq!(dirs.len(), 1);
    }

    #[test]
    fn collect_workspace_dirs_with_default() {
        let cfg = OpenAcosmiConfig {
            agents: Some(oa_types::agents::AgentsConfig {
                defaults: Some(oa_types::agent_defaults::AgentDefaultsConfig {
                    workspace: Some("~/my-workspace".to_owned()),
                    ..Default::default()
                }),
                list: None,
            }),
            ..Default::default()
        };
        let dirs = collect_workspace_dirs(Some(&cfg));
        assert_eq!(dirs.len(), 1);
        let dir_str = dirs[0].to_string_lossy();
        assert!(dir_str.contains("my-workspace"));
    }

    #[test]
    fn shorten_home_path_with_home() {
        if let Some(home) = dirs::home_dir() {
            let test_path = format!("{}/test/path", home.display());
            let shortened = shorten_home_path(&test_path);
            assert!(shortened.starts_with("~/"));
            assert!(shortened.ends_with("test/path"));
        }
    }

    #[test]
    fn shorten_home_path_without_home() {
        let shortened = shorten_home_path("/tmp/no-home");
        assert_eq!(shortened, "/tmp/no-home");
    }
}
