//! Build script for the `OpenAcosmi` CLI binary.
//!
//! Injects the git commit hash (short) into the binary as `GIT_HASH`
//! environment variable, available at compile time via `env!("GIT_HASH")`.
//! If git is unavailable or the working directory is not a git repo, falls
//! back to "unknown".
//!
//! Source: `backend/internal/cli/version.go` - build-time ldflags pattern

fn main() {
    // Inject git commit hash
    let hash = std::process::Command::new("git")
        .args(["rev-parse", "--short", "HEAD"])
        .output()
        .ok()
        .and_then(|o| {
            if o.status.success() {
                String::from_utf8(o.stdout)
                    .ok()
                    .map(|s| s.trim().to_string())
            } else {
                None
            }
        })
        .unwrap_or_else(|| "unknown".to_string());

    println!("cargo:rustc-env=GIT_HASH={hash}");

    // Re-run if git HEAD changes
    println!("cargo:rerun-if-changed=../../.git/HEAD");
    println!("cargo:rerun-if-changed=../../.git/refs/heads/");
}
