# Scenarios

Each is what a developer is trying to find out, the commands they'd run, and what to check.
`$CC` is the binary under test; `<app>` and `<env>` are the app named for this run and one of its
environments.

## Where does my app run?
`$CC app -a <app>`
- One row per environment, with its region and URL, sorted by name.
- The environments match the `<env>-<app>` namespaces on each `ccloud-<region>` context.

## Did my pull request's CI pass, and how have the last weeks gone?
`$CC app -a <app> --summary`, then `$CC workflow list -a <app>`, then
`$CC workflow view <run> -a <app>` for a run from the list.
- The summary's checks agree with `workflow list` for the same pull request.
- Medians are in whole seconds; percentages and counts read naturally ("1 run", "2 runs").
- `view` lists steps in the order they ran, the same way every time, with a reason on a failed one.

## Why is my environment slow?
`$CC top -a <app>`, then `$CC top -a <app> -e <env> -v`, then
`$CC logs -a <app> -e <env> -l warn --since 1h`, then `$CC net -a <app> -e <env>`.
- `top` is a tree: the app, its environments (prod, stage, ci, then features in number order), and
  each one's workloads under their Deployment or StatefulSet, not one row per pod.
- Pod counts match `kubectl get pods`; a container that isn't running shows with its reason.
- Issues are colored by severity; "no limit" hints are grey, not red.
- `net` groups egress and drops under the workload that opened them, lists only hosts with
  traffic, and colors only values past their thresholds.

## Is prod erroring?
`$CC logs -a <app> -e prod -l error --since 6h`, `$CC logs containers -a <app> -e prod`, and
`$CC net -a <app>` for every environment at once.
- A note appears when `--tail` cut lines off.
- Following (`-f`) for a minute shows each line once.

## Back up before a risky change
`$CC db backup -a <app> -e stage --note "live qa"` (stage only; it adds a backup and changes
nothing else).
- Progress goes to stderr, and stdout ends with the backup's label, "just now", a size with its
  unit, and the note.
- The label is the newest in `pgbackrest info --stanza=db` run in the stage primary's `database`
  container, with the note as its annotation.
- A note with a quote in it is refused before anything is submitted.

## Scripts and pipes
`$CC top -a <app> >out 2>err`, `$CC net -a <app> | cat`,
`$CC logs -a <app> -e prod --since 10m -o json`.
- stdout has no `\x1b` and no spinner frames; stderr has the plain progress lines.
- Every JSON line parses, and the last one is `{"next": …}`.

## Color off and on
`NO_COLOR=1 script -q /dev/null $CC top -a <app>` and `CLICOLOR_FORCE=1 $CC top -a <app> | cat -v`.
- The first has no color codes, the second has them, and both line up.

## Errors a developer will hit
- Outside an app repo without `-a`: says to pass `--app`.
- An app no region runs: says so, with no crash.
- An environment that doesn't exist: lists the ones that do.
- An unknown region: names the valid ones.
- Each exits non-zero and prints nothing on stdout.
