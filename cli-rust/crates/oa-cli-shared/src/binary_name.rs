//! CLI binary name helpers.
//!
//! Keeps the legacy `openacosmi` compatibility name while allowing the new
//! `crabclaw` binary to coexist. Formatting/help code should prefer these
//! helpers instead of hard-coding one binary name.

use std::path::Path;

/// Environment variable used to pin the current CLI display name.
pub const CLI_NAME_ENV: &str = "OPENACOSMI_CLI_NAME";
/// Preferred environment variable used to pin the current CLI display name.
pub const PRIMARY_CLI_NAME_ENV: &str = "CRABCLAW_CLI_NAME";

/// Legacy compatibility CLI name.
pub const LEGACY_CLI_NAME: &str = "openacosmi";

/// Preferred new CLI name.
pub const PRIMARY_CLI_NAME: &str = "crabclaw";

fn normalize_cli_name(raw: &str) -> Option<&'static str> {
    let basename = Path::new(raw)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .unwrap_or(raw);
    let trimmed = basename.trim();
    if trimmed.is_empty() {
        return None;
    }
    let normalized = trimmed
        .strip_suffix(".exe")
        .unwrap_or(trimmed)
        .to_ascii_lowercase();
    match normalized.as_str() {
        PRIMARY_CLI_NAME => Some(PRIMARY_CLI_NAME),
        LEGACY_CLI_NAME => Some(LEGACY_CLI_NAME),
        _ => None,
    }
}

/// Resolve the effective CLI name from an explicit env override and argv[0].
pub fn resolve_cli_name(env_value: Option<&str>, arg0: Option<&str>) -> &'static str {
    if let Some(value) = env_value.and_then(normalize_cli_name) {
        return value;
    }
    if let Some(value) = arg0.and_then(normalize_cli_name) {
        return value;
    }
    LEGACY_CLI_NAME
}

/// Resolve the current CLI name for user-facing output.
pub fn current_cli_name() -> &'static str {
    let env_value = std::env::var(PRIMARY_CLI_NAME_ENV)
        .ok()
        .or_else(|| std::env::var(CLI_NAME_ENV).ok());
    let arg0 = std::env::args().next();
    resolve_cli_name(env_value.as_deref(), arg0.as_deref())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolves_from_env_override() {
        assert_eq!(
            resolve_cli_name(Some("crabclaw"), Some("/usr/local/bin/openacosmi")),
            PRIMARY_CLI_NAME
        );
    }

    #[test]
    fn resolves_from_arg0_when_env_missing() {
        assert_eq!(
            resolve_cli_name(None, Some("/usr/local/bin/crabclaw")),
            PRIMARY_CLI_NAME
        );
        assert_eq!(
            resolve_cli_name(None, Some("C:\\Tools\\openacosmi.exe")),
            LEGACY_CLI_NAME
        );
    }

    #[test]
    fn falls_back_to_legacy_name() {
        assert_eq!(resolve_cli_name(None, Some("oa-cli")), LEGACY_CLI_NAME);
        assert_eq!(resolve_cli_name(Some("unknown"), None), LEGACY_CLI_NAME);
    }

    #[test]
    fn primary_cli_env_name_matches_brand() {
        assert_eq!(PRIMARY_CLI_NAME_ENV, "CRABCLAW_CLI_NAME");
    }
}
