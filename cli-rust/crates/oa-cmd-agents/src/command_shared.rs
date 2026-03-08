/// Shared utilities for agent management commands.
///
/// Source: `src/commands/agents.command-shared.ts`
use anyhow::Result;

use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::read_config_file_snapshot;
use oa_types::config::OpenAcosmiConfig;

/// Read and validate the configuration file.
///
/// Returns `Ok(config)` if valid, or `Err` with a description of the
/// validation issues when the config file exists but is invalid.
///
/// Source: `src/commands/agents.command-shared.ts` - `requireValidConfig`
pub async fn require_valid_config() -> Result<OpenAcosmiConfig> {
    let snapshot = read_config_file_snapshot().await?;
    if snapshot.exists && !snapshot.valid {
        let issues = if snapshot.issues.is_empty() {
            "Unknown validation issue.".to_owned()
        } else {
            snapshot
                .issues
                .iter()
                .map(|issue| format!("- {}: {}", issue.path, issue.message))
                .collect::<Vec<_>>()
                .join("\n")
        };
        anyhow::bail!(
            "Config invalid:\n{issues}\nFix the config or run {}.",
            format_cli_command("crabclaw doctor")
        );
    }
    Ok(snapshot.config)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn require_valid_config_returns_result() {
        // `require_valid_config` reads from disk. In CI or test environments
        // the config file may not exist, which should yield a valid default,
        // or may exist with project-specific content.
        // Either way, the function should not panic.
        let result = require_valid_config().await;
        // We just verify it doesn't panic; the result depends on the env.
        let _cfg = result.unwrap_or_default();
    }
}
