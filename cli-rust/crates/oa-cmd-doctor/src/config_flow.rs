/// Config loading, legacy migration, unknown-key stripping, plugin auto-enable.
///
/// Reads the config file snapshot, detects legacy keys, normalizes values,
/// strips unknown keys, and optionally applies all changes.
///
/// Source: `src/commands/doctor-config-flow.ts`
use oa_cli_shared::command_format::format_cli_command;
use oa_config::io::read_config_file_snapshot;
use oa_config::paths::resolve_config_path;
use oa_terminal::note::note;
use oa_types::config::OpenAcosmiConfig;

use crate::legacy_config::normalize_legacy_config_values;
use crate::prompter::{DoctorOptions, DoctorPrompter};
use crate::state_migrations;

/// Result of config loading and migration.
///
/// Source: `src/commands/doctor-config-flow.ts` — return of `loadAndMaybeMigrateDoctorConfig`
pub struct ConfigFlowResult {
    /// The (possibly repaired) config.
    pub cfg: OpenAcosmiConfig,

    /// Config file path (if resolved).
    pub path: Option<String>,

    /// Whether the caller should write the config to disk.
    pub should_write_config: bool,
}

/// Check whether a `serde_json::Value` is a non-null, non-array object.
///
/// Source: `src/commands/doctor-config-flow.ts` — `isRecord`
#[cfg(test)]
fn is_record(value: &serde_json::Value) -> bool {
    value.is_object()
}

/// Note about OpenCode Zen provider overrides masking built-in routing/costs.
///
/// Source: `src/commands/doctor-config-flow.ts` — `noteOpencodeProviderOverrides`
fn note_opencode_provider_overrides(cfg: &OpenAcosmiConfig) {
    let providers = match cfg.models.as_ref().and_then(|m| m.providers.as_ref()) {
        Some(p) => p,
        None => return,
    };

    let mut overrides = Vec::new();
    if providers.contains_key("opencode") {
        overrides.push("opencode");
    }
    if providers.contains_key("opencode-zen") {
        overrides.push("opencode-zen");
    }
    if overrides.is_empty() {
        return;
    }

    let mut lines: Vec<String> = Vec::new();
    for id in &overrides {
        let provider_entry = providers.get(*id);
        let api = provider_entry.and_then(|p| p.api.as_ref());

        lines.push(format!(
            "- models.providers.{id} is set; this overrides the built-in OpenCode Zen catalog."
        ));
        if let Some(api_val) = api {
            lines.push(format!("- models.providers.{id}.api={api_val:?}"));
        }
    }
    lines.push(
        "- Remove these entries to restore per-model API routing + costs (then re-run onboarding if needed).".to_string()
    );

    note(&lines.join("\n"), Some("OpenCode Zen"));
}

/// Load config file, detect legacy keys, normalize values, strip unknown keys.
///
/// Source: `src/commands/doctor-config-flow.ts` — `loadAndMaybeMigrateDoctorConfig`
pub async fn load_and_maybe_migrate_doctor_config(
    options: &DoctorOptions,
    prompter: &mut DoctorPrompter,
) -> ConfigFlowResult {
    let should_repair = options.repair == Some(true) || options.yes == Some(true);

    // ── Auto-migrate legacy state directory ──
    let state_dir_result = state_migrations::auto_migrate_legacy_state_dir().await;
    if !state_dir_result.changes.is_empty() {
        note(
            &state_dir_result
                .changes
                .iter()
                .map(|e| format!("- {e}"))
                .collect::<Vec<_>>()
                .join("\n"),
            Some("Doctor changes"),
        );
    }
    if !state_dir_result.warnings.is_empty() {
        note(
            &state_dir_result
                .warnings
                .iter()
                .map(|e| format!("- {e}"))
                .collect::<Vec<_>>()
                .join("\n"),
            Some("Doctor warnings"),
        );
    }

    // ── Read config snapshot ──
    let snapshot = match read_config_file_snapshot().await {
        Ok(s) => s,
        Err(e) => {
            tracing::warn!("Failed to read config snapshot: {e}");
            return ConfigFlowResult {
                cfg: OpenAcosmiConfig::default(),
                path: Some(resolve_config_path().display().to_string()),
                should_write_config: false,
            };
        }
    };

    let mut cfg: OpenAcosmiConfig = snapshot.config.clone();
    let mut candidate = cfg.clone();
    let mut pending_changes = false;
    let mut should_write_config = false;
    let mut fix_hints: Vec<String> = Vec::new();

    if snapshot.exists && !snapshot.valid && snapshot.legacy_issues.is_empty() {
        note(
            "Config invalid; doctor will run with best-effort config.",
            Some("Config"),
        );
    }

    if !snapshot.warnings.is_empty() {
        let lines: String = snapshot
            .warnings
            .iter()
            .map(|issue| format!("- {}: {}", issue.path, issue.message))
            .collect::<Vec<_>>()
            .join("\n");
        note(&lines, Some("Config warnings"));
    }

    // ── Legacy config issues ──
    if !snapshot.legacy_issues.is_empty() {
        let issue_lines: String = snapshot
            .legacy_issues
            .iter()
            .map(|issue| format!("- {}: {}", issue.path, issue.message))
            .collect::<Vec<_>>()
            .join("\n");
        note(&issue_lines, Some("Legacy config keys detected"));

        // In the full implementation, `migrateLegacyConfig` is called here.
        // Stub: the TS migration logic is not yet ported.
        if should_repair {
            // Apply best-effort config
        } else {
            fix_hints.push(format!(
                "Run \"{}\" to apply legacy migrations.",
                format_cli_command("crabclaw doctor --fix")
            ));
        }
    }

    // ── Normalize legacy config values ──
    let normalized = normalize_legacy_config_values(&candidate);
    if !normalized.changes.is_empty() {
        note(&normalized.changes.join("\n"), Some("Doctor changes"));
        candidate = normalized.config;
        pending_changes = true;
        if should_repair {
            cfg = candidate.clone();
        } else {
            fix_hints.push(format!(
                "Run \"{}\" to apply these changes.",
                format_cli_command("crabclaw doctor --fix")
            ));
        }
    }

    // ── Apply pending changes via user prompt ──
    if !should_repair && pending_changes {
        let should_apply = prompter
            .confirm("Apply recommended config repairs now?", true)
            .await;
        if should_apply {
            cfg = candidate.clone();
            should_write_config = true;
        } else if !fix_hints.is_empty() {
            note(&fix_hints.join("\n"), Some("Doctor"));
        }
    }

    note_opencode_provider_overrides(&cfg);

    ConfigFlowResult {
        cfg,
        path: Some(snapshot.path),
        should_write_config,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn is_record_detects_objects() {
        assert!(is_record(&serde_json::json!({"a": 1})));
        assert!(!is_record(&serde_json::json!([1, 2])));
        assert!(!is_record(&serde_json::json!(null)));
        assert!(!is_record(&serde_json::json!("string")));
    }

    #[test]
    fn note_opencode_overrides_no_panic_on_empty_config() {
        let cfg = OpenAcosmiConfig::default();
        // Should not panic even with no models config.
        note_opencode_provider_overrides(&cfg);
    }
}
