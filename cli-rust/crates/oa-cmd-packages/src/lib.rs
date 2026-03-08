/// Package management commands for Claw Acosmi CLI.
///
/// Provides `packages` subcommands: browse, detail, install, remove, installed.
/// Delegates to Gateway RPC: `packages.catalog.browse`, `packages.catalog.detail`,
/// `packages.install`, `packages.remove`, `packages.installed`.

use anyhow::Result;
use clap::Parser;

use oa_cli_shared::progress::with_progress;
use oa_config::io::load_config;
use oa_gateway_rpc::call::{CallGatewayOptions, call_gateway};

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

/// CLI arguments for `packages browse`.
#[derive(Debug, Parser)]
pub struct PackagesBrowseArgs {
    /// Filter by kind (skill, plugin, bundle).
    #[arg(long)]
    pub kind: Option<String>,

    /// Search keyword.
    #[arg(long)]
    pub keyword: Option<String>,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `packages detail`.
#[derive(Debug, Parser)]
pub struct PackagesDetailArgs {
    /// Package ID.
    pub id: String,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `packages install`.
#[derive(Debug, Parser)]
pub struct PackagesInstallArgs {
    /// Package ID.
    pub id: String,

    /// Package kind (skill, plugin, bundle). Defaults to skill.
    #[arg(long, default_value = "skill")]
    pub kind: String,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `packages remove`.
#[derive(Debug, Parser)]
pub struct PackagesRemoveArgs {
    /// Package ID.
    pub id: String,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

/// CLI arguments for `packages installed`.
#[derive(Debug, Parser)]
pub struct PackagesInstalledArgs {
    /// Filter by kind (skill, plugin, bundle).
    #[arg(long)]
    pub kind: Option<String>,

    /// Output result as JSON.
    #[arg(long)]
    pub json: bool,
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

/// Execute `packages browse` — browse available packages in the catalog.
pub async fn packages_browse_command(args: &PackagesBrowseArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();

    let mut params = serde_json::Map::new();
    if let Some(ref kind) = args.kind {
        params.insert("kind".to_string(), serde_json::Value::String(kind.clone()));
    }
    if let Some(ref keyword) = args.keyword {
        params.insert(
            "keyword".to_string(),
            serde_json::Value::String(keyword.clone()),
        );
    }

    let opts = CallGatewayOptions {
        method: "packages.catalog.browse".to_string(),
        config: Some(cfg),
        params: if params.is_empty() {
            None
        } else {
            Some(serde_json::Value::Object(params))
        },
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Browsing packages\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
        return Ok(());
    }

    // Human-readable table: name / kind / version
    if let Some(packages) = result.get("items").and_then(|p| p.as_array()) {
        if packages.is_empty() {
            println!("No packages found.");
            return Ok(());
        }
        println!("{:<30} {:<12} {}", "NAME", "KIND", "VERSION");
        println!("{}", "-".repeat(56));
        for pkg in packages {
            let name = pkg
                .get("name")
                .and_then(|n| n.as_str())
                .unwrap_or("(unknown)");
            let kind = pkg
                .get("kind")
                .and_then(|k| k.as_str())
                .unwrap_or("unknown");
            let version = pkg
                .get("version")
                .and_then(|v| v.as_str())
                .unwrap_or("-");
            println!("{name:<30} {kind:<12} {version}");
        }
    } else {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }

    Ok(())
}

/// Execute `packages detail` — show details of a specific package.
pub async fn packages_detail_command(args: &PackagesDetailArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "packages.catalog.detail".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "id": args.id })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading package details\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        println!("Package: {}", args.id);
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }
    Ok(())
}

/// Execute `packages install` — install a package by ID.
pub async fn packages_install_command(args: &PackagesInstallArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "packages.install".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "id": args.id, "kind": args.kind })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Installing package\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        let key = result
            .get("record")
            .and_then(|r| r.get("key"))
            .and_then(|k| k.as_str())
            .unwrap_or(&args.id);
        println!("Installed: {key}");
    }
    Ok(())
}

/// Execute `packages remove` — remove a package by ID.
pub async fn packages_remove_command(args: &PackagesRemoveArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();
    let opts = CallGatewayOptions {
        method: "packages.remove".to_string(),
        config: Some(cfg),
        params: Some(serde_json::json!({ "id": args.id })),
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Removing package\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    } else {
        let ok = result
            .get("success")
            .and_then(|s| s.as_bool())
            .unwrap_or(false);
        if ok {
            println!("Removed: {}", args.id);
        } else {
            println!("Remove may have failed. Response:");
            println!(
                "{}",
                serde_json::to_string_pretty(&result).unwrap_or_default()
            );
        }
    }
    Ok(())
}

/// Execute `packages installed` — list installed packages.
pub async fn packages_installed_command(args: &PackagesInstalledArgs) -> Result<()> {
    let cfg = load_config().unwrap_or_default();

    let mut params = serde_json::Map::new();
    if let Some(ref kind) = args.kind {
        params.insert("kind".to_string(), serde_json::Value::String(kind.clone()));
    }

    let opts = CallGatewayOptions {
        method: "packages.installed".to_string(),
        config: Some(cfg),
        params: if params.is_empty() {
            None
        } else {
            Some(serde_json::Value::Object(params))
        },
        ..Default::default()
    };

    let result: serde_json::Value = if args.json {
        call_gateway(opts).await?
    } else {
        with_progress("Loading installed packages\u{2026}", call_gateway(opts)).await?
    };

    if args.json {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
        return Ok(());
    }

    // Human-readable table: key / kind / version / installedAt
    if let Some(packages) = result.get("records").and_then(|p| p.as_array()) {
        if packages.is_empty() {
            println!("No installed packages.");
            return Ok(());
        }
        println!(
            "{:<30} {:<12} {:<10} {}",
            "KEY", "KIND", "VERSION", "INSTALLED AT"
        );
        println!("{}", "-".repeat(72));
        for pkg in packages {
            let key = pkg
                .get("key")
                .and_then(|k| k.as_str())
                .unwrap_or("(unknown)");
            let kind = pkg
                .get("kind")
                .and_then(|k| k.as_str())
                .unwrap_or("unknown");
            let version = pkg
                .get("version")
                .and_then(|v| v.as_str())
                .unwrap_or("-");
            let installed_at = pkg
                .get("installedAt")
                .and_then(|i| i.as_str())
                .unwrap_or("-");
            println!("{key:<30} {kind:<12} {version:<10} {installed_at}");
        }
    } else {
        println!(
            "{}",
            serde_json::to_string_pretty(&result).unwrap_or_default()
        );
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn packages_browse_args_defaults() {
        let args = PackagesBrowseArgs {
            kind: None,
            keyword: None,
            json: false,
        };
        assert!(!args.json);
        assert!(args.kind.is_none());
        assert!(args.keyword.is_none());
    }

    #[test]
    fn packages_browse_args_with_filters() {
        let args = PackagesBrowseArgs {
            kind: Some("skill".to_string()),
            keyword: Some("browser".to_string()),
            json: true,
        };
        assert!(args.json);
        assert_eq!(args.kind.as_deref(), Some("skill"));
        assert_eq!(args.keyword.as_deref(), Some("browser"));
    }

    #[test]
    fn packages_detail_args_construction() {
        let args = PackagesDetailArgs {
            id: "my-package".to_string(),
            json: true,
        };
        assert_eq!(args.id, "my-package");
        assert!(args.json);
    }

    #[test]
    fn packages_install_args_construction() {
        let args = PackagesInstallArgs {
            id: "some-pkg".to_string(),
            kind: "skill".to_string(),
            json: false,
        };
        assert_eq!(args.id, "some-pkg");
        assert_eq!(args.kind, "skill");
        assert!(!args.json);
    }

    #[test]
    fn packages_install_args_with_kind() {
        let args = PackagesInstallArgs {
            id: "my-plugin".to_string(),
            kind: "plugin".to_string(),
            json: true,
        };
        assert_eq!(args.id, "my-plugin");
        assert_eq!(args.kind, "plugin");
        assert!(args.json);
    }

    #[test]
    fn packages_remove_args_construction() {
        let args = PackagesRemoveArgs {
            id: "old-pkg".to_string(),
            json: false,
        };
        assert_eq!(args.id, "old-pkg");
        assert!(!args.json);
    }

    #[test]
    fn packages_installed_args_defaults() {
        let args = PackagesInstalledArgs {
            kind: None,
            json: false,
        };
        assert!(!args.json);
        assert!(args.kind.is_none());
    }

    #[test]
    fn packages_installed_args_with_kind() {
        let args = PackagesInstalledArgs {
            kind: Some("plugin".to_string()),
            json: true,
        };
        assert!(args.json);
        assert_eq!(args.kind.as_deref(), Some("plugin"));
    }
}
