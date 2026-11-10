# AGENTS.md

Guidance for coding agents (Claude Code, pi, Gemini CLI) when working in ccloud.
`CLAUDE.md` imports this file; edit here only.

## Project Overview

ccloud is the developer CLI for apps running in CloudWalk Cloud, or anyone's Custom Cloud. Its
first command group, `clusters`, signs a developer in to a region's Kubernetes API through
Cloudflare Access and an OIDC provider; logs and dev workflows follow (`specs/`).

## Development Philosophy

- Strict outside-in TDD: one failing test, red, minimal green, refactor.
- Simplest change that works; no speculative abstractions.
- Comments are for users, tests are for maintainers: names and tests carry intent, and there are no explanatory comments.

## The Agent's Role and Constraints

- Land work as small, focused pull requests; CI proves validity. Every merge to `main` releases.
- Edit files only with the Edit/Write tools; never stream edits.
- Verify behavior with tests, not by hand.

## Commands

```
make check
```

Always use the project's canonical targets above, never raw tooling.

## Architecture

- `cmd/` — Cobra commands, one file per command, taking the cloud from `cmd.Run`.
- `internal/cloud` — the `Cloud` and `Region` values every command reads.
- `internal/cloud/cloudwalk` — the only package that knows a real datacenter.
- `internal/kubeconfig` — writing and switching `ccloud-<region>` contexts.

Nothing outside `internal/cloud/cloudwalk` may name a CloudWalk host, region, issuer or domain.

## Testing and Verification

Ginkgo specs run commands through `cmd.Run` against the `example` cloud in `cmd/cmd_suite_test.go`,
with fake `kubectl`, `cloudflared` and `kubectl-oidc_login` on `PATH`. Coverage gate 85%, ratchet
only upward.

## Configuration and Secrets

The OIDC client is stamped in at release from Actions secrets ([docs/releasing.md](docs/releasing.md)),
never committed.

## Maintaining This File

Capture user preferences and corrections here as they come up so they are not repeated.
