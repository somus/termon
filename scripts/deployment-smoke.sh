#!/usr/bin/env bash
set -euo pipefail

host=${1:-}
https_url=${2:-}
timeout_seconds=${TERMON_SMOKE_TIMEOUT:-10}

if [[ -z "$host" ]]; then
  echo "usage: $0 HOST [HTTPS_URL]" >&2
  exit 2
fi
if [[ -z "$https_url" ]]; then
  https_url="https://${host}/"
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

key_file="$workdir/ssh-22"
ssh-keyscan -T "$timeout_seconds" -p 22 -t ed25519 "$host" 2>/dev/null >"$key_file"
if [[ ! -s "$key_file" ]]; then
  echo "SSH on ${host}:22 did not return an Ed25519 host key" >&2
  exit 1
fi

fingerprint=$(ssh-keygen -lf "$key_file" -E sha256 | awk '{print $2}' | sort -u)
curl --fail --silent --show-error --location \
  --connect-timeout "$timeout_seconds" \
  --max-time "$timeout_seconds" \
  --output /dev/null \
  "$https_url"

printf 'SSH/22: ok\nHTTPS: ok\nHost key: %s\n' "$fingerprint"
