/// System presence — use local infra to detect running gateway.
use anyhow::Result;

use oa_config::paths::resolve_state_dir;

fn resolve_gateway_port_from_lookup<F>(env: F) -> u16
where
    F: Fn(&str) -> Option<String>,
{
    ["CRABCLAW_GATEWAY_PORT", "OPENACOSMI_GATEWAY_PORT"]
        .into_iter()
        .find_map(|key| {
            env(key)
                .map(|value| value.trim().to_string())
                .filter(|value| !value.is_empty())
        })
        .unwrap_or_else(|| "18001".to_string())
        .parse()
        .unwrap_or(18001)
}

fn resolve_gateway_port_from_env() -> u16 {
    resolve_gateway_port_from_lookup(|key| std::env::var(key).ok())
}

/// Check whether a gateway instance is running on this machine.
pub fn system_presence_command(json: bool) -> Result<()> {
    let state_dir = resolve_state_dir();
    let gateway_port = resolve_gateway_port_from_env();

    // Check for lock file
    let lock_path = state_dir.join("gateway.lock");
    let lock_exists = lock_path.exists();

    // Try TCP probe
    let addr = format!("127.0.0.1:{gateway_port}");
    let running = std::net::TcpStream::connect_timeout(
        &addr.parse().unwrap(),
        std::time::Duration::from_millis(500),
    )
    .is_ok();

    if json {
        let payload = serde_json::json!({
            "gatewayRunning": running,
            "gatewayPort": gateway_port,
            "lockFileExists": lock_exists,
            "stateDir": state_dir.to_string_lossy(),
        });
        println!("{}", serde_json::to_string_pretty(&payload)?);
        return Ok(());
    }

    if running {
        println!("✅ Gateway running on port {gateway_port}");
    } else {
        println!("⚠️  Gateway not running (port {gateway_port})");
    }
    if lock_exists {
        println!("   Lock file: {}", lock_path.display());
    }
    println!("   State dir: {}", state_dir.display());

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn resolve_gateway_port_from_env_prefers_crabclaw() {
        let env = HashMap::from([
            ("OPENACOSMI_GATEWAY_PORT", "18081"),
            ("CRABCLAW_GATEWAY_PORT", "18091"),
        ]);
        let resolved =
            resolve_gateway_port_from_lookup(|key| env.get(key).map(|value| (*value).to_string()));
        assert_eq!(resolved, 18091);
    }
}
