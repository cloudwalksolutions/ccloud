# `ccloud logs`

`ccloud logs <app> [--env <lifecycle|feature>] [--follow]` queries Loki through Grafana's API with
the developer's Access session, filtered to the namespaces of apps their domain owns, and prints
them readably: one line per entry, timestamp, pod, level, message, with JSON bodies flattened.

Grafana itself stays unrestricted; the CLI only filters and formats. Loki holds five years of logs,
including CI pods that are deleted an hour after they finish, so this outlives `kubectl logs`.

The cloud definition gains the Grafana host per region.
