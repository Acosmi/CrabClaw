/// Authentication profile health checks and repair.
///
/// Detects deprecated CLI auth profiles (Claude CLI, Codex CLI),
/// repairs OAuth profile ID mismatches, and reports auth cooldowns
/// and expiry status.
///
/// Source: `src/commands/doctor-auth.ts`
use std::collections::{HashMap, HashSet};

use oa_cli_shared::command_format::format_cli_command;
use oa_terminal::note::note;
use oa_types::auth::AuthConfig;
use oa_types::config::OpenAcosmiConfig;

use crate::prompter::DoctorPrompter;

/// Well-known deprecated external CLI profile IDs.
///
/// Source: `src/commands/doctor-auth.ts`
const CLAUDE_CLI_PROFILE_ID: &str = "claude-cli:anthropic";

/// Source: `src/commands/doctor-auth.ts`
const CODEX_CLI_PROFILE_ID: &str = "codex-cli:openai-codex";

// ---------------------------------------------------------------------------
// OAuth profile-id mismatch repair
// ---------------------------------------------------------------------------

/// Attempt to detect and repair an Anthropic OAuth profile-ID mismatch.
///
/// Returns the (possibly updated) config.
///
/// Source: `src/commands/doctor-auth.ts` — `maybeRepairAnthropicOAuthProfileId`
pub async fn maybe_repair_anthropic_oauth_profile_id(
    cfg: OpenAcosmiConfig,
    prompter: &mut DoctorPrompter,
) -> OpenAcosmiConfig {
    // In the Rust port, the auth-profile store is not yet wired.
    // Stub: return the config unchanged.
    let _ = prompter;
    cfg
}

// ---------------------------------------------------------------------------
// Deprecated CLI auth profile removal
// ---------------------------------------------------------------------------

/// Prune entries from `auth.order` that reference the given profile IDs.
///
/// Source: `src/commands/doctor-auth.ts` — `pruneAuthOrder`
fn prune_auth_order(
    order: Option<&HashMap<String, Vec<String>>>,
    profile_ids: &HashSet<String>,
) -> (Option<HashMap<String, Vec<String>>>, bool) {
    let Some(order) = order else {
        return (None, false);
    };
    let mut changed = false;
    let mut next: HashMap<String, Vec<String>> = HashMap::new();
    for (provider, list) in order {
        let filtered: Vec<String> = list
            .iter()
            .filter(|id| !profile_ids.contains(id.as_str()))
            .cloned()
            .collect();
        if filtered.len() != list.len() {
            changed = true;
        }
        if !filtered.is_empty() {
            next.insert(provider.clone(), filtered);
        }
    }
    let result = if next.is_empty() { None } else { Some(next) };
    (result, changed)
}

/// Remove deprecated auth profiles from config and return updated config.
///
/// Source: `src/commands/doctor-auth.ts` — `pruneAuthProfiles`
fn prune_auth_profiles(
    cfg: &OpenAcosmiConfig,
    profile_ids: &HashSet<String>,
) -> (OpenAcosmiConfig, bool) {
    let profiles = cfg.auth.as_ref().and_then(|a| a.profiles.as_ref());
    let order = cfg.auth.as_ref().and_then(|a| a.order.as_ref());

    let mut changed = false;
    let next_profiles = profiles.map(|p| {
        let mut cloned = p.clone();
        for id in profile_ids {
            if cloned.remove(id).is_some() {
                changed = true;
            }
        }
        cloned
    });

    let (pruned_order, order_changed) = prune_auth_order(order, profile_ids);
    if order_changed {
        changed = true;
    }

    if !changed {
        return (cfg.clone(), false);
    }

    let next_auth =
        if next_profiles.as_ref().is_some_and(|p| !p.is_empty()) || pruned_order.is_some() {
            Some(AuthConfig {
                profiles: next_profiles.filter(|p| !p.is_empty()),
                order: pruned_order,
                cooldowns: cfg.auth.as_ref().and_then(|a| a.cooldowns.clone()),
            })
        } else {
            None
        };

    let mut next_cfg = cfg.clone();
    next_cfg.auth = next_auth;
    (next_cfg, true)
}

/// Detect and offer to remove deprecated external CLI auth profiles.
///
/// Source: `src/commands/doctor-auth.ts` — `maybeRemoveDeprecatedCliAuthProfiles`
pub async fn maybe_remove_deprecated_cli_auth_profiles(
    cfg: OpenAcosmiConfig,
    prompter: &mut DoctorPrompter,
) -> OpenAcosmiConfig {
    let profiles = cfg.auth.as_ref().and_then(|a| a.profiles.as_ref());
    let mut deprecated = HashSet::new();

    if profiles.is_some_and(|p| p.contains_key(CLAUDE_CLI_PROFILE_ID)) {
        deprecated.insert(CLAUDE_CLI_PROFILE_ID.to_string());
    }
    if profiles.is_some_and(|p| p.contains_key(CODEX_CLI_PROFILE_ID)) {
        deprecated.insert(CODEX_CLI_PROFILE_ID.to_string());
    }

    if deprecated.is_empty() {
        return cfg;
    }

    let mut lines =
        vec!["Deprecated external CLI auth profiles detected (no longer supported):".to_string()];
    if deprecated.contains(CLAUDE_CLI_PROFILE_ID) {
        lines.push(format!(
            "- {CLAUDE_CLI_PROFILE_ID} (Anthropic): use setup-token -> {}",
            format_cli_command("crabclaw models auth setup-token")
        ));
    }
    if deprecated.contains(CODEX_CLI_PROFILE_ID) {
        lines.push(format!(
            "- {CODEX_CLI_PROFILE_ID} (OpenAI Codex): use OAuth -> {}",
            format_cli_command("crabclaw models auth login --provider openai-codex")
        ));
    }
    note(&lines.join("\n"), Some("Auth profiles"));

    let should_remove = prompter
        .confirm_repair("Remove deprecated CLI auth profiles now?", true)
        .await;
    if !should_remove {
        return cfg;
    }

    let (pruned_cfg, changed) = prune_auth_profiles(&cfg, &deprecated);
    if changed {
        let removal_lines: Vec<String> = deprecated
            .iter()
            .map(|id| format!("- removed {id} from config"))
            .collect();
        note(&removal_lines.join("\n"), Some("Doctor changes"));
    }
    pruned_cfg
}

// ---------------------------------------------------------------------------
// Auth profile health report
// ---------------------------------------------------------------------------

/// Describe auth issue for a profile.
///
/// Source: `src/commands/doctor-auth.ts` — `AuthIssue`
#[derive(Debug, Clone)]
pub struct AuthIssue {
    /// The profile id.
    pub profile_id: String,
    /// Provider key.
    pub provider: String,
    /// Status string (e.g. "expired", "expiring", "missing").
    pub status: String,
    /// Milliseconds remaining until expiry (if known).
    pub remaining_ms: Option<u64>,
}

/// Format a human-readable hint for an auth issue.
///
/// Source: `src/commands/doctor-auth.ts` — `formatAuthIssueHint`
fn format_auth_issue_hint(issue: &AuthIssue) -> String {
    if issue.provider == "anthropic" && issue.profile_id == CLAUDE_CLI_PROFILE_ID {
        return format!(
            "Deprecated profile. Use {} or {}.",
            format_cli_command("crabclaw models auth setup-token"),
            format_cli_command("crabclaw configure")
        );
    }
    if issue.provider == "openai-codex" && issue.profile_id == CODEX_CLI_PROFILE_ID {
        return format!(
            "Deprecated profile. Use {} or {}.",
            format_cli_command("crabclaw models auth login --provider openai-codex"),
            format_cli_command("crabclaw configure")
        );
    }
    format!(
        "Re-auth via `{}` or `{}`.",
        format_cli_command("crabclaw configure"),
        format_cli_command("crabclaw onboard")
    )
}

/// Format one line of auth issue output.
///
/// Source: `src/commands/doctor-auth.ts` — `formatAuthIssueLine`
pub fn format_auth_issue_line(issue: &AuthIssue) -> String {
    let remaining = issue
        .remaining_ms
        .map(|ms| format!(" ({ms}ms)"))
        .unwrap_or_default();
    let hint = format_auth_issue_hint(issue);
    format!(
        "- {}: {}{} -- {}",
        issue.profile_id, issue.status, remaining, hint
    )
}

/// Report auth profile health: cooldowns, expiring tokens, missing tokens.
///
/// Source: `src/commands/doctor-auth.ts` — `noteAuthProfileHealth`
pub async fn note_auth_profile_health(_cfg: &OpenAcosmiConfig, _prompter: &mut DoctorPrompter) {
    // Stub: auth-profile store is not yet wired in the Rust port.
    // When wired, this will check cooldowns, detect expired tokens,
    // and offer to refresh OAuth tokens.
}

#[cfg(test)]
mod tests {
    use super::*;
    use oa_types::auth::AuthProfileConfig;
    use oa_types::auth::AuthProfileMode;

    #[test]
    fn prune_auth_order_removes_matching_ids() {
        let mut order = HashMap::new();
        order.insert(
            "anthropic".to_string(),
            vec!["a".to_string(), "b".to_string()],
        );
        let ids: HashSet<String> = ["a".to_string()].into_iter().collect();
        let (result, changed) = prune_auth_order(Some(&order), &ids);
        assert!(changed);
        let result = result.as_ref().expect("should have result");
        assert_eq!(result.get("anthropic").map(Vec::len), Some(1));
    }

    #[test]
    fn prune_auth_order_noop_when_no_match() {
        let mut order = HashMap::new();
        order.insert("anthropic".to_string(), vec!["x".to_string()]);
        let ids: HashSet<String> = ["a".to_string()].into_iter().collect();
        let (_, changed) = prune_auth_order(Some(&order), &ids);
        assert!(!changed);
    }

    #[test]
    fn prune_auth_profiles_removes_deprecated() {
        let mut profiles = HashMap::new();
        profiles.insert(
            CLAUDE_CLI_PROFILE_ID.to_string(),
            AuthProfileConfig {
                provider: "anthropic".to_string(),
                mode: AuthProfileMode::Token,
                email: None,
            },
        );
        profiles.insert(
            "keep-me".to_string(),
            AuthProfileConfig {
                provider: "other".to_string(),
                mode: AuthProfileMode::ApiKey,
                email: None,
            },
        );
        let mut cfg = OpenAcosmiConfig::default();
        cfg.auth = Some(AuthConfig {
            profiles: Some(profiles),
            order: None,
            cooldowns: None,
        });

        let ids: HashSet<String> = [CLAUDE_CLI_PROFILE_ID.to_string()].into_iter().collect();
        let (next, changed) = prune_auth_profiles(&cfg, &ids);
        assert!(changed);
        let remaining = next
            .auth
            .as_ref()
            .and_then(|a| a.profiles.as_ref())
            .expect("profiles should remain");
        assert!(!remaining.contains_key(CLAUDE_CLI_PROFILE_ID));
        assert!(remaining.contains_key("keep-me"));
    }

    #[test]
    fn format_auth_issue_line_includes_hint() {
        let issue = AuthIssue {
            profile_id: "test:provider".to_string(),
            provider: "provider".to_string(),
            status: "expired".to_string(),
            remaining_ms: Some(3600),
        };
        let line = format_auth_issue_line(&issue);
        assert!(line.contains("expired"));
        assert!(line.contains("3600ms"));
        assert!(line.contains("Re-auth"));
    }
}
