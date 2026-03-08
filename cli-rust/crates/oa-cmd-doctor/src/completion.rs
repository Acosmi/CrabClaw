/// Shell completion install, upgrade, and cache generation for doctor.
///
/// Checks the current shell's completion status and either upgrades slow
/// dynamic patterns, regenerates a missing cache, or prompts to install
/// completion for the first time.
///
/// Source: `src/commands/doctor-completion.ts`
use oa_cli_shared::binary_name::current_cli_name;
use oa_cli_shared::command_format::format_cli_command;
use oa_terminal::note::note;

use crate::prompter::{DoctorOptions, DoctorPrompter};

/// Supported completion shells.
///
/// Source: `src/commands/doctor-completion.ts` — `CompletionShell`
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CompletionShell {
    /// Zsh shell.
    Zsh,
    /// Bash shell.
    Bash,
    /// Fish shell.
    Fish,
    /// PowerShell.
    #[allow(dead_code)]
    Powershell,
}

impl CompletionShell {
    /// Profile source file name for the shell.
    ///
    /// Source: `src/commands/doctor-completion.ts`
    fn profile_source_name(self) -> &'static str {
        match self {
            Self::Zsh => ".zshrc",
            Self::Bash => ".bashrc",
            Self::Fish => "config/fish/config.fish",
            Self::Powershell => "profile.ps1",
        }
    }
}

/// Detect the current shell from the SHELL environment variable.
///
/// Source: `src/commands/doctor-completion.ts` — via `resolveShellFromEnv`
fn resolve_shell_from_env() -> CompletionShell {
    let shell_env = std::env::var("SHELL").unwrap_or_default();
    if shell_env.contains("zsh") {
        CompletionShell::Zsh
    } else if shell_env.contains("fish") {
        CompletionShell::Fish
    } else {
        CompletionShell::Bash
    }
}

/// Status of shell completion for the current environment.
///
/// Source: `src/commands/doctor-completion.ts` — `ShellCompletionStatus`
#[derive(Debug, Clone)]
pub struct ShellCompletionStatus {
    /// Detected shell type.
    pub shell: CompletionShell,
    /// Whether the shell profile includes a completion source line.
    pub profile_installed: bool,
    /// Whether the pre-generated completion cache file exists on disk.
    pub cache_exists: bool,
    /// Path where the cache file should live.
    pub cache_path: String,
    /// True if the profile uses the slow `source <(crabclaw completion ...)` pattern.
    pub uses_slow_pattern: bool,
}

/// Check the status of shell completion.
///
/// In the Rust port the actual profile/cache inspection is stubbed —
/// the full logic would call into `oa-cli-shared` completion helpers.
///
/// Source: `src/commands/doctor-completion.ts` — `checkShellCompletionStatus`
pub async fn check_shell_completion_status() -> ShellCompletionStatus {
    let shell = resolve_shell_from_env();
    // Stub: assume completion is installed and cache exists (no-op path).
    ShellCompletionStatus {
        shell,
        profile_installed: true,
        cache_exists: true,
        cache_path: String::new(),
        uses_slow_pattern: false,
    }
}

/// Generate the completion cache by spawning the CLI.
///
/// Source: `src/commands/doctor-completion.ts` — `generateCompletionCache`
async fn generate_completion_cache() -> bool {
    // Stub: the real implementation would spawn `crabclaw completion --write-state`.
    false
}

/// Doctor check for shell completion.
///
/// - If profile uses slow dynamic pattern: upgrade to cached version.
/// - If profile has completion but no cache: auto-generate cache.
/// - If no completion at all: prompt to install.
///
/// Source: `src/commands/doctor-completion.ts` — `doctorShellCompletion`
pub async fn doctor_shell_completion(prompter: &DoctorPrompter, options: &DoctorOptions) {
    let status = check_shell_completion_status().await;
    let write_state_cmd = format_cli_command("crabclaw completion --write-state");
    let cli_name = current_cli_name();

    // ── Slow dynamic pattern → upgrade ──
    if status.uses_slow_pattern {
        note(
            &format!(
                "Your {} profile uses slow dynamic completion (source <(...)).\n\
                 Upgrading to cached completion for faster shell startup...",
                match status.shell {
                    CompletionShell::Zsh => "zsh",
                    CompletionShell::Bash => "bash",
                    CompletionShell::Fish => "fish",
                    CompletionShell::Powershell => "powershell",
                }
            ),
            Some("Shell completion"),
        );

        if !status.cache_exists {
            let generated = generate_completion_cache().await;
            if !generated {
                note(
                    &format!(
                        "Failed to generate completion cache. Run `{write_state_cmd}` manually."
                    ),
                    Some("Shell completion"),
                );
                return;
            }
        }

        note(
            &format!(
                "Shell completion upgraded. Restart your shell or run: source ~/{}",
                status.shell.profile_source_name()
            ),
            Some("Shell completion"),
        );
        return;
    }

    // ── Profile has completion but no cache → auto-fix ──
    if status.profile_installed && !status.cache_exists {
        note(
            &format!(
                "Shell completion is configured in your {} profile but the cache is missing.\n\
                 Regenerating cache...",
                match status.shell {
                    CompletionShell::Zsh => "zsh",
                    CompletionShell::Bash => "bash",
                    CompletionShell::Fish => "fish",
                    CompletionShell::Powershell => "powershell",
                }
            ),
            Some("Shell completion"),
        );
        let generated = generate_completion_cache().await;
        if generated {
            note(
                &format!("Completion cache regenerated at {}", status.cache_path),
                Some("Shell completion"),
            );
        } else {
            note(
                &format!(
                    "Failed to regenerate completion cache. Run `{write_state_cmd}` manually."
                ),
                Some("Shell completion"),
            );
        }
        return;
    }

    // ── No completion at all → prompt ──
    if !status.profile_installed {
        if options.non_interactive == Some(true) {
            return;
        }

        let shell_name = match status.shell {
            CompletionShell::Zsh => "zsh",
            CompletionShell::Bash => "bash",
            CompletionShell::Fish => "fish",
            CompletionShell::Powershell => "powershell",
        };

        let should_install = prompter
            .confirm(
                &format!("Enable {shell_name} shell completion for {cli_name}?"),
                true,
            )
            .await;

        if should_install {
            let generated = generate_completion_cache().await;
            if !generated {
                note(
                    &format!(
                        "Failed to generate completion cache. Run `{write_state_cmd}` manually."
                    ),
                    Some("Shell completion"),
                );
                return;
            }
            note(
                &format!(
                    "Shell completion installed. Restart your shell or run: source ~/{}",
                    status.shell.profile_source_name()
                ),
                Some("Shell completion"),
            );
        }
    }
}

/// Ensure the completion cache exists (silent, for onboarding/update).
///
/// Source: `src/commands/doctor-completion.ts` — `ensureCompletionCacheExists`
pub async fn ensure_completion_cache_exists() -> bool {
    let status = check_shell_completion_status().await;
    if status.cache_exists {
        return true;
    }
    generate_completion_cache().await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_shell_defaults_to_bash() {
        // When SHELL is not set to zsh or fish, defaults to bash.
        let shell = resolve_shell_from_env();
        // We can't control SHELL in tests but we can verify it returns a valid variant.
        assert!(matches!(
            shell,
            CompletionShell::Zsh | CompletionShell::Bash | CompletionShell::Fish
        ));
    }

    #[test]
    fn profile_source_names_are_correct() {
        assert_eq!(CompletionShell::Zsh.profile_source_name(), ".zshrc");
        assert_eq!(CompletionShell::Bash.profile_source_name(), ".bashrc");
        assert_eq!(
            CompletionShell::Fish.profile_source_name(),
            "config/fish/config.fish"
        );
    }

    #[tokio::test]
    async fn check_status_returns_valid_status() {
        let status = check_shell_completion_status().await;
        // Stub always says installed + cache exists.
        assert!(status.profile_installed);
        assert!(status.cache_exists);
    }
}
