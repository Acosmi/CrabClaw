/// Gateway probe authentication and self-presence resolution.
///
/// Source: `src/commands/status.gateway-probe.ts`
use oa_types::config::OpenAcosmiConfig;

use crate::format::GatewayProbeAuth;

fn preferred_gateway_auth_env<F>(getenv: F, keys: &[&str]) -> Option<String>
where
    F: Fn(&str) -> Option<String>,
{
    keys.iter().find_map(|key| {
        getenv(key)
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())
    })
}

/// Resolve authentication credentials for probing the gateway.
///
/// In remote mode, uses `gateway.remote.token` / `gateway.remote.password`.
/// In local mode, uses environment variables first, then config `gateway.auth`.
///
/// Source: `src/commands/status.gateway-probe.ts` - `resolveGatewayProbeAuth`
#[must_use]
pub fn resolve_gateway_probe_auth(cfg: &OpenAcosmiConfig) -> GatewayProbeAuth {
    let gw = cfg.gateway.as_ref();
    let is_remote = gw
        .and_then(|g| g.mode.as_ref())
        .is_some_and(|m| *m == oa_types::gateway::GatewayMode::Remote);

    let remote = if is_remote {
        gw.and_then(|g| g.remote.as_ref())
    } else {
        None
    };

    let auth_token = gw
        .and_then(|g| g.auth.as_ref())
        .and_then(|a| a.token.as_deref())
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(String::from);

    let auth_password = gw
        .and_then(|g| g.auth.as_ref())
        .and_then(|a| a.password.as_deref())
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(String::from);

    let token = if is_remote {
        remote
            .and_then(|r| r.token.as_deref())
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(String::from)
    } else {
        preferred_gateway_auth_env(
            |key| std::env::var(key).ok(),
            &["CRABCLAW_GATEWAY_TOKEN", "OPENACOSMI_GATEWAY_TOKEN"],
        )
        .or(auth_token)
    };

    let env_password = preferred_gateway_auth_env(
        |key| std::env::var(key).ok(),
        &["CRABCLAW_GATEWAY_PASSWORD", "OPENACOSMI_GATEWAY_PASSWORD"],
    );

    let password = env_password.or_else(|| {
        if is_remote {
            remote
                .and_then(|r| r.password.as_deref())
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(String::from)
        } else {
            auth_password
        }
    });

    GatewayProbeAuth { token, password }
}

/// Extract "self" presence entry from a gateway presence array.
///
/// Looks for an entry with `mode == "gateway"` and `reason == "self"`.
///
/// Source: `src/commands/status.gateway-probe.ts` - `pickGatewaySelfPresence`
#[must_use]
pub fn pick_gateway_self_presence(
    presence: Option<&serde_json::Value>,
) -> Option<GatewaySelfPresence> {
    let arr = presence?.as_array()?;
    let entry = arr.iter().find(|e| {
        let obj = e.as_object();
        obj.is_some_and(|o| {
            o.get("mode")
                .and_then(|v| v.as_str())
                .is_some_and(|m| m == "gateway")
                && o.get("reason")
                    .and_then(|v| v.as_str())
                    .is_some_and(|r| r == "self")
        })
    })?;
    let obj = entry.as_object()?;
    Some(GatewaySelfPresence {
        host: obj.get("host").and_then(|v| v.as_str()).map(String::from),
        ip: obj.get("ip").and_then(|v| v.as_str()).map(String::from),
        version: obj
            .get("version")
            .and_then(|v| v.as_str())
            .map(String::from),
        platform: obj
            .get("platform")
            .and_then(|v| v.as_str())
            .map(String::from),
    })
}

/// Self-identification of a gateway instance.
///
/// Source: `src/commands/status.gateway-probe.ts` - `pickGatewaySelfPresence` return
#[derive(Debug, Clone, Default, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GatewaySelfPresence {
    /// Hostname.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub host: Option<String>,
    /// IP address.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ip: Option<String>,
    /// Application version.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
    /// Platform identifier.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub platform: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn resolve_auth_default_config() {
        let cfg = OpenAcosmiConfig::default();
        let auth = resolve_gateway_probe_auth(&cfg);
        assert!(auth.token.is_none());
        assert!(auth.password.is_none());
    }

    #[test]
    fn resolve_auth_remote_mode() {
        let cfg = OpenAcosmiConfig {
            gateway: Some(oa_types::gateway::GatewayConfig {
                mode: Some(oa_types::gateway::GatewayMode::Remote),
                remote: Some(oa_types::gateway::GatewayRemoteConfig {
                    token: Some("remote-token".to_string()),
                    password: Some("remote-pass".to_string()),
                    ..Default::default()
                }),
                ..Default::default()
            }),
            ..Default::default()
        };
        let auth = resolve_gateway_probe_auth(&cfg);
        assert_eq!(auth.token.as_deref(), Some("remote-token"));
        assert_eq!(auth.password.as_deref(), Some("remote-pass"));
    }

    #[test]
    fn resolve_auth_local_mode_with_config() {
        let cfg = OpenAcosmiConfig {
            gateway: Some(oa_types::gateway::GatewayConfig {
                mode: Some(oa_types::gateway::GatewayMode::Local),
                auth: Some(oa_types::gateway::GatewayAuthConfig {
                    token: Some("local-token".to_string()),
                    password: Some("local-pass".to_string()),
                    ..Default::default()
                }),
                ..Default::default()
            }),
            ..Default::default()
        };
        let auth = resolve_gateway_probe_auth(&cfg);
        // Environment variables take priority, but in test they're not set,
        // so config values should be used.
        assert!(auth.token.is_some());
        assert!(auth.password.is_some());
    }

    #[test]
    fn resolve_auth_local_mode_prefers_crabclaw_env() {
        let env = HashMap::from([
            ("OPENACOSMI_GATEWAY_TOKEN", "old-token"),
            ("CRABCLAW_GATEWAY_TOKEN", "new-token"),
            ("OPENACOSMI_GATEWAY_PASSWORD", "old-pass"),
            ("CRABCLAW_GATEWAY_PASSWORD", "new-pass"),
        ]);
        let token = preferred_gateway_auth_env(
            |key| env.get(key).map(|value| (*value).to_string()),
            &["CRABCLAW_GATEWAY_TOKEN", "OPENACOSMI_GATEWAY_TOKEN"],
        );
        let password = preferred_gateway_auth_env(
            |key| env.get(key).map(|value| (*value).to_string()),
            &["CRABCLAW_GATEWAY_PASSWORD", "OPENACOSMI_GATEWAY_PASSWORD"],
        );
        assert_eq!(token.as_deref(), Some("new-token"));
        assert_eq!(password.as_deref(), Some("new-pass"));
    }

    #[test]
    fn pick_self_presence_from_array() {
        let val = serde_json::json!([
            {"mode": "client", "reason": "connect"},
            {"mode": "gateway", "reason": "self", "host": "my-host", "ip": "10.0.0.1", "version": "1.2.3", "platform": "linux"}
        ]);
        let result = pick_gateway_self_presence(Some(&val));
        assert!(result.is_some());
        let self_pres = result.unwrap_or_default();
        assert_eq!(self_pres.host.as_deref(), Some("my-host"));
        assert_eq!(self_pres.ip.as_deref(), Some("10.0.0.1"));
        assert_eq!(self_pres.version.as_deref(), Some("1.2.3"));
    }

    #[test]
    fn pick_self_presence_not_found() {
        let val = serde_json::json!([
            {"mode": "client", "reason": "connect"}
        ]);
        let result = pick_gateway_self_presence(Some(&val));
        assert!(result.is_none());
    }

    #[test]
    fn pick_self_presence_null() {
        assert!(pick_gateway_self_presence(None).is_none());
    }

    #[test]
    fn gateway_self_presence_serializes() {
        let p = GatewaySelfPresence {
            host: Some("h".to_string()),
            ip: None,
            version: Some("v1".to_string()),
            platform: None,
        };
        let json = serde_json::to_value(&p).unwrap_or_default();
        assert_eq!(json["host"], "h");
        assert_eq!(json["version"], "v1");
        assert!(json.get("ip").is_none());
    }
}
