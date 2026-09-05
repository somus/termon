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

scan_key() {
  local port=$1
  local output=$2
  ssh-keyscan -T "$timeout_seconds" -p "$port" -t ed25519 "$host" 2>/dev/null >"$output"
  if [[ ! -s "$output" ]]; then
    echo "SSH on ${host}:${port} did not return an Ed25519 host key" >&2
    return 1
  fi
}

scan_key 22 "$workdir/ssh-22"
scan_key 443 "$workdir/ssh-443"

key_22=$(awk '{print $2 " " $3}' "$workdir/ssh-22" | sort -u)
key_443=$(awk '{print $2 " " $3}' "$workdir/ssh-443" | sort -u)
if [[ "$key_22" != "$key_443" ]]; then
  echo "SSH host keys differ between ports 22 and 443" >&2
  exit 1
fi

fingerprint=$(ssh-keygen -lf "$workdir/ssh-22" -E sha256 | awk '{print $2}' | sort -u)
curl --fail --silent --show-error --location \
  --connect-timeout "$timeout_seconds" \
  --max-time "$timeout_seconds" \
  --output /dev/null \
  "$https_url"

printf 'SSH/22: ok\nSSH/443: ok\nHTTPS: ok\nHost key: %s\n' "$fingerprint"
