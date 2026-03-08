pub mod agent_command;
pub mod agent_via_gateway;
pub mod delivery;
pub mod model_defaults;
pub mod run_context;
pub mod session;
pub mod session_store;
/// Single agent commands for Crab Claw CLI.
///
/// This crate implements the core agent command pipeline: parsing CLI options,
/// resolving sessions, running the agent (either locally or via gateway),
/// and delivering results. It mirrors the TypeScript implementation in
/// `src/commands/agent.ts`, `src/commands/agent-via-gateway.ts`, and the
/// `src/commands/agent/` directory.
///
/// Source: `src/commands/agent.ts`, `src/commands/agent-via-gateway.ts`,
///         `src/commands/agent/*.ts`
pub mod types;
