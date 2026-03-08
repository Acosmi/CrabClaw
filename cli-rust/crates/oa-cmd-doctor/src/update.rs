/// Pre-doctor update offer.
///
/// Before running the full doctor flow, offers to update from git
/// (if the install is a git checkout) or suggests using the package
/// manager for non-git installs.
///
/// Source: `src/commands/doctor-update.ts`
use oa_cli_shared::command_format::format_cli_command;
use oa_infra::env::{is_truthy_env_value, preferred_env_value};
use oa_terminal::note::note;

use crate::prompter::DoctorOptions;

/// Result of the pre-doctor update check.
///
/// Source: `src/commands/doctor-update.ts` — return of `maybeOfferUpdateBeforeDoctor`
#[derive(Debug, Clone, Default)]
pub struct UpdateResult {
    /// Whether an update was performed.
    pub updated: bool,
    /// Whether the update handled the doctor run (update includes doctor).
    pub handled: bool,
}

/// Detect whether the given root is a git checkout.
///
/// Source: `src/commands/doctor-update.ts` — `detectOpenAcosmiGitCheckout`
async fn detect_git_checkout(root: &str) -> &'static str {
    let result = tokio::process::Command::new("git")
        .args(["-C", root, "rev-parse", "--show-toplevel"])
        .output()
        .await;

    match result {
        Ok(output) if output.status.success() => {
            let stdout = String::from_utf8_lossy(&output.stdout);
            if stdout.trim() == root {
                "git"
            } else {
                "not-git"
            }
        }
        Ok(output) => {
            let stderr = String::from_utf8_lossy(&output.stderr);
            if stderr.to_lowercase().contains("not a git repository") {
                "not-git"
            } else {
                "unknown"
            }
        }
        Err(_) => "unknown",
    }
}

/// Offer to update OpenAcosmi from git (or note the package-manager update path).
///
/// Source: `src/commands/doctor-update.ts` — `maybeOfferUpdateBeforeDoctor`
pub async fn maybe_offer_update_before_doctor(options: &DoctorOptions) -> UpdateResult {
    let update_in_progress = is_truthy_env_value(
        preferred_env_value(&[
            "CRABCLAW_UPDATE_IN_PROGRESS",
            "OPENACOSMI_UPDATE_IN_PROGRESS",
        ])
        .as_deref(),
    );

    let can_offer = !update_in_progress
        && options.non_interactive != Some(true)
        && options.yes != Some(true)
        && options.repair != Some(true);

    if !can_offer {
        return UpdateResult::default();
    }

    // The Rust port does not resolve a "package root" in the same way as TS.
    // We check whether the current executable's directory is a git checkout.
    let exe_dir = std::env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|d| d.to_string_lossy().to_string()));

    let Some(root) = exe_dir else {
        return UpdateResult::default();
    };

    let git = detect_git_checkout(&root).await;
    if git == "not-git" {
        note(
            &format!(
                "This install is not a git checkout.\n\
                 Run `{}` to update via your package manager (npm/pnpm), then rerun doctor.",
                format_cli_command("crabclaw update")
            ),
            Some("Update"),
        );
    }

    // In the Rust port we do not actually perform the git update; that
    // is handled by the TypeScript update runner.
    UpdateResult::default()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn update_skipped_in_non_interactive() {
        let options = DoctorOptions {
            non_interactive: Some(true),
            ..Default::default()
        };
        let result = maybe_offer_update_before_doctor(&options).await;
        assert!(!result.updated);
        assert!(!result.handled);
    }

    #[tokio::test]
    async fn update_skipped_in_yes_mode() {
        let options = DoctorOptions {
            yes: Some(true),
            ..Default::default()
        };
        let result = maybe_offer_update_before_doctor(&options).await;
        assert!(!result.updated);
    }

    #[tokio::test]
    async fn detect_git_checkout_temp_dir() {
        let tmp = std::env::temp_dir();
        let result = detect_git_checkout(&tmp.to_string_lossy()).await;
        // Temp dir is not a git repo.
        assert!(result == "not-git" || result == "unknown");
    }
}
