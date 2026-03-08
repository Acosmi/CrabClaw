pub mod add;
pub mod capabilities;
pub mod list;
pub mod login;
pub mod logout;
pub mod logs;
pub mod remove;
pub mod resolve;
/// Channel management commands for Crab Claw CLI.
///
/// Provides subcommands for listing, adding, removing, resolving,
/// inspecting capabilities, viewing logs, and checking status of chat
/// channel accounts.
///
/// Source: `src/commands/channels.ts`, `src/commands/channels/*.ts`
pub mod shared;
pub mod status;
