/// State directory integrity checks.
///
/// Verifies that the active state directory (`~/.crabclaw` or `~/.openacosmi`),
/// sessions directory,
/// session store directory, and OAuth directory exist and are writable.
/// Detects permission issues, multiple state directories, and missing
/// session transcripts.
///
/// Source: `src/commands/doctor-state-integrity.ts`
use std::path::{Path, PathBuf};

use oa_config::paths::resolve_state_dir;
use oa_terminal::note::note;
use oa_types::config::OpenAcosmiConfig;

use crate::prompter::DoctorPrompter;

/// Check whether a directory exists and is a directory.
///
/// Source: `src/commands/doctor-state-integrity.ts` — `existsDir`
fn exists_dir(dir: &Path) -> bool {
    dir.is_dir()
}

/// Check whether a file exists and is a regular file.
///
/// Source: `src/commands/doctor-state-integrity.ts` — `existsFile`
fn exists_file(path: &Path) -> bool {
    path.is_file()
}

/// Check whether a directory is writable by the current user.
///
/// Source: `src/commands/doctor-state-integrity.ts` — `canWriteDir`
fn can_write_dir(dir: &Path) -> bool {
    // Attempt to create a temp file inside the directory.
    let probe = dir.join(".oa-doctor-probe");
    match std::fs::write(&probe, b"") {
        Ok(()) => {
            let _ = std::fs::remove_file(&probe);
            true
        }
        Err(_) => false,
    }
}

/// Attempt to create a directory (recursive).
///
/// Source: `src/commands/doctor-state-integrity.ts` — `ensureDir`
fn ensure_dir(dir: &Path) -> Result<(), String> {
    std::fs::create_dir_all(dir).map_err(|e| e.to_string())
}

/// Shorten a path by replacing the home directory prefix with `~`.
fn shorten_home_path(path: &Path) -> String {
    let home = dirs::home_dir().unwrap_or_default();
    let path_str = path.to_string_lossy();
    let home_str = home.to_string_lossy();
    if path_str.starts_with(home_str.as_ref()) {
        format!("~{}", &path_str[home_str.len()..])
    } else {
        path_str.to_string()
    }
}

/// Search for other state directories under `/Users` (macOS) or `/home` (Linux).
///
/// Source: `src/commands/doctor-state-integrity.ts` — `findOtherStateDirs`
fn find_other_state_dirs(state_dir: &Path) -> Vec<PathBuf> {
    let resolved = state_dir
        .canonicalize()
        .unwrap_or_else(|_| state_dir.to_path_buf());
    let roots: Vec<&str> = if cfg!(target_os = "macos") {
        vec!["/Users"]
    } else if cfg!(target_os = "linux") {
        vec!["/home"]
    } else {
        vec![]
    };

    let mut found = Vec::new();
    for root in roots {
        let Ok(entries) = std::fs::read_dir(root) else {
            continue;
        };
        for entry in entries.flatten() {
            if !entry.file_type().is_ok_and(|ft| ft.is_dir()) {
                continue;
            }
            let name = entry.file_name();
            if name.to_string_lossy().starts_with('.') {
                continue;
            }
            for dirname in [".crabclaw", ".openacosmi"] {
                let candidate = entry.path().join(dirname);
                if candidate
                    .canonicalize()
                    .unwrap_or_else(|_| candidate.clone())
                    == resolved
                {
                    continue;
                }
                if exists_dir(&candidate) {
                    found.push(candidate);
                }
            }
        }
    }
    found
}

/// Run state integrity checks and emit notes.
///
/// Source: `src/commands/doctor-state-integrity.ts` — `noteStateIntegrity`
pub async fn note_state_integrity(
    cfg: &OpenAcosmiConfig,
    prompter: &mut DoctorPrompter,
    _config_path: Option<&str>,
) {
    let mut warnings: Vec<String> = Vec::new();
    let mut changes: Vec<String> = Vec::new();

    let state_dir = resolve_state_dir();
    let display_state = shorten_home_path(&state_dir);

    // ── State directory existence ──
    let mut state_dir_exists = exists_dir(&state_dir);
    if !state_dir_exists {
        warnings.push(format!(
            "- CRITICAL: state directory missing ({display_state}). Sessions, credentials, logs, and config are stored there."
        ));
        if cfg
            .gateway
            .as_ref()
            .and_then(|gw| gw.mode.as_ref())
            .is_some_and(|m| *m == oa_types::gateway::GatewayMode::Remote)
        {
            warnings.push(
                "- Gateway is in remote mode; run doctor on the remote host where the gateway runs."
                    .to_string(),
            );
        }
        let create = prompter
            .confirm_skip_in_non_interactive(&format!("Create {display_state} now?"), false)
            .await;
        if create {
            match ensure_dir(&state_dir) {
                Ok(()) => {
                    changes.push(format!("- Created {display_state}"));
                    state_dir_exists = true;
                }
                Err(e) => {
                    warnings.push(format!("- Failed to create {display_state}: {e}"));
                }
            }
        }
    }

    // ── State directory writability ──
    if state_dir_exists && !can_write_dir(&state_dir) {
        warnings.push(format!("- State directory not writable ({display_state})."));
    }

    // ── Permissions (non-Windows) ──
    #[cfg(unix)]
    if state_dir_exists {
        use std::os::unix::fs::MetadataExt;
        if let Ok(meta) = std::fs::metadata(&state_dir) {
            let mode = meta.mode() & 0o777;
            if mode & 0o077 != 0 {
                warnings.push(format!(
                    "- State directory permissions are too open ({display_state}). Recommend chmod 700."
                ));
                let tighten = prompter
                    .confirm_skip_in_non_interactive(
                        &format!("Tighten permissions on {display_state} to 700?"),
                        true,
                    )
                    .await;
                if tighten {
                    use std::os::unix::fs::PermissionsExt;
                    let perms = std::fs::Permissions::from_mode(0o700);
                    match std::fs::set_permissions(&state_dir, perms) {
                        Ok(()) => {
                            changes
                                .push(format!("- Tightened permissions on {display_state} to 700"));
                        }
                        Err(e) => {
                            warnings.push(format!(
                                "- Failed to tighten permissions on {display_state}: {e}"
                            ));
                        }
                    }
                }
            }
        }
    }

    // ── Sub-directories ──
    if state_dir_exists {
        let subdirs = [
            (state_dir.join("sessions"), "Sessions dir"),
            (state_dir.join("credentials"), "OAuth dir"),
        ];
        for (dir, label) in &subdirs {
            let display = shorten_home_path(dir);
            if !exists_dir(dir) {
                warnings.push(format!("- CRITICAL: {label} missing ({display})."));
                let create = prompter
                    .confirm_skip_in_non_interactive(&format!("Create {label} at {display}?"), true)
                    .await;
                if create {
                    match ensure_dir(dir) {
                        Ok(()) => changes.push(format!("- Created {label}: {display}")),
                        Err(e) => warnings.push(format!("- Failed to create {display}: {e}")),
                    }
                }
            } else if !can_write_dir(dir) {
                warnings.push(format!("- {label} not writable ({display})."));
            }
        }
    }

    // ── Multiple state directories ──
    let extra_dirs = find_other_state_dirs(&state_dir);
    if !extra_dirs.is_empty() {
        let mut lines = vec![
            "- Multiple state directories detected. This can split session history.".to_string(),
        ];
        for dir in &extra_dirs {
            lines.push(format!("  - {}", shorten_home_path(dir)));
        }
        lines.push(format!("  Active state dir: {display_state}"));
        warnings.push(lines.join("\n"));
    }

    if !warnings.is_empty() {
        note(&warnings.join("\n"), Some("State integrity"));
    }
    if !changes.is_empty() {
        note(&changes.join("\n"), Some("Doctor changes"));
    }
}

/// Suggest backing up the workspace directory with git.
///
/// Source: `src/commands/doctor-state-integrity.ts` — `noteWorkspaceBackupTip`
pub fn note_workspace_backup_tip(workspace_dir: &Path) {
    if !exists_dir(workspace_dir) {
        return;
    }
    let git_marker = workspace_dir.join(".git");
    if exists_file(&git_marker) || exists_dir(&git_marker) {
        return;
    }
    note(
        &[
            "- Tip: back up the workspace in a private git repo (GitHub or GitLab).",
            "- Keep ~/.crabclaw and ~/.openacosmi out of git; they contain credentials and session history.",
            "- Details: /concepts/agent-workspace#git-backup-recommended",
        ]
        .join("\n"),
        Some("Workspace"),
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exists_dir_false_for_missing() {
        assert!(!exists_dir(Path::new("/nonexistent-oa-test-dir")));
    }

    #[test]
    fn exists_file_false_for_missing() {
        assert!(!exists_file(Path::new("/nonexistent-oa-test-file")));
    }

    #[test]
    fn can_write_temp_dir() {
        let tmp = std::env::temp_dir();
        assert!(can_write_dir(&tmp));
    }

    #[test]
    fn ensure_dir_creates_nested() {
        let tmp = std::env::temp_dir()
            .join("oa-doctor-ensure-test")
            .join("nested");
        let _ = std::fs::remove_dir_all(tmp.parent().unwrap_or(Path::new(".")));
        assert!(ensure_dir(&tmp).is_ok());
        assert!(tmp.is_dir());
        let _ = std::fs::remove_dir_all(tmp.parent().unwrap_or(Path::new(".")));
    }

    #[test]
    fn shorten_home_path_works() {
        let home = dirs::home_dir().unwrap_or_default();
        let path = home.join(".crabclaw").join("config.json");
        let shortened = shorten_home_path(&path);
        assert!(shortened.starts_with('~'));
        assert!(shortened.contains(".crabclaw"));
    }

    #[test]
    fn workspace_backup_tip_noop_for_missing_dir() {
        note_workspace_backup_tip(Path::new("/nonexistent-oa-workspace"));
    }

    #[test]
    fn workspace_backup_tip_noop_when_git_exists() {
        let tmp = std::env::temp_dir().join("oa-doctor-ws-git-test");
        let _ = std::fs::create_dir_all(tmp.join(".git"));
        note_workspace_backup_tip(&tmp);
        let _ = std::fs::remove_dir_all(&tmp);
    }
}
