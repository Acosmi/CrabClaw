/// Agent execution via gateway RPC.
///
/// Sends an agent message through the gateway WebSocket endpoint instead
/// of running the agent locally. This is the default mode when the gateway
/// is available; the CLI falls back to local execution on failure.
///
/// Source: `src/commands/agent-via-gateway.ts`
use anyhow::{Result, bail};

use oa_agents::scope::list_agent_ids;
use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::load_config;
use oa_routing::session_key::normalize_agent_id;

use crate::session::resolve_session_key_for_request;
use crate::types::{AgentCliOpts, AgentGatewayPayload, GatewayAgentResponse};

/// Parse a timeout value from CLI opts and config.
///
/// Source: `src/commands/agent-via-gateway.ts` - `parseTimeoutSeconds`
pub fn parse_timeout_seconds(timeout: Option<&str>, config_timeout: Option<u64>) -> Result<u64> {
    let raw = match timeout {
        Some(t) => {
            let parsed: i64 = t
                .trim()
                .parse()
                .map_err(|_| anyhow::anyhow!("--timeout must be a positive integer (seconds)"))?;
            parsed
        }
        None => {
            let default = config_timeout.unwrap_or(600);
            #[allow(clippy::cast_possible_wrap)]
            return Ok(default);
        }
    };

    if raw <= 0 {
        bail!("--timeout must be a positive integer (seconds)");
    }

    #[allow(clippy::cast_sign_loss)]
    Ok(raw as u64)
}

/// Format a payload for human-readable log output.
///
/// Source: `src/commands/agent-via-gateway.ts` - `formatPayloadForLog`
pub fn format_payload_for_log(payload: &AgentGatewayPayload) -> String {
    let mut lines: Vec<String> = Vec::new();

    if let Some(ref text) = payload.text {
        let trimmed = text.trim_end();
        if !trimmed.is_empty() {
            lines.push(trimmed.to_owned());
        }
    }

    let single_media = payload
        .media_url
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty());

    let media: Vec<&str> = if let Some(ref urls) = payload.media_urls {
        urls.iter().map(String::as_str).collect()
    } else if let Some(url) = single_media {
        vec![url]
    } else {
        vec![]
    };

    for url in media {
        lines.push(format!("MEDIA:{url}"));
    }

    lines.join("\n").trim_end().to_owned()
}

/// Validate the CLI options for an agent-via-gateway call.
///
/// Source: `src/commands/agent-via-gateway.ts` - `agentViaGatewayCommand` validation
pub fn validate_gateway_opts(opts: &AgentCliOpts) -> Result<()> {
    let body = opts.message.trim();
    if body.is_empty() {
        bail!("Message (--message) is required");
    }
    if opts.to.is_none() && opts.session_id.is_none() && opts.agent.is_none() {
        bail!("Pass --to <E.164>, --session-id, or --agent to choose a session");
    }
    Ok(())
}

/// Execute the agent command via gateway RPC.
///
/// Sends the message to the gateway, waits for the response, and
/// formats the output for the terminal. Returns the raw response
/// for JSON mode or the formatted payloads for text mode.
///
/// Source: `src/commands/agent-via-gateway.ts` - `agentViaGatewayCommand`
pub async fn agent_via_gateway_command(opts: &AgentCliOpts) -> Result<GatewayAgentResponse> {
    validate_gateway_opts(opts)?;

    let cfg = load_config()?;

    // Validate agent id.
    let agent_id = opts
        .agent
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(|raw| {
            let normalized = normalize_agent_id(Some(raw));
            let known = list_agent_ids(&cfg);
            if !known.contains(&normalized) {
                Err(anyhow::anyhow!(
                    "Unknown agent id \"{raw}\". Use \"{}\" to see configured agents.",
                    format_cli_command("crabclaw agents list")
                ))
            } else {
                Ok(normalized)
            }
        })
        .transpose()?;

    let config_timeout = cfg
        .agents
        .as_ref()
        .and_then(|a| a.defaults.as_ref())
        .and_then(|d| d.timeout_seconds);
    let timeout_seconds = parse_timeout_seconds(opts.timeout.as_deref(), config_timeout)?;
    let _gateway_timeout_ms = (timeout_seconds + 30) * 1000;

    // Resolve session key.
    let session_key_resolution = resolve_session_key_for_request(
        &cfg,
        opts.to.as_deref(),
        opts.session_id.as_deref(),
        None,
        agent_id.as_deref(),
    );

    let _session_key = session_key_resolution.session_key;
    let _idempotency_key = opts
        .run_id
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map_or_else(|| uuid::Uuid::new_v4().to_string(), String::from);

    // TODO: Make the actual gateway RPC call.
    // The full implementation calls `call_gateway` with method "agent" and
    // the resolved parameters, then formats the response.

    let response = GatewayAgentResponse {
        status: Some("stub".to_owned()),
        summary: Some("Gateway agent call not yet connected".to_owned()),
        ..Default::default()
    };

    Ok(response)
}

/// Execute the agent CLI command with gateway-first, local-fallback strategy.
///
/// If `--local` is set, runs directly. Otherwise tries the gateway and
/// falls back to local execution on failure.
///
/// Source: `src/commands/agent-via-gateway.ts` - `agentCliCommand`
pub async fn agent_cli_command(opts: &AgentCliOpts) -> Result<serde_json::Value> {
    if opts.local == Some(true) {
        // Run locally via agent_command.
        let local_opts = crate::types::AgentCommandOpts {
            message: opts.message.clone(),
            agent_id: opts.agent.clone(),
            to: opts.to.clone(),
            session_id: opts.session_id.clone(),
            thinking: opts.thinking.clone(),
            verbose: opts.verbose.clone(),
            json: opts.json,
            timeout: opts.timeout.clone(),
            deliver: opts.deliver,
            reply_to: opts.reply_to.clone(),
            reply_channel: opts.reply_channel.clone(),
            reply_account_id: opts.reply_account.clone(),
            best_effort_deliver: opts.best_effort_deliver,
            lane: opts.lane.clone(),
            run_id: opts.run_id.clone(),
            extra_system_prompt: opts.extra_system_prompt.clone(),
            ..Default::default()
        };
        return crate::agent_command::agent_command(&local_opts).await;
    }

    // Try gateway first, fall back to local.
    match agent_via_gateway_command(opts).await {
        Ok(response) => Ok(serde_json::to_value(&response)?),
        Err(err) => {
            tracing::warn!("Gateway agent failed; falling back to embedded: {err}");
            let local_opts = crate::types::AgentCommandOpts {
                message: opts.message.clone(),
                agent_id: opts.agent.clone(),
                to: opts.to.clone(),
                session_id: opts.session_id.clone(),
                thinking: opts.thinking.clone(),
                verbose: opts.verbose.clone(),
                json: opts.json,
                timeout: opts.timeout.clone(),
                deliver: opts.deliver,
                reply_to: opts.reply_to.clone(),
                reply_channel: opts.reply_channel.clone(),
                reply_account_id: opts.reply_account.clone(),
                best_effort_deliver: opts.best_effort_deliver,
                lane: opts.lane.clone(),
                run_id: opts.run_id.clone(),
                extra_system_prompt: opts.extra_system_prompt.clone(),
                ..Default::default()
            };
            crate::agent_command::agent_command(&local_opts).await
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_payload_text_only() {
        let payload = AgentGatewayPayload {
            text: Some("Hello world".to_owned()),
            media_url: None,
            media_urls: None,
        };
        assert_eq!(format_payload_for_log(&payload), "Hello world");
    }

    #[test]
    fn format_payload_with_media_urls() {
        let payload = AgentGatewayPayload {
            text: Some("Check this".to_owned()),
            media_url: None,
            media_urls: Some(vec![
                "https://a.com/1.png".to_owned(),
                "https://a.com/2.png".to_owned(),
            ]),
        };
        let result = format_payload_for_log(&payload);
        assert!(result.contains("Check this"));
        assert!(result.contains("MEDIA:https://a.com/1.png"));
        assert!(result.contains("MEDIA:https://a.com/2.png"));
    }

    #[test]
    fn format_payload_single_media_url() {
        let payload = AgentGatewayPayload {
            text: None,
            media_url: Some("https://a.com/photo.jpg".to_owned()),
            media_urls: None,
        };
        assert_eq!(
            format_payload_for_log(&payload),
            "MEDIA:https://a.com/photo.jpg"
        );
    }

    #[test]
    fn format_payload_empty() {
        let payload = AgentGatewayPayload::default();
        assert_eq!(format_payload_for_log(&payload), "");
    }

    #[test]
    fn validate_gateway_opts_empty_message() {
        let opts = AgentCliOpts {
            message: "".to_owned(),
            ..Default::default()
        };
        assert!(validate_gateway_opts(&opts).is_err());
    }

    #[test]
    fn validate_gateway_opts_no_session_routing() {
        let opts = AgentCliOpts {
            message: "Hello".to_owned(),
            ..Default::default()
        };
        let err = validate_gateway_opts(&opts).unwrap_err().to_string();
        assert!(err.contains("--to"));
    }

    #[test]
    fn validate_gateway_opts_with_agent() {
        let opts = AgentCliOpts {
            message: "Hello".to_owned(),
            agent: Some("mybot".to_owned()),
            ..Default::default()
        };
        assert!(validate_gateway_opts(&opts).is_ok());
    }

    #[test]
    fn parse_timeout_valid() {
        assert_eq!(parse_timeout_seconds(Some("120"), None).unwrap_or(0), 120);
    }

    #[test]
    fn parse_timeout_negative() {
        assert!(parse_timeout_seconds(Some("-1"), None).is_err());
    }

    #[test]
    fn parse_timeout_default() {
        assert_eq!(parse_timeout_seconds(None, Some(300)).unwrap_or(0), 300);
    }

    #[test]
    fn parse_timeout_fallback() {
        assert_eq!(parse_timeout_seconds(None, None).unwrap_or(0), 600);
    }
}
