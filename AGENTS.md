# AGENTS.md

Guidance for coding agents (Claude Code, pi, Gemini CLI) when working in ccloud.
`CLAUDE.md` imports this file; edit here only.

## Project Overview

ccloud is the developer CLI for apps running in CloudWalk Cloud, or anyone's Custom Cloud.
`clusters` signs a developer in to a region's Kubernetes API through Cloudflare Access and an OIDC
provider; `workflow` reads their app's CI runs through that cluster; `logs` reads their app's logs
through the region's gateway, whose contract is [docs/gateway.md](docs/gateway.md). Inside an app
repo, the cloud's repo directory (`.ccloud` unless the cloud says otherwise) names the app. What
comes next is in `specs/`.

## Development Philosophy

- Strict outside-in TDD: one failing test, red, minimal green, refactor.
- Simplest change that works; no speculative abstractions.
- Comments are for users, tests are for maintainers: names and tests carry intent, and there are no explanatory comments.

## The Agent's Role and Constraints

- Land work as small, focused pull requests; CI proves validity. Every merge to `main` releases.
- Edit files only with the Edit/Write tools; never stream edits.
- Verify behavior with tests, not by hand.
- Before a PR merges, `make build` it with the OIDC client exported and run its QA steps against
  the real clusters with `./ccloud`; after it releases, upgrade and run them again.
- Progress goes to stderr, data to stdout: a wait gets a `step`, and specs give stdout and stderr
  separate buffers.
- Every table goes through `newTable` (`cmd/table.go`), never `text/tabwriter`, which counts color
  codes as width and misaligns colored cells.

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
with fake `kubectl`, `cloudflared` and `kubectl-oidc_login` on `PATH`. Coverage is gated
at 100% with nothing ignored: anything a user can meet, a spec can make happen, and a branch no
spec can reach is restructured away.

## Configuration and Secrets

The OIDC client is stamped in at release from Actions secrets, and by `make build` from
`GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` in the environment ([docs/releasing.md](docs/releasing.md)),
never committed. `make build` does not echo them.

## Maintaining This File

Capture user preferences and corrections here as they come up so they are not repeated.
