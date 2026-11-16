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

## `GET /v1/apps/{app}/resources`

Query: `env` (optional, one environment). What each container of those environments uses, and the
pressure on the nodes they run on. Raw numbers; the client decides what to flag.

```json
{"environments": [{"name": "prod", "namespace": "prod-shop",
  "containers": [{"pod": "api-7f9c", "container": "api", "node": "node02",
    "cpu": {"used": 0.31, "request": 0.25, "limit": 1}, "memory": {"used": 412000000, "limit": 1073741824},
    "throttled": 0.02, "restarts": 0, "oomKills": 0}],
  "nodes": [{"name": "node02", "cpuWait": 0.22, "memoryStall": 0.01, "conditions": ["DiskPressure"]}]}]}
```

CPU is in cores and memory in bytes. `request` and `limit` are absent when unset, `throttled` (the
share of CPU periods held back) when there is no CPU limit. `restarts` and `oomKills` count the last
24 hours. `cpuWait` and `memoryStall` are the share of time the node's tasks waited.

## `GET /v1/apps/{app}/network`

Query: `env` (required), `since` (a Go duration, default `1h`). One environment's traffic over that
window. 400 without `env`; 404 when this region hosts no such environment the caller may read.

```json
{"ingress": {"requestsPerSecond": 3.2, "serverErrors": 0.005, "p95Seconds": 0.21},
 "egress": [{"destination": "hydro.nationalmap.gov", "port": 443, "connections": 412, "unanswered": 398, "reset": 3}],
 "drops": [{"destination": "10.0.4.9", "port": 5432, "reason": "POLICY_DENIED", "count": 12}]}
```

`serverErrors` is the share of requests answered 5xx. `egress` lists internet destinations by DNS
name where the cloud saw the lookup, else by IP: `connections` opened, `unanswered` of them with no
SYN-ACK back, and `reset` by the far end. `drops` are connections the network refused.
