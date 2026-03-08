/// Environment variable utilities.
///
/// Provides helpers for checking boolean-like env var values, and
/// normalizing environment variables (e.g., ZAI key aliases).
///
/// Source: `src/infra/env.ts`
use tracing::info;

/// Truthy string values recognized by [`is_truthy_env_value`].
const TRUTHY_VALUES: &[&str] = &["true", "1", "yes", "on"];

/// Falsy string values recognized for completeness.
const FALSY_VALUES: &[&str] = &["false", "0", "no", "off"];

/// Check if an environment variable value is truthy.
///
/// Recognized truthy values (case-insensitive): `1`, `true`, `yes`, `on`.
///
/// Returns `false` for `None`, empty strings, and unrecognized values.
pub fn is_truthy_env_value(value: Option<&str>) -> bool {
    match value {
        Some(v) => {
            let normalized = v.trim().to_lowercase();
            TRUTHY_VALUES.contains(&normalized.as_str())
        }
        None => false,
    }
}

/// Parse a string value into an optional boolean.
///
/// Returns `Some(true)` for truthy values, `Some(false)` for falsy values,
/// and `None` for unrecognized or empty values.
///
/// Recognized truthy values: `1`, `true`, `yes`, `on`.
/// Recognized falsy values: `0`, `false`, `no`, `off`.
pub fn parse_boolean_value(value: Option<&str>) -> Option<bool> {
    let v = value?;
    let normalized = v.trim().to_lowercase();
    if normalized.is_empty() {
        return None;
    }
    if TRUTHY_VALUES.contains(&normalized.as_str()) {
        return Some(true);
    }
    if FALSY_VALUES.contains(&normalized.as_str()) {
        return Some(false);
    }
    None
}

/// Return the first non-empty environment variable value from a prioritized list.
pub fn preferred_env_value(keys: &[&str]) -> Option<String> {
    for key in keys {
        if let Ok(value) = std::env::var(key) {
            let trimmed = value.trim().to_string();
            if !trimmed.is_empty() {
                return Some(trimmed);
            }
        }
    }
    None
}

#[cfg(test)]
fn preferred_env_value_from_map(
    values: &std::collections::HashMap<&str, &str>,
    keys: &[&str],
) -> Option<String> {
    for key in keys {
        if let Some(value) = values.get(key) {
            let trimmed = value.trim().to_string();
            if !trimmed.is_empty() {
                return Some(trimmed);
            }
        }
    }
    None
}

/// Format an environment variable value for logging, optionally redacting it.
///
/// If `redact` is true, returns `<redacted>`. Otherwise, collapses whitespace
/// and truncates to 160 characters.
fn format_env_value(value: &str, redact: bool) -> String {
    if redact {
        return "<redacted>".to_owned();
    }
    let single_line: String = value.split_whitespace().collect::<Vec<_>>().join(" ");
    if single_line.len() <= 160 {
        single_line
    } else {
        format!("{}...", &single_line[..160])
    }
}

/// Log an accepted environment variable option.
///
/// Logs the env var key, value (possibly redacted), and description at info level.
/// Skips logging if the value is empty or unset.
pub fn log_accepted_env_option(key: &str, description: &str, redact: bool) {
    let value = std::env::var(key).ok();
    let raw = match &value {
        Some(v) if !v.trim().is_empty() => v.as_str(),
        _ => return,
    };
    info!(
        "env: {}={} ({})",
        key,
        format_env_value(raw, redact),
        description
    );
}

/// Normalize ZAI environment variables.
///
/// If `ZAI_API_KEY` is not set (or empty) but `Z_AI_API_KEY` is set,
/// copies the value of `Z_AI_API_KEY` to `ZAI_API_KEY`.
///
/// # Safety
///
/// This function uses `std::env::set_var` which is unsafe in Rust 2024 edition
/// because modifying environment variables is not thread-safe. Callers must
/// ensure this is called before spawning threads (e.g., during startup).
pub fn normalize_zai_env() {
    let zai_key = std::env::var("ZAI_API_KEY")
        .ok()
        .filter(|v| !v.trim().is_empty());

    if zai_key.is_none() {
        if let Ok(z_ai_key) = std::env::var("Z_AI_API_KEY") {
            if !z_ai_key.trim().is_empty() {
                // SAFETY: This is called during single-threaded startup,
                // before any threads are spawned.
                unsafe {
                    std::env::set_var("ZAI_API_KEY", &z_ai_key);
                }
            }
        }
    }
}

/// Normalize all environment variables.
///
/// Currently this normalizes ZAI env vars. Additional normalization
/// rules may be added in the future.
///
/// # Safety
///
/// Must be called during single-threaded startup before spawning threads,
/// as it modifies environment variables.
pub fn normalize_env() {
    normalize_zai_env();
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn test_is_truthy_env_value() {
        assert!(is_truthy_env_value(Some("1")));
        assert!(is_truthy_env_value(Some("true")));
        assert!(is_truthy_env_value(Some("True")));
        assert!(is_truthy_env_value(Some("TRUE")));
        assert!(is_truthy_env_value(Some("yes")));
        assert!(is_truthy_env_value(Some("on")));
        assert!(is_truthy_env_value(Some("  true  ")));
    }

    #[test]
    fn test_is_not_truthy_env_value() {
        assert!(!is_truthy_env_value(None));
        assert!(!is_truthy_env_value(Some("")));
        assert!(!is_truthy_env_value(Some("0")));
        assert!(!is_truthy_env_value(Some("false")));
        assert!(!is_truthy_env_value(Some("no")));
        assert!(!is_truthy_env_value(Some("off")));
        assert!(!is_truthy_env_value(Some("random")));
    }

    #[test]
    fn test_parse_boolean_value() {
        assert_eq!(parse_boolean_value(Some("true")), Some(true));
        assert_eq!(parse_boolean_value(Some("1")), Some(true));
        assert_eq!(parse_boolean_value(Some("yes")), Some(true));
        assert_eq!(parse_boolean_value(Some("on")), Some(true));

        assert_eq!(parse_boolean_value(Some("false")), Some(false));
        assert_eq!(parse_boolean_value(Some("0")), Some(false));
        assert_eq!(parse_boolean_value(Some("no")), Some(false));
        assert_eq!(parse_boolean_value(Some("off")), Some(false));

        assert_eq!(parse_boolean_value(None), None);
        assert_eq!(parse_boolean_value(Some("")), None);
        assert_eq!(parse_boolean_value(Some("  ")), None);
        assert_eq!(parse_boolean_value(Some("maybe")), None);
    }

    #[test]
    fn test_format_env_value_redacted() {
        assert_eq!(format_env_value("secret", true), "<redacted>");
    }

    #[test]
    fn test_format_env_value_short() {
        assert_eq!(format_env_value("hello world", false), "hello world");
    }

    #[test]
    fn test_format_env_value_long() {
        let long_value = "a ".repeat(200);
        let formatted = format_env_value(&long_value, false);
        assert!(formatted.len() <= 164); // 160 + "..."
        assert!(formatted.ends_with("..."));
    }

    #[test]
    fn test_normalize_zai_env() {
        // Save existing values
        let saved_zai = std::env::var("ZAI_API_KEY").ok();
        let saved_z_ai = std::env::var("Z_AI_API_KEY").ok();

        // SAFETY: Test environment, single test thread assumed.
        unsafe {
            std::env::remove_var("ZAI_API_KEY");
            std::env::set_var("Z_AI_API_KEY", "test-key");
        }

        normalize_zai_env();

        assert_eq!(
            std::env::var("ZAI_API_KEY").ok().as_deref(),
            Some("test-key")
        );

        // Restore
        unsafe {
            std::env::remove_var("ZAI_API_KEY");
            std::env::remove_var("Z_AI_API_KEY");
            if let Some(v) = saved_zai {
                std::env::set_var("ZAI_API_KEY", v);
            }
            if let Some(v) = saved_z_ai {
                std::env::set_var("Z_AI_API_KEY", v);
            }
        }
    }

    #[test]
    fn test_preferred_env_value_from_map_prefers_first_non_empty() {
        let values = HashMap::from([
            ("OPENACOSMI_UPDATE_IN_PROGRESS", "1"),
            ("CRABCLAW_UPDATE_IN_PROGRESS", "0"),
        ]);
        let preferred = preferred_env_value_from_map(
            &values,
            &[
                "CRABCLAW_UPDATE_IN_PROGRESS",
                "OPENACOSMI_UPDATE_IN_PROGRESS",
            ],
        );
        assert_eq!(preferred.as_deref(), Some("0"));
    }
}
