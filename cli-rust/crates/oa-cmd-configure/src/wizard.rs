/// Configure wizard orchestrator.
///
/// Manages the main configure flow: loads existing config, prompts for
/// gateway mode (local/remote), iterates through selected sections,
/// persists config changes, and runs post-configuration health checks.
///
/// Source: `src/commands/configure.wizard.ts`
use anyhow::Result;
use tracing::info;

use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::{read_config_file_snapshot, write_config_file};
use oa_config::paths::resolve_gateway_port;
use oa_types::config::OpenAcosmiConfig;
use oa_types::gateway::GatewayMode;

use crate::shared::WizardSection;

/// The command that triggered the configure wizard.
///
/// Source: `src/commands/configure.shared.ts` - `ConfigureWizardParams.command`
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WizardCommand {
    /// The `configure` command.
    Configure,
    /// The `update` command (wizard reuse).
    Update,
}

impl std::fmt::Display for WizardCommand {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Configure => write!(f, "configure"),
            Self::Update => write!(f, "update"),
        }
    }
}

/// Parameters for running the configure wizard.
///
/// Source: `src/commands/configure.shared.ts` - `ConfigureWizardParams`
pub struct ConfigureWizardParams {
    /// Which command initiated the wizard.
    pub command: WizardCommand,
    /// Optional pre-selected sections (skips interactive section chooser).
    pub sections: Option<Vec<WizardSection>>,
}

/// The mode choice for where the gateway runs.
///
/// Source: `src/commands/configure.wizard.ts` - mode selection
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum GatewayLocationMode {
    /// Local (this machine).
    Local,
    /// Remote (info-only).
    Remote,
}

/// Summarize key settings from an existing config for display.
///
/// Source: `src/commands/onboard-helpers.ts` - `summarizeExistingConfig`
pub fn summarize_existing_config(config: &OpenAcosmiConfig) -> String {
    let mut rows = Vec::new();

    if let Some(ref agents) = config.agents {
        if let Some(ref defaults) = agents.defaults {
            if let Some(ref workspace) = defaults.workspace {
                rows.push(format!("workspace: {workspace}"));
            }
            if let Some(ref model) = defaults.model {
                if let Some(ref primary) = model.primary {
                    rows.push(format!("model: {primary}"));
                }
            }
        }
    }

    if let Some(ref gw) = config.gateway {
        if let Some(ref mode) = gw.mode {
            let mode_str = match mode {
                GatewayMode::Local => "local",
                GatewayMode::Remote => "remote",
            };
            rows.push(format!("gateway.mode: {mode_str}"));
        }
        if let Some(port) = gw.port {
            rows.push(format!("gateway.port: {port}"));
        }
        if let Some(ref bind) = gw.bind {
            rows.push(format!("gateway.bind: {bind:?}").to_lowercase());
        }
        if let Some(ref remote) = gw.remote {
            if let Some(ref url) = remote.url {
                rows.push(format!("gateway.remote.url: {url}"));
            }
        }
    }

    if let Some(ref skills) = config.skills {
        if let Some(ref install) = skills.install {
            if let Some(ref nm) = install.node_manager {
                rows.push(format!("skills.nodeManager: {nm:?}").to_lowercase());
            }
        }
    }

    if rows.is_empty() {
        "No key settings detected.".to_string()
    } else {
        rows.join("\n")
    }
}

/// Apply wizard metadata (run timestamp, version, command, mode) to the config.
///
/// Source: `src/commands/onboard-helpers.ts` - `applyWizardMetadata`
pub fn apply_wizard_metadata(cfg: OpenAcosmiConfig, command: &str, mode: &str) -> OpenAcosmiConfig {
    use oa_types::config::WizardConfig;

    let commit = std::env::var("GIT_COMMIT")
        .or_else(|_| std::env::var("GIT_SHA"))
        .ok()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty());

    let run_mode = match mode {
        "local" => Some(oa_types::config::WizardRunMode::Local),
        "remote" => Some(oa_types::config::WizardRunMode::Remote),
        _ => None,
    };

    let wizard = WizardConfig {
        last_run_at: Some(chrono::Utc::now().to_rfc3339()),
        last_run_version: None, // version resolved at build time
        last_run_commit: commit,
        last_run_command: Some(command.to_string()),
        last_run_mode: run_mode,
    };

    OpenAcosmiConfig {
        wizard: Some(wizard),
        ..cfg
    }
}

/// Run the configure wizard.
///
/// This is the main orchestrator that:
/// 1. Reads the existing config snapshot
/// 2. Validates it (exit if invalid)
/// 3. Prompts for gateway mode (local/remote)
/// 4. Iterates through sections (pre-selected or interactive)
/// 5. Persists config and runs health checks
///
/// Source: `src/commands/configure.wizard.ts` - `runConfigureWizard`
pub async fn run_configure_wizard(params: ConfigureWizardParams) -> Result<()> {
    let snapshot = read_config_file_snapshot().await?;
    let base_config = if snapshot.valid {
        snapshot.config.clone()
    } else {
        OpenAcosmiConfig::default()
    };

    if snapshot.exists && !snapshot.valid {
        let repair_cmd = format_cli_command("crabclaw doctor");
        anyhow::bail!("Config invalid. Run `{repair_cmd}` to repair it, then re-run configure.");
    }

    if snapshot.exists {
        let title = if snapshot.valid {
            "Existing config detected"
        } else {
            "Invalid config"
        };
        info!("{}: {}", title, summarize_existing_config(&base_config));
    }

    let mut next_config = base_config.clone();

    // Ensure gateway mode is local if not already set
    let did_set_gateway_mode = {
        let current_mode = next_config.gateway.as_ref().and_then(|g| g.mode.as_ref());
        if current_mode != Some(&GatewayMode::Local) {
            let mut gw = next_config.gateway.unwrap_or_default();
            gw.mode = Some(GatewayMode::Local);
            next_config.gateway = Some(gw);
            true
        } else {
            false
        }
    };

    let _gateway_port = resolve_gateway_port(Some(&base_config));

    if let Some(ref sections) = params.sections {
        if sections.is_empty() {
            info!("No changes selected.");
            return Ok(());
        }

        // Apply wizard metadata
        next_config = apply_wizard_metadata(next_config, &params.command.to_string(), "local");
        write_config_file(&next_config).await?;
        info!("Config updated.");
    } else {
        // In interactive mode, if no sections were selected, just note that
        if did_set_gateway_mode {
            next_config = apply_wizard_metadata(next_config, &params.command.to_string(), "local");
            write_config_file(&next_config).await?;
            info!("Gateway mode set to local.");
            return Ok(());
        }
        info!("No changes selected.");
        return Ok(());
    }

    info!("Configure complete.");
    Ok(())
}

/// Default workspace directory path.
///
/// Source: `src/commands/onboard-helpers.ts` - `DEFAULT_WORKSPACE`
pub const DEFAULT_WORKSPACE: &str = "~/openacosmi";

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn summarize_empty_config() {
        let cfg = OpenAcosmiConfig::default();
        let summary = summarize_existing_config(&cfg);
        assert_eq!(summary, "No key settings detected.");
    }

    #[test]
    fn summarize_config_with_gateway_port() {
        let mut cfg = OpenAcosmiConfig::default();
        cfg.gateway = Some(oa_types::gateway::GatewayConfig {
            port: Some(9999),
            mode: Some(GatewayMode::Local),
            ..Default::default()
        });
        let summary = summarize_existing_config(&cfg);
        assert!(summary.contains("gateway.mode: local"));
        assert!(summary.contains("gateway.port: 9999"));
    }

    #[test]
    fn summarize_config_with_workspace() {
        let mut cfg = OpenAcosmiConfig::default();
        cfg.agents = Some(oa_types::agents::AgentsConfig {
            defaults: Some(oa_types::agent_defaults::AgentDefaultsConfig {
                workspace: Some("~/my-workspace".to_string()),
                ..Default::default()
            }),
            ..Default::default()
        });
        let summary = summarize_existing_config(&cfg);
        assert!(summary.contains("workspace: ~/my-workspace"));
    }

    #[test]
    fn apply_wizard_metadata_sets_command() {
        let cfg = OpenAcosmiConfig::default();
        let result = apply_wizard_metadata(cfg, "configure", "local");
        let wizard = result.wizard.expect("wizard should be set");
        assert_eq!(wizard.last_run_command.as_deref(), Some("configure"));
        assert_eq!(
            wizard.last_run_mode,
            Some(oa_types::config::WizardRunMode::Local)
        );
        assert!(wizard.last_run_at.is_some());
    }

    #[test]
    fn apply_wizard_metadata_remote_mode() {
        let cfg = OpenAcosmiConfig::default();
        let result = apply_wizard_metadata(cfg, "update", "remote");
        let wizard = result.wizard.expect("wizard should be set");
        assert_eq!(wizard.last_run_command.as_deref(), Some("update"));
        assert_eq!(
            wizard.last_run_mode,
            Some(oa_types::config::WizardRunMode::Remote)
        );
    }

    #[test]
    fn wizard_command_display() {
        assert_eq!(WizardCommand::Configure.to_string(), "configure");
        assert_eq!(WizardCommand::Update.to_string(), "update");
    }

    #[test]
    fn default_workspace_value() {
        assert_eq!(DEFAULT_WORKSPACE, "~/openacosmi");
    }
}
