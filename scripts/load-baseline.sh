#!/bin/sh
# Run the reproducible 4 CPU / 4 GiB multi-Dojo baseline.
set -eu
cd "$(dirname "$0")/.."

image=${TERMON_LOAD_IMAGE:-termon-load:local}
rounds=${TERMON_LOAD_ROUNDS:-10}
levels=${TERMON_LOAD_LEVELS:-"32 64 128 256"}
synchronous=${TERMON_SQLITE_SYNCHRONOUS:-normal}

docker build --progress=plain -f Dockerfile.loadtest -t "$image" .
for trainers in $levels; do
	echo "baseline: trainers=$trainers rounds=$rounds cpus=4 memory=4096MiB synchronous=$synchronous"
	docker run --rm \
		--cpus=4 \
		--memory=4g \
		--memory-swap=4g \
		"$image" \
		-database /termon.db \
		-sqlite-synchronous "$synchronous" \
		-trainers "$trainers" \
		-rounds "$rounds"
done
