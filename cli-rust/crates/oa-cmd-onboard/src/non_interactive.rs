/// Non-interactive onboarding flow.
///
/// Dispatches to local or remote non-interactive onboarding based on the
/// `--mode` flag. Validates config state before proceeding.
///
/// Source: `src/commands/onboard-non-interactive.ts`
use anyhow::Result;
use tracing::info;

use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::{read_config_file_snapshot, write_config_file};
use oa_types::config::OpenAcosmiConfig;
use oa_types::gateway::{
    GatewayAuthMode, GatewayBindMode, GatewayMode, GatewayRemoteConfig, GatewayTailscaleConfig,
    GatewayTailscaleMode,
};

use crate::helpers::{DEFAULT_WORKSPACE, random_token, resolve_user_path};
use crate::types::OnboardOptions;

/// Run non-interactive onboarding for a local gateway.
///
/// Applies auth, gateway, and workspace configuration from CLI flags
/// without any user prompts.
///
/// Source: `src/commands/onboard-non-interactive/local.ts` - `runNonInteractiveOnboardingLocal`
async fn run_non_interactive_local(
    opts: &OnboardOptions,
    base_config: OpenAcosmiConfig,
) -> Result<()> {
    let mut cfg = base_config;

    // Set gateway mode to local
    let mut gw = cfg.gateway.unwrap_or_default();
    gw.mode = Some(GatewayMode::Local);

    // Apply port
    if let Some(port) = opts.gateway_port {
        gw.port = Some(port);
    }

    // Apply bind mode
    if let Some(ref bind) = opts.gateway_bind {
        gw.bind = match bind.as_str() {
            "loopback" => Some(GatewayBindMode::Loopback),
            "lan" => Some(GatewayBindMode::Lan),
            "auto" => Some(GatewayBindMode::Auto),
            "custom" => Some(GatewayBindMode::Custom),
            "tailnet" => Some(GatewayBindMode::Tailnet),
            _ => Some(GatewayBindMode::Loopback),
        };
    }

    // Apply gateway auth
    let auth_mode = opts.gateway_auth.as_deref().unwrap_or("token");
    let mut auth = gw.auth.unwrap_or_default();
    match auth_mode {
        "token" => {
            auth.mode = Some(GatewayAuthMode::Token);
            let token = opts.gateway_token.clone().unwrap_or_else(random_token);
            auth.token = Some(token);
        }
        "password" => {
            auth.mode = Some(GatewayAuthMode::Password);
            if let Some(ref pw) = opts.gateway_password {
                auth.password = Some(pw.clone());
            }
        }
        _ => {
            auth.mode = Some(GatewayAuthMode::Token);
            auth.token = Some(random_token());
        }
    }
    gw.auth = Some(auth);

    // Apply tailscale
    if let Some(ref ts_mode) = opts.tailscale {
        let mode = match ts_mode.as_str() {
            "serve" => Some(GatewayTailscaleMode::Serve),
            "funnel" => Some(GatewayTailscaleMode::Funnel),
            _ => Some(GatewayTailscaleMode::Off),
        };
        gw.tailscale = Some(GatewayTailscaleConfig {
            mode,
            reset_on_exit: opts.tailscale_reset_on_exit,
        });
    }

    cfg.gateway = Some(gw);

    // Apply workspace
    let workspace = opts.workspace.as_deref().unwrap_or(DEFAULT_WORKSPACE);
    let resolved_workspace = resolve_user_path(workspace);

    let mut agents = cfg.agents.unwrap_or_default();
    let mut defaults = agents.defaults.unwrap_or_default();
    defaults.workspace = Some(resolved_workspace);
    agents.defaults = Some(defaults);
    cfg.agents = Some(agents);

    // Write config
    write_config_file(&cfg).await?;
    info!("Non-interactive local onboarding complete. Config written.");
    Ok(())
}

/// Run non-interactive onboarding for a remote gateway.
///
/// Configures the remote gateway URL and auth token from CLI flags.
///
/// Source: `src/commands/onboard-non-interactive/remote.ts` - `runNonInteractiveOnboardingRemote`
async fn run_non_interactive_remote(
    opts: &OnboardOptions,
    base_config: OpenAcosmiConfig,
) -> Result<()> {
    let mut cfg = base_config;

    let remote_url = opts.remote_url.as_deref().unwrap_or("ws://127.0.0.1:19001");
    let remote_token = opts.remote_token.clone();

    let mut gw = cfg.gateway.unwrap_or_default();
    gw.mode = Some(GatewayMode::Remote);
    gw.remote = Some(GatewayRemoteConfig {
        url: Some(remote_url.to_string()),
        token: remote_token,
        ..Default::default()
    });
    cfg.gateway = Some(gw);

    write_config_file(&cfg).await?;
    info!("Non-interactive remote onboarding complete. Config written.");
    Ok(())
}

/// Run the non-interactive onboarding flow.
///
/// Validates the existing config, determines mode (local/remote), and
/// dispatches to the appropriate handler.
///
/// Source: `src/commands/onboard-non-interactive.ts` - `runNonInteractiveOnboarding`
pub async fn run_non_interactive_onboarding(opts: &OnboardOptions) -> Result<()> {
    let snapshot = read_config_file_snapshot().await?;

    if snapshot.exists && !snapshot.valid {
        let repair_cmd = format_cli_command("crabclaw doctor");
        anyhow::bail!("Config invalid. Run `{repair_cmd}` to repair it, then re-run onboarding.");
    }

    let base_config = if snapshot.valid {
        snapshot.config
    } else {
        OpenAcosmiConfig::default()
    };

    let mode = opts.mode.as_deref().unwrap_or("local");
    match mode {
        "local" => run_non_interactive_local(opts, base_config).await,
        "remote" => run_non_interactive_remote(opts, base_config).await,
        other => {
            anyhow::bail!("Invalid --mode \"{other}\" (use local|remote).");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn gateway_bind_mode_parsing() {
        let modes = [
            ("loopback", GatewayBindMode::Loopback),
            ("lan", GatewayBindMode::Lan),
            ("auto", GatewayBindMode::Auto),
            ("custom", GatewayBindMode::Custom),
            ("tailnet", GatewayBindMode::Tailnet),
        ];
        for (input, expected) in modes {
            let result = match input {
                "loopback" => GatewayBindMode::Loopback,
                "lan" => GatewayBindMode::Lan,
                "auto" => GatewayBindMode::Auto,
                "custom" => GatewayBindMode::Custom,
                "tailnet" => GatewayBindMode::Tailnet,
                _ => GatewayBindMode::Loopback,
            };
            assert_eq!(result, expected);
        }
    }

    #[test]
    fn gateway_auth_mode_parsing() {
        let modes = [
            ("token", GatewayAuthMode::Token),
            ("password", GatewayAuthMode::Password),
        ];
        for (input, expected) in modes {
            let result = match input {
                "token" => GatewayAuthMode::Token,
                "password" => GatewayAuthMode::Password,
                _ => GatewayAuthMode::Token,
            };
            assert_eq!(result, expected);
        }
    }

    #[test]
    fn tailscale_mode_parsing() {
        let modes = [
            ("off", GatewayTailscaleMode::Off),
            ("serve", GatewayTailscaleMode::Serve),
            ("funnel", GatewayTailscaleMode::Funnel),
        ];
        for (input, expected) in modes {
            let result = match input {
                "serve" => GatewayTailscaleMode::Serve,
                "funnel" => GatewayTailscaleMode::Funnel,
                _ => GatewayTailscaleMode::Off,
            };
            assert_eq!(result, expected);
        }
    }
}
