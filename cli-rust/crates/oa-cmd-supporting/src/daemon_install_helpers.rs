/// Gateway daemon install plan helpers.
///
/// Provides functions to build a gateway install plan (program arguments,
/// working directory, environment), detect dev mode, and generate error hints.
///
/// Source: `src/commands/daemon-install-helpers.ts`
use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use oa_cli_shared::command_format::format_cli_command;
use oa_types::config::OpenAcosmiConfig;

use crate::daemon_runtime::GatewayDaemonRuntime;

/// Gateway install plan describing how to launch the gateway process.
///
/// Source: `src/commands/daemon-install-helpers.ts` - `GatewayInstallPlan`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GatewayInstallPlan {
    /// The program arguments (command + args) to launch the gateway.
    pub program_arguments: Vec<String>,
    /// Optional working directory for the gateway process.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub working_directory: Option<String>,
    /// Environment variables for the gateway process.
    pub environment: HashMap<String, String>,
}

/// Parameters for building a gateway install plan.
///
/// Source: `src/commands/daemon-install-helpers.ts` - `buildGatewayInstallPlan` params
#[derive(Debug, Clone)]
pub struct BuildGatewayInstallPlanParams {
    /// Current environment variables.
    pub env: HashMap<String, String>,
    /// The gateway port to bind on.
    pub port: u16,
    /// The runtime to use (Node or Bun).
    pub runtime: GatewayDaemonRuntime,
    /// Optional authentication token.
    pub token: Option<String>,
    /// Whether running in dev mode (auto-detected if not provided).
    pub dev_mode: Option<bool>,
    /// Optional explicit path to the Node/Bun binary.
    pub node_path: Option<String>,
    /// Full config to extract env vars from.
    pub config: Option<OpenAcosmiConfig>,
}

/// Check whether the current process is running in gateway dev mode.
///
/// Returns `true` if the first argument looks like a TypeScript source file
/// under a `/src/` directory, indicating an in-tree development launch.
///
/// Source: `src/commands/daemon-install-helpers.ts` - `resolveGatewayDevMode`
pub fn resolve_gateway_dev_mode(argv: &[String]) -> bool {
    let entry = argv.get(1);
    let entry = match entry {
        Some(e) => e,
        None => return false,
    };
    let normalized = entry.replace('\\', "/");
    normalized.contains("/src/") && normalized.ends_with(".ts")
}

/// Build a gateway install plan.
///
/// Resolves program arguments, working directory, and environment variables
/// for launching the gateway daemon. This is a simplified version that
/// delegates to the daemon crate for platform-specific details.
///
/// Source: `src/commands/daemon-install-helpers.ts` - `buildGatewayInstallPlan`
pub async fn build_gateway_install_plan(
    params: &BuildGatewayInstallPlanParams,
) -> anyhow::Result<GatewayInstallPlan> {
    let dev_mode = params
        .dev_mode
        .unwrap_or_else(|| resolve_gateway_dev_mode(&[]));

    let node_path = params
        .node_path
        .clone()
        .unwrap_or_else(|| params.runtime.to_string());

    // Build basic program arguments.
    // In production the daemon crate resolves the full gateway entry point.
    // Here we build a minimal representation.
    let mut program_arguments = vec![node_path];
    if dev_mode {
        program_arguments.push("--dev".to_owned());
    }
    program_arguments.push("--port".to_owned());
    program_arguments.push(params.port.to_string());

    // Build environment.
    let mut environment: HashMap<String, String> = HashMap::new();

    // Merge config env vars first (lower priority).
    if let Some(ref cfg) = params.config {
        if let Some(ref env_cfg) = cfg.env {
            if let Some(ref vars) = env_cfg.vars {
                for (key, value) in vars {
                    environment.insert(key.clone(), value.clone());
                }
            }
        }
    }

    // Service-specific vars (higher priority).
    environment.insert("CRABCLAW_GATEWAY_PORT".to_owned(), params.port.to_string());
    environment.insert(
        "OPENACOSMI_GATEWAY_PORT".to_owned(),
        params.port.to_string(),
    );
    if let Some(ref token) = params.token {
        environment.insert("CRABCLAW_GATEWAY_TOKEN".to_owned(), token.clone());
        environment.insert("OPENACOSMI_GATEWAY_TOKEN".to_owned(), token.clone());
    }

    // Merge caller-provided env (but don't override the above).
    for (key, value) in &params.env {
        environment
            .entry(key.clone())
            .or_insert_with(|| value.clone());
    }

    Ok(GatewayInstallPlan {
        program_arguments,
        working_directory: None,
        environment,
    })
}

/// Generate a platform-appropriate error hint for gateway install failures.
///
/// Source: `src/commands/daemon-install-helpers.ts` - `gatewayInstallErrorHint`
pub fn gateway_install_error_hint(platform: &str) -> String {
    if platform == "win32" || platform == "windows" {
        "Tip: rerun from an elevated PowerShell (Start -> type PowerShell -> right-click -> Run as administrator) or skip service install.".to_owned()
    } else {
        let cmd = format_cli_command("crabclaw gateway install");
        format!("Tip: rerun `{cmd}` after fixing the error.")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dev_mode_detected_for_ts_source() {
        let argv = vec![
            "node".to_owned(),
            "/home/user/project/src/index.ts".to_owned(),
        ];
        assert!(resolve_gateway_dev_mode(&argv));
    }

    #[test]
    fn dev_mode_false_for_non_ts() {
        let argv = vec![
            "node".to_owned(),
            "/home/user/project/dist/index.js".to_owned(),
        ];
        assert!(!resolve_gateway_dev_mode(&argv));
    }

    #[test]
    fn dev_mode_false_for_empty_argv() {
        assert!(!resolve_gateway_dev_mode(&[]));
    }

    #[test]
    fn dev_mode_false_for_single_arg() {
        assert!(!resolve_gateway_dev_mode(&["node".to_owned()]));
    }

    #[test]
    fn dev_mode_normalizes_backslashes() {
        let argv = vec![
            "node".to_owned(),
            "C:\\Users\\dev\\project\\src\\main.ts".to_owned(),
        ];
        assert!(resolve_gateway_dev_mode(&argv));
    }

    #[test]
    fn error_hint_unix() {
        let hint = gateway_install_error_hint("linux");
        assert!(hint.contains("gateway install"));
    }

    #[test]
    fn error_hint_windows() {
        let hint = gateway_install_error_hint("win32");
        assert!(hint.contains("PowerShell"));
    }

    #[tokio::test]
    async fn build_plan_sets_port_in_env() {
        let params = BuildGatewayInstallPlanParams {
            env: HashMap::new(),
            port: 8080,
            runtime: GatewayDaemonRuntime::Node,
            token: Some("test-token".to_owned()),
            dev_mode: Some(false),
            node_path: Some("/usr/bin/node".to_owned()),
            config: None,
        };
        let plan = build_gateway_install_plan(&params)
            .await
            .expect("should build plan");
        assert_eq!(
            plan.environment.get("CRABCLAW_GATEWAY_PORT"),
            Some(&"8080".to_owned())
        );
        assert_eq!(
            plan.environment.get("OPENACOSMI_GATEWAY_PORT"),
            Some(&"8080".to_owned())
        );
        assert_eq!(
            plan.environment.get("CRABCLAW_GATEWAY_TOKEN"),
            Some(&"test-token".to_owned())
        );
        assert_eq!(
            plan.environment.get("OPENACOSMI_GATEWAY_TOKEN"),
            Some(&"test-token".to_owned())
        );
        assert!(plan.program_arguments.contains(&"/usr/bin/node".to_owned()));
        assert!(plan.program_arguments.contains(&"8080".to_owned()));
    }

    #[test]
    fn install_plan_serialization() {
        let plan = GatewayInstallPlan {
            program_arguments: vec!["node".to_owned(), "gateway.js".to_owned()],
            working_directory: Some("/opt/openacosmi".to_owned()),
            environment: HashMap::from([("PORT".to_owned(), "8080".to_owned())]),
        };
        let json = serde_json::to_string(&plan).expect("should serialize");
        assert!(json.contains("programArguments"));
        assert!(json.contains("workingDirectory"));
    }
}
