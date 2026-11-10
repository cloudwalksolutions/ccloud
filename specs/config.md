# Your cloud from config

Today a build serves the one cloud compiled into it (`internal/cloud/cloudwalk`). Next:

- `ccloud init <url>` fetches a cloud definition over HTTPS into `~/.config/ccloud/config.yaml`,
  and `main` loads it in place of `cloudwalk.Cloud()` (`--config` / `CCLOUD_CONFIG` to pick
  another file). Trust rides on the URL's web PKI; the cluster CAs come from the definition, never
  trust-on-first-use. The datacenter publishes the definition from the values it already owns.
- With that, the OIDC client travels in the definition and leaves the build.
- Developer ID signing and notarization before onboarding people outside CloudWalk: GoReleaser's
  `notarize.macos` with an Apple Developer account and its five secrets.
