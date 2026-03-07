/// Uninstall command implementation.
///
/// Provides a multi-scope uninstall flow: gateway service, state+config,
/// workspace directories, and macOS app. Supports dry-run, non-interactive,
/// and auto-confirm modes.
///
/// Source: `src/commands/uninstall.ts`
use std::collections::HashSet;
use std::path::Path;

use anyhow::Result;
use tracing::{error, info};

use oa_config::io::load_config;
use oa_config::paths::{is_nix_mode, resolve_config_path, resolve_state_dir};

use crate::cleanup_utils::{collect_workspace_dirs, is_path_within, remove_path};

/// Individual uninstall scope items.
///
/// Source: `src/commands/uninstall.ts` - `UninstallScope`
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum UninstallScope {
    /// Remove the gateway service (launchd/systemd).
    Service,
    /// Remove the state directory and config.
    State,
    /// Remove workspace directories.
    Workspace,
    /// Remove the macOS app bundle.
    App,
}

impl UninstallScope {
    /// Parse a scope string.
    pub fn from_str(s: &str) -> Option<Self> {
        match s.trim().to_lowercase().as_str() {
            "service" => Some(Self::Service),
            "state" => Some(Self::State),
            "workspace" => Some(Self::Workspace),
            "app" => Some(Self::App),
            _ => None,
        }
    }
}

/// Options for the uninstall command.
///
/// Source: `src/commands/uninstall.ts` - `UninstallOptions`
#[derive(Debug, Clone, Default)]
pub struct UninstallOptions {
    /// Include the gateway service scope.
    pub service: bool,
    /// Include the state+config scope.
    pub state: bool,
    /// Include workspace directories scope.
    pub workspace: bool,
    /// Include the macOS app scope.
    pub app: bool,
    /// Include all scopes.
    pub all: bool,
    /// Skip confirmation prompts.
    pub yes: bool,
    /// Disable interactive prompts entirely.
    pub non_interactive: bool,
    /// Preview actions without executing them.
    pub dry_run: bool,
}

/// Build the set of selected scopes from the options.
///
/// Source: `src/commands/uninstall.ts` - `buildScopeSelection`
fn build_scope_selection(opts: &UninstallOptions) -> (HashSet<UninstallScope>, bool) {
    let had_explicit = opts.all || opts.service || opts.state || opts.workspace || opts.app;
    let mut scopes = HashSet::new();
    if opts.all || opts.service {
        scopes.insert(UninstallScope::Service);
    }
    if opts.all || opts.state {
        scopes.insert(UninstallScope::State);
    }
    if opts.all || opts.workspace {
        scopes.insert(UninstallScope::Workspace);
    }
    if opts.all || opts.app {
        scopes.insert(UninstallScope::App);
    }
    (scopes, had_explicit)
}

/// Stop and uninstall the gateway service.
///
/// Source: `src/commands/uninstall.ts` - `stopAndUninstallService`
fn stop_and_uninstall_service() -> bool {
    if is_nix_mode() {
        error!("Nix mode detected; service uninstall is disabled.");
        return false;
    }

    // Check if loaded.
    let loaded = match oa_daemon::service::is_gateway_service_enabled(None) {
        Ok(v) => v,
        Err(e) => {
            error!("Gateway service check failed: {e}");
            return false;
        }
    };

    if !loaded {
        info!("Gateway service is not loaded.");
        return true;
    }

    // Stop.
    if let Err(e) = oa_daemon::service::stop_gateway_service(None) {
        error!("Gateway stop failed: {e}");
    }

    // Uninstall.
    match oa_daemon::service::uninstall_gateway_service(None) {
        Ok(()) => true,
        Err(e) => {
            error!("Gateway uninstall failed: {e}");
            false
        }
    }
}

/// Remove the macOS app bundle if on macOS.
///
/// Source: `src/commands/uninstall.ts` - `removeMacApp`
async fn remove_mac_app(dry_run: bool) {
    if cfg!(not(target_os = "macos")) {
        return;
    }
    remove_path(
        "/Applications/OpenAcosmi.app",
        dry_run,
        Some("/Applications/OpenAcosmi.app"),
    )
    .await;
}

/// Resolve the OAuth credentials directory.
fn resolve_oauth_dir(state_dir: &Path) -> std::path::PathBuf {
    oa_config::paths::resolve_oauth_dir(state_dir)
}

/// Run the uninstall command.
///
/// Removes selected components: gateway service, state+config, workspace
/// directories, and/or the macOS app bundle.
///
/// Source: `src/commands/uninstall.ts` - `uninstallCommand`
pub async fn uninstall_command(opts: &UninstallOptions) -> Result<()> {
    let (mut scopes, had_explicit) = build_scope_selection(opts);
    let interactive = !opts.non_interactive;

    if !interactive && !opts.yes {
        error!("Non-interactive mode requires --yes.");
        anyhow::bail!("Non-interactive mode requires --yes.");
    }

    if !had_explicit {
        if !interactive {
            error!("Non-interactive mode requires explicit scopes (use --all).");
            anyhow::bail!("Non-interactive mode requires explicit scopes (use --all).");
        }
        // In interactive mode, prompt user. Default to service+state+workspace.
        // TODO: Wire up interactive multiselect prompt.
        info!("No scopes provided; defaulting to service+state+workspace.");
        scopes.insert(UninstallScope::Service);
        scopes.insert(UninstallScope::State);
        scopes.insert(UninstallScope::Workspace);
    }

    if scopes.is_empty() {
        info!("Nothing selected.");
        return Ok(());
    }

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

    // Service removal.
    if scopes.contains(&UninstallScope::Service) {
        if dry_run {
            info!("[dry-run] remove gateway service");
        } else {
            stop_and_uninstall_service();
        }
    }

    // State removal.
    if scopes.contains(&UninstallScope::State) {
        let state_dir_str = state_dir.display().to_string();
        remove_path(&state_dir_str, dry_run, Some(&state_dir_str)).await;
        if !config_inside_state {
            remove_path(&config_path_str, dry_run, Some(&config_path_str)).await;
        }
        if !oauth_inside_state {
            remove_path(&oauth_dir_str, dry_run, Some(&oauth_dir_str)).await;
        }
    }

    // Workspace removal.
    if scopes.contains(&UninstallScope::Workspace) {
        for workspace in &workspace_dirs {
            let ws_str = workspace.display().to_string();
            remove_path(&ws_str, dry_run, Some(&ws_str)).await;
        }
    }

    // macOS app removal.
    if scopes.contains(&UninstallScope::App) {
        remove_mac_app(dry_run).await;
    }

    info!("CLI still installed. Remove via npm/pnpm if desired.");

    // Hint about preserved workspaces.
    if scopes.contains(&UninstallScope::State) && !scopes.contains(&UninstallScope::Workspace) {
        if let Some(home) = dirs::home_dir() {
            let home_str = home.display().to_string();
            if workspace_dirs
                .iter()
                .any(|d| d.display().to_string().starts_with(&home_str))
            {
                info!("Tip: workspaces were preserved. Re-run with --workspace to remove them.");
            }
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn scope_from_str_valid() {
        assert_eq!(
            UninstallScope::from_str("service"),
            Some(UninstallScope::Service)
        );
        assert_eq!(
            UninstallScope::from_str("state"),
            Some(UninstallScope::State)
        );
        assert_eq!(
            UninstallScope::from_str("workspace"),
            Some(UninstallScope::Workspace)
        );
        assert_eq!(UninstallScope::from_str("app"), Some(UninstallScope::App));
    }

    #[test]
    fn scope_from_str_case_insensitive() {
        assert_eq!(
            UninstallScope::from_str("SERVICE"),
            Some(UninstallScope::Service)
        );
        assert_eq!(
            UninstallScope::from_str("State"),
            Some(UninstallScope::State)
        );
    }

    #[test]
    fn scope_from_str_invalid() {
        assert!(UninstallScope::from_str("").is_none());
        assert!(UninstallScope::from_str("other").is_none());
    }

    #[test]
    fn build_scope_all() {
        let opts = UninstallOptions {
            all: true,
            ..Default::default()
        };
        let (scopes, had_explicit) = build_scope_selection(&opts);
        assert!(had_explicit);
        assert!(scopes.contains(&UninstallScope::Service));
        assert!(scopes.contains(&UninstallScope::State));
        assert!(scopes.contains(&UninstallScope::Workspace));
        assert!(scopes.contains(&UninstallScope::App));
    }

    #[test]
    fn build_scope_partial() {
        let opts = UninstallOptions {
            service: true,
            state: true,
            ..Default::default()
        };
        let (scopes, had_explicit) = build_scope_selection(&opts);
        assert!(had_explicit);
        assert!(scopes.contains(&UninstallScope::Service));
        assert!(scopes.contains(&UninstallScope::State));
        assert!(!scopes.contains(&UninstallScope::Workspace));
        assert!(!scopes.contains(&UninstallScope::App));
    }

    #[test]
    fn build_scope_none() {
        let opts = UninstallOptions::default();
        let (scopes, had_explicit) = build_scope_selection(&opts);
        assert!(!had_explicit);
        assert!(scopes.is_empty());
    }

    #[test]
    fn default_options() {
        let opts = UninstallOptions::default();
        assert!(!opts.service);
        assert!(!opts.state);
        assert!(!opts.workspace);
        assert!(!opts.app);
        assert!(!opts.all);
        assert!(!opts.yes);
        assert!(!opts.non_interactive);
        assert!(!opts.dry_run);
    }
}
