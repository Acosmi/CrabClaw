/// Message command implementation.
///
/// Provides the CLI entry point for sending messages, polls, and running
/// channel-specific message actions (react, read, search, etc.)
///
/// Source: `src/commands/message.ts`
use std::collections::HashMap;

use anyhow::{Result, bail};
use serde::{Deserialize, Serialize};
use tracing::info;

use crate::message_format::{build_message_cli_json, format_message_cli_text};

/// Known channel message action names.
///
/// Source: `src/channels/plugins/types.ts` - `CHANNEL_MESSAGE_ACTION_NAMES`
pub const CHANNEL_MESSAGE_ACTION_NAMES: &[&str] = &[
    "send",
    "poll",
    "react",
    "read",
    "search",
    "list-pins",
    "pin",
    "unpin",
    "edit",
    "delete",
    "reactions",
    "threads",
    "thread-replies",
    "reply",
];

/// Result of a message action run.
///
/// Source: `src/infra/outbound/message-action-runner.ts` - `MessageActionRunResult`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MessageActionRunResult {
    /// The action that was run.
    pub action: String,
    /// The channel this action ran on.
    pub channel: String,
    /// Whether this was a dry-run.
    pub dry_run: bool,
    /// Who handled the action (plugin, core, dry-run).
    pub handled_by: String,
    /// The result kind (send, poll, broadcast, or the action name).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub kind: Option<String>,
    /// The raw payload returned by the action.
    #[serde(default)]
    pub payload: serde_json::Value,
    /// Optional send result (for core-handled sends).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub send_result: Option<serde_json::Value>,
    /// Optional poll result (for core-handled polls).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub poll_result: Option<serde_json::Value>,
}

/// Options for the message command.
///
/// Source: `src/commands/message.ts` - `opts` parameter
#[derive(Debug, Clone, Default)]
pub struct MessageCommandOptions {
    /// The action to perform (default: "send").
    pub action: Option<String>,
    /// Whether to output JSON.
    pub json: bool,
    /// Whether this is a dry-run.
    pub dry_run: bool,
    /// Additional parameters passed to the action.
    pub params: HashMap<String, serde_json::Value>,
}

/// Validate and normalize a message action name.
///
/// Source: `src/commands/message.ts` - action matching
pub fn normalize_action_name(input: &str) -> Result<String> {
    let trimmed = input.trim();
    let action_input = if trimmed.is_empty() { "send" } else { trimmed };

    for &name in CHANNEL_MESSAGE_ACTION_NAMES {
        if name.eq_ignore_ascii_case(action_input) {
            return Ok(name.to_owned());
        }
    }

    bail!("Unknown message action: {action_input}")
}

/// Run the message command.
///
/// Executes the specified message action and formats the output as either
/// JSON or human-readable text.
///
/// Source: `src/commands/message.ts` - `messageCommand`
///
/// NOTE: The actual message action execution requires the full gateway RPC
/// and channel plugin infrastructure. This function provides the CLI
/// orchestration layer; the execution is delegated to the gateway.
pub async fn message_command(opts: &MessageCommandOptions) -> Result<()> {
    let action = normalize_action_name(opts.action.as_deref().unwrap_or("send"))?;

    // In the full implementation, this would:
    // 1. Load config
    // 2. Create outbound send dependencies
    // 3. Run the message action via the gateway or direct plugin
    // 4. Format and display the result
    //
    // For now, we provide the orchestration structure with a placeholder
    // that the integration layer will fill in.

    let result = MessageActionRunResult {
        action: action.clone(),
        channel: opts
            .params
            .get("channel")
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_owned(),
        dry_run: opts.dry_run,
        handled_by: if opts.dry_run {
            "dry-run".to_owned()
        } else {
            "core".to_owned()
        },
        kind: Some(action.clone()),
        payload: serde_json::Value::Object(serde_json::Map::new()),
        send_result: None,
        poll_result: None,
    };

    if opts.json {
        let envelope = build_message_cli_json(&result);
        let json_str = serde_json::to_string_pretty(&envelope)?;
        info!("{json_str}");
        return Ok(());
    }

    let lines = format_message_cli_text(&result);
    for line in &lines {
        info!("{line}");
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalize_action_send() {
        assert_eq!(normalize_action_name("send").expect("valid"), "send");
        assert_eq!(normalize_action_name("SEND").expect("valid"), "send");
        assert_eq!(normalize_action_name("").expect("valid"), "send");
    }

    #[test]
    fn normalize_action_poll() {
        assert_eq!(normalize_action_name("poll").expect("valid"), "poll");
    }

    #[test]
    fn normalize_action_react() {
        assert_eq!(normalize_action_name("react").expect("valid"), "react");
    }

    #[test]
    fn normalize_action_all_known() {
        for &name in CHANNEL_MESSAGE_ACTION_NAMES {
            assert_eq!(
                normalize_action_name(name).expect("valid"),
                name,
                "Failed for action: {name}"
            );
        }
    }

    #[test]
    fn normalize_action_unknown() {
        assert!(normalize_action_name("unknown_action").is_err());
    }

    #[test]
    fn normalize_action_case_insensitive() {
        assert_eq!(
            normalize_action_name("List-Pins").expect("valid"),
            "list-pins"
        );
    }

    #[test]
    fn action_names_contains_core() {
        assert!(CHANNEL_MESSAGE_ACTION_NAMES.contains(&"send"));
        assert!(CHANNEL_MESSAGE_ACTION_NAMES.contains(&"poll"));
        assert!(CHANNEL_MESSAGE_ACTION_NAMES.contains(&"react"));
        assert!(CHANNEL_MESSAGE_ACTION_NAMES.contains(&"read"));
    }

    #[test]
    fn message_result_serialization() {
        let result = MessageActionRunResult {
            action: "send".to_owned(),
            channel: "telegram".to_owned(),
            dry_run: false,
            handled_by: "core".to_owned(),
            kind: Some("send".to_owned()),
            payload: serde_json::json!({"messageId": "123"}),
            send_result: None,
            poll_result: None,
        };
        let json = serde_json::to_string(&result).expect("should serialize");
        assert!(json.contains("\"action\":\"send\""));
        assert!(json.contains("\"channel\":\"telegram\""));
        assert!(json.contains("\"dryRun\":false"));
        assert!(json.contains("\"handledBy\":\"core\""));
    }
}
