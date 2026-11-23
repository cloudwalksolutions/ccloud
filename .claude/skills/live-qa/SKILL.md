---
description: QA a ccloud build against the real clusters the way a developer uses it, and judge whether its output is right and readable. Use before a ccloud PR merges (local) and after it releases (brew).
argument-hint: "[local|brew] <app> [scenario …]"
---

Live QA for ccloud: $ARGUMENTS

1. **The binary.** `local`: run `make build` here (it reads the OIDC client from the git-ignored
   `.env`) and use this repo's `./ccloud` by absolute path. `brew`: `brew update && brew upgrade
   ccloud`, then `ccloud`, and check `ccloud --version` matches the newest release.
2. **The app.** The second argument names a real app to QA against. Never write its name, its
   environments or its hosts into anything public (PRs, issues, commits): this repo is public, so
   call them `<app>` and `<env>` there.
3. **The scenarios.** Run the ones named after the app, or every one in
   [scenarios.md](scenarios.md). Run commands under `bash -c` with arguments split by `read -ra`
   (zsh does not split a variable), and always print what came back, so an error can never pass
   for a match.
4. **Judge what you saw**, not what the code says:
   - the numbers are true (counts match `kubectl get` on the `ccloud-<region>` context; usage is
     within ~30% of `kubectl top`);
   - columns line up, every number has its unit, and color only marks meaning;
   - piped output has no color codes or spinner frames on stdout, and progress lines go to stderr;
   - an error says what went wrong and what to do next, exits non-zero, and prints nothing on stdout.
5. **Report** in the session: each scenario as works, wrong (with the command, what it printed,
   what it should have) or blocked (with the command the user must run, such as a sign-in).
   A wrong result is a bug: write the failing spec that reproduces it before fixing it.

Never run what changes something (`workflow rerun`, `workflow cancel`, `db reset`, `env pull`,
`clusters auth`, `db tunnel --background`) or what prints a secret (`db tunnel -s`,
`go version -m` on a ccloud binary, reading `.env`).
