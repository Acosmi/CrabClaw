/// Core agent command execution.
///
/// Validates command options, resolves sessions, configures model and
/// thinking parameters, and orchestrates the agent run pipeline.
/// This is the entry point for `crabclaw agent --message "..."`.
///
/// Source: `src/commands/agent.ts` - `agentCommand`
use anyhow::{Result, bail};
use tracing::info;

use oa_agents::scope::list_agent_ids;
use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::load_config;
use oa_routing::session_key::normalize_agent_id;

use crate::session::{normalize_think_level, normalize_verbose_level, resolve_session};
use crate::types::AgentCommandOpts;

/// Thinking levels formatted for error messages.
///
/// Source: `src/auto-reply/thinking.ts` - `formatThinkingLevels`
const THINKING_LEVELS_HINT: &str = "off, minimal, low, medium, high, xhigh";

/// Default timeout in seconds when none is specified.
///
/// Source: `src/agents/timeout.ts` - default
const DEFAULT_TIMEOUT_SECONDS: u64 = 600;

/// Validate common agent command preconditions.
///
/// Checks that a non-empty message is provided, that at least one session
/// routing parameter is present, and that the agent id (if specified)
/// exists in the configuration.
///
/// Source: `src/commands/agent.ts` - `agentCommand` validation block
pub fn validate_agent_opts(opts: &AgentCommandOpts) -> Result<()> {
    let body = opts.message.trim();
    if body.is_empty() {
        bail!("Message (--message) is required");
    }

    if opts.to.is_none()
        && opts.session_id.is_none()
        && opts.session_key.is_none()
        && opts.agent_id.is_none()
    {
        bail!("Pass --to <E.164>, --session-id, or --agent to choose a session");
    }

    Ok(())
}

/// Validate and resolve the agent id override.
///
/// Checks that the specified agent id exists in the loaded config.
/// Returns the normalized agent id if one was provided.
///
/// Source: `src/commands/agent.ts` - agent id validation
pub fn validate_agent_id(agent_id_raw: Option<&str>) -> Result<Option<String>> {
    let raw = match agent_id_raw {
        Some(s) => {
            let trimmed = s.trim();
            if trimmed.is_empty() {
                return Ok(None);
            }
            trimmed
        }
        None => return Ok(None),
    };

    let normalized = normalize_agent_id(Some(raw));
    let cfg = load_config()?;
    let known_agents = list_agent_ids(&cfg);

    if !known_agents.contains(&normalized) {
        bail!(
            "Unknown agent id \"{raw}\". Use \"{}\" to see configured agents.",
            format_cli_command("crabclaw agents list")
        );
    }

    Ok(Some(normalized))
}

/// Parse and validate the timeout parameter.
///
/// Returns the timeout in seconds, falling back to the config value or
/// the default of 600 seconds.
///
/// Source: `src/commands/agent.ts` - timeout parsing
pub fn parse_timeout_seconds(
    timeout_raw: Option<&str>,
    config_timeout: Option<u64>,
) -> Result<u64> {
    match timeout_raw {
        Some(raw) => {
            let parsed: i64 = raw
                .trim()
                .parse()
                .map_err(|_| anyhow::anyhow!("--timeout must be a positive integer (seconds)"))?;
            if parsed <= 0 {
                bail!("--timeout must be a positive integer (seconds)");
            }
            #[allow(clippy::cast_sign_loss)]
            Ok(parsed as u64)
        }
        None => Ok(config_timeout.unwrap_or(DEFAULT_TIMEOUT_SECONDS)),
    }
}

/// Execute the agent command.
///
/// This is the primary entry point. It validates inputs, resolves the
/// session, configures model/thinking parameters, and delegates to the
/// agent runner. Currently, the actual LLM invocation is stubbed;
/// session resolution and validation are fully implemented.
///
/// Source: `src/commands/agent.ts` - `agentCommand`
pub async fn agent_command(opts: &AgentCommandOpts) -> Result<serde_json::Value> {
    validate_agent_opts(opts)?;

    let cfg = load_config()?;
    let agent_id_override = validate_agent_id(opts.agent_id.as_deref())?;

    // Validate thinking level.
    let think_override = normalize_think_level(opts.thinking.as_deref());
    if opts.thinking.is_some() && think_override.is_none() {
        bail!("Invalid thinking level. Use one of: {THINKING_LEVELS_HINT}.");
    }

    let think_once = normalize_think_level(opts.thinking_once.as_deref());
    if opts.thinking_once.is_some() && think_once.is_none() {
        bail!("Invalid one-shot thinking level. Use one of: {THINKING_LEVELS_HINT}.");
    }

    // Validate verbose level.
    let verbose_override = normalize_verbose_level(opts.verbose.as_deref());
    if opts.verbose.is_some() && verbose_override.is_none() {
        bail!("Invalid verbose level. Use \"on\", \"full\", or \"off\".");
    }

    // Parse timeout.
    let config_timeout = cfg
        .agents
        .as_ref()
        .and_then(|a| a.defaults.as_ref())
        .and_then(|d| d.timeout_seconds);
    let _timeout_seconds = parse_timeout_seconds(opts.timeout.as_deref(), config_timeout)?;

    // Resolve session.
    let session_resolution = resolve_session(
        &cfg,
        opts.to.as_deref(),
        opts.session_id.as_deref(),
        opts.session_key.as_deref(),
        agent_id_override.as_deref().or(opts.agent_id.as_deref()),
    );

    info!(
        session_id = %session_resolution.session_id,
        session_key = ?session_resolution.session_key,
        is_new = session_resolution.is_new_session,
        "Resolved agent session"
    );

    // Resolve thinking level (persisted -> override -> config default).
    let resolved_think_level = think_once
        .or(think_override)
        .or(session_resolution.persisted_thinking);

    let _resolved_verbose_level = verbose_override.or(session_resolution.persisted_verbose);

    // TODO: Full agent execution pipeline.
    // The complete pipeline includes:
    // 1. Build workspace skill snapshot
    // 2. Persist thinking/verbose overrides to session store
    // 3. Resolve model (allowlist, stored overrides, auth profiles)
    // 4. Run with model fallback via runEmbeddedPiAgent / runCliAgent
    // 5. Update session store with token usage
    // 6. Deliver result via deliverAgentCommandResult

    let result = serde_json::json!({
        "sessionId": session_resolution.session_id,
        "sessionKey": session_resolution.session_key,
        "isNewSession": session_resolution.is_new_session,
        "thinkingLevel": resolved_think_level,
        "status": "stub",
        "message": "Agent execution pipeline not yet connected to LLM runtime"
    });

    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::AgentCommandOpts;

    #[test]
    fn validate_opts_empty_message() {
        let opts = AgentCommandOpts {
            message: "".to_owned(),
            to: Some("+15551234567".to_owned()),
            ..Default::default()
        };
        let result = validate_agent_opts(&opts);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("Message"));
    }

    #[test]
    fn validate_opts_whitespace_message() {
        let opts = AgentCommandOpts {
            message: "   ".to_owned(),
            to: Some("+15551234567".to_owned()),
            ..Default::default()
        };
        assert!(validate_agent_opts(&opts).is_err());
    }

    #[test]
    fn validate_opts_no_session_routing() {
        let opts = AgentCommandOpts {
            message: "Hello".to_owned(),
            ..Default::default()
        };
        let result = validate_agent_opts(&opts);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("--to"));
    }

    #[test]
    fn validate_opts_with_to() {
        let opts = AgentCommandOpts {
            message: "Hello".to_owned(),
            to: Some("+15551234567".to_owned()),
            ..Default::default()
        };
        assert!(validate_agent_opts(&opts).is_ok());
    }

    #[test]
    fn validate_opts_with_session_id() {
        let opts = AgentCommandOpts {
            message: "Hello".to_owned(),
            session_id: Some("my-session".to_owned()),
            ..Default::default()
        };
        assert!(validate_agent_opts(&opts).is_ok());
    }

    #[test]
    fn validate_opts_with_agent_id() {
        let opts = AgentCommandOpts {
            message: "Hello".to_owned(),
            agent_id: Some("mybot".to_owned()),
            ..Default::default()
        };
        assert!(validate_agent_opts(&opts).is_ok());
    }

    #[test]
    fn parse_timeout_valid() {
        assert_eq!(parse_timeout_seconds(Some("30"), None).unwrap_or(0), 30);
    }

    #[test]
    fn parse_timeout_invalid() {
        assert!(parse_timeout_seconds(Some("abc"), None).is_err());
    }

    #[test]
    fn parse_timeout_negative() {
        assert!(parse_timeout_seconds(Some("-5"), None).is_err());
    }

    #[test]
    fn parse_timeout_zero() {
        assert!(parse_timeout_seconds(Some("0"), None).is_err());
    }

    #[test]
    fn parse_timeout_default() {
        assert_eq!(
            parse_timeout_seconds(None, None).unwrap_or(0),
            DEFAULT_TIMEOUT_SECONDS
        );
    }

    #[test]
    fn parse_timeout_from_config() {
        assert_eq!(parse_timeout_seconds(None, Some(120)).unwrap_or(0), 120);
    }
}
