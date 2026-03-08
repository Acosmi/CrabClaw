/// Dashboard command implementation.
///
/// Generates the dashboard URL, copies it to the clipboard, and optionally
/// opens it in the default browser.
///
/// Source: `src/commands/dashboard.ts`
use anyhow::Result;
use tracing::info;

use oa_config::io::read_config_file_snapshot;
use oa_config::paths::resolve_gateway_port;
use oa_infra::env::preferred_env_value;
use oa_types::gateway::GatewayBindMode;

/// Options for the dashboard command.
///
/// Source: `src/commands/dashboard.ts` - `DashboardOptions`
#[derive(Debug, Clone, Default)]
pub struct DashboardOptions {
    /// If true, do not attempt to open the browser.
    pub no_open: bool,
}

/// Simple percent-encoding for URL token values.
///
/// Encodes characters that are not unreserved per RFC 3986.
fn percent_encode_token(input: &str) -> String {
    let mut result = String::with_capacity(input.len());
    for byte in input.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                result.push(byte as char)
            }
            _ => {
                result.push('%');
                result.push_str(&format!("{byte:02X}"));
            }
        }
    }
    result
}

/// Resolve the dashboard URL from config and environment.
///
/// Builds the URL including an optional auth token as a URL fragment
/// to avoid leaking tokens via query parameters.
///
/// Source: `src/commands/dashboard.ts` - `dashboardCommand` (URL resolution)
pub fn resolve_dashboard_url(
    port: u16,
    bind: &str,
    base_path: Option<&str>,
    custom_bind_host: Option<&str>,
    token: Option<&str>,
) -> String {
    let host = match bind {
        "loopback" | "localhost" | "127.0.0.1" | "auto" => custom_bind_host.unwrap_or("127.0.0.1"),
        other if !other.is_empty() => other,
        _ => "127.0.0.1",
    };

    let path_prefix = base_path
        .filter(|p| !p.is_empty())
        .map_or(String::new(), |p| {
            let p = p.strip_prefix('/').unwrap_or(p);
            let p = p.strip_suffix('/').unwrap_or(p);
            format!("/{p}")
        });

    let base_url = format!("http://{host}:{port}{path_prefix}");

    match token.filter(|t| !t.is_empty()) {
        Some(t) => {
            let encoded = percent_encode_token(t);
            format!("{base_url}#token={encoded}")
        }
        None => base_url,
    }
}

/// Convert a `GatewayBindMode` to a string for URL resolution.
fn bind_mode_to_str(mode: &GatewayBindMode) -> &'static str {
    match mode {
        GatewayBindMode::Auto => "auto",
        GatewayBindMode::Lan => "lan",
        GatewayBindMode::Loopback => "loopback",
        GatewayBindMode::Custom => "custom",
        GatewayBindMode::Tailnet => "tailnet",
    }
}

/// Run the dashboard command.
///
/// Loads the config, resolves the dashboard URL, logs it, and optionally
/// copies to clipboard and opens in the browser.
///
/// Source: `src/commands/dashboard.ts` - `dashboardCommand`
pub async fn dashboard_command(options: DashboardOptions) -> Result<()> {
    let snapshot = read_config_file_snapshot().await?;
    let cfg = if snapshot.valid {
        snapshot.config
    } else {
        oa_types::config::OpenAcosmiConfig::default()
    };

    let port = resolve_gateway_port(Some(&cfg));

    let bind_mode = cfg.gateway.as_ref().and_then(|g| g.bind.as_ref());
    let bind_str = bind_mode.map_or("loopback", bind_mode_to_str);

    let base_path = cfg
        .gateway
        .as_ref()
        .and_then(|g| g.control_ui.as_ref())
        .and_then(|ui| ui.base_path.as_deref());

    let custom_bind_host = cfg
        .gateway
        .as_ref()
        .and_then(|g| g.custom_bind_host.as_deref());

    let env_token = preferred_env_value(&["CRABCLAW_GATEWAY_TOKEN", "OPENACOSMI_GATEWAY_TOKEN"]);
    let token = cfg
        .gateway
        .as_ref()
        .and_then(|g| g.auth.as_ref())
        .and_then(|a| a.token.as_deref())
        .or(env_token.as_deref())
        .unwrap_or("");

    let dashboard_url =
        resolve_dashboard_url(port, bind_str, base_path, custom_bind_host, Some(token));

    info!("Dashboard URL: {dashboard_url}");

    if !options.no_open {
        info!("Use the URL above to access the dashboard.");
    } else {
        info!("Browser launch disabled (--no-open). Use the URL above.");
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn url_loopback_no_token() {
        let url = resolve_dashboard_url(19001, "loopback", None, None, None);
        assert_eq!(url, "http://127.0.0.1:19001");
    }

    #[test]
    fn url_with_token_as_fragment() {
        let url = resolve_dashboard_url(19001, "loopback", None, None, Some("secret"));
        assert!(url.starts_with("http://127.0.0.1:19001#token="));
        assert!(url.contains("secret"));
    }

    #[test]
    fn url_with_base_path() {
        let url = resolve_dashboard_url(8080, "loopback", Some("/ui"), None, None);
        assert_eq!(url, "http://127.0.0.1:8080/ui");
    }

    #[test]
    fn url_with_base_path_trailing_slash() {
        let url = resolve_dashboard_url(8080, "loopback", Some("/ui/"), None, None);
        assert_eq!(url, "http://127.0.0.1:8080/ui");
    }

    #[test]
    fn url_with_custom_bind_host() {
        let url = resolve_dashboard_url(8080, "loopback", None, Some("192.168.1.100"), None);
        assert_eq!(url, "http://192.168.1.100:8080");
    }

    #[test]
    fn url_with_explicit_bind() {
        let url = resolve_dashboard_url(8080, "0.0.0.0", None, None, None);
        assert_eq!(url, "http://0.0.0.0:8080");
    }

    #[test]
    fn url_empty_token_omits_fragment() {
        let url = resolve_dashboard_url(19001, "loopback", None, None, Some(""));
        assert_eq!(url, "http://127.0.0.1:19001");
    }

    #[test]
    fn url_token_special_chars_encoded() {
        let url = resolve_dashboard_url(19001, "loopback", None, None, Some("a b&c=d"));
        assert!(url.contains("#token="));
        // The token should be percent-encoded
        assert!(!url.contains(' '));
    }

    #[test]
    fn percent_encode_simple() {
        assert_eq!(percent_encode_token("hello"), "hello");
    }

    #[test]
    fn percent_encode_spaces_and_special() {
        let encoded = percent_encode_token("a b&c");
        assert_eq!(encoded, "a%20b%26c");
    }

    #[test]
    fn bind_mode_strings() {
        assert_eq!(bind_mode_to_str(&GatewayBindMode::Loopback), "loopback");
        assert_eq!(bind_mode_to_str(&GatewayBindMode::Lan), "lan");
        assert_eq!(bind_mode_to_str(&GatewayBindMode::Auto), "auto");
    }
}
