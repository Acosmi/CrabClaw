/// Reset command implementation.
///
/// Provides a multi-scope reset flow: config-only, config+credentials+sessions,
/// or full reset (state dir + workspace). Supports dry-run, non-interactive,
/// and auto-confirm modes.
///
/// Source: `src/commands/reset.ts`
use std::path::Path;

use anyhow::Result;
use tracing::{error, info};

use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::load_config;
use oa_config::paths::{is_nix_mode, resolve_config_path, resolve_state_dir};

use crate::cleanup_utils::{
    collect_workspace_dirs, is_path_within, list_agent_session_dirs, remove_path,
};

/// The scope of a reset operation.
///
/// Source: `src/commands/reset.ts` - `ResetScope`
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ResetScope {
    /// Remove only the config file.
    Config,
    /// Remove config, credentials, and session data.
    ConfigCredsAndSessions,
    /// Full reset: state directory, config, and workspaces.
    Full,
}

impl ResetScope {
    /// Parse a scope string.
    ///
    /// Source: `src/commands/reset.ts` - scope validation
    pub fn from_str(s: &str) -> Option<Self> {
        match s.trim() {
            "config" => Some(Self::Config),
            "config+creds+sessions" => Some(Self::ConfigCredsAndSessions),
            "full" => Some(Self::Full),
            _ => None,
        }
    }

    /// Return the canonical string form.
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Config => "config",
            Self::ConfigCredsAndSessions => "config+creds+sessions",
            Self::Full => "full",
        }
    }
}

/// Options for the reset command.
///
/// Source: `src/commands/reset.ts` - `ResetOptions`
#[derive(Debug, Clone, Default)]
pub struct ResetOptions {
    /// Which scope to reset.
    pub scope: Option<ResetScope>,
    /// Skip confirmation prompts.
    pub yes: bool,
    /// Disable interactive prompts entirely.
    pub non_interactive: bool,
    /// Preview actions without executing them.
    pub dry_run: bool,
}

/// Stop the gateway service if running and not in Nix mode.
///
/// Source: `src/commands/reset.ts` - `stopGatewayIfRunning`
async fn stop_gateway_if_running(dry_run: bool) {
    if is_nix_mode() {
        return;
    }

    if dry_run {
        info!("[dry-run] stop gateway service");
        return;
    }

    match oa_daemon::service::stop_gateway_service(None) {
        Ok(()) => info!("Gateway service stopped."),
        Err(e) => error!("Gateway stop failed: {e}"),
    }
}

/// Run the reset command.
///
/// Removes configuration, credentials, sessions, state, and/or workspace
/// directories depending on the selected scope.
///
/// Source: `src/commands/reset.ts` - `resetCommand`
pub async fn reset_command(opts: &ResetOptions) -> Result<()> {
    let interactive = !opts.non_interactive;
    if !interactive && !opts.yes {
        error!("Non-interactive mode requires --yes.");
        anyhow::bail!("Non-interactive mode requires --yes.");
    }

    let scope = match opts.scope {
        Some(s) => s,
        None => {
            if !interactive {
                error!("Non-interactive mode requires --scope.");
                anyhow::bail!("Non-interactive mode requires --scope.");
            }
            // In interactive mode we would prompt; for now default to config.
            // TODO: Wire up interactive prompt via terminal crate.
            info!("No scope provided; defaulting to config-only reset.");
            ResetScope::Config
        }
    };

    let dry_run = opts.dry_run;
    let cfg = load_config().ok();
    let state_dir = resolve_state_dir();
    let config_path = resolve_config_path();
    let config_path_str = config_path.display().to_string();
    let oauth_dir = resolve_oauth_dir(&state_dir);
    let oauth_dir_str = oauth_dir.display().to_string();
    let config_inside_state = is_path_within(&config_path, &state_dir);
    let oauth_inside_state = is_path_within(&oauth_dir, &state_dir);
    let workspace_dirs = collect_workspace_dirs(cfg.as_ref());

    match scope {
        ResetScope::Config => {
            remove_path(&config_path_str, dry_run, Some(&config_path_str)).await;
        }
        ResetScope::ConfigCredsAndSessions => {
            stop_gateway_if_running(dry_run).await;

            remove_path(&config_path_str, dry_run, Some(&config_path_str)).await;
            remove_path(&oauth_dir_str, dry_run, Some(&oauth_dir_str)).await;

            let session_dirs = list_agent_session_dirs(&state_dir).await;
            for dir in &session_dirs {
                let dir_str = dir.display().to_string();
                remove_path(&dir_str, dry_run, Some(&dir_str)).await;
            }

            let next_cmd = format_cli_command("crabclaw onboard --install-daemon");
            info!("Next: {next_cmd}");
        }
        ResetScope::Full => {
            stop_gateway_if_running(dry_run).await;

            let state_dir_str = state_dir.display().to_string();
            remove_path(&state_dir_str, dry_run, Some(&state_dir_str)).await;

            if !config_inside_state {
                remove_path(&config_path_str, dry_run, Some(&config_path_str)).await;
            }
            if !oauth_inside_state {
                remove_path(&oauth_dir_str, dry_run, Some(&oauth_dir_str)).await;
            }

            for workspace in &workspace_dirs {
                let ws_str = workspace.display().to_string();
                remove_path(&ws_str, dry_run, Some(&ws_str)).await;
            }

            let next_cmd = format_cli_command("crabclaw onboard --install-daemon");
            info!("Next: {next_cmd}");
        }
    }

    Ok(())
}

/// Resolve the OAuth credentials directory.
///
/// Source: `src/commands/reset.ts` - `resolveOAuthDir`
fn resolve_oauth_dir(state_dir: &Path) -> std::path::PathBuf {
    oa_config::paths::resolve_oauth_dir(state_dir)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn scope_from_str_valid() {
        assert_eq!(ResetScope::from_str("config"), Some(ResetScope::Config));
        assert_eq!(
            ResetScope::from_str("config+creds+sessions"),
            Some(ResetScope::ConfigCredsAndSessions)
        );
        assert_eq!(ResetScope::from_str("full"), Some(ResetScope::Full));
    }

    #[test]
    fn scope_from_str_invalid() {
        assert!(ResetScope::from_str("").is_none());
        assert!(ResetScope::from_str("partial").is_none());
        assert!(ResetScope::from_str("Config").is_none());
    }

    #[test]
    fn scope_roundtrip() {
        for scope in &[
            ResetScope::Config,
            ResetScope::ConfigCredsAndSessions,
            ResetScope::Full,
        ] {
            assert_eq!(ResetScope::from_str(scope.as_str()), Some(*scope));
        }
    }

    #[test]
    fn default_options() {
        let opts = ResetOptions::default();
        assert!(opts.scope.is_none());
        assert!(!opts.yes);
        assert!(!opts.non_interactive);
        assert!(!opts.dry_run);
    }
}
