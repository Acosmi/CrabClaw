/// Desktop shell launcher for the Rust CLI.
///
/// Resolves the native desktop host binary (`openacosmi-desktop`) and starts
/// it as either a foreground process (`--wait`) or a detached child.
use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::process::Stdio;

use anyhow::{Context, Result};
use serde_json::json;
use tokio::process::Command;
use tracing::info;

use oa_terminal::theme::Theme;

const DESKTOP_BINARY_ENV: &str = "OPENACOSMI_DESKTOP_BINARY";

/// Options for launching the desktop shell.
#[derive(Debug, Clone, Default)]
pub struct DesktopOptions {
    /// Override the gateway port passed to the desktop shell.
    pub port: Option<u16>,
    /// Explicit Control UI directory for development use.
    pub control_ui_dir: Option<String>,
    /// If true, wait for the desktop process to exit.
    pub wait: bool,
    /// Output machine-readable JSON.
    pub json: bool,
}

fn desktop_binary_name() -> &'static str {
    if cfg!(target_os = "windows") {
        "openacosmi-desktop.exe"
    } else {
        "openacosmi-desktop"
    }
}

fn append_unique_candidate(
    candidates: &mut Vec<PathBuf>,
    seen: &mut HashSet<PathBuf>,
    candidate: PathBuf,
) {
    if seen.insert(candidate.clone()) {
        candidates.push(candidate);
    }
}

fn repo_dev_binary_candidates(base: &Path) -> Vec<PathBuf> {
    let mut result = Vec::new();
    let name = desktop_binary_name();
    for relative in [
        Path::new("backend/cmd/desktop/bin").join(name),
        Path::new("backend/bin").join(name),
        Path::new("bin").join(name),
    ] {
        result.push(base.join(relative));
    }
    result
}

fn resolve_desktop_binary_with(
    explicit_override: Option<PathBuf>,
    current_exe: Option<PathBuf>,
    current_dir: Option<PathBuf>,
    path_lookup: Option<PathBuf>,
) -> Result<PathBuf> {
    if let Some(path) = explicit_override {
        if path.exists() {
            return Ok(path);
        }
        anyhow::bail!(
            "{DESKTOP_BINARY_ENV} is set to \"{}\" but the file does not exist",
            path.display()
        );
    }

    let mut candidates = Vec::new();
    let mut seen = HashSet::new();

    if let Some(exe) = current_exe {
        if let Some(dir) = exe.parent() {
            append_unique_candidate(&mut candidates, &mut seen, dir.join(desktop_binary_name()));
            for candidate in repo_dev_binary_candidates(dir) {
                append_unique_candidate(&mut candidates, &mut seen, candidate);
            }
        }
    }

    if let Some(dir) = current_dir {
        append_unique_candidate(&mut candidates, &mut seen, dir.join(desktop_binary_name()));
        for candidate in repo_dev_binary_candidates(&dir) {
            append_unique_candidate(&mut candidates, &mut seen, candidate);
        }
    }

    for candidate in candidates {
        if candidate.exists() {
            return Ok(candidate);
        }
    }

    if let Some(path) = path_lookup {
        return Ok(path);
    }

    anyhow::bail!(
        "Desktop binary \"{}\" not found.\n\
         Searched:\n\
         - ${DESKTOP_BINARY_ENV}\n\
         - sibling of current executable\n\
         - repo build outputs (backend/cmd/desktop/bin, backend/bin, bin)\n\
         - PATH",
        desktop_binary_name()
    )
}

fn resolve_desktop_binary() -> Result<PathBuf> {
    let explicit = std::env::var(DESKTOP_BINARY_ENV).ok().map(PathBuf::from);
    let current_exe = std::env::current_exe().ok();
    let current_dir = std::env::current_dir().ok();
    let path_lookup = which::which(desktop_binary_name()).ok();
    resolve_desktop_binary_with(explicit, current_exe, current_dir, path_lookup)
}

/// Launch the native desktop shell binary.
pub async fn desktop_command(options: DesktopOptions) -> Result<()> {
    let desktop_binary = resolve_desktop_binary()?;

    info!(
        binary = %desktop_binary.display(),
        port = options.port,
        control_ui_dir = options.control_ui_dir,
        wait = options.wait,
        "desktop launch requested"
    );

    let mut command = Command::new(&desktop_binary);
    if let Some(port) = options.port {
        command.arg("--port").arg(port.to_string());
    }
    if let Some(ref control_ui_dir) = options.control_ui_dir {
        command.arg("--control-ui-dir").arg(control_ui_dir);
    }

    if options.wait {
        command
            .stdout(Stdio::inherit())
            .stderr(Stdio::inherit())
            .stdin(Stdio::inherit());
    } else {
        command
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .stdin(Stdio::null());
    }

    let mut child = command.spawn().with_context(|| {
        format!(
            "failed to start desktop binary: {}",
            desktop_binary.display()
        )
    })?;

    if options.wait {
        let status = child
            .wait()
            .await
            .context("failed while waiting for desktop process")?;
        if status.success() {
            if options.json {
                println!(
                    "{}",
                    serde_json::to_string_pretty(&json!({
                        "status": "ok",
                        "binary": desktop_binary.display().to_string(),
                        "waited": true,
                        "exitCode": status.code(),
                    }))?
                );
            } else {
                println!("{}", Theme::muted("Desktop shell exited."));
            }
            return Ok(());
        }

        let code = status.code().unwrap_or(-1);
        anyhow::bail!("desktop process exited with code {code}");
    }

    let pid = child.id();
    if options.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&json!({
                "status": "launched",
                "binary": desktop_binary.display().to_string(),
                "pid": pid,
                "waited": false,
            }))?
        );
    } else {
        println!(
            "{} {}",
            Theme::success("Desktop shell launched:"),
            Theme::muted(&desktop_binary.display().to_string()),
        );
        if let Some(pid) = pid {
            println!(
                "  {} {}",
                Theme::muted("PID:"),
                Theme::accent(&pid.to_string())
            );
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_desktop_binary_prefers_explicit_override() {
        let temp = tempfile::tempdir().expect("tempdir");
        let binary = temp.path().join(desktop_binary_name());
        std::fs::write(&binary, b"binary").expect("write binary");

        let resolved = resolve_desktop_binary_with(Some(binary.clone()), None, None, None)
            .expect("resolve binary");
        assert_eq!(resolved, binary);
    }

    #[test]
    fn resolve_desktop_binary_finds_repo_dev_candidate() {
        let temp = tempfile::tempdir().expect("tempdir");
        let bin_dir = temp.path().join("backend/cmd/desktop/bin");
        std::fs::create_dir_all(&bin_dir).expect("create bin dir");
        let binary = bin_dir.join(desktop_binary_name());
        std::fs::write(&binary, b"binary").expect("write binary");

        let resolved =
            resolve_desktop_binary_with(None, None, Some(temp.path().to_path_buf()), None)
                .expect("resolve binary");
        assert_eq!(resolved, binary);
    }

    #[test]
    fn resolve_desktop_binary_uses_path_lookup_last() {
        let temp = tempfile::tempdir().expect("tempdir");
        let binary = temp.path().join(desktop_binary_name());
        std::fs::write(&binary, b"binary").expect("write binary");

        let resolved = resolve_desktop_binary_with(None, None, None, Some(binary.clone()))
            .expect("resolve binary");
        assert_eq!(resolved, binary);
    }
}
