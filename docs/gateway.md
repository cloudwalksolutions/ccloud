# The gateway contract

Each region of a cloud serves one HTTPS gateway (`Region.Gateway`) that ccloud asks for whatever
the Kubernetes API does not hold. Any cloud implements this contract; ccloud knows nothing else
about it. Every call carries `Authorization: Bearer <the caller's cluster ID token>`, and the
gateway answers as that caller, exactly as the cluster's RBAC would.

## `GET /v1/logs`

Query: `namespace` (one or more, required), `start`, `end` (RFC 3339), `limit`, `direction`
(`forward` or `backward`), `pod` (prefix), `container`, `contains`, `regex`, `level` (`error` or
`warn`).

```json
{"entries": [{"time": "2026-10-26T07:33:21Z", "namespace": "prod-shop", "pod": "api-7f9c", "container": "api", "line": "db query failed"}]}
```

403 when the caller may not read pod logs in every namespace named.

## `GET /v1/apps/{app}`

The environments this region hosts for the app that the caller may read.

```json
{"environments": [{"name": "ci", "namespace": "ci-shop"}, {"name": "prod", "namespace": "prod-shop", "url": "https://shop.example"}]}
```

`url` is absent when the environment serves nothing. An empty list means this region hosts none the
caller can see.
