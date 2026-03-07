/// Message CLI output formatting.
///
/// Provides functions to format message action results for both JSON
/// and human-readable terminal output. Handles send, poll, broadcast,
/// react, read, search, and other channel actions.
///
/// Source: `src/commands/message-format.ts`
use serde::{Deserialize, Serialize};

use crate::message::MessageActionRunResult;

/// JSON envelope for CLI message output.
///
/// Source: `src/commands/message-format.ts` - `MessageCliJsonEnvelope`
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MessageCliJsonEnvelope {
    /// The action that was executed.
    pub action: String,
    /// The channel the action ran on.
    pub channel: String,
    /// Whether this was a dry-run.
    pub dry_run: bool,
    /// Who handled the action.
    pub handled_by: String,
    /// The raw payload returned by the action.
    pub payload: serde_json::Value,
}

/// Shorten a string to a maximum character count, appending ellipsis.
///
/// Source: `src/commands/message-format.ts` - `shortenText`
fn shorten_text(value: &str, max_len: usize) -> String {
    let chars: Vec<char> = value.chars().collect();
    if chars.len() <= max_len {
        return value.to_owned();
    }
    if max_len <= 1 {
        return String::new();
    }
    let truncated: String = chars[..max_len.saturating_sub(1)].iter().collect();
    format!("{truncated}\u{2026}")
}

/// Extract a message ID from a payload.
///
/// Source: `src/commands/message-format.ts` - `extractMessageId`
fn extract_message_id(payload: &serde_json::Value) -> Option<String> {
    // Direct messageId.
    if let Some(id) = payload.get("messageId").and_then(|v| v.as_str()) {
        let trimmed = id.trim();
        if !trimmed.is_empty() {
            return Some(trimmed.to_owned());
        }
    }
    // Nested under result.messageId.
    if let Some(result) = payload.get("result") {
        if let Some(id) = result.get("messageId").and_then(|v| v.as_str()) {
            let trimmed = id.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_owned());
            }
        }
    }
    None
}

/// Build the JSON envelope for CLI output.
///
/// Source: `src/commands/message-format.ts` - `buildMessageCliJson`
pub fn build_message_cli_json(result: &MessageActionRunResult) -> MessageCliJsonEnvelope {
    MessageCliJsonEnvelope {
        action: result.action.clone(),
        channel: result.channel.clone(),
        dry_run: result.dry_run,
        handled_by: result.handled_by.clone(),
        payload: result.payload.clone(),
    }
}

/// Format a message action result as human-readable text lines.
///
/// Source: `src/commands/message-format.ts` - `formatMessageCliText`
pub fn format_message_cli_text(result: &MessageActionRunResult) -> Vec<String> {
    let kind = result.kind.as_deref().unwrap_or(&result.action);

    // Dry-run.
    if result.handled_by == "dry-run" {
        return vec![format!(
            "[dry-run] would run {} via {}",
            result.action, result.channel
        )];
    }

    // Broadcast.
    if kind == "broadcast" {
        return format_broadcast(result);
    }

    // Send.
    if kind == "send" {
        let msg_id = extract_message_id(&result.payload);
        let suffix = msg_id
            .map(|id| format!(" Message ID: {id}"))
            .unwrap_or_default();
        return vec![format!("Sent via {}.{suffix}", result.channel)];
    }

    // Poll.
    if kind == "poll" {
        let msg_id = extract_message_id(&result.payload);
        let suffix = msg_id
            .map(|id| format!(" Message ID: {id}"))
            .unwrap_or_default();
        return vec![format!("Poll sent via {}.{suffix}", result.channel)];
    }

    // React.
    if result.action == "react" {
        return format_react(&result.payload);
    }

    // Generic success.
    vec![format!("{} via {}.", result.action, result.channel)]
}

/// Format broadcast results.
///
/// Source: `src/commands/message-format.ts` - broadcast branch
fn format_broadcast(result: &MessageActionRunResult) -> Vec<String> {
    let results = result
        .payload
        .get("results")
        .and_then(|v| v.as_array())
        .cloned()
        .unwrap_or_default();

    let ok_count = results
        .iter()
        .filter(|r| r.get("ok").and_then(|v| v.as_bool()).unwrap_or(false))
        .count();
    let total = results.len();
    let failed = total - ok_count;

    let mut lines = vec![format!(
        "Broadcast complete ({ok_count}/{total} succeeded, {failed} failed)"
    )];

    for entry in results.iter().take(50) {
        let channel = entry.get("channel").and_then(|v| v.as_str()).unwrap_or("?");
        let ok = entry.get("ok").and_then(|v| v.as_bool()).unwrap_or(false);
        let status = if ok { "ok" } else { "error" };
        let error_text = if ok {
            String::new()
        } else {
            entry
                .get("error")
                .and_then(|v| v.as_str())
                .unwrap_or("unknown error")
                .to_owned()
        };
        let target = entry.get("to").and_then(|v| v.as_str()).unwrap_or("");
        lines.push(format!(
            "  {channel} {target} [{status}]{err}",
            err = if error_text.is_empty() {
                String::new()
            } else {
                format!(" {}", shorten_text(&error_text, 48))
            }
        ));
    }

    lines
}

/// Format react action results.
///
/// Source: `src/commands/message-format.ts` - react branch
fn format_react(payload: &serde_json::Value) -> Vec<String> {
    if let Some(added) = payload.get("added").and_then(|v| v.as_str()) {
        let trimmed = added.trim();
        if !trimmed.is_empty() {
            return vec![format!("Reaction added: {trimmed}")];
        }
    }

    if let Some(removed) = payload.get("removed").and_then(|v| v.as_str()) {
        let trimmed = removed.trim();
        if !trimmed.is_empty() {
            return vec![format!("Reaction removed: {trimmed}")];
        }
    }

    if let Some(removed_arr) = payload.get("removed").and_then(|v| v.as_array()) {
        let list: Vec<String> = removed_arr
            .iter()
            .filter_map(|v| v.as_str())
            .map(|s| s.trim().to_owned())
            .filter(|s| !s.is_empty())
            .collect();
        let suffix = if list.is_empty() {
            String::new()
        } else {
            format!(": {}", list.join(", "))
        };
        return vec![format!("Reactions removed{suffix}")];
    }

    vec!["Reaction updated.".to_owned()]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shorten_text_within_limit() {
        assert_eq!(shorten_text("hello", 10), "hello");
    }

    #[test]
    fn shorten_text_exact_limit() {
        assert_eq!(shorten_text("hello", 5), "hello");
    }

    #[test]
    fn shorten_text_over_limit() {
        let result = shorten_text("hello world", 6);
        assert_eq!(result, "hello\u{2026}");
    }

    #[test]
    fn extract_message_id_direct() {
        let payload = serde_json::json!({"messageId": "abc123"});
        assert_eq!(extract_message_id(&payload), Some("abc123".to_owned()));
    }

    #[test]
    fn extract_message_id_nested() {
        let payload = serde_json::json!({"result": {"messageId": "nested123"}});
        assert_eq!(extract_message_id(&payload), Some("nested123".to_owned()));
    }

    #[test]
    fn extract_message_id_missing() {
        let payload = serde_json::json!({"other": "value"});
        assert!(extract_message_id(&payload).is_none());
    }

    #[test]
    fn extract_message_id_empty() {
        let payload = serde_json::json!({"messageId": ""});
        assert!(extract_message_id(&payload).is_none());
    }

    #[test]
    fn build_json_envelope() {
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
        let envelope = build_message_cli_json(&result);
        assert_eq!(envelope.action, "send");
        assert_eq!(envelope.channel, "telegram");
        assert!(!envelope.dry_run);
    }

    #[test]
    fn format_dry_run() {
        let result = MessageActionRunResult {
            action: "send".to_owned(),
            channel: "telegram".to_owned(),
            dry_run: true,
            handled_by: "dry-run".to_owned(),
            kind: Some("send".to_owned()),
            payload: serde_json::Value::Null,
            send_result: None,
            poll_result: None,
        };
        let lines = format_message_cli_text(&result);
        assert_eq!(lines.len(), 1);
        assert!(lines[0].contains("[dry-run]"));
    }

    #[test]
    fn format_send() {
        let result = MessageActionRunResult {
            action: "send".to_owned(),
            channel: "whatsapp".to_owned(),
            dry_run: false,
            handled_by: "core".to_owned(),
            kind: Some("send".to_owned()),
            payload: serde_json::json!({"messageId": "msg-1"}),
            send_result: None,
            poll_result: None,
        };
        let lines = format_message_cli_text(&result);
        assert_eq!(lines.len(), 1);
        assert!(lines[0].contains("Sent via whatsapp"));
        assert!(lines[0].contains("msg-1"));
    }

    #[test]
    fn format_poll() {
        let result = MessageActionRunResult {
            action: "poll".to_owned(),
            channel: "discord".to_owned(),
            dry_run: false,
            handled_by: "core".to_owned(),
            kind: Some("poll".to_owned()),
            payload: serde_json::Value::Object(serde_json::Map::new()),
            send_result: None,
            poll_result: None,
        };
        let lines = format_message_cli_text(&result);
        assert!(lines[0].contains("Poll sent via discord"));
    }

    #[test]
    fn format_react_added() {
        let result = MessageActionRunResult {
            action: "react".to_owned(),
            channel: "slack".to_owned(),
            dry_run: false,
            handled_by: "plugin".to_owned(),
            kind: None,
            payload: serde_json::json!({"added": "thumbsup"}),
            send_result: None,
            poll_result: None,
        };
        let lines = format_message_cli_text(&result);
        assert!(lines[0].contains("Reaction added: thumbsup"));
    }

    #[test]
    fn format_react_removed() {
        let result = MessageActionRunResult {
            action: "react".to_owned(),
            channel: "slack".to_owned(),
            dry_run: false,
            handled_by: "plugin".to_owned(),
            kind: None,
            payload: serde_json::json!({"removed": "thumbsdown"}),
            send_result: None,
            poll_result: None,
        };
        let lines = format_message_cli_text(&result);
        assert!(lines[0].contains("Reaction removed: thumbsdown"));
    }

    #[test]
    fn format_generic_action() {
        let result = MessageActionRunResult {
            action: "search".to_owned(),
            channel: "discord".to_owned(),
            dry_run: false,
            handled_by: "plugin".to_owned(),
            kind: Some("search".to_owned()),
            payload: serde_json::Value::Object(serde_json::Map::new()),
            send_result: None,
            poll_result: None,
        };
        let lines = format_message_cli_text(&result);
        assert!(lines[0].contains("search via discord"));
    }
}
