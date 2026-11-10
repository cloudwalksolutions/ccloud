# Releasing

Every merge to `main` is a release. `.github/workflows/release.yml` tags the next patch from the
latest `v*` tag, runs GoReleaser (darwin/linux × amd64/arm64 archives, checksums, a GitHub
Release, the `ccloud` cask in `cloudwalksolutions/homebrew-tap`), and pushes the tag only after the
release succeeded, so a failed run leaves nothing behind and can be re-run.

Merges touching only Markdown, `docs/` or `specs/` do not release. Bump minor or major by pushing
the tag yourself; the next merge patches from it.

Actions secrets, set by hand with `gh secret set <NAME> -R cloudwalksolutions/ccloud`:

- `HOMEBREW_TAP_TOKEN` — a fine-grained PAT with Contents read/write on `homebrew-tap`.
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` — CloudWalk's Google Desktop OAuth client, the
  audience its API servers trust. Google treats a desktop client's secret as public; it is kept out
  of the repo only so another cloud's build never carries it.

The binary is unsigned; the cask strips the quarantine attribute on install.

To redo a release, `gh release delete vX.Y.Z --yes --cleanup-tag` and re-run the workflow.
