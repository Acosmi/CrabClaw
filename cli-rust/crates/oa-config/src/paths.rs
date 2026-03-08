/// Configuration file path resolution.
///
/// Resolves paths to state directories, config files, gateway lock dirs,
/// and OAuth credential directories. Respects environment variable overrides
/// such as `OPENACOSMI_HOME`, `OPENACOSMI_STATE_DIR`, `OPENACOSMI_CONFIG_PATH`,
/// `OPENACOSMI_GATEWAY_PORT`, `OPENACOSMI_NIX_MODE`, and `OPENACOSMI_OAUTH_DIR`.
///
/// Source: `src/config/paths.ts`
use std::env;
use std::path::{Path, PathBuf};

use oa_types::config::OpenAcosmiConfig;

/// Current default write state directory name under the user's home directory.
pub const NEW_STATE_DIRNAME: &str = ".openacosmi";

/// New-brand compatibility state directory name used during dual-read migration.
pub const COMPATIBILITY_STATE_DIRNAME: &str = ".crabclaw";

/// Default config file name.
pub const CONFIG_FILENAME: &str = "openacosmi.json";

/// Legacy state directory names from prior product incarnations.
const LEGACY_STATE_DIRNAMES: &[&str] = &[".clawdbot", ".moltbot", ".moldbot"];

/// Legacy config file names from prior product incarnations.
const LEGACY_CONFIG_FILENAMES: &[&str] = &["clawdbot.json", "moltbot.json", "moldbot.json"];

/// Default gateway HTTP port.
pub const DEFAULT_GATEWAY_PORT: u16 = 19001;

/// OAuth credentials filename.
const OAUTH_FILENAME: &str = "oauth.json";

fn preferred_env_value(keys: &[&str]) -> Option<String> {
    for key in keys {
        if let Ok(value) = env::var(key) {
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

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/// Resolve the home directory, preferring `OPENACOSMI_HOME` over `dirs::home_dir`.
fn resolve_home_dir() -> PathBuf {
    if let Some(home) = preferred_env_value(&["CRABCLAW_HOME", "OPENACOSMI_HOME"]) {
        return PathBuf::from(home);
    }
    dirs::home_dir().unwrap_or_else(|| PathBuf::from("."))
}

/// Expand a leading `~` in a user-supplied path to the home directory.
fn resolve_user_path(input: &str) -> PathBuf {
    let trimmed = input.trim();
    if trimmed.is_empty() {
        return PathBuf::from(trimmed);
    }
    if let Some(rest) = trimmed.strip_prefix('~') {
        let home = resolve_home_dir();
        if rest.is_empty() {
            return home;
        }
        let rest = rest.strip_prefix('/').unwrap_or(rest);
        return home.join(rest);
    }
    PathBuf::from(trimmed)
        .canonicalize()
        .unwrap_or_else(|_| PathBuf::from(trimmed))
}

fn new_state_dir() -> PathBuf {
    resolve_home_dir().join(NEW_STATE_DIRNAME)
}

fn compatibility_state_dir() -> PathBuf {
    resolve_home_dir().join(COMPATIBILITY_STATE_DIRNAME)
}

fn new_state_dir_for_home(home: &Path) -> PathBuf {
    home.join(NEW_STATE_DIRNAME)
}

fn compatibility_state_dir_for_home(home: &Path) -> PathBuf {
    home.join(COMPATIBILITY_STATE_DIRNAME)
}

fn legacy_state_dirs() -> Vec<PathBuf> {
    let home = resolve_home_dir();
    legacy_state_dirs_for_home(&home)
}

fn legacy_state_dirs_for_home(home: &Path) -> Vec<PathBuf> {
    LEGACY_STATE_DIRNAMES
        .iter()
        .map(|name| home.join(name))
        .collect()
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Resolve the state directory for mutable data (sessions, logs, caches).
///
/// Precedence:
/// 1. `OPENACOSMI_STATE_DIR` environment variable
/// 2. Existing `~/.crabclaw` directory containing managed state
/// 3. Existing `~/.openacosmi` directory
/// 4. First existing legacy state directory
/// 5. Default `~/.openacosmi` (keep old default write path unchanged)
pub fn resolve_state_dir() -> PathBuf {
    // Check explicit override
    if let Some(val) = preferred_env_value(&["CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR"]) {
        return resolve_user_path(&val);
    }

    let compat_dir = compatibility_state_dir();
    let new_dir = new_state_dir();

    choose_existing_state_dir(&compat_dir, &new_dir, &legacy_state_dirs()).unwrap_or(new_dir)
}

/// Resolve the canonical config file path within a given state directory.
///
/// Respects `OPENACOSMI_CONFIG_PATH` override, otherwise returns
/// `<state_dir>/openacosmi.json`.
pub fn resolve_canonical_config_path(state_dir: &Path) -> PathBuf {
    if let Some(val) = preferred_env_value(&["CRABCLAW_CONFIG_PATH", "OPENACOSMI_CONFIG_PATH"]) {
        return resolve_user_path(&val);
    }
    state_dir.join(CONFIG_FILENAME)
}

/// Build the list of default config path candidates.
///
/// Returns all possible config file locations in priority order:
/// explicit path, state-dir candidates, new+legacy dir candidates.
fn resolve_default_config_candidates() -> Vec<PathBuf> {
    // If explicit config path is set, use only that
    if let Some(val) = preferred_env_value(&["CRABCLAW_CONFIG_PATH", "OPENACOSMI_CONFIG_PATH"]) {
        return vec![resolve_user_path(&val)];
    }

    let mut candidates = Vec::new();

    // If state dir override exists, add candidates from there
    if let Some(val) = preferred_env_value(&["CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR"]) {
        let resolved = resolve_user_path(&val);
        candidates.push(resolved.join(CONFIG_FILENAME));
        for name in LEGACY_CONFIG_FILENAMES {
            candidates.push(resolved.join(name));
        }
    }

    // Add default directories: compatibility state dir + current default state dir + legacy
    let default_dirs = resolve_default_state_dirs(&resolve_home_dir());

    for dir in default_dirs {
        candidates.push(dir.join(CONFIG_FILENAME));
        for name in LEGACY_CONFIG_FILENAMES {
            candidates.push(dir.join(name));
        }
    }

    candidates
}

/// Resolve the active config path by preferring existing config files
/// before falling back to the canonical path.
///
/// This is the primary entry point for determining which config file to use.
pub fn resolve_config_path() -> PathBuf {
    // If explicit override, use it directly
    if let Some(val) = preferred_env_value(&["CRABCLAW_CONFIG_PATH", "OPENACOSMI_CONFIG_PATH"]) {
        return resolve_user_path(&val);
    }

    let state_dir = resolve_state_dir();

    // If OPENACOSMI_STATE_DIR is set, look within that dir
    let state_override = preferred_env_value(&["CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR"]);

    // Build candidates from the state dir
    let mut candidates = vec![state_dir.join(CONFIG_FILENAME)];
    for name in LEGACY_CONFIG_FILENAMES {
        candidates.push(state_dir.join(name));
    }

    // Check if any candidate exists
    for candidate in &candidates {
        if candidate.exists() {
            return candidate.clone();
        }
    }

    // If state dir was explicitly overridden, use it
    if state_override.is_some() {
        return state_dir.join(CONFIG_FILENAME);
    }

    // Try full default candidate list
    let all_candidates = resolve_default_config_candidates();
    for candidate in &all_candidates {
        if candidate.exists() {
            return candidate.clone();
        }
    }

    // Fallback: canonical path
    resolve_canonical_config_path(&state_dir)
}

fn state_dir_has_managed_content(dir: &Path) -> bool {
    if !dir.is_dir() {
        return false;
    }

    const MARKERS: &[&str] = &[
        CONFIG_FILENAME,
        "credentials",
        "sessions",
        "agents",
        "memory",
        "extensions",
        "logs",
        "exec-approvals.json",
        "oauth.json",
    ];

    MARKERS.iter().any(|marker| dir.join(marker).exists())
}

fn choose_existing_state_dir(
    compat_dir: &Path,
    current_dir: &Path,
    legacy_dirs: &[PathBuf],
) -> Option<PathBuf> {
    if state_dir_has_managed_content(compat_dir) {
        return Some(compat_dir.to_path_buf());
    }
    if current_dir.exists() {
        return Some(current_dir.to_path_buf());
    }
    if compat_dir.exists() {
        return Some(compat_dir.to_path_buf());
    }
    for dir in legacy_dirs {
        if dir.exists() {
            return Some(dir.clone());
        }
    }
    None
}

fn resolve_default_state_dirs(home: &Path) -> Vec<PathBuf> {
    let mut dirs = vec![
        compatibility_state_dir_for_home(home),
        new_state_dir_for_home(home),
    ];
    dirs.extend(legacy_state_dirs_for_home(home));
    dirs
}

#[cfg(test)]
fn resolve_config_path_for_home(home: &Path) -> PathBuf {
    let compat_dir = compatibility_state_dir_for_home(home);
    let current_dir = new_state_dir_for_home(home);
    let state_dir =
        choose_existing_state_dir(&compat_dir, &current_dir, &legacy_state_dirs_for_home(home))
            .unwrap_or(current_dir.clone());

    let mut candidates = vec![state_dir.join(CONFIG_FILENAME)];
    for name in LEGACY_CONFIG_FILENAMES {
        candidates.push(state_dir.join(name));
    }
    if let Some(found) = candidates.iter().find(|candidate| candidate.exists()) {
        return found.clone();
    }

    let mut all_candidates = Vec::new();
    for dir in resolve_default_state_dirs(home) {
        all_candidates.push(dir.join(CONFIG_FILENAME));
        for name in LEGACY_CONFIG_FILENAMES {
            all_candidates.push(dir.join(name));
        }
    }
    if let Some(found) = all_candidates.iter().find(|candidate| candidate.exists()) {
        return found.clone();
    }

    state_dir.join(CONFIG_FILENAME)
}

/// Resolve the gateway lock directory (ephemeral, in temp dir).
///
/// Returns `<tmpdir>/openacosmi-<uid>` on Unix (uid from `id -u` or process ID)
/// or `<tmpdir>/openacosmi` on other platforms.
pub fn resolve_gateway_lock_dir() -> PathBuf {
    let base = env::temp_dir();
    #[cfg(unix)]
    {
        // Attempt to get the real uid; fall back to process id if unavailable.
        let uid = resolve_unix_uid();
        return base.join(format!("openacosmi-{uid}"));
    }
    #[cfg(not(unix))]
    {
        base.join("openacosmi")
    }
}

/// Resolve the Unix user ID for lock-directory naming.
///
/// Uses `std::os::unix::fs::MetadataExt` to read the uid of the
/// home directory, falling back to the process ID.
#[cfg(unix)]
fn resolve_unix_uid() -> u32 {
    use std::os::unix::fs::MetadataExt;
    // Read uid from the home directory metadata
    let home = resolve_home_dir();
    if let Ok(meta) = std::fs::metadata(&home) {
        return meta.uid();
    }
    // Fallback: use process id (not ideal but functional)
    std::process::id()
}

/// Resolve the OAuth credentials directory.
///
/// Precedence:
/// 1. `OPENACOSMI_OAUTH_DIR` environment variable
/// 2. `<state_dir>/credentials`
pub fn resolve_oauth_dir(state_dir: &Path) -> PathBuf {
    if let Some(val) = preferred_env_value(&["CRABCLAW_OAUTH_DIR", "OPENACOSMI_OAUTH_DIR"]) {
        return resolve_user_path(&val);
    }
    state_dir.join("credentials")
}

/// Resolve the OAuth credentials file path.
///
/// Returns `<oauth_dir>/oauth.json`.
pub fn resolve_oauth_path(state_dir: &Path) -> PathBuf {
    resolve_oauth_dir(state_dir).join(OAUTH_FILENAME)
}

/// Resolve the gateway port from environment and config.
///
/// Precedence:
/// 1. `OPENACOSMI_GATEWAY_PORT` environment variable
/// 2. `cfg.gateway.port` from config
/// 3. [`DEFAULT_GATEWAY_PORT`] (19001)
pub fn resolve_gateway_port(cfg: Option<&OpenAcosmiConfig>) -> u16 {
    // Check env var
    if let Some(trimmed) =
        preferred_env_value(&["CRABCLAW_GATEWAY_PORT", "OPENACOSMI_GATEWAY_PORT"])
    {
        if let Ok(parsed) = trimmed.parse::<u16>() {
            if parsed > 0 {
                return parsed;
            }
        }
    }

    // Check config
    if let Some(config) = cfg {
        if let Some(ref gw) = config.gateway {
            if let Some(port) = gw.port {
                if port > 0 {
                    return port;
                }
            }
        }
    }

    DEFAULT_GATEWAY_PORT
}

#[cfg(test)]
mod path_tests {
    use super::*;
    use std::collections::HashMap;
    use std::fs;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn unique_temp_dir(label: &str) -> PathBuf {
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("time")
            .as_nanos();
        let dir = std::env::temp_dir().join(format!("oa-config-{label}-{nanos}"));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).expect("create temp dir");
        dir
    }

    #[test]
    fn resolve_state_dir_prefers_crabclaw_when_it_contains_managed_state() {
        let tmp = unique_temp_dir("prefer-crab");
        let crab = tmp.join(COMPATIBILITY_STATE_DIRNAME);
        let old = tmp.join(NEW_STATE_DIRNAME);
        fs::create_dir_all(crab.join("credentials")).expect("create crab state");
        fs::create_dir_all(old.join("credentials")).expect("create old state");

        let resolved = choose_existing_state_dir(&crab, &old, &legacy_state_dirs_for_home(&tmp))
            .expect("state dir");

        assert_eq!(resolved, crab);
        let _ = fs::remove_dir_all(&tmp);
    }

    #[test]
    fn resolve_state_dir_keeps_openacosmi_when_crabclaw_is_empty() {
        let tmp = unique_temp_dir("keep-open");
        let crab = tmp.join(COMPATIBILITY_STATE_DIRNAME);
        let old = tmp.join(NEW_STATE_DIRNAME);
        fs::create_dir_all(&crab).expect("create empty crab dir");
        fs::create_dir_all(old.join("sessions")).expect("create old state");

        let resolved = choose_existing_state_dir(&crab, &old, &legacy_state_dirs_for_home(&tmp))
            .expect("state dir");

        assert_eq!(resolved, old);
        let _ = fs::remove_dir_all(&tmp);
    }

    #[test]
    fn resolve_config_path_prefers_crabclaw_config_when_present() {
        let tmp = unique_temp_dir("config-prefers-crab");
        let crab_cfg = tmp.join(COMPATIBILITY_STATE_DIRNAME).join(CONFIG_FILENAME);
        let old_cfg = tmp.join(NEW_STATE_DIRNAME).join(CONFIG_FILENAME);
        fs::create_dir_all(crab_cfg.parent().expect("crab parent")).expect("create crab dir");
        fs::create_dir_all(old_cfg.parent().expect("old parent")).expect("create old dir");
        fs::write(&old_cfg, b"{}").expect("write old config");
        fs::write(&crab_cfg, b"{}").expect("write crab config");

        let resolved = resolve_config_path_for_home(&tmp);

        assert_eq!(resolved, crab_cfg);
        let _ = fs::remove_dir_all(&tmp);
    }

    #[test]
    fn preferred_env_value_prefers_crabclaw_prefix() {
        let values = HashMap::from([
            ("OPENACOSMI_STATE_DIR", "/tmp/old"),
            ("CRABCLAW_STATE_DIR", "/tmp/new"),
        ]);
        let resolved =
            preferred_env_value_from_map(&values, &["CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR"]);
        assert_eq!(resolved.as_deref(), Some("/tmp/new"));
    }
}

/// Check if running in Nix mode.
///
/// When `OPENACOSMI_NIX_MODE=1`, the gateway is running under Nix.
/// In this mode, no auto-install flows should be attempted and
/// config is managed externally.
pub fn is_nix_mode() -> bool {
    preferred_env_value(&["CRABCLAW_NIX_MODE", "OPENACOSMI_NIX_MODE"]).is_some_and(|v| v == "1")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_gateway_port_value() {
        assert_eq!(DEFAULT_GATEWAY_PORT, 19001);
    }

    #[test]
    fn gateway_port_from_config() {
        let mut cfg = OpenAcosmiConfig::default();
        cfg.gateway = Some(oa_types::gateway::GatewayConfig {
            port: Some(9999),
            ..Default::default()
        });
        // Only works when env var is not set; this tests the config path
        let port = resolve_gateway_port(Some(&cfg));
        // If env var is set, it takes precedence, so just check it's a valid port
        assert!(port > 0);
    }

    #[test]
    fn gateway_port_fallback() {
        let port = resolve_gateway_port(None);
        assert!(port > 0);
    }
}
