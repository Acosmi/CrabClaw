/// Gateway RPC call abstraction.
///
/// Provides a high-level one-shot `call_gateway()` function that opens a
/// WebSocket connection to the gateway, performs the connect handshake,
/// sends a single RPC request, and returns the response. Intended for
/// CLI commands that need to make a single gateway call.
///
/// Also provides `build_gateway_connection_details()` which resolves the
/// target URL, auth tokens, and TLS settings from config, environment,
/// and CLI overrides.
///
/// Source: `src/gateway/call.ts`
use std::time::Duration;

use anyhow::{Context, Result, bail};
use oa_infra::env::preferred_env_value;
use tokio::sync::mpsc;
// tracing is available for future debug logging.
use uuid::Uuid;

use oa_config::io::load_config;
use oa_config::paths::{resolve_config_path, resolve_gateway_port};
use oa_types::config::OpenAcosmiConfig;
use oa_types::gateway::GatewayMode;

use crate::auth::{
    ExplicitGatewayAuth, ensure_explicit_gateway_auth, resolve_explicit_gateway_auth,
};
use crate::client::{GatewayClientError, GatewayClientOptions, connect_gateway};
use crate::net::pick_primary_lan_ipv4;
use crate::protocol::{EventFrame, GatewayClientId, GatewayClientMode, PROTOCOL_VERSION};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/// Options for a single gateway RPC call.
///
/// Source: `src/gateway/call.ts` (`CallGatewayOptions`)
#[derive(Debug, Clone, Default)]
pub struct CallGatewayOptions {
    /// Gateway WebSocket URL override.
    pub url: Option<String>,
    /// Token credential.
    pub token: Option<String>,
    /// Password credential.
    pub password: Option<String>,
    /// TLS certificate fingerprint for pinning.
    pub tls_fingerprint: Option<String>,
    /// Pre-loaded configuration (avoids re-loading from disk).
    pub config: Option<OpenAcosmiConfig>,
    /// RPC method to invoke.
    pub method: String,
    /// RPC parameters (serialized JSON).
    pub params: Option<serde_json::Value>,
    /// If true, wait for a final (non-ack) response.
    pub expect_final: Option<bool>,
    /// Timeout in milliseconds (default: 10 000).
    pub timeout_ms: Option<u64>,
    /// Client identifier.
    pub client_name: Option<GatewayClientId>,
    /// Human-readable client display name.
    pub client_display_name: Option<String>,
    /// Client software version.
    pub client_version: Option<String>,
    /// Platform string.
    pub platform: Option<String>,
    /// Client operational mode.
    pub mode: Option<GatewayClientMode>,
    /// Instance identifier.
    pub instance_id: Option<String>,
    /// Minimum protocol version.
    pub min_protocol: Option<u32>,
    /// Maximum protocol version.
    pub max_protocol: Option<u32>,
    /// Config path override for error messages.
    pub config_path: Option<String>,
}

/// Details about a resolved gateway connection target.
///
/// Source: `src/gateway/call.ts` (`GatewayConnectionDetails`)
#[derive(Debug, Clone)]
pub struct GatewayConnectionDetails {
    /// WebSocket URL to connect to.
    pub url: String,
    /// Human-readable description of how the URL was resolved.
    pub url_source: String,
    /// Bind mode detail (for local connections).
    pub bind_detail: Option<String>,
    /// Warning when remote mode is misconfigured.
    pub remote_fallback_note: Option<String>,
    /// Multi-line summary message for error output.
    pub message: String,
}

// ---------------------------------------------------------------------------
// Connection details builder
// ---------------------------------------------------------------------------

/// Build gateway connection details from config, environment, and CLI overrides.
///
/// Resolves the WebSocket URL, determines whether the connection is local or
/// remote, and assembles a human-readable summary.
///
/// Source: `src/gateway/call.ts` (`buildGatewayConnectionDetails`)
#[must_use]
pub fn build_gateway_connection_details(
    config: &OpenAcosmiConfig,
    url_override: Option<&str>,
    config_path_override: Option<&str>,
) -> GatewayConnectionDetails {
    let config_path = config_path_override
        .map(String::from)
        .unwrap_or_else(|| resolve_config_path().to_string_lossy().to_string());

    let gw = config.gateway.as_ref();
    let is_remote_mode = gw
        .and_then(|g| g.mode.as_ref())
        .is_some_and(|m| *m == GatewayMode::Remote);
    let remote = if is_remote_mode {
        gw.and_then(|g| g.remote.as_ref())
    } else {
        None
    };

    let tls_enabled = gw
        .and_then(|g| g.tls.as_ref())
        .and_then(|t| t.enabled)
        .unwrap_or(false);

    let local_port = resolve_gateway_port(Some(config));
    let bind_mode = gw
        .and_then(|g| g.bind.as_ref())
        .cloned()
        .unwrap_or(oa_types::gateway::GatewayBindMode::Loopback);

    let prefer_lan = bind_mode == oa_types::gateway::GatewayBindMode::Lan;
    let lan_ipv4 = if prefer_lan {
        pick_primary_lan_ipv4()
    } else {
        None
    };

    let scheme = if tls_enabled { "wss" } else { "ws" };

    let local_url = if prefer_lan {
        if let Some(ref ip) = lan_ipv4 {
            format!("{scheme}://{ip}:{local_port}")
        } else {
            format!("{scheme}://127.0.0.1:{local_port}")
        }
    } else {
        format!("{scheme}://127.0.0.1:{local_port}")
    };

    let url_override_trimmed = url_override
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(String::from);

    let remote_url = remote
        .and_then(|r| r.url.as_deref())
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(String::from);

    let remote_misconfigured =
        is_remote_mode && url_override_trimmed.is_none() && remote_url.is_none();

    let url = url_override_trimmed
        .as_deref()
        .or(remote_url.as_deref())
        .unwrap_or(&local_url)
        .to_string();

    let url_source = if url_override_trimmed.is_some() {
        "cli --url".to_string()
    } else if remote_url.is_some() {
        "config gateway.remote.url".to_string()
    } else if remote_misconfigured {
        "missing gateway.remote.url (fallback local)".to_string()
    } else if prefer_lan {
        if let Some(ref ip) = lan_ipv4 {
            format!("local lan {ip}")
        } else {
            "local loopback".to_string()
        }
    } else {
        "local loopback".to_string()
    };

    let remote_fallback_note = if remote_misconfigured {
        Some(
            "Warn: gateway.mode=remote but gateway.remote.url is missing; \
             set gateway.remote.url or switch gateway.mode=local."
                .to_string(),
        )
    } else {
        None
    };

    let bind_detail = if url_override_trimmed.is_none() && remote_url.is_none() {
        Some(format!("Bind: {bind_mode:?}").to_lowercase())
    } else {
        None
    };

    let message = [
        Some(format!("Gateway target: {url}")),
        Some(format!("Source: {url_source}")),
        Some(format!("Config: {config_path}")),
        bind_detail.clone(),
        remote_fallback_note.clone(),
    ]
    .into_iter()
    .flatten()
    .collect::<Vec<_>>()
    .join("\n");

    GatewayConnectionDetails {
        url,
        url_source,
        bind_detail,
        remote_fallback_note,
        message,
    }
}

// ---------------------------------------------------------------------------
// One-shot call
// ---------------------------------------------------------------------------

/// Make a single RPC call to the gateway.
///
/// Opens a WebSocket connection, performs the connect handshake, sends the
/// request, waits for the response (up to `timeout_ms`), and closes the
/// connection. Returns the response payload deserialized as type `T`.
///
/// Source: `src/gateway/call.ts` (`callGateway`)
pub async fn call_gateway<T: serde::de::DeserializeOwned>(opts: CallGatewayOptions) -> Result<T> {
    let timeout_ms = opts.timeout_ms.unwrap_or(10_000);
    let safe_timeout = Duration::from_millis(timeout_ms.max(1).min(2_147_483_647));

    let config = match opts.config {
        Some(ref c) => c.clone(),
        None => load_config().unwrap_or_default(),
    };

    let gw = config.gateway.as_ref();
    let is_remote_mode = gw
        .and_then(|g| g.mode.as_ref())
        .is_some_and(|m| *m == GatewayMode::Remote);
    let remote = if is_remote_mode {
        gw.and_then(|g| g.remote.as_ref())
    } else {
        None
    };

    let url_override = opts
        .url
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(String::from);

    let explicit_auth = resolve_explicit_gateway_auth(Some(&ExplicitGatewayAuth {
        token: opts.token.clone(),
        password: opts.password.clone(),
    }));

    let config_path = opts
        .config_path
        .clone()
        .unwrap_or_else(|| resolve_config_path().to_string_lossy().to_string());

    ensure_explicit_gateway_auth(
        url_override.as_deref(),
        &explicit_auth,
        "Fix: pass --token or --password (or gatewayToken in tools).",
        Some(&config_path),
    )
    .map_err(|e| anyhow::anyhow!("{e}"))?;

    let remote_url = remote
        .and_then(|r| r.url.as_deref())
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(String::from);

    if is_remote_mode && url_override.is_none() && remote_url.is_none() {
        bail!(
            "gateway remote mode misconfigured: gateway.remote.url missing\n\
             Config: {config_path}\n\
             Fix: set gateway.remote.url, or set gateway.mode=local."
        );
    }

    let connection_details =
        build_gateway_connection_details(&config, url_override.as_deref(), Some(&config_path));

    let target_url = connection_details.url.clone();

    // Resolve auth token from multiple sources.
    let auth_token_config = gw
        .and_then(|g| g.auth.as_ref())
        .and_then(|a| a.token.clone());
    let token = explicit_auth.token.or_else(|| {
        if url_override.is_some() {
            return None;
        }
        if is_remote_mode {
            remote
                .and_then(|r| r.token.as_deref())
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(String::from)
        } else {
            preferred_env_value(&["CRABCLAW_GATEWAY_TOKEN", "OPENACOSMI_GATEWAY_TOKEN"])
                .or_else(|| {
                    std::env::var("CLAWDBOT_GATEWAY_TOKEN")
                        .ok()
                        .map(|s| s.trim().to_string())
                        .filter(|s| !s.is_empty())
                })
                .or_else(|| {
                    auth_token_config
                        .as_deref()
                        .map(str::trim)
                        .filter(|s| !s.is_empty())
                        .map(String::from)
                })
        }
    });

    let auth_password_config = gw
        .and_then(|g| g.auth.as_ref())
        .and_then(|a| a.password.clone());
    let password = explicit_auth.password.or_else(|| {
        if url_override.is_some() {
            return None;
        }
        preferred_env_value(&["CRABCLAW_GATEWAY_PASSWORD", "OPENACOSMI_GATEWAY_PASSWORD"])
            .or_else(|| {
                std::env::var("CLAWDBOT_GATEWAY_PASSWORD")
                    .ok()
                    .map(|s| s.trim().to_string())
                    .filter(|s| !s.is_empty())
            })
            .or_else(|| {
                if is_remote_mode {
                    remote
                        .and_then(|r| r.password.as_deref())
                        .map(str::trim)
                        .filter(|s| !s.is_empty())
                        .map(String::from)
                } else {
                    auth_password_config
                        .as_deref()
                        .map(str::trim)
                        .filter(|s| !s.is_empty())
                        .map(String::from)
                }
            })
    });

    let client_opts = GatewayClientOptions {
        url: Some(target_url),
        token,
        password,
        tls_fingerprint: opts.tls_fingerprint,
        instance_id: opts
            .instance_id
            .or_else(|| Some(Uuid::new_v4().to_string())),
        client_name: Some(opts.client_name.unwrap_or(GatewayClientId::Cli)),
        client_display_name: opts.client_display_name,
        client_version: opts.client_version.or_else(|| Some("dev".to_string())),
        platform: opts.platform,
        mode: Some(opts.mode.unwrap_or(GatewayClientMode::Cli)),
        role: Some("operator".to_string()),
        scopes: Some(vec![
            "operator.admin".to_string(),
            "operator.approvals".to_string(),
            "operator.pairing".to_string(),
        ]),
        caps: Some(vec![]),
        commands: None,
        permissions: None,
        path_env: None,
        min_protocol: opts.min_protocol.or(Some(PROTOCOL_VERSION)),
        max_protocol: opts.max_protocol.or(Some(PROTOCOL_VERSION)),
    };

    let (event_tx, _event_rx) = mpsc::channel::<EventFrame>(64);

    // Connect with timeout.
    let connect_result = tokio::time::timeout(safe_timeout, connect_gateway(client_opts, event_tx))
        .await
        .map_err(|_| {
            anyhow::anyhow!(
                "gateway timeout after {timeout_ms}ms\n{}",
                connection_details.message
            )
        })?
        .map_err(|e| {
            anyhow::anyhow!(
                "gateway connection failed: {e}\n{}",
                connection_details.message
            )
        })?;

    let (client, _hello) = connect_result;

    let expect_final = opts.expect_final.unwrap_or(false);

    // Send the actual RPC request with timeout.
    let result = tokio::time::timeout(
        safe_timeout,
        client.request(&opts.method, opts.params, expect_final),
    )
    .await
    .map_err(|_| {
        anyhow::anyhow!(
            "gateway timeout after {timeout_ms}ms\n{}",
            connection_details.message
        )
    })?
    .map_err(|e| match e {
        GatewayClientError::ConnectionClosed { code, reason } => {
            let hint = crate::protocol::describe_gateway_close_code(code).unwrap_or("");
            let suffix = if hint.is_empty() {
                String::new()
            } else {
                format!(" {hint}")
            };
            let reason_text = if reason.trim().is_empty() {
                "no close reason".to_string()
            } else {
                reason
            };
            anyhow::anyhow!(
                "gateway closed ({code}{suffix}): {reason_text}\n{}",
                connection_details.message
            )
        }
        other => anyhow::anyhow!("{other}"),
    })?;

    // Stop the client.
    client.stop().await;

    // Deserialize the response payload.
    serde_json::from_value::<T>(result).context("failed to deserialize gateway response")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Generate a random idempotency key (UUID v4).
///
/// Source: `src/gateway/call.ts` (`randomIdempotencyKey`)
#[must_use]
pub fn random_idempotency_key() -> String {
    Uuid::new_v4().to_string()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn idempotency_key_is_uuid_format() {
        let key = random_idempotency_key();
        // UUID v4 is 36 chars with hyphens.
        assert_eq!(key.len(), 36);
        assert_eq!(key.chars().filter(|c| *c == '-').count(), 4);
    }

    #[test]
    fn idempotency_keys_are_unique() {
        let a = random_idempotency_key();
        let b = random_idempotency_key();
        assert_ne!(a, b);
    }

    #[test]
    fn connection_details_local_default() {
        let config = OpenAcosmiConfig::default();
        let details = build_gateway_connection_details(&config, None, Some("/tmp/test.json"));
        assert!(details.url.starts_with("ws://127.0.0.1:"));
        assert_eq!(details.url_source, "local loopback");
        assert!(details.message.contains("Gateway target:"));
    }

    #[test]
    fn connection_details_url_override() {
        let config = OpenAcosmiConfig::default();
        let details = build_gateway_connection_details(
            &config,
            Some("wss://remote.example.com:443"),
            Some("/tmp/test.json"),
        );
        assert_eq!(details.url, "wss://remote.example.com:443");
        assert_eq!(details.url_source, "cli --url");
    }

    #[test]
    fn connection_details_remote_mode_misconfigured() {
        let mut config = OpenAcosmiConfig::default();
        config.gateway = Some(oa_types::gateway::GatewayConfig {
            mode: Some(GatewayMode::Remote),
            ..Default::default()
        });
        let details = build_gateway_connection_details(&config, None, Some("/tmp/test.json"));
        assert!(details.remote_fallback_note.is_some());
        assert!(details.url_source.contains("missing"));
    }

    #[test]
    fn call_gateway_options_default() {
        let opts = CallGatewayOptions {
            method: "test".to_string(),
            ..Default::default()
        };
        assert_eq!(opts.method, "test");
        assert!(opts.url.is_none());
        assert!(opts.timeout_ms.is_none());
    }
}
