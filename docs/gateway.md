# The gateway contract

Each region of a cloud serves one HTTPS gateway (`Region.Gateway`) that ccloud asks for whatever
the Kubernetes API does not hold. Any cloud implements this contract; ccloud knows nothing else
about it. Every call carries `Authorization: Bearer <the caller's cluster ID token>`, and the
gateway answers as that caller, exactly as the cluster's RBAC would.

## `GET /v1/logs`

Query: `namespace` (one or more, required), `start`, `end` (RFC 3339), `limit` (1 to 5000),
`direction` (`forward` or `backward`), `pod` (prefix), `container` (one or more), `contains`,
`regex`, `level` (`error` or `warn`).

```json
{"entries": [{"time": "2026-10-26T07:33:21Z", "namespace": "prod-acme", "pod": "api-7f9c", "container": "api",
  "stream": "stderr", "level": "error", "line": "db query failed"}], "truncated": false}
```

Entries come oldest first. `stream` is `stdout` or `stderr`. `level` is `error`, `warn` or empty, by
the same patterns `level` filters on. `truncated` is true when `limit` cut the answer short. 403
when the caller may not read pod logs in every namespace named; 400 for a parameter it cannot use.

## `GET /v1/logs/containers`

Query: `namespace` (one or more, required), `start`. The containers those namespaces have logged
since `start`.

```json
{"containers": ["api", "nginx"]}
```

## `GET /v1/logs/stream`

The same query as `/v1/logs` but `start` alone, answering NDJSON (`application/x-ndjson`): one
entry per line as it arrives, each sent once, from `start` on (a few seconds ago when absent). A
quiet stream sends `{}` now and then so proxies keep it open. A stream ends with one line naming
why: `{"end": "max duration"}` (reconnect from the last entry's time), `{"end": "access revoked"}`,
or another reason. 429 when the caller already follows as many streams as the gateway allows.

## `GET /v1/apps/{app}`

The environments this region hosts for the app that the caller may read.

```json
{"environments": [{"name": "ci", "namespace": "ci-acme"}, {"name": "prod", "namespace": "prod-acme", "url": "https://acme.example"}]}
```

`url` is absent when the environment serves nothing. An empty list means this region hosts none the
caller can see.

## `GET /v1/apps/{app}/resources`

Query: `env` (optional, one environment). What each container of those environments uses, and the
pressure on the nodes they run on. Raw numbers; the client decides what to flag.

```json
{"environments": [{"name": "prod", "namespace": "prod-acme",
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

## `GET /v1/apps/{app}/summary`

Query: `weeks` (default 4). Served by the region running the app's `ci` environment; 404 there for a
caller who may not read it.

```json
{"pulls": [{"number": 105, "title": "Search by lake", "author": "dev", "branch": "feat/BOAT-105/search",
   "openedHoursAgo": 72, "draft": false, "reviewed": true, "environment": "https://boat-105.acme.dev",
   "checks": [{"flow": "api-tests", "status": "Succeeded", "run": "api-tests-x1"}]}],
 "flow": {"merged": 2, "openToMergeHours": 12, "firstReviewHours": 1.5},
 "delivery": {"deploys": 1, "mergeToProdHours": 1},
 "ci": [{"flow": "api-tests", "runs": 3, "medianSeconds": 700, "firstTryPassRate": 0.5, "reruns": 1}]}
```

`checks` are each flow's latest run on the pull request's head commit. `flow`, `delivery` and `ci`
cover the last `weeks`: medians in hours, one prod deploy per commit deployed, and a commit's first
CI run per flow as its first try.
