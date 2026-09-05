#!/bin/sh
# Run real concurrent SSH sessions against a 4 CPU / 4 GiB termond container.
set -eu
cd "$(dirname "$0")/.."

image=${TERMON_SSH_IMAGE:-termon-ssh-baseline:local}
levels=${TERMON_SSH_LEVELS:-"256 512"}
hold=${TERMON_SSH_HOLD:-2s}
startup_timeout=${TERMON_SSH_STARTUP_TIMEOUT:-15s}
container="termon-ssh-baseline-$$"

cleanup() {
	docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker build --progress=plain -f Dockerfile.ssh-baseline -t "$image" .
docker run --rm -d \
	--name "$container" \
	--cpus=4 \
	--memory=4g \
	--memory-swap=4g \
	-p 127.0.0.1::2222 \
	"$image" >/dev/null

address=$(docker port "$container" 2222/tcp)
for attempt in $(seq 1 100); do
	if nc -z "${address%:*}" "${address##*:}"; then
		break
	fi
	if [ "$attempt" -eq 100 ]; then
		docker logs "$container"
		exit 1
	fi
	sleep 0.1
done

for trainers in $levels; do
	echo "ssh-baseline: trainers=$trainers hold=$hold startup_timeout=$startup_timeout cpus=4 memory=4096MiB address=$address"
	go run ./cmd/termon-ssh-load \
		-address "$address" \
		-trainers "$trainers" \
		-hold "$hold" \
		-startup-timeout "$startup_timeout"
done

oom=$(docker inspect -f '{{.State.OOMKilled}}' "$container")
if [ "$oom" != "false" ]; then
	echo "termond was OOM-killed" >&2
	exit 1
fi
