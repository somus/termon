#!/bin/sh
# Run independent checks together, then give the race detector the machine.
set -u
cd "$(dirname "$0")/.."

work=$(mktemp -d "${TMPDIR:-/tmp}/termon-check.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

run_check() {
	name=$1
	shift
	"$@" >"$work/$name.log" 2>&1 &
	eval "${name}_pid=$!"
}

wait_check() {
	name=$1
	eval "pid=\${${name}_pid}"
	if wait "$pid"; then
		printf '%s\n' "ok $name"
		return 0
	fi
	printf '%s\n' "failed $name" >&2
	cat "$work/$name.log" >&2
	return 1
}

run_check lint sh -c 'go run ./cmd/checkgocognit && go tool golangci-lint run'
run_check build sh -c 'go vet ./... && go build ./...'
run_check vuln go tool govulncheck ./...
run_check balance go run ./cmd/balancerun -content ./content -fail-gates -capture

failed=0
for check in lint build vuln balance; do
	wait_check "$check" || failed=1
done
[ "$failed" -eq 0 ] || exit 1

printf '%s\n' 'running race tests without competing checks'
go test -race ./...
