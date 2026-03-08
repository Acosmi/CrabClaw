/// Gateway daemon runtime summary and hint formatting.
///
/// Formats the gateway service runtime status into human-readable summaries
/// and actionable repair hints (LaunchAgent, systemd, Windows task).
///
/// Source: `src/commands/doctor-format.ts`
use oa_cli_shared::command_format::format_cli_command;

/// Runtime information for the gateway service.
///
/// Source: `src/commands/doctor-format.ts` — `GatewayServiceRuntime`
#[derive(Debug, Clone, Default)]
pub struct GatewayServiceRuntime {
    /// Overall status: "running", "stopped", "unknown", etc.
    pub status: Option<String>,
    /// Process ID if running.
    pub pid: Option<u32>,
    /// Service state (e.g. "active", "inactive").
    pub state: Option<String>,
    /// Sub-state from systemd (e.g. "running", "dead").
    pub sub_state: Option<String>,
    /// Last exit status code.
    pub last_exit_status: Option<i32>,
    /// Human-readable exit reason.
    pub last_exit_reason: Option<String>,
    /// Last run result string.
    pub last_run_result: Option<String>,
    /// Last run timestamp.
    pub last_run_time: Option<String>,
    /// Extra detail string.
    pub detail: Option<String>,
    /// Cached LaunchAgent label (macOS).
    pub cached_label: Option<String>,
    /// True if the systemd unit file is missing.
    pub missing_unit: bool,
}

/// Format a compact one-line gateway runtime summary.
///
/// Source: `src/commands/doctor-format.ts` — `formatGatewayRuntimeSummary`
pub fn format_gateway_runtime_summary(runtime: Option<&GatewayServiceRuntime>) -> Option<String> {
    let runtime = runtime?;
    let status = runtime.status.as_deref().unwrap_or("unknown");
    let mut details: Vec<String> = Vec::new();

    if let Some(pid) = runtime.pid {
        details.push(format!("pid {pid}"));
    }
    if let Some(ref state) = runtime.state {
        if state.to_lowercase() != status {
            details.push(format!("state {state}"));
        }
    }
    if let Some(ref sub) = runtime.sub_state {
        details.push(format!("sub {sub}"));
    }
    if let Some(exit_status) = runtime.last_exit_status {
        details.push(format!("last exit {exit_status}"));
    }
    if let Some(ref reason) = runtime.last_exit_reason {
        details.push(format!("reason {reason}"));
    }
    if let Some(ref result) = runtime.last_run_result {
        details.push(format!("last run {result}"));
    }
    if let Some(ref time) = runtime.last_run_time {
        details.push(format!("last run time {time}"));
    }
    if let Some(ref detail) = runtime.detail {
        details.push(detail.clone());
    }

    if details.is_empty() {
        Some(status.to_string())
    } else {
        Some(format!("{status} ({})", details.join(", ")))
    }
}

/// Runtime hint options for platform-specific diagnostics.
///
/// Source: `src/commands/doctor-format.ts` — `RuntimeHintOptions`
#[derive(Debug, Clone, Default)]
pub struct RuntimeHintOptions {
    /// Override for the platform (defaults to current OS).
    pub platform: Option<String>,
}

/// Build actionable repair hints for the given gateway runtime state.
///
/// Source: `src/commands/doctor-format.ts` — `buildGatewayRuntimeHints`
pub fn build_gateway_runtime_hints(
    runtime: Option<&GatewayServiceRuntime>,
    _options: &RuntimeHintOptions,
) -> Vec<String> {
    let mut hints = Vec::new();
    let Some(runtime) = runtime else {
        return hints;
    };

    let platform = std::env::consts::OS;

    // Missing unit file → suggest install.
    if runtime.missing_unit {
        hints.push(format!(
            "Service not installed. Run: {}",
            format_cli_command("crabclaw gateway install")
        ));
        return hints;
    }

    // Cached LaunchAgent label but plist missing (macOS).
    if runtime.cached_label.is_some() && platform == "macos" {
        hints.push(
            "LaunchAgent label cached but plist missing. Clear with: launchctl bootout gui/$UID/<label>"
                .to_string(),
        );
        hints.push(format!(
            "Then reinstall: {}",
            format_cli_command("crabclaw gateway install")
        ));
    }

    // Stopped service.
    if runtime.status.as_deref() == Some("stopped") {
        hints.push("Service is loaded but not running (likely exited immediately).".to_string());
        match platform {
            "macos" => {
                hints.push("Check launchd stdout/stderr logs.".to_string());
            }
            "linux" => {
                hints.push(
                    "Logs: journalctl --user -u openacosmi-gateway.service -n 200 --no-pager"
                        .to_string(),
                );
            }
            _ => {}
        }
    }

    hints
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn summary_none_for_no_runtime() {
        assert!(format_gateway_runtime_summary(None).is_none());
    }

    #[test]
    fn summary_status_only() {
        let rt = GatewayServiceRuntime {
            status: Some("running".to_string()),
            ..Default::default()
        };
        let s = format_gateway_runtime_summary(Some(&rt));
        assert_eq!(s, Some("running".to_string()));
    }

    #[test]
    fn summary_with_details() {
        let rt = GatewayServiceRuntime {
            status: Some("running".to_string()),
            pid: Some(12345),
            last_exit_status: Some(0),
            ..Default::default()
        };
        let s = format_gateway_runtime_summary(Some(&rt)).unwrap_or_default();
        assert!(s.contains("pid 12345"));
        assert!(s.contains("last exit 0"));
    }

    #[test]
    fn hints_empty_for_no_runtime() {
        let hints = build_gateway_runtime_hints(None, &RuntimeHintOptions::default());
        assert!(hints.is_empty());
    }

    #[test]
    fn hints_suggest_install_for_missing_unit() {
        let rt = GatewayServiceRuntime {
            missing_unit: true,
            ..Default::default()
        };
        let hints = build_gateway_runtime_hints(Some(&rt), &RuntimeHintOptions::default());
        assert!(!hints.is_empty());
        assert!(hints[0].contains("not installed"));
    }
}
