# Deploy Termon through Dokploy

This runbook deploys `termond` and `sslh` from [`compose.production.yml`](../compose.production.yml), keeps all durable state in the `termon-data` named volume, and places `sslh` in front of SSH on TCP ports 22 and 443. It targets a fresh Ubuntu 24.04 VPS with Dokploy and uses UTC for every schedule and timestamp.

The repository prepares the deployment, but the operator must record the live values and restore-drill evidence in the final checklist before declaring a deployment verified.

## Production layout

```text
TCP 22 ─┐                          ┌─ SSH + PROXY v1 ─ termond:2222
        ├─ sslh Compose service ──┤
TCP 443 ┘                          └─ TLS ──────────── dokploy-traefik:4443

UDP 443 ───────────────────────────────────────── Traefik HTTP/3
TCP 80  ───────────────────────────────────────── Traefik HTTP/ACME
```

`termond` expects PROXY headers because every public SSH path passes through `sslh`. Never publish container port 2222 publicly while `-proxy-protocol` is enabled; a direct client cannot supply the required header, and an untrusted client must not be allowed to forge one.

## Record release compatibility

The current binary supports relational schema version **4** and Save version **1**. Before each deployment, add the candidate image digest and its supported versions to the release record:

| Released at (UTC) | Git commit | Image digest | Maximum schema | Maximum Save | Pre-upgrade backup |
|---|---|---|---:|---:|---|
| `YYYY-MM-DD HH:MM` | `GIT_COMMIT` | `sha256:IMAGE_DIGEST` | 4 | 1 | `BACKUP_ID` |

Replace the following:

- `GIT_COMMIT`: the deployed Git commit
- `IMAGE_DIGEST`: the immutable digest reported by Docker or Dokploy
- `BACKUP_ID`: the successful stopped-container backup created before the upgrade

A previous image is a safe rollback only if both of its maximum versions are at least the versions reported by the current `/readyz` response. Termon has no down migrations. If the previous image is incompatible, stop writes and restore `BACKUP_ID` before starting it.

## Provision the VPS

1. Create an Ubuntu 24.04 VPS with at least 4 vCPU and 4 GiB RAM.
2. Add grey-clouded Cloudflare `A` and, when the VPS has IPv6, `AAAA` records for `termon.sh`. Grey-clouding is required because normal Cloudflare proxying does not forward arbitrary SSH traffic.
3. Move administrative OpenSSH from port 22 to port 22022 before installing `sslh`:

   ```bash
   sudo install -m 0644 /etc/ssh/sshd_config /etc/ssh/sshd_config.before-termon
   printf 'Port 22022\n' | sudo tee /etc/ssh/sshd_config.d/20-admin-port.conf
   sudo sshd -t
   sudo systemctl reload ssh
   ```

4. Open a second terminal and confirm that `ssh -p 22022 ADMIN_USER@VPS_IP` works. Keep the first session open until the new session succeeds.
5. Allow TCP 22, 80, 443, and 22022 plus UDP 443 in both the provider firewall and UFW. Restrict 22022 to the administrator's source network when possible. Enable the provider's SYN-flood or connection-rate protection for TCP 22 and 443 when available; application quotas cannot protect `sslh` from a flood that never reaches termond.

   Docker-published ports may bypass ordinary UFW forwarding policy. Treat the provider firewall as the outer control, and inspect the host's `DOCKER-USER`/nftables path after Docker is installed rather than assuming these UFW rules constrain container traffic:

   ```bash
   sudo ufw allow 22/tcp
   sudo ufw allow 80/tcp
   sudo ufw allow 443/tcp
   sudo ufw allow 443/udp
   sudo ufw allow 22022/tcp
   sudo ufw enable
   ```

6. Install Dokploy with its current official installation command from [the Dokploy installation guide](https://docs.dokploy.com/docs/core/installation). Do not copy an old installer command from this repository.

## Move Dokploy HTTPS behind sslh

Dokploy's Traefik owns TCP 443 by default. Move its `websecure` listener to TCP 4443 before deploying the Compose service; `sslh` reaches it over the external `dokploy-network`. Leave UDP 443 on Traefik so HTTP/3 continues to work.

1. Persist the alternate TLS port on the Dokploy service so later Traefik recreations keep it:

   ```bash
   sudo docker service update \
     --env-add TRAEFIK_SSL_PORT=4443 \
     --env-add TRAEFIK_HTTP3_PORT=443 \
     dokploy
   ```

2. Back up the Traefik configuration and change only the `websecure` address:

   ```bash
   sudo cp /etc/dokploy/traefik/traefik.yml \
     /etc/dokploy/traefik/traefik.yml.before-sslh
   sudo python3 - <<'PY'
   from pathlib import Path

   path = Path('/etc/dokploy/traefik/traefik.yml')
   text = path.read_text()
   marker = 'websecure:'
   before, separator, after = text.partition(marker)
   if not separator or 'address: :443' not in after:
       raise SystemExit('expected websecure address :443 exactly once')
   after = after.replace('address: :443', 'address: :4443', 1)
   path.write_text(before + separator + after)
   PY
   ```

3. Record the existing Traefik image, remove the old container, and recreate the default fresh-install container with TCP 4443 bound to loopback:

   ```bash
   TRAEFIK_IMAGE=$(sudo docker inspect -f '{{.Config.Image}}' dokploy-traefik)
   sudo docker rm -f dokploy-traefik
   sudo docker run -d \
     --name dokploy-traefik \
     --restart always \
     -v /etc/dokploy/traefik/traefik.yml:/etc/traefik/traefik.yml \
     -v /etc/dokploy/traefik/dynamic:/etc/dokploy/traefik/dynamic \
     -v /var/run/docker.sock:/var/run/docker.sock \
     -p 80:80 \
     -p 127.0.0.1:4443:4443 \
     -p 443:443/udp \
     "$TRAEFIK_IMAGE"
   sudo docker network connect dokploy-network dokploy-traefik
   ```

   This command matches a fresh Dokploy installation. If `docker inspect dokploy-traefik` showed extra mounts, environment variables, or published ports before removal, reproduce them in the replacement command.

4. Confirm that HTTPS reaches Traefik directly before adding the multiplexer:

   ```bash
   curl --resolve termon.sh:4443:127.0.0.1 https://termon.sh:4443/
   ```

The Compose service uses a digest-pinned `sslh-select` 2.3.1 image because Ubuntu 24.04's older package ignores the backend PROXY-protocol and connection-limit settings in [`deploy/sslh.cfg`](../deploy/sslh.cfg). Recheck the port layout after every Dokploy or Traefik upgrade. Stop and correct Traefik if it reclaims TCP 443; do not leave two containers racing for the public listener.

## Publish an immutable release

The [`Release Please` workflow](../.github/workflows/release-please.yml) reads Conventional Commit subjects on `main`, maintains a release PR and changelog, and creates the version tag and GitHub Release when that PR merges. The manifest starts at `0.0.0`, so the first `feat:` release is `v0.1.0`; before 1.0, feature and fix bumps follow [`release-please-config.json`](../release-please-config.json).

Publishing the GitHub Release triggers the separate [`Release` workflow](../.github/workflows/release.yml). It checks out that exact tag, runs the complete repository check, builds the production container, rejects fixable High or Critical vulnerabilities, and publishes `vMAJOR.MINOR.PATCH`, `MAJOR.MINOR.PATCH`, commit-SHA, and `latest` tags to GHCR with provenance and an SBOM. A manual dispatch may republish an existing release tag after an infrastructure failure; it must not invent a tag. The workflow summary prints the only supported production reference:

```text
ghcr.io/OWNER/REPOSITORY@sha256:DIGEST
```

Use the digest reference, not either convenience tag. A tag can move; the digest ties deployment, rollback, telemetry version, and incident evidence to the bytes that passed the release checks. Before the first release, configure the repository package as readable by the production deployment or give Dokploy a read-only GHCR credential.

## Deploy termond in Dokploy

1. Merge the Release Please PR, wait for the GitHub Release and image-publishing workflow to succeed, and copy the digest from its workflow summary.
2. Create a **Compose** service in Dokploy and select the repository, production branch, and `compose.production.yml`. Dokploy reads the Compose definition from Git but does not build the application image.
3. Set `TERMON_IMAGE` to the complete `ghcr.io/...@sha256:...` value printed in the workflow summary. Reject a value without `@sha256:` during operator review. Set `POSTHOG_API_KEY` to the Termon Production project token, `POSTHOG_HOST=https://us.i.posthog.com`, and `TERMON_ENVIRONMENT=production`; never reuse the Development token. Set `POSTHOG_LOGS_ENABLED=true` only after reviewing PostHog Logs billing and validating the same integration in Development. An empty key disables all PostHog delivery safely.
4. Deploy the service. Dokploy pulls the prebuilt [`Containerfile`](../Containerfile) artifact, creates the stable `termon-data` volume, and attaches termond only to the internal `termon-edge` network. The `sslh` service is the only other member of that network; it publishes TCP 22 and 443 and separately joins Dokploy's external network for TLS forwarding. Compose caps `sslh` at 1 CPU, 128 MiB, and 128 processes, and caps termond at 2 CPUs, 2 GiB, and 512 processes so ingress pressure cannot consume the entire VPS.
5. Find the running container and confirm its health:

   ```bash
   TERMON_CONTAINER=$(sudo docker ps --filter volume=termon-data --format '{{.ID}}' | head -n1)
   test -n "$TERMON_CONTAINER"
   sudo docker inspect --format '{{.State.Health.Status}}' "$TERMON_CONTAINER"
   sudo docker exec "$TERMON_CONTAINER" /app/termond \
     -healthcheck-url http://127.0.0.1:9090/readyz
   ```

6. Record the generated host-key fingerprint through the volume mount, then publish it through a trusted HTTPS page:

   ```bash
   DATA_DIR=$(sudo docker volume inspect termon-data --format '{{.Mountpoint}}')
   sudo ssh-keygen -y -f "$DATA_DIR/host-key" | ssh-keygen -lf -
   ```

7. Confirm that the deployment exposes only the intended listeners, resource limits are active, and only `sslh` and termond belong to the internal edge network. The termond metrics and readiness endpoint remain inside its network namespace:

   ```bash
   sudo ss -lntup | grep -E ':(22|80|443|4443|22022)\b'
   sudo docker port "$TERMON_CONTAINER"
   sudo docker inspect "$TERMON_CONTAINER" \
     --format 'memory={{.HostConfig.Memory}} nano_cpus={{.HostConfig.NanoCpus}} pids={{.HostConfig.PidsLimit}}'
   EDGE_NETWORK=$(sudo docker inspect "$TERMON_CONTAINER" \
     --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}')
   sudo docker network inspect "$EDGE_NETWORK" \
     --format '{{range $id, $container := .Containers}}{{$container.Name}} {{end}}'
   ```

   The first command must not show a host listener for port 2222. `docker port` must print nothing for termond, and the internal network membership must contain only the Compose `sslh` and termond containers. Stop deployment if another container is attached; any member of this network can reach the PROXY-trusting listener.

## Configure stopped-container backups

Use one dedicated S3 bucket or prefix for Termon. Require TLS to the S3 endpoint, enable the provider's server-side encryption at rest, and grant the Dokploy credential only list/read/write/delete access to that bucket or prefix. Store the credential only in Dokploy's destination settings, restrict access to production operators, and rotate it according to the provider's credential policy.

1. Add the S3 destination in Dokploy under **Destinations** and verify its connection.
2. Add a Volume Backup for `termon-data` with **Turn off Container** enabled. A live copy is not acceptable because SQLite may have committed data in the WAL sidecar.
3. Set the schedule to `0 3 * * *` in UTC, which starts one backup daily at 03:00 UTC.
4. Set an S3 lifecycle rule that expires backup objects after 14 days. Dokploy schedules the copy; the bucket lifecycle enforces retention.
5. Trigger one manual backup before launch and record its backup ID, object path, start time, finish time, byte size, and measured SSH downtime.
6. Confirm that Dokploy restarted termond and that Docker reports it healthy.

The backup window is scheduled downtime: new SSH sessions fail and active Battles are abandoned without a result while the container is stopped. Announce the measured window to players and keep the 03:00 UTC schedule outside the expected activity peak.

## Verify a restore drill

Run this drill before launch and at least after any storage or backup-provider change.

1. Connect two test Trainers, finish one multiplayer Battle, and record both handles, W/L totals, the Battle ID from JSON logs, and the SSH host-key fingerprint.
2. Trigger a stopped-container backup and record its ID.
3. Change one test Trainer after the backup so the restored state is distinguishable.
4. Stop the Compose service in Dokploy.
5. Restore the recorded Volume Backup into `termon-data` without starting termond midway through the copy.
6. Start the service and wait for Docker health to report `healthy`. Startup runs SQLite `quick_check`, applies supported forward migrations, and refuses a relational schema or Save version newer than the binary.
7. Verify the restored database while the service is stopped:

   ```bash
   DATA_DIR=$(sudo docker volume inspect termon-data --format '{{.Mountpoint}}')
   sudo sqlite3 "$DATA_DIR/termon.db" 'PRAGMA integrity_check;'
   sudo sqlite3 -header -column "$DATA_DIR/termon.db" \
     'SELECT handle, wins, losses, save_version FROM trainers ORDER BY handle;'
   sudo sqlite3 -header -column "$DATA_DIR/termon.db" \
     'SELECT id, winner_id, loser_id, reason, completed_at FROM battle_results ORDER BY completed_at DESC LIMIT 5;'
   ```

8. Start the service, confirm the readiness probe, and reconnect both Trainers. Verify that their Saves and the recorded Battle Result match the backup point.
9. Recompute the SSH host-key fingerprint and compare it byte-for-byte with the pre-backup fingerprint.
10. Record the drill date, operator, backup ID, restore duration, downtime, integrity result, schema/Save versions, Trainer check, Battle Result check, host-key fingerprint check, and final outcome.

## Upgrade and roll back

1. Record the candidate's `TERMON_IMAGE` digest plus schema and Save compatibility in the release table.
2. Trigger a stopped-container backup and record its ID before deploying an image that can migrate data.
3. Replace `TERMON_IMAGE` in Dokploy with the candidate digest, deploy it, then check Docker health, `/readyz`, logs, both SSH ports, one Trainer load, and one completed test Battle.
4. Roll back directly by restoring the previous `TERMON_IMAGE` value only when that image supports the current schema and Save versions.
5. Otherwise stop the service, restore the pre-upgrade backup with writes stopped, set `TERMON_IMAGE` to the previous digest, and deploy it before accepting SSH sessions.

Never copy only `termon.db`; the named volume also contains WAL/SHM sidecars and the SSH host key.

## Logs and smoke checks

Read application logs in Dokploy or with Docker:

```bash
sudo docker logs --since 15m "$TERMON_CONTAINER"
SSLH_CONTAINER=$(sudo docker ps --filter ancestor=ghcr.io/yrutschle/sslh --format '{{.ID}}' | head -n1)
sudo docker logs --since 15m "$SSLH_CONTAINER"
sudo docker logs --since 15m dokploy-traefik
```

Run the repeatable protocol and host-key check from a machine outside the VPS:

```bash
./scripts/deployment-smoke.sh termon.sh
```

The script requires `ssh-keyscan`, `ssh-keygen`, and `curl`. It fails unless SSH responds on ports 22 and 443 with the same Ed25519 host key and HTTPS succeeds through Traefik. Then open an interactive session with `ssh termon.sh`, confirm the Termon banner, and verify in the JSON log that `source` is the client's real public address rather than a Docker-network address. The script verifies reachability and identity but cannot inspect the server-side PROXY result.

## Deployment sign-off

Do not mark a live deployment verified until every field has evidence:

- [ ] Grey-clouded DNS resolves to the VPS.
- [ ] TCP 22 and TCP 443 both reach termond through `sslh` with real client addresses.
- [ ] HTTPS still reaches Traefik; UDP 443 remains available for HTTP/3.
- [ ] Docker health is `healthy`, and readiness reports schema 4, Save 1, WAL, and `synchronous=normal`.
- [ ] The `termon-data` volume survives a restart and retains the host key.
- [ ] A daily 03:00 UTC stopped-container backup succeeds, with 14-day encrypted S3 retention and least-privilege access.
- [ ] The restore drill recovers Trainer data, the recorded Battle Result, schema/Save compatibility, and the exact host key.
- [ ] Backup and restore downtime is measured and communicated.
- [ ] Product events and deliberate Error Tracking arrive in PostHog project `594989`; Logs also arrive when `POSTHOG_LOGS_ENABLED=true`.
- [ ] Upgrade and rollback records include image digests, compatibility, and the pre-upgrade backup ID.

Cloudflare private-beta TCP enrollment remains a separate future migration; it does not block this VPS deployment.
