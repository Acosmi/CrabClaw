/// Shared utilities for model management commands.
///
/// Source: `src/commands/models/shared.ts`
use std::collections::HashSet;

use anyhow::{Context, Result, bail};
use oa_agents::model_selection::{
    ModelRef, build_model_alias_index, parse_model_ref, resolve_model_ref_from_string,
};
use oa_agents::scope::list_agent_ids;
use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::{read_config_file_snapshot, write_config_file};
use oa_routing::session_key::normalize_agent_id;
use oa_types::config::OpenAcosmiConfig;

/// Ensure that `--json` and `--plain` are not both set.
///
/// Source: `src/commands/models/shared.ts` - `ensureFlagCompatibility`
pub fn ensure_flag_compatibility(json: bool, plain: bool) -> Result<()> {
    if json && plain {
        bail!("Choose either --json or --plain, not both.");
    }
    Ok(())
}

/// Format a token count for display: values under 1024 are shown as-is,
/// values >= 1024 are shown as `Nk`. Returns `"-"` for missing/non-finite values.
///
/// Source: `src/commands/models/shared.ts` - `formatTokenK`
#[must_use]
pub fn format_token_k(value: Option<u64>) -> String {
    match value {
        None => "-".to_owned(),
        Some(v) if v == 0 => "-".to_owned(),
        Some(v) if v < 1024 => format!("{v}"),
        Some(v) => format!("{}k", v / 1024),
    }
}

/// Format a latency value in milliseconds: values < 1000 are shown as `Nms`,
/// values >= 1000 are shown as `N.Ns`. Returns `"-"` for `None`.
///
/// Source: `src/commands/models/shared.ts` - `formatMs`
#[must_use]
pub fn format_ms(value: Option<f64>) -> String {
    match value {
        None => "-".to_owned(),
        Some(v) if !v.is_finite() => "-".to_owned(),
        #[allow(clippy::cast_possible_truncation)] // guarded by v < 1000.0
        Some(v) if v < 1000.0 => format!("{}ms", v.round() as i64),
        Some(v) => {
            let seconds = (v / 100.0).round() / 10.0;
            format!("{seconds}s")
        }
    }
}

/// Read the config snapshot, validate it, apply a mutator, and write it back.
///
/// Returns the updated configuration.
///
/// Source: `src/commands/models/shared.ts` - `updateConfig`
pub async fn update_config<F>(mutator: F) -> Result<OpenAcosmiConfig>
where
    F: FnOnce(OpenAcosmiConfig) -> Result<OpenAcosmiConfig>,
{
    let snapshot = read_config_file_snapshot()
        .await
        .context("Failed to read config snapshot")?;
    if !snapshot.valid {
        let issues = snapshot
            .issues
            .iter()
            .map(|issue| format!("- {}: {}", issue.path, issue.message))
            .collect::<Vec<_>>()
            .join("\n");
        bail!("Invalid config at {}\n{issues}", snapshot.path);
    }
    let next = mutator(snapshot.config)?;
    write_config_file(&next)
        .await
        .context("Failed to write config file")?;
    Ok(next)
}

/// Resolve a raw model string into a canonical `(provider, model)` pair,
/// consulting aliases and the default provider.
///
/// Source: `src/commands/models/shared.ts` - `resolveModelTarget`
pub fn resolve_model_target(raw: &str, cfg: &OpenAcosmiConfig) -> Result<ModelRef> {
    let alias_index = build_model_alias_index(cfg, DEFAULT_PROVIDER);
    let resolved = resolve_model_ref_from_string(raw, DEFAULT_PROVIDER, Some(&alias_index));
    match resolved {
        Some((model_ref, _alias)) => Ok(model_ref),
        None => bail!("Invalid model reference: {raw}"),
    }
}

/// Build the set of configured model keys from the allowlist in config.
///
/// Source: `src/commands/models/shared.ts` - `buildAllowlistSet`
#[must_use]
pub fn build_allowlist_set(cfg: &OpenAcosmiConfig) -> HashSet<String> {
    let mut allowed = HashSet::new();
    let models = match cfg
        .agents
        .as_ref()
        .and_then(|a| a.defaults.as_ref())
        .and_then(|d| d.models.as_ref())
    {
        Some(m) => m,
        None => return allowed,
    };
    for raw in models.keys() {
        if let Some(parsed) = parse_model_ref(raw, DEFAULT_PROVIDER) {
            allowed.insert(model_key(&parsed.provider, &parsed.model));
        }
    }
    allowed
}

/// Normalize a model alias string, ensuring it's non-empty and contains only
/// allowed characters (letters, numbers, dots, underscores, colons, dashes).
///
/// Source: `src/commands/models/shared.ts` - `normalizeAlias`
pub fn normalize_alias(alias: &str) -> Result<String> {
    let trimmed = alias.trim();
    if trimmed.is_empty() {
        bail!("Alias cannot be empty.");
    }
    let valid = trimmed
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '_' | ':' | '-'));
    if !valid {
        bail!("Alias must use letters, numbers, dots, underscores, colons, or dashes.");
    }
    Ok(trimmed.to_owned())
}

/// Resolve an agent ID from raw input, verifying it's a known agent.
///
/// Returns `None` if no raw agent ID is provided.
///
/// Source: `src/commands/models/shared.ts` - `resolveKnownAgentId`
pub fn resolve_known_agent_id(
    cfg: &OpenAcosmiConfig,
    raw_agent_id: Option<&str>,
) -> Result<Option<String>> {
    let raw = match raw_agent_id {
        Some(r) => r.trim(),
        None => return Ok(None),
    };
    if raw.is_empty() {
        return Ok(None);
    }
    let agent_id = normalize_agent_id(Some(raw));
    let known_agents = list_agent_ids(cfg);
    if !known_agents.contains(&agent_id) {
        bail!(
            "Unknown agent id \"{raw}\". Use \"{}\" to see configured agents.",
            format_cli_command("crabclaw agents list")
        );
    }
    Ok(Some(agent_id))
}

/// Re-export commonly used constants.
///
/// Source: `src/commands/models/shared.ts`
pub use oa_agents::defaults::DEFAULT_MODEL;
/// Re-export of the default provider identifier.
///
/// Source: `src/commands/models/shared.ts`
pub use oa_agents::defaults::DEFAULT_PROVIDER;
/// Re-export `model_key` builder.
///
/// Source: `src/commands/models/shared.ts`
pub use oa_agents::model_selection::model_key;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn flag_compatibility_both_set() {
        assert!(ensure_flag_compatibility(true, true).is_err());
    }

    #[test]
    fn flag_compatibility_json_only() {
        assert!(ensure_flag_compatibility(true, false).is_ok());
    }

    #[test]
    fn flag_compatibility_plain_only() {
        assert!(ensure_flag_compatibility(false, true).is_ok());
    }

    #[test]
    fn flag_compatibility_neither() {
        assert!(ensure_flag_compatibility(false, false).is_ok());
    }

    #[test]
    fn format_token_k_none() {
        assert_eq!(format_token_k(None), "-");
    }

    #[test]
    fn format_token_k_zero() {
        assert_eq!(format_token_k(Some(0)), "-");
    }

    #[test]
    fn format_token_k_small() {
        assert_eq!(format_token_k(Some(500)), "500");
    }

    #[test]
    fn format_token_k_large() {
        assert_eq!(format_token_k(Some(200_000)), "195k");
    }

    #[test]
    fn format_ms_none() {
        assert_eq!(format_ms(None), "-");
    }

    #[test]
    fn format_ms_small() {
        assert_eq!(format_ms(Some(250.0)), "250ms");
    }

    #[test]
    fn format_ms_seconds() {
        assert_eq!(format_ms(Some(1500.0)), "1.5s");
    }

    #[test]
    fn normalize_alias_valid() {
        assert_eq!(normalize_alias("my-alias").unwrap(), "my-alias");
        assert_eq!(normalize_alias("alias.v2").unwrap(), "alias.v2");
        assert_eq!(normalize_alias("a:b_c").unwrap(), "a:b_c");
    }

    #[test]
    fn normalize_alias_empty() {
        assert!(normalize_alias("").is_err());
        assert!(normalize_alias("   ").is_err());
    }

    #[test]
    fn normalize_alias_invalid_chars() {
        assert!(normalize_alias("alias with spaces").is_err());
        assert!(normalize_alias("alias/path").is_err());
    }

    #[test]
    fn build_allowlist_set_empty_config() {
        let cfg = OpenAcosmiConfig::default();
        let result = build_allowlist_set(&cfg);
        assert!(result.is_empty());
    }

    #[test]
    fn build_allowlist_set_with_models() {
        use oa_types::agent_defaults::{AgentDefaultsConfig, AgentModelEntryConfig};
        use oa_types::agents::AgentsConfig;
        use std::collections::HashMap;

        let mut models = HashMap::new();
        models.insert(
            "anthropic/claude-opus-4-6".to_owned(),
            AgentModelEntryConfig::default(),
        );
        let cfg = OpenAcosmiConfig {
            agents: Some(AgentsConfig {
                defaults: Some(AgentDefaultsConfig {
                    models: Some(models),
                    ..Default::default()
                }),
                list: None,
            }),
            ..Default::default()
        };
        let result = build_allowlist_set(&cfg);
        assert!(result.contains("anthropic/claude-opus-4-6"));
    }

    #[test]
    fn resolve_known_agent_id_none() {
        let cfg = OpenAcosmiConfig::default();
        assert_eq!(resolve_known_agent_id(&cfg, None).unwrap(), None);
    }

    #[test]
    fn resolve_known_agent_id_empty() {
        let cfg = OpenAcosmiConfig::default();
        assert_eq!(resolve_known_agent_id(&cfg, Some("")).unwrap(), None);
    }

    #[test]
    fn resolve_known_agent_id_default() {
        // Default config has "main" agent
        let cfg = OpenAcosmiConfig::default();
        let result = resolve_known_agent_id(&cfg, Some("main")).unwrap();
        assert_eq!(result, Some("main".to_owned()));
    }

    #[test]
    fn resolve_known_agent_id_unknown() {
        let cfg = OpenAcosmiConfig::default();
        assert!(resolve_known_agent_id(&cfg, Some("nonexistent")).is_err());
    }
}
