# Termon

Termon is a multiplayer monster-battling game served over SSH. Explore the Dojo, build a party of original monsters, complete solo activities, and fight turn-based battles without installing a game client.

## Quick start

You need Go 1.27 or later and OpenSSH. From the repository root:

```sh
go run ./cmd/termond -listen 127.0.0.1:2222
```

Connect from another terminal:

```sh
ssh -p 2222 localhost
```

The development command uses an unprivileged port. The binary itself defaults to `127.0.0.1:22`, so a process with permission to bind that port accepts `ssh localhost` without a port flag. Public deployments must bind or publish port 22, as shown below.

Your SSH public key identifies your Trainer. The first connection creates a save and guides you through choosing a starter and completing two Capture Lessons. Afterward, explore the Dojo, manage your party, run Expeditions, spar with Master Sable, or press `f` to configure and join the multiplayer Queue.

## Run with Docker

The container keeps its non-root SSH listener on port 2222 and publishes it on the host's standard SSH port:

```sh
docker build -f Containerfile -t termond .
docker run --rm -v termon-data:/data -p 22:2222 termond
ssh localhost
```

Port 22 must be free on the host. Keep the `termon-data` volume: it contains the SQLite database and SSH host key.

## Development

```sh
./scripts/check.sh              # complete pre-merge verification
go tool lefthook install        # install Git hooks once
```

The check script runs independent checks concurrently, then runs race tests without competing CPU-heavy work. Go tools are pinned in `go.mod`; you don't need global tool installations.

## Documentation

- [Documentation index](docs/README.md) — gameplay, operations, and design decisions.
- [Local multiplayer walkthrough](docs/local-mvp.md) — run an isolated two-client test.
- [Deployment runbook](docs/deployment.md) — provision Dokploy, configure sslh, back up, restore, upgrade, and roll back.
- [Operations guide](docs/operations.md) — runtime topology, metrics, limits, and recovery behavior.
- [Domain vocabulary](CONTEXT.md) — canonical game terms used in code and documentation.

See [AGENTS.md](AGENTS.md) for repository conventions and commands used by coding agents.

## License

Termon is released under the [MIT License](LICENSE).
