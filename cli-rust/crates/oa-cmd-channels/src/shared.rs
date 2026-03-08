/// Shared helpers for channel commands: config validation, label formatting,
/// and wizard detection.
///
/// Source: `src/commands/channels/shared.ts`
use anyhow::Result;

use oa_channels::registry::{ChatChannelId, get_chat_channel_meta};
use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::read_config_file_snapshot;
use oa_routing::session_key::DEFAULT_ACCOUNT_ID;
use oa_types::config::OpenAcosmiConfig;

/// Validate the config file and return the parsed config.
///
/// If the config exists but is invalid, prints the issues and exits.
/// Returns `Ok(None)` when no config file exists (treated as valid-empty).
///
/// Source: `src/commands/channels/shared.ts` — `requireValidConfig`
pub async fn require_valid_config() -> Result<Option<OpenAcosmiConfig>> {
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
        eprintln!("Config invalid:\n{issues}");
        eprintln!(
            "Fix the config or run {}.",
            format_cli_command("crabclaw doctor")
        );
        std::process::exit(1);
    }
    Ok(Some(snapshot.config))
}

/// Format an account label, appending the display name in parentheses when present.
///
/// Source: `src/commands/channels/shared.ts` — `formatAccountLabel`
#[must_use]
pub fn format_account_label(account_id: &str, name: Option<&str>) -> String {
    let base = if account_id.is_empty() {
        DEFAULT_ACCOUNT_ID
    } else {
        account_id
    };
    match name {
        Some(n) if !n.trim().is_empty() => format!("{base} ({})", n.trim()),
        _ => base.to_owned(),
    }
}

/// Return the human-readable label for a channel, falling back to the raw ID.
///
/// Source: `src/commands/channels/shared.ts` — `channelLabel`
#[must_use]
pub fn channel_label(channel: ChatChannelId) -> &'static str {
    get_chat_channel_meta(channel).label
}

/// Format a combined channel + account label with optional styling functions.
///
/// Source: `src/commands/channels/shared.ts` — `formatChannelAccountLabel`
#[must_use]
pub fn format_channel_account_label(
    channel: ChatChannelId,
    account_id: &str,
    name: Option<&str>,
    channel_style: Option<fn(&str) -> String>,
    account_style: Option<fn(&str) -> String>,
) -> String {
    let channel_text = channel_label(channel);
    let account_text = format_account_label(account_id, name);
    let styled_channel = match channel_style {
        Some(f) => f(channel_text),
        None => channel_text.to_owned(),
    };
    let styled_account = match account_style {
        Some(f) => f(&account_text),
        None => account_text,
    };
    format!("{styled_channel} {styled_account}")
}

/// Determine whether a wizard (interactive) flow should be used.
///
/// Returns `true` when no CLI flags were provided (i.e. `has_flags == false`).
///
/// Source: `src/commands/channels/shared.ts` — `shouldUseWizard`
#[must_use]
pub fn should_use_wizard(has_flags: Option<bool>) -> bool {
    has_flags == Some(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_account_label_without_name() {
        assert_eq!(format_account_label("default", None), "default");
    }

    #[test]
    fn format_account_label_with_name() {
        assert_eq!(
            format_account_label("acct1", Some("My Bot")),
            "acct1 (My Bot)"
        );
    }

    #[test]
    fn format_account_label_empty_name_ignored() {
        assert_eq!(format_account_label("acct1", Some("  ")), "acct1");
    }

    #[test]
    fn format_account_label_empty_id_falls_back() {
        assert_eq!(format_account_label("", None), "default");
    }

    #[test]
    fn channel_label_returns_meta_label() {
        assert_eq!(channel_label(ChatChannelId::Discord), "Discord");
        assert_eq!(channel_label(ChatChannelId::Telegram), "Telegram");
        assert_eq!(channel_label(ChatChannelId::Slack), "Slack");
    }

    #[test]
    fn format_channel_account_label_no_style() {
        let label =
            format_channel_account_label(ChatChannelId::Discord, "default", None, None, None);
        assert_eq!(label, "Discord default");
    }

    #[test]
    fn format_channel_account_label_with_name() {
        let label = format_channel_account_label(
            ChatChannelId::Telegram,
            "acct1",
            Some("My Bot"),
            None,
            None,
        );
        assert_eq!(label, "Telegram acct1 (My Bot)");
    }

    #[test]
    fn should_use_wizard_no_flags() {
        assert!(should_use_wizard(Some(false)));
    }

    #[test]
    fn should_use_wizard_has_flags() {
        assert!(!should_use_wizard(Some(true)));
    }

    #[test]
    fn should_use_wizard_none() {
        assert!(!should_use_wizard(None));
    }
}
