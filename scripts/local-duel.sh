#!/bin/sh
# Start an isolated localhost server and print commands for two SSH clients.
set -eu
cd "$(dirname "$0")/.."

port=${TERMON_PORT:-2222}
if [ -n "${TERMON_RUN_DIR:-}" ]; then
	run_dir=$TERMON_RUN_DIR
	mkdir -p "$run_dir"
else
	run_dir=$(mktemp -d "${TMPDIR:-/tmp}/termon-mvp.XXXXXX")
fi

mkdir -p "$run_dir/content" "$run_dir/data" "$run_dir/ssh"
cp -R content/. "$run_dir/content/"

for trainer in trainer-a trainer-b; do
	if [ ! -f "$run_dir/ssh/$trainer" ]; then
		ssh-keygen -q -t ed25519 -N "" -C "termon-$trainer" -f "$run_dir/ssh/$trainer"
	fi
done

echo "Isolated state: $run_dir"
echo
echo "Terminal A:"
echo "  ssh -F /dev/null -tt -i \"$run_dir/ssh/trainer-a\" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $port 127.0.0.1"
echo
echo "Terminal B:"
echo "  ssh -F /dev/null -tt -i \"$run_dir/ssh/trainer-b\" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $port 127.0.0.1"
echo
echo "In the Dojo, press F in both clients to join the Queue."
echo "Starting termond on 127.0.0.1:$port. Press Ctrl+C to stop it."

exec go run ./cmd/termond \
	-content "$run_dir/content" \
	-database "$run_dir/data/termon.db" \
	-host-key "$run_dir/ssh/termond_ed25519" \
	-listen "127.0.0.1:$port"
