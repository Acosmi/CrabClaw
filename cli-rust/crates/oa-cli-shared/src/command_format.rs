/// Command output formatting.
///
/// Formats CLI command strings for display, injecting the active
/// profile `--profile` flag when the `OPENACOSMI_PROFILE` environment
/// variable is set.
///
/// Source: `src/cli/command-format.ts`
use crate::binary_name::current_cli_name;
use regex::Regex;
use std::sync::LazyLock;

/// Regex matching a CLI invocation prefix (e.g. `npx openacosmi`, `crabclaw`).
///
/// Source: `src/cli/command-format.ts` – `CLI_PREFIX_RE`
static CLI_PREFIX_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^(?:(pnpm|npm|bunx|npx)\s+)?(?:openacosmi|crabclaw)\b")
        .expect("CLI_PREFIX_RE is a valid regex")
});

/// Regex detecting an existing `--profile` flag.
///
/// Source: `src/cli/command-format.ts` – `PROFILE_FLAG_RE`
static PROFILE_FLAG_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"(?:^|\s)--profile(?:\s|=|$)").expect("PROFILE_FLAG_RE is a valid regex")
});

/// Regex detecting an existing `--dev` flag.
///
/// Source: `src/cli/command-format.ts` – `DEV_FLAG_RE`
static DEV_FLAG_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"(?:^|\s)--dev(?:\s|$)").expect("DEV_FLAG_RE is a valid regex"));

/// Normalize a profile name: trim whitespace, return `None` for empty strings.
///
/// Source: `src/cli/profile-utils.ts` – `normalizeProfileName`
fn normalize_profile_name(raw: Option<&str>) -> Option<String> {
    let trimmed = raw?.trim();
    if trimmed.is_empty() {
        return None;
    }
    Some(trimmed.to_string())
}

fn resolve_profile_env(primary_env: Option<&str>, legacy_env: Option<&str>) -> Option<String> {
    normalize_profile_name(primary_env).or_else(|| normalize_profile_name(legacy_env))
}

/// Format a CLI command string for display.
///
/// When the `OPENACOSMI_PROFILE` environment variable is set and the
/// command does not already include `--profile` or `--dev`, the
/// `--profile <name>` flag is injected immediately after the CLI prefix.
///
/// Source: `src/cli/command-format.ts` – `formatCliCommand`
pub fn format_cli_command(command: &str) -> String {
    format_cli_command_with_env_and_cli(
        command,
        resolve_profile_env(
            std::env::var("CRABCLAW_PROFILE").ok().as_deref(),
            std::env::var("OPENACOSMI_PROFILE").ok().as_deref(),
        )
        .as_deref(),
        current_cli_name(),
    )
}

/// Format a CLI command string with an explicit profile value.
///
/// This is the testable inner implementation of [`format_cli_command`].
///
/// Source: `src/cli/command-format.ts` – `formatCliCommand`
pub fn format_cli_command_with_env(command: &str, profile_env: Option<&str>) -> String {
    format_cli_command_with_env_and_cli(command, profile_env, current_cli_name())
}

/// Format a CLI command string with an explicit profile value and CLI name.
pub fn format_cli_command_with_env_and_cli(
    command: &str,
    profile_env: Option<&str>,
    cli_name: &str,
) -> String {
    if !CLI_PREFIX_RE.is_match(command) {
        return command.to_string();
    }

    let rewritten = CLI_PREFIX_RE
        .replace(command, |caps: &regex::Captures<'_>| match caps.get(1) {
            Some(pm) => format!("{} {cli_name}", pm.as_str()),
            None => cli_name.to_string(),
        })
        .into_owned();

    let profile = match normalize_profile_name(profile_env) {
        Some(p) => p,
        None => return rewritten,
    };

    if PROFILE_FLAG_RE.is_match(&rewritten) || DEV_FLAG_RE.is_match(&rewritten) {
        return rewritten;
    }

    CLI_PREFIX_RE
        .replace(&rewritten, |caps: &regex::Captures<'_>| {
            format!("{} --profile {profile}", &caps[0])
        })
        .into_owned()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn injects_profile_into_bare_command() {
        let result = format_cli_command_with_env_and_cli(
            "openacosmi doctor --fix",
            Some("staging"),
            "crabclaw",
        );
        assert_eq!(result, "crabclaw --profile staging doctor --fix");
    }

    #[test]
    fn injects_profile_into_npx_command() {
        let result =
            format_cli_command_with_env_and_cli("npx openacosmi status", Some("dev"), "crabclaw");
        assert_eq!(result, "npx crabclaw --profile dev status");
    }

    #[test]
    fn rewrites_name_without_profile() {
        let result = format_cli_command_with_env_and_cli("openacosmi status", None, "crabclaw");
        assert_eq!(result, "crabclaw status");
    }

    #[test]
    fn no_injection_with_empty_profile() {
        let result =
            format_cli_command_with_env_and_cli("openacosmi status", Some("  "), "crabclaw");
        assert_eq!(result, "crabclaw status");
    }

    #[test]
    fn no_injection_when_profile_flag_present() {
        let result = format_cli_command_with_env_and_cli(
            "openacosmi --profile prod status",
            Some("staging"),
            "crabclaw",
        );
        assert_eq!(result, "crabclaw --profile prod status");
    }

    #[test]
    fn no_injection_when_dev_flag_present() {
        let result = format_cli_command_with_env_and_cli(
            "openacosmi --dev status",
            Some("staging"),
            "crabclaw",
        );
        assert_eq!(result, "crabclaw --dev status");
    }

    #[test]
    fn no_injection_for_non_cli_command() {
        let result = format_cli_command_with_env_and_cli("ls -la", Some("staging"), "crabclaw");
        assert_eq!(result, "ls -la");
    }

    #[test]
    fn normalize_profile_name_trims() {
        assert_eq!(
            normalize_profile_name(Some("  prod  ")),
            Some("prod".to_string())
        );
    }

    #[test]
    fn normalize_profile_name_none_for_empty() {
        assert_eq!(normalize_profile_name(Some("")), None);
        assert_eq!(normalize_profile_name(None), None);
    }

    #[test]
    fn resolve_profile_env_prefers_crabclaw() {
        let resolved = resolve_profile_env(Some("crab"), Some("open"));
        assert_eq!(resolved.as_deref(), Some("crab"));
    }
}
