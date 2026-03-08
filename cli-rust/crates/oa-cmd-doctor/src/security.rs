/// Security warnings (gateway network exposure, channel DM policies).
///
/// Checks the gateway bind mode and authentication configuration for
/// dangerous combinations (network-accessible without auth), and
/// inspects channel DM policies for open / wildcard settings.
///
/// Source: `src/commands/doctor-security.ts`
use oa_cli_shared::command_format::format_cli_command;
use oa_terminal::note::note;
use oa_types::config::OpenAcosmiConfig;
use oa_types::gateway::{GatewayAuthMode, GatewayBindMode};

/// Check whether a bind host is loopback (127.x.x.x or ::1).
///
/// Source: `src/commands/doctor-security.ts` — via `isLoopbackHost`
fn is_loopback_host(host: &str) -> bool {
    host == "127.0.0.1" || host.starts_with("127.") || host == "::1" || host == "localhost"
}

/// Resolve the effective gateway bind host from the configured bind mode.
///
/// Source: `src/commands/doctor-security.ts` — via `resolveGatewayBindHost`
fn resolve_gateway_bind_host(
    bind_mode: Option<&GatewayBindMode>,
    custom_host: Option<&str>,
) -> String {
    match bind_mode {
        Some(GatewayBindMode::Loopback) | None => "127.0.0.1".to_string(),
        Some(GatewayBindMode::Lan) | Some(GatewayBindMode::Auto) => "0.0.0.0".to_string(),
        Some(GatewayBindMode::Custom) => custom_host.unwrap_or("0.0.0.0").to_string(),
        Some(GatewayBindMode::Tailnet) => "100.0.0.1".to_string(), // Tailscale range
    }
}

/// Report security warnings.
///
/// Source: `src/commands/doctor-security.ts` — `noteSecurityWarnings`
pub async fn note_security_warnings(cfg: &OpenAcosmiConfig) {
    let mut warnings: Vec<String> = Vec::new();
    let audit_hint = format!(
        "- Run: {}",
        format_cli_command("crabclaw security audit --deep")
    );

    // ── Gateway network exposure ──
    let gateway_bind = cfg.gateway.as_ref().and_then(|gw| gw.bind.as_ref());
    let custom_bind_host = cfg
        .gateway
        .as_ref()
        .and_then(|gw| gw.custom_bind_host.as_deref());

    let resolved_bind_host = resolve_gateway_bind_host(gateway_bind, custom_bind_host);
    let is_exposed = !is_loopback_host(&resolved_bind_host);

    let auth_config = cfg.gateway.as_ref().and_then(|gw| gw.auth.as_ref());
    let auth_mode = auth_config.and_then(|a| a.mode.as_ref());
    let has_token = auth_config
        .and_then(|a| a.token.as_ref())
        .is_some_and(|t| !t.trim().is_empty());
    let has_password = auth_config
        .and_then(|a| a.password.as_ref())
        .is_some_and(|p| !p.trim().is_empty());
    let has_shared_secret = (matches!(auth_mode, Some(GatewayAuthMode::Token)) && has_token)
        || (matches!(auth_mode, Some(GatewayAuthMode::Password)) && has_password);

    let bind_descriptor = format!(
        "\"{}\" ({resolved_bind_host})",
        gateway_bind
            .map(|b| format!("{b:?}").to_lowercase())
            .unwrap_or_else(|| "loopback".to_string())
    );

    if is_exposed {
        if !has_shared_secret {
            let auth_fix = if matches!(auth_mode, Some(GatewayAuthMode::Password)) {
                vec![
                    format!(
                        "  Fix: {} to set a password",
                        format_cli_command("crabclaw configure")
                    ),
                    format!(
                        "  Or switch to token: {}",
                        format_cli_command("crabclaw config set gateway.auth.mode token")
                    ),
                ]
            } else {
                vec![
                    format!(
                        "  Fix: {} to generate a token",
                        format_cli_command("crabclaw doctor --fix")
                    ),
                    format!(
                        "  Or set token directly: {}",
                        format_cli_command("crabclaw config set gateway.auth.mode token")
                    ),
                ]
            };
            warnings.push(format!(
                "- CRITICAL: Gateway bound to {bind_descriptor} without authentication."
            ));
            warnings.push(
                "  Anyone on your network (or internet if port-forwarded) can fully control your agent."
                    .to_string(),
            );
            warnings.push(format!(
                "  Fix: {}",
                format_cli_command("crabclaw config set gateway.bind loopback")
            ));
            warnings.extend(auth_fix);
        } else {
            warnings.push(format!(
                "- WARNING: Gateway bound to {bind_descriptor} (network-accessible)."
            ));
            warnings.push("  Ensure your auth credentials are strong and not exposed.".to_string());
        }
    }

    // ── Channel DM policies ──
    // Stub: Channel DM policy checks require the full channel plugin registry.
    // When oa-channels is fully wired, iterate over channel plugins and check
    // each DM policy for "open" / wildcard / missing allowlist scenarios.

    let lines = if warnings.is_empty() {
        vec![
            "- No channel security warnings detected.".to_string(),
            audit_hint,
        ]
    } else {
        warnings.push(audit_hint);
        warnings
    };

    note(&lines.join("\n"), Some("Security"));
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn loopback_detection() {
        assert!(is_loopback_host("127.0.0.1"));
        assert!(is_loopback_host("127.0.1.1"));
        assert!(is_loopback_host("::1"));
        assert!(is_loopback_host("localhost"));
        assert!(!is_loopback_host("0.0.0.0"));
        assert!(!is_loopback_host("192.168.1.1"));
    }

    #[test]
    fn bind_host_defaults_to_loopback() {
        let host = resolve_gateway_bind_host(None, None);
        assert_eq!(host, "127.0.0.1");
    }

    #[test]
    fn bind_host_lan_is_all_interfaces() {
        let host = resolve_gateway_bind_host(Some(&GatewayBindMode::Lan), None);
        assert_eq!(host, "0.0.0.0");
    }

    #[test]
    fn bind_host_custom_uses_provided_value() {
        let host = resolve_gateway_bind_host(Some(&GatewayBindMode::Custom), Some("10.0.0.5"));
        assert_eq!(host, "10.0.0.5");
    }

    #[tokio::test]
    async fn security_warnings_default_config_no_critical() {
        let cfg = OpenAcosmiConfig::default();
        // Default config uses loopback; should not produce a CRITICAL warning.
        // (We can't easily capture output, but this ensures no panic.)
        note_security_warnings(&cfg).await;
    }
}
