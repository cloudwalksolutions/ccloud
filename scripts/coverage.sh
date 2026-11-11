#!/usr/bin/env bash
set -euo pipefail

go test -count=1 ./... -coverprofile=cover.out -covermode=atomic -coverpkg=./...
report=$(go run github.com/vladopajic/go-test-coverage/v2@v2.23.0 --config=./.testcoverage.yml) || {
  echo "$report"
  exit 1
}
echo "$report"

total=$(sed -n 's/^Total test coverage: \([0-9]*\).*/\1/p' <<<"$report")
gate=$(sed -n 's/^ *total: *\([0-9]*\).*/\1/p' .testcoverage.yml)
if ((total > gate)); then
  echo "Coverage is ${total}% but the gate is ${gate}%: raise threshold.total in .testcoverage.yml to ${total}." >&2
  exit 1
fi
