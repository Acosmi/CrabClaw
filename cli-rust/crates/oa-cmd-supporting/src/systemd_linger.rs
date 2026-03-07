/// Systemd user linger management.
///
/// Ensures systemd user lingering is enabled so that the gateway service
/// continues running after logout. Provides both interactive (with prompts)
/// and non-interactive flows.
///
/// Source: `src/commands/systemd-linger.ts`
use anyhow::Result;
use tracing::{error, info};

/// Status of systemd user lingering for a given user.
///
/// Source: `src/commands/systemd-linger.ts` - `readSystemdUserLingerStatus` return
#[derive(Debug, Clone)]
pub struct LingerStatus {
    /// The username.
    pub user: String,
    /// Whether lingering is enabled ("yes" or "no").
    pub linger: String,
}

/// Result of enabling lingering.
///
/// Source: `src/commands/systemd-linger.ts` - `enableSystemdUserLinger` return
#[derive(Debug, Clone)]
pub struct LingerEnableResult {
    /// Whether enabling succeeded.
    pub ok: bool,
    /// Stdout from the command.
    pub stdout: String,
    /// Stderr from the command.
    pub stderr: String,
}

/// Check whether systemd user services are available.
///
/// Source: `src/daemon/systemd.ts` - `isSystemdUserServiceAvailable`
pub async fn is_systemd_user_service_available() -> bool {
    if cfg!(not(target_os = "linux")) {
        return false;
    }

    let output = tokio::process::Command::new("systemctl")
        .args(["--user", "status"])
        .output()
        .await;

    // systemctl --user status returns 0 or non-zero; we just check if it runs.
    output.is_ok()
}

/// Read the current linger status for the current user.
///
/// Uses `loginctl` to determine the username and linger state.
///
/// Source: `src/daemon/systemd.ts` - `readSystemdUserLingerStatus`
pub async fn read_systemd_user_linger_status() -> Option<LingerStatus> {
    if cfg!(not(target_os = "linux")) {
        return None;
    }

    // Get the current username.
    let user = std::env::var("USER")
        .or_else(|_| std::env::var("LOGNAME"))
        .ok()?;

    if user.trim().is_empty() {
        return None;
    }

    // Check linger status.
    let output = tokio::process::Command::new("loginctl")
        .args(["show-user", &user, "--property=Linger"])
        .output()
        .await
        .ok()?;

    let stdout = String::from_utf8_lossy(&output.stdout);
    let linger = if stdout.contains("Linger=yes") {
        "yes".to_owned()
    } else {
        "no".to_owned()
    };

    Some(LingerStatus { user, linger })
}

/// Enable systemd user lingering for a user.
///
/// Source: `src/daemon/systemd.ts` - `enableSystemdUserLinger`
pub async fn enable_systemd_user_linger(user: &str, use_sudo: bool) -> LingerEnableResult {
    let mut cmd = if use_sudo {
        let mut c = tokio::process::Command::new("sudo");
        c.args(["loginctl", "enable-linger", user]);
        c
    } else {
        let mut c = tokio::process::Command::new("loginctl");
        c.args(["enable-linger", user]);
        c
    };

    match cmd.output().await {
        Ok(output) => LingerEnableResult {
            ok: output.status.success(),
            stdout: String::from_utf8_lossy(&output.stdout).to_string(),
            stderr: String::from_utf8_lossy(&output.stderr).to_string(),
        },
        Err(e) => LingerEnableResult {
            ok: false,
            stdout: String::new(),
            stderr: format!("Command failed: {e}"),
        },
    }
}

/// Ensure systemd user lingering is enabled (interactive mode).
///
/// Checks the current linger status and attempts to enable it,
/// first without sudo and then with sudo if needed.
///
/// Source: `src/commands/systemd-linger.ts` - `ensureSystemdUserLingerInteractive`
pub async fn ensure_systemd_user_linger_interactive(prompt_enabled: bool) -> Result<()> {
    if cfg!(not(target_os = "linux")) {
        return Ok(());
    }

    if !prompt_enabled {
        return Ok(());
    }

    if !is_systemd_user_service_available().await {
        info!("Systemd user services are unavailable. Skipping lingering checks.");
        return Ok(());
    }

    let status = match read_systemd_user_linger_status().await {
        Some(s) => s,
        None => {
            info!(
                "Unable to read loginctl linger status. Ensure systemd + loginctl are available."
            );
            return Ok(());
        }
    };

    if status.linger == "yes" {
        return Ok(());
    }

    info!("Systemd user services stop when you log out or go idle, which kills the Gateway.");
    info!("Enabling lingering now (may require sudo; writes /var/lib/systemd/linger).");

    // Try without sudo first.
    let result = enable_systemd_user_linger(&status.user, false).await;
    if result.ok {
        info!("Enabled systemd lingering for {}.", status.user);
        return Ok(());
    }

    // Try with sudo.
    let result = enable_systemd_user_linger(&status.user, true).await;
    if result.ok {
        info!("Enabled systemd lingering for {}.", status.user);
        return Ok(());
    }

    error!(
        "Failed to enable lingering: {}",
        if !result.stderr.is_empty() {
            &result.stderr
        } else if !result.stdout.is_empty() {
            &result.stdout
        } else {
            "unknown error"
        }
    );
    info!("Run manually: sudo loginctl enable-linger {}", status.user);

    Ok(())
}

/// Ensure systemd user lingering is enabled (non-interactive mode).
///
/// Silently attempts to enable lingering using the non-interactive sudo mode.
///
/// Source: `src/commands/systemd-linger.ts` - `ensureSystemdUserLingerNonInteractive`
pub async fn ensure_systemd_user_linger_non_interactive() -> Result<()> {
    if cfg!(not(target_os = "linux")) {
        return Ok(());
    }

    if !is_systemd_user_service_available().await {
        return Ok(());
    }

    let status = match read_systemd_user_linger_status().await {
        Some(s) => s,
        None => return Ok(()),
    };

    if status.linger == "yes" {
        return Ok(());
    }

    // Try without sudo (non-interactive).
    let result = enable_systemd_user_linger(&status.user, false).await;
    if result.ok {
        info!("Enabled systemd lingering for {}.", status.user);
        return Ok(());
    }

    info!(
        "Systemd lingering is disabled for {}. Run: sudo loginctl enable-linger {}",
        status.user, status.user
    );

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn linger_status_structure() {
        let status = LingerStatus {
            user: "testuser".to_owned(),
            linger: "yes".to_owned(),
        };
        assert_eq!(status.user, "testuser");
        assert_eq!(status.linger, "yes");
    }

    #[test]
    fn linger_enable_result_success() {
        let result = LingerEnableResult {
            ok: true,
            stdout: String::new(),
            stderr: String::new(),
        };
        assert!(result.ok);
    }

    #[test]
    fn linger_enable_result_failure() {
        let result = LingerEnableResult {
            ok: false,
            stdout: String::new(),
            stderr: "Permission denied".to_owned(),
        };
        assert!(!result.ok);
        assert!(result.stderr.contains("Permission denied"));
    }

    #[cfg(not(target_os = "linux"))]
    #[tokio::test]
    async fn systemd_not_available_on_non_linux() {
        assert!(!is_systemd_user_service_available().await);
    }

    #[cfg(not(target_os = "linux"))]
    #[tokio::test]
    async fn linger_status_none_on_non_linux() {
        let status = read_systemd_user_linger_status().await;
        assert!(status.is_none());
    }

    #[cfg(not(target_os = "linux"))]
    #[tokio::test]
    async fn ensure_interactive_noop_on_non_linux() {
        let result = ensure_systemd_user_linger_interactive(true).await;
        assert!(result.is_ok());
    }

    #[cfg(not(target_os = "linux"))]
    #[tokio::test]
    async fn ensure_non_interactive_noop_on_non_linux() {
        let result = ensure_systemd_user_linger_non_interactive().await;
        assert!(result.is_ok());
    }
}
