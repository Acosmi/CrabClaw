/// Gateway daemon runtime type and options.
///
/// Defines the supported daemon runtime environments (Node, Bun)
/// and their display metadata.
///
/// Source: `src/commands/daemon-runtime.ts`
use serde::{Deserialize, Serialize};

/// Gateway daemon runtime environment.
///
/// Source: `src/commands/daemon-runtime.ts` - `GatewayDaemonRuntime`
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum GatewayDaemonRuntime {
    /// Node.js runtime (recommended).
    Node,
    /// Bun runtime.
    Bun,
}

impl std::fmt::Display for GatewayDaemonRuntime {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Node => write!(f, "node"),
            Self::Bun => write!(f, "bun"),
        }
    }
}

/// Default gateway daemon runtime.
///
/// Source: `src/commands/daemon-runtime.ts` - `DEFAULT_GATEWAY_DAEMON_RUNTIME`
pub const DEFAULT_GATEWAY_DAEMON_RUNTIME: GatewayDaemonRuntime = GatewayDaemonRuntime::Node;

/// Runtime option for display in interactive prompts.
///
/// Source: `src/commands/daemon-runtime.ts` - `GATEWAY_DAEMON_RUNTIME_OPTIONS`
#[derive(Debug, Clone)]
pub struct RuntimeOption {
    /// The runtime value.
    pub value: GatewayDaemonRuntime,
    /// Display label.
    pub label: &'static str,
    /// Optional hint text.
    pub hint: Option<&'static str>,
}

/// Available gateway daemon runtime options.
///
/// Source: `src/commands/daemon-runtime.ts` - `GATEWAY_DAEMON_RUNTIME_OPTIONS`
pub const GATEWAY_DAEMON_RUNTIME_OPTIONS: &[RuntimeOption] = &[RuntimeOption {
    value: GatewayDaemonRuntime::Node,
    label: "Node (recommended)",
    hint: Some("Required for WhatsApp + Telegram. Bun can corrupt memory on reconnect."),
}];

/// Check whether a string is a valid gateway daemon runtime.
///
/// Source: `src/commands/daemon-runtime.ts` - `isGatewayDaemonRuntime`
pub fn is_gateway_daemon_runtime(value: Option<&str>) -> bool {
    matches!(value.map(str::trim).unwrap_or(""), "node" | "bun")
}

/// Parse a string into a `GatewayDaemonRuntime`.
///
/// Source: `src/commands/daemon-runtime.ts` - `isGatewayDaemonRuntime`
pub fn parse_gateway_daemon_runtime(value: &str) -> Option<GatewayDaemonRuntime> {
    match value.trim().to_lowercase().as_str() {
        "node" => Some(GatewayDaemonRuntime::Node),
        "bun" => Some(GatewayDaemonRuntime::Bun),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_runtime_is_node() {
        assert_eq!(DEFAULT_GATEWAY_DAEMON_RUNTIME, GatewayDaemonRuntime::Node);
    }

    #[test]
    fn is_valid_runtime() {
        assert!(is_gateway_daemon_runtime(Some("node")));
        assert!(is_gateway_daemon_runtime(Some("bun")));
        assert!(!is_gateway_daemon_runtime(Some("deno")));
        assert!(!is_gateway_daemon_runtime(None));
    }

    #[test]
    fn parse_runtime_valid() {
        assert_eq!(
            parse_gateway_daemon_runtime("node"),
            Some(GatewayDaemonRuntime::Node)
        );
        assert_eq!(
            parse_gateway_daemon_runtime("BUN"),
            Some(GatewayDaemonRuntime::Bun)
        );
    }

    #[test]
    fn parse_runtime_invalid() {
        assert!(parse_gateway_daemon_runtime("deno").is_none());
        assert!(parse_gateway_daemon_runtime("").is_none());
    }

    #[test]
    fn runtime_display() {
        assert_eq!(GatewayDaemonRuntime::Node.to_string(), "node");
        assert_eq!(GatewayDaemonRuntime::Bun.to_string(), "bun");
    }

    #[test]
    fn runtime_options_non_empty() {
        assert!(!GATEWAY_DAEMON_RUNTIME_OPTIONS.is_empty());
        assert_eq!(
            GATEWAY_DAEMON_RUNTIME_OPTIONS[0].value,
            GatewayDaemonRuntime::Node
        );
    }
}
