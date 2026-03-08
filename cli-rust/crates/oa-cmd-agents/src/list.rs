/// Agents list command: display all configured agents with their summaries.
///
/// Source: `src/commands/agents.commands.list.ts`
use anyhow::Result;

use oa_cli_shared::command_format::format_cli_command;
use oa_routing::session_key::normalize_agent_id;
use oa_types::agents::AgentBinding;
use oa_types::config::OpenAcosmiConfig;

use crate::bindings::describe_binding;
use crate::config::{AgentSummary, build_agent_summaries};

/// Format a single agent summary for text display.
///
/// Source: `src/commands/agents.commands.list.ts` - `formatSummary`
fn format_summary(summary: &AgentSummary) -> String {
    let default_tag = if summary.is_default { " (default)" } else { "" };
    let header = match &summary.name {
        Some(name) if name != &summary.id => format!("{}{default_tag} ({name})", summary.id),
        _ => format!("{}{default_tag}", summary.id),
    };

    let mut identity_parts = Vec::new();
    if let Some(ref emoji) = summary.identity_emoji {
        identity_parts.push(emoji.clone());
    }
    if let Some(ref name) = summary.identity_name {
        identity_parts.push(name.clone());
    }
    let identity_line = if identity_parts.is_empty() {
        None
    } else {
        Some(identity_parts.join(" "))
    };
    let identity_source_label = summary.identity_source.as_deref().map(|s| match s {
        "identity" => "IDENTITY.md",
        "config" => "config",
        _ => s,
    });

    let mut lines = vec![format!("- {header}")];
    if let Some(ref identity) = identity_line {
        let source_suffix = identity_source_label
            .map(|s| format!(" ({s})"))
            .unwrap_or_default();
        lines.push(format!("  Identity: {identity}{source_suffix}"));
    }
    lines.push(format!(
        "  Workspace: {}",
        shorten_home_path(&summary.workspace)
    ));
    lines.push(format!(
        "  Agent dir: {}",
        shorten_home_path(&summary.agent_dir)
    ));
    if let Some(ref model) = summary.model {
        lines.push(format!("  Model: {model}"));
    }
    lines.push(format!("  Routing rules: {}", summary.bindings));

    if let Some(ref routes) = summary.routes {
        if !routes.is_empty() {
            lines.push(format!("  Routing: {}", routes.join(", ")));
        }
    }
    if let Some(ref providers) = summary.providers {
        if !providers.is_empty() {
            lines.push("  Providers:".to_owned());
            for provider in providers {
                lines.push(format!("    - {provider}"));
            }
        }
    }
    if let Some(ref details) = summary.binding_details {
        if !details.is_empty() {
            lines.push("  Routing rules:".to_owned());
            for detail in details {
                lines.push(format!("    - {detail}"));
            }
        }
    }
    lines.join("\n")
}

/// Shorten a path by replacing the home directory prefix with `~`.
///
/// Source: `src/utils.ts` - `shortenHomePath`
fn shorten_home_path(path: &str) -> String {
    if let Some(home) = dirs::home_dir() {
        let home_str = home.display().to_string();
        if path.starts_with(&home_str) {
            return format!("~{}", &path[home_str.len()..]);
        }
    }
    path.to_owned()
}

/// Execute the agents list command.
///
/// Source: `src/commands/agents.commands.list.ts` - `agentsListCommand`
pub fn agents_list_command(
    cfg: &OpenAcosmiConfig,
    json: bool,
    show_bindings: bool,
) -> Result<String> {
    let mut summaries = build_agent_summaries(cfg);

    // Build binding map
    let mut binding_map: std::collections::HashMap<String, Vec<&AgentBinding>> =
        std::collections::HashMap::new();
    for binding in cfg.bindings.as_deref().unwrap_or_default() {
        let agent_id = normalize_agent_id(Some(&binding.agent_id));
        binding_map.entry(agent_id).or_default().push(binding);
    }

    if show_bindings {
        for summary in &mut summaries {
            if let Some(bindings) = binding_map.get(&summary.id) {
                if !bindings.is_empty() {
                    summary.binding_details =
                        Some(bindings.iter().map(|b| describe_binding(b)).collect());
                }
            }
        }
    }

    // Add default routing for default agent
    for summary in &mut summaries {
        if let Some(bindings) = binding_map.get(&summary.id) {
            if !bindings.is_empty() {
                let route_set: Vec<String> = bindings
                    .iter()
                    .map(|b| b.r#match.channel.clone())
                    .collect::<std::collections::HashSet<_>>()
                    .into_iter()
                    .collect();
                summary.routes = Some(route_set);
            }
        }
        if summary.is_default && summary.routes.is_none() {
            summary.routes = Some(vec!["default (no explicit rules)".to_owned()]);
        }
    }

    if json {
        return Ok(serde_json::to_string_pretty(&summaries)?);
    }

    let mut lines = vec!["Agents:".to_owned()];
    for summary in &summaries {
        lines.push(format_summary(summary));
    }
    lines.push(
        "Routing rules map channel/account/peer to an agent. Use --bindings for full rules."
            .to_owned(),
    );
    lines.push(format!(
        "Channel status reflects local config/creds. For live health: {}.",
        format_cli_command("crabclaw channels status --probe")
    ));
    Ok(lines.join("\n"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use oa_types::agents::{AgentConfig, AgentsConfig};

    fn make_agent(id: &str) -> AgentConfig {
        AgentConfig {
            id: id.to_owned(),
            default: None,
            name: None,
            workspace: None,
            agent_dir: None,
            model: None,
            skills: None,
            memory_search: None,
            human_delay: None,
            heartbeat: None,
            identity: None,
            group_chat: None,
            subagents: None,
            sandbox: None,
            tools: None,
        }
    }

    fn make_config(agents: Vec<AgentConfig>) -> OpenAcosmiConfig {
        OpenAcosmiConfig {
            agents: Some(AgentsConfig {
                defaults: None,
                list: Some(agents),
            }),
            ..Default::default()
        }
    }

    #[test]
    fn list_default_config() {
        let cfg = OpenAcosmiConfig::default();
        let result = agents_list_command(&cfg, false, false);
        assert!(result.is_ok());
        let output = result.expect("should succeed");
        assert!(output.contains("Agents:"));
        assert!(output.contains("main"));
    }

    #[test]
    fn list_json_output() {
        let cfg = make_config(vec![make_agent("alpha"), make_agent("beta")]);
        let result = agents_list_command(&cfg, true, false);
        assert!(result.is_ok());
        let output = result.expect("should succeed");
        let parsed: serde_json::Value = serde_json::from_str(&output).unwrap_or_default();
        assert!(parsed.is_array());
        let arr = parsed.as_array().unwrap_or(&Vec::new()).clone();
        assert_eq!(arr.len(), 2);
    }

    #[test]
    fn list_with_bindings() {
        use oa_types::agents::{AgentBinding, AgentBindingMatch};
        let cfg = OpenAcosmiConfig {
            agents: Some(AgentsConfig {
                defaults: None,
                list: Some(vec![make_agent("alpha")]),
            }),
            bindings: Some(vec![AgentBinding {
                agent_id: "alpha".to_owned(),
                r#match: AgentBindingMatch {
                    channel: "discord".to_owned(),
                    account_id: None,
                    peer: None,
                    guild_id: None,
                    team_id: None,
                },
            }]),
            ..Default::default()
        };
        let result = agents_list_command(&cfg, false, true);
        assert!(result.is_ok());
        let output = result.expect("should succeed");
        assert!(output.contains("discord"));
    }

    #[test]
    fn format_summary_simple() {
        let summary = AgentSummary {
            id: "test".to_owned(),
            name: Some("Test Bot".to_owned()),
            identity_name: None,
            identity_emoji: None,
            identity_source: None,
            workspace: "/tmp/workspace".to_owned(),
            agent_dir: "/tmp/agent".to_owned(),
            model: Some("anthropic/claude-opus-4-6".to_owned()),
            bindings: 2,
            binding_details: None,
            routes: Some(vec!["discord".to_owned()]),
            providers: None,
            is_default: true,
        };
        let result = format_summary(&summary);
        assert!(result.contains("test (default) (Test Bot)"));
        assert!(result.contains("Model: anthropic/claude-opus-4-6"));
        assert!(result.contains("Routing: discord"));
    }

    #[test]
    fn shorten_home_path_works() {
        // This test is system-dependent but should at least not crash
        let result = shorten_home_path("/some/random/path");
        assert_eq!(result, "/some/random/path");
    }
}
