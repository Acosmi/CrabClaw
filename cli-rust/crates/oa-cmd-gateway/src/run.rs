/// Gateway foreground run command.
///
/// Starts the gateway in foreground mode by spawning the Go `acosmi`
/// binary as a child process. Resolves the port from config or CLI
/// arguments, prints a startup banner, and blocks until the child exits
/// or a shutdown signal is received.
///
/// Source: `src/commands/gateway-run.ts`
use std::path::PathBuf;
use std::process::Stdio;

use anyhow::{Context, Result};
use tracing::info;

use oa_config::io::load_config;
use oa_config::paths::resolve_gateway_port;
use oa_infra::env::preferred_env_value;
use oa_terminal::theme::Theme;

/// Default gateway port when no config or CLI override is provided.
const DEFAULT_PORT: u16 = 19001;

/// Name of the Go Gateway binary.
const GATEWAY_BINARY_NAME: &str = "acosmi";

/// Environment variable to override the gateway binary path.
const GATEWAY_BINARY_ENV: &str = "OPENACOSMI_GATEWAY_BINARY";
const PRIMARY_GATEWAY_BINARY_ENV: &str = "CRABCLAW_GATEWAY_BINARY";

/// Resolve the Go Gateway binary path.
///
/// Discovery order:
/// 1. `CRABCLAW_GATEWAY_BINARY` / `OPENACOSMI_GATEWAY_BINARY` environment variable (explicit override)
/// 2. Sibling of the current executable: `<exe_dir>/acosmi`
/// 3. PATH lookup via `which`
fn resolve_gateway_binary() -> Result<PathBuf> {
    // 1. Explicit override.
    if let Some(explicit) = preferred_env_value(&[PRIMARY_GATEWAY_BINARY_ENV, GATEWAY_BINARY_ENV]) {
        let path = PathBuf::from(&explicit);
        if path.exists() {
            info!(path = %path.display(), "gateway binary from env");
            return Ok(path);
        }
        anyhow::bail!(
            "{PRIMARY_GATEWAY_BINARY_ENV} / {GATEWAY_BINARY_ENV} points to \"{explicit}\" but the file does not exist"
        );
    }

    // 2. Sibling of current executable.
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let sibling = dir.join(GATEWAY_BINARY_NAME);
            if sibling.exists() {
                info!(path = %sibling.display(), "gateway binary from sibling");
                return Ok(sibling);
            }
        }
    }

    // 3. PATH lookup.
    if let Ok(output) = std::process::Command::new("which")
        .arg(GATEWAY_BINARY_NAME)
        .output()
    {
        if output.status.success() {
            let path_str = String::from_utf8_lossy(&output.stdout).trim().to_string();
            if !path_str.is_empty() {
                let path = PathBuf::from(&path_str);
                info!(path = %path.display(), "gateway binary from PATH");
                return Ok(path);
            }
        }
    }

    anyhow::bail!(
        "Go Gateway binary \"{GATEWAY_BINARY_NAME}\" not found.\n\
         Searched:\n\
         - ${PRIMARY_GATEWAY_BINARY_ENV} / ${GATEWAY_BINARY_ENV} (not set)\n\
         - sibling of current executable\n\
         - PATH\n\n\
         Build it with: cd backend && go build -o build/{GATEWAY_BINARY_NAME} ./cmd/{GATEWAY_BINARY_NAME}"
    )
}

/// Start the gateway in foreground mode.
///
/// Resolves the listen port from the following precedence:
/// 1. `port` argument (CLI override)
/// 2. `gateway.port` in config
/// 3. `OPENACOSMI_GATEWAY_PORT` environment variable
/// 4. [`DEFAULT_PORT`] (19001)
///
/// Spawns the Go Gateway binary as a child process, forwarding
/// `--port` and optionally `--control-ui-dir` arguments. Inherits
/// stdout/stderr so logs are visible. On Ctrl-C the child process
/// receives SIGINT directly (same process group) and this function
/// waits for it to exit cleanly.
///
/// Source: `src/commands/gateway-run.ts` - `gatewayRunCommand`
pub async fn gateway_run_command(port: Option<u16>, control_ui_dir: Option<&str>) -> Result<()> {
    let config = load_config().unwrap_or_default();

    // Resolve the effective port.
    let effective_port = port.unwrap_or_else(|| {
        let from_config = resolve_gateway_port(Some(&config));
        // If config returned the library default (19001), use our command default instead.
        if from_config == oa_config::paths::DEFAULT_GATEWAY_PORT {
            DEFAULT_PORT
        } else {
            from_config
        }
    });

    // Resolve the Go gateway binary.
    let gateway_bin = resolve_gateway_binary()?;

    // Print startup banner.
    println!(
        "\n{} on port {}",
        Theme::heading("Gateway starting (foreground)"),
        Theme::accent(&effective_port.to_string()),
    );

    println!(
        "  {} {}",
        Theme::muted("Binary:"),
        Theme::muted(&gateway_bin.display().to_string()),
    );

    if let Some(ui_dir) = control_ui_dir {
        println!("  {} {}", Theme::info("Control UI:"), Theme::muted(ui_dir),);
    }

    println!(
        "  {} {}",
        Theme::muted("Press"),
        Theme::info("Ctrl-C to stop"),
    );
    println!();

    info!(
        port = effective_port,
        binary = %gateway_bin.display(),
        control_ui_dir = control_ui_dir,
        "gateway foreground start: spawning Go binary"
    );

    // Build the child process command.
    let mut cmd = tokio::process::Command::new(&gateway_bin);
    cmd.arg("--port").arg(effective_port.to_string());
    if let Some(ui_dir) = control_ui_dir {
        cmd.arg("--control-ui-dir").arg(ui_dir);
    }
    // Inherit stdout/stderr so gateway logs are visible to the user.
    cmd.stdout(Stdio::inherit()).stderr(Stdio::inherit());

    // Spawn the Go gateway.
    let mut child = cmd.spawn().with_context(|| {
        format!(
            "failed to spawn Go Gateway binary: {}",
            gateway_bin.display()
        )
    })?;

    // Wait for either the child to exit or a Ctrl-C signal.
    // On Unix the child is in the same process group, so it also
    // receives SIGINT when the user presses Ctrl-C. We just need
    // to wait for it to finish its graceful shutdown.
    tokio::select! {
        exit_result = child.wait() => {
            let status = exit_result.context("failed to wait for Go Gateway process")?;
            if status.success() {
                println!("\n{}", Theme::muted("Gateway stopped."));
                info!("gateway foreground shutdown (child exited successfully)");
            } else {
                let code = status.code().unwrap_or(-1);
                println!(
                    "\n{}: exit code {}",
                    Theme::error("Gateway exited with error"),
                    code,
                );
                info!(code, "gateway foreground shutdown (child exited with error)");
                anyhow::bail!("Go Gateway process exited with code {code}");
            }
        }
        _ = tokio::signal::ctrl_c() => {
            // The child already received SIGINT (same process group).
            // Give it time to shut down gracefully.
            info!("received Ctrl-C, waiting for Go Gateway to shut down...");
            println!("\n{}", Theme::muted("Shutting down..."));
            match child.wait().await {
                Ok(status) => {
                    let code = status.code().unwrap_or(0);
                    info!(code, "gateway shutdown after Ctrl-C");
                    println!("{}", Theme::muted("Gateway stopped."));
                }
                Err(e) => {
                    info!(error = %e, "error waiting for gateway after Ctrl-C");
                    println!("{}: {e}", Theme::error("Error during shutdown"));
                }
            }
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_port_value() {
        assert_eq!(DEFAULT_PORT, 19001);
    }

    #[test]
    fn gateway_binary_name_is_acosmi() {
        assert_eq!(GATEWAY_BINARY_NAME, "acosmi");
    }

    #[test]
    fn gateway_binary_env_var_name() {
        assert_eq!(GATEWAY_BINARY_ENV, "OPENACOSMI_GATEWAY_BINARY");
    }

    #[test]
    fn primary_gateway_binary_env_var_name() {
        assert_eq!(PRIMARY_GATEWAY_BINARY_ENV, "CRABCLAW_GATEWAY_BINARY");
    }
}
