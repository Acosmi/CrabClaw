# Desktop Build Skeleton

This directory stores the non-destructive Phase 3 build scaffold for the
desktop shell.

Scope of this scaffold:

- keep build and packaging conventions in one isolated place
- document expected platform assets and metadata
- avoid touching the current runtime, CLI naming, or active CI workflows

Current constraints:

- `Taskfile.yml` files are placeholders only
- example metadata files are not wired into any live build
- active GitHub Actions workflows are intentionally not added yet

When runtime-safe build wiring is approved later, this directory is the place
to turn the examples into active assets.
