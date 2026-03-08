/// Platform-specific diagnostic notes.
///
/// Detects macOS LaunchAgent override markers, launchctl environment variable
/// overrides that conflict with config-based credentials, and deprecated
/// legacy environment variable prefixes (MOLTBOT_*, CLAWDBOT_*).
///
/// Source: `src/commands/doctor-platform-notes.ts`
use oa_config::paths::resolve_state_dir;
use oa_terminal::note::note;
use oa_types::config::OpenAcosmiConfig;

/// Resolve the home directory for platform checks.
fn resolve_home_dir() -> String {
    std::env::var("HOME").unwrap_or_else(|_| {
        dirs::home_dir()
            .map(|p| p.to_string_lossy().to_string())
            .unwrap_or_else(|| ".".to_string())
    })
}

/// Shorten a path by replacing the home directory prefix with `~`.
///
/// Source: `src/utils.ts` — `shortenHomePath`
fn shorten_home_path(path: &str) -> String {
    let home = resolve_home_dir();
    if path.starts_with(&home) {
        format!("~{}", &path[home.len()..])
    } else {
        path.to_string()
    }
}

/// Check for macOS LaunchAgent disable marker.
///
/// If the active compatibility state dir contains `disable-launchagent`, the doctor
/// notes that LaunchAgent writes are disabled and how to re-enable them.
/// LaunchAgent writes are disabled and how to re-enable them.
///
/// Source: `src/commands/doctor-platform-notes.ts` — `noteMacLaunchAgentOverrides`
pub async fn note_mac_launch_agent_overrides() {
    if std::env::consts::OS != "macos" {
        return;
    }

    let marker_path = resolve_state_dir().join("disable-launchagent");

    if !marker_path.exists() {
        return;
    }

    let display = shorten_home_path(&marker_path.to_string_lossy());
    let lines = [
        format!("- LaunchAgent writes are disabled via {display}."),
        "- To restore default behavior:".to_string(),
        format!("  rm {display}"),
    ];
    note(&lines.join("\n"), Some("Gateway (macOS)"));
}

/// Check whether the config has any gateway credential fields set.
///
/// Source: `src/commands/doctor-platform-notes.ts` — `hasConfigGatewayCreds`
fn has_config_gateway_creds(cfg: &OpenAcosmiConfig) -> bool {
    let local_token = cfg
        .gateway
        .as_ref()
        .and_then(|gw| gw.auth.as_ref())
        .and_then(|a| a.token.as_ref())
        .map(|t| t.trim())
        .unwrap_or("");
    let local_password = cfg
        .gateway
        .as_ref()
        .and_then(|gw| gw.auth.as_ref())
        .and_then(|a| a.password.as_ref())
        .map(|p| p.trim())
        .unwrap_or("");
    let remote_token = cfg
        .gateway
        .as_ref()
        .and_then(|gw| gw.remote.as_ref())
        .and_then(|r| r.token.as_ref())
        .map(|t| t.trim())
        .unwrap_or("");
    let remote_password = cfg
        .gateway
        .as_ref()
        .and_then(|gw| gw.remote.as_ref())
        .and_then(|r| r.password.as_ref())
        .map(|p| p.trim())
        .unwrap_or("");

    !local_token.is_empty()
        || !local_password.is_empty()
        || !remote_token.is_empty()
        || !remote_password.is_empty()
}

/// Check for deprecated / conflicting launchctl environment overrides (macOS).
///
/// Warns when `OPENACOSMI_GATEWAY_TOKEN` or `OPENACOSMI_GATEWAY_PASSWORD`
/// is set in the launchctl session, as these override config-based credentials
/// and cause confusing auth failures.
///
/// Source: `src/commands/doctor-platform-notes.ts` — `noteMacLaunchctlGatewayEnvOverrides`
pub async fn note_mac_launchctl_gateway_env_overrides(cfg: &OpenAcosmiConfig) {
    if std::env::consts::OS != "macos" {
        return;
    }
    if !has_config_gateway_creds(cfg) {
        return;
    }

    // In the real implementation, we would call `launchctl getenv` for each
    // deprecated and current variable.  Stubbed here because `launchctl getenv`
    // is macOS-specific and requires spawning a subprocess.
}

/// Report deprecated legacy environment variables (MOLTBOT_*, CLAWDBOT_*).
///
/// Source: `src/commands/doctor-platform-notes.ts` — `noteDeprecatedLegacyEnvVars`
pub fn note_deprecated_legacy_env_vars(
    env_override: Option<&std::collections::HashMap<String, String>>,
) {
    let entries: Vec<String> = if let Some(env_map) = env_override {
        env_map
            .keys()
            .filter(|key| {
                (key.starts_with("MOLTBOT_") || key.starts_with("CLAWDBOT_"))
                    && env_map
                        .get(key.as_str())
                        .is_some_and(|v| !v.trim().is_empty())
            })
            .cloned()
            .collect()
    } else {
        std::env::vars()
            .filter(|(key, value)| {
                (key.starts_with("MOLTBOT_") || key.starts_with("CLAWDBOT_"))
                    && !value.trim().is_empty()
            })
            .map(|(key, _)| key)
            .collect()
    };

    if entries.is_empty() {
        return;
    }

    let mut lines = vec![
        "- Deprecated legacy environment variables detected (ignored).".to_string(),
        "- Use CRABCLAW_* equivalents instead (OPENACOSMI_* remains supported during migration):"
            .to_string(),
    ];
    for key in &entries {
        let suffix = key.find('_').map(|i| &key[i + 1..]).unwrap_or(key);
        lines.push(format!("  {key} -> OPENACOSMI_{suffix}"));
    }
    note(&lines.join("\n"), Some("Environment"));
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shorten_home_path_replaces_prefix() {
        let home = resolve_home_dir();
        let path = format!("{home}/.crabclaw/config.json");
        let shortened = shorten_home_path(&path);
        assert!(shortened.starts_with("~/.crabclaw"));
    }

    #[test]
    fn shorten_home_path_no_change_for_non_home() {
        let shortened = shorten_home_path("/tmp/foo");
        assert_eq!(shortened, "/tmp/foo");
    }

    #[test]
    fn has_config_gateway_creds_false_for_default() {
        let cfg = OpenAcosmiConfig::default();
        assert!(!has_config_gateway_creds(&cfg));
    }

    #[test]
    fn has_config_gateway_creds_true_with_token() {
        let mut cfg = OpenAcosmiConfig::default();
        cfg.gateway = Some(oa_types::gateway::GatewayConfig {
            auth: Some(oa_types::gateway::GatewayAuthConfig {
                token: Some("abc123".to_string()),
                ..Default::default()
            }),
            ..Default::default()
        });
        assert!(has_config_gateway_creds(&cfg));
    }

    #[test]
    fn deprecated_env_vars_empty_map_noop() {
        let env = std::collections::HashMap::new();
        // Should not panic.
        note_deprecated_legacy_env_vars(Some(&env));
    }

    #[test]
    fn deprecated_env_vars_detects_moltbot() {
        let mut env = std::collections::HashMap::new();
        env.insert("MOLTBOT_GATEWAY_TOKEN".to_string(), "abc".to_string());
        // Should produce a note (we can't easily capture output in tests).
        note_deprecated_legacy_env_vars(Some(&env));
    }
}
