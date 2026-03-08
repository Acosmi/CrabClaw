/// Display utilities for sandbox CLI output.
///
/// Provides functions for rendering container lists, summaries, and
/// recreate previews/results to the terminal.
///
/// Source: `src/commands/sandbox-display.ts`
use oa_cli_shared::command_format::format_cli_command;

use crate::formatters::{
    SandboxBrowserInfo, SandboxContainerInfo, count_mismatches, count_running,
    format_duration_compact, format_image_match, format_simple_status, format_status,
};

/// Display a list of sandbox compute containers.
///
/// Source: `src/commands/sandbox-display.ts` — `displayContainers`
pub fn display_containers(containers: &[SandboxContainerInfo]) {
    if containers.is_empty() {
        println!("No sandbox containers found.");
        return;
    }

    let now_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64;

    println!("\nSandbox Containers:\n");
    for container in containers {
        println!("  {}", container.container_name);
        println!("    Status:  {}", format_status(container.running));
        println!(
            "    Image:   {} {}",
            container.image,
            format_image_match(container.image_match)
        );
        let age = now_ms.saturating_sub(container.created_at_ms);
        let idle = now_ms.saturating_sub(container.last_used_at_ms);
        println!("    Age:     {}", format_duration_compact(age));
        println!("    Idle:    {}", format_duration_compact(idle));
        println!("    Session: {}", container.session_key);
        println!();
    }
}

/// Display a list of sandbox browser containers.
///
/// Source: `src/commands/sandbox-display.ts` — `displayBrowsers`
pub fn display_browsers(browsers: &[SandboxBrowserInfo]) {
    if browsers.is_empty() {
        println!("No sandbox browser containers found.");
        return;
    }

    let now_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64;

    println!("\nSandbox Browser Containers:\n");
    for browser in browsers {
        println!("  {}", browser.container_name);
        println!("    Status:  {}", format_status(browser.running));
        println!(
            "    Image:   {} {}",
            browser.image,
            format_image_match(browser.image_match)
        );
        println!("    CDP:     {}", browser.cdp_port);
        if let Some(novnc) = browser.no_vnc_port {
            println!("    noVNC:   {novnc}");
        }
        let age = now_ms.saturating_sub(browser.created_at_ms);
        let idle = now_ms.saturating_sub(browser.last_used_at_ms);
        println!("    Age:     {}", format_duration_compact(age));
        println!("    Idle:    {}", format_duration_compact(idle));
        println!("    Session: {}", browser.session_key);
        println!();
    }
}

/// Display a summary of container counts and mismatch warnings.
///
/// Source: `src/commands/sandbox-display.ts` — `displaySummary`
pub fn display_summary(containers: &[SandboxContainerInfo], browsers: &[SandboxBrowserInfo]) {
    let total_count = containers.len() + browsers.len();
    let running_count = count_running(containers) + count_running(browsers);
    let mismatch_count = count_mismatches(containers) + count_mismatches(browsers);

    println!("Total: {total_count} ({running_count} running)");

    if mismatch_count > 0 {
        println!("\n  {mismatch_count} container(s) with image mismatch detected.");
        println!(
            "   Run '{}' to update all containers.",
            format_cli_command("crabclaw sandbox recreate --all")
        );
    }
}

/// Display a preview of containers to be recreated.
///
/// Source: `src/commands/sandbox-display.ts` — `displayRecreatePreview`
pub fn display_recreate_preview(
    containers: &[SandboxContainerInfo],
    browsers: &[SandboxBrowserInfo],
) {
    println!("\nContainers to be recreated:\n");

    if !containers.is_empty() {
        println!("Sandbox Containers:");
        for container in containers {
            println!(
                "  - {} ({})",
                container.container_name,
                format_simple_status(container.running)
            );
        }
    }

    if !browsers.is_empty() {
        if !containers.is_empty() {
            println!();
        }
        println!("Browser Containers:");
        for browser in browsers {
            println!(
                "  - {} ({})",
                browser.container_name,
                format_simple_status(browser.running)
            );
        }
    }

    let total = containers.len() + browsers.len();
    println!("\nTotal: {total} container(s)");
}

/// Display the result of a recreate operation.
///
/// Source: `src/commands/sandbox-display.ts` — `displayRecreateResult`
pub fn display_recreate_result(success_count: usize, fail_count: usize) {
    println!("\nDone: {success_count} removed, {fail_count} failed");

    if success_count > 0 {
        println!("\nContainers will be automatically recreated when the agent is next used.");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_container(name: &str, running: bool, image_match: bool) -> SandboxContainerInfo {
        SandboxContainerInfo {
            running,
            image_match,
            container_name: name.to_owned(),
            session_key: "agent:main:main".to_owned(),
            image: "openacosmi/sandbox:latest".to_owned(),
            created_at_ms: 1_700_000_000_000,
            last_used_at_ms: 1_700_000_000_000,
        }
    }

    fn make_browser(name: &str, running: bool) -> SandboxBrowserInfo {
        SandboxBrowserInfo {
            running,
            image_match: true,
            container_name: name.to_owned(),
            session_key: "agent:main:main".to_owned(),
            image: "openacosmi/browser:latest".to_owned(),
            created_at_ms: 1_700_000_000_000,
            last_used_at_ms: 1_700_000_000_000,
            cdp_port: 9222,
            no_vnc_port: Some(6080),
        }
    }

    #[test]
    fn display_summary_empty() {
        // Just verify it doesn't panic
        display_summary(&[], &[]);
    }

    #[test]
    fn display_summary_with_containers() {
        let containers = vec![
            make_container("c1", true, true),
            make_container("c2", false, false),
        ];
        let browsers = vec![make_browser("b1", true)];
        // Just verify it doesn't panic
        display_summary(&containers, &browsers);
    }

    #[test]
    fn display_recreate_result_success() {
        // Verify it doesn't panic
        display_recreate_result(3, 0);
    }

    #[test]
    fn display_recreate_result_with_failures() {
        // Verify it doesn't panic
        display_recreate_result(2, 1);
    }
}
