# AGENTS.md

Termon is a single-binary SSH multiplayer monster-battling game built with Wish and Bubble Tea.

## Work in this repository

Use the Go tools pinned in `go.mod`; no global tool installation is required.

```sh
go tool golangci-lint fmt       # format before reviewing a diff
go test ./...                    # normal verification
go test -race ./...              # required for concurrency changes and before merging
go tool golangci-lint run        # repository lint policy
go vet ./...
go build ./...
go tool govulncheck ./...
```

Run the complete pre-merge suite with `./scripts/check.sh`; it runs lint, vet/build, vulnerability scanning, and the Balance Run concurrently, then gives the race detector the machine by itself. Run the server locally on an unprivileged port with `go run ./cmd/termond -listen 127.0.0.1:2222`. Install the repository hooks once with `go tool lefthook install`.

## Architecture

- `cmd/termond` is the server composition root. `internal/server.Hub` owns authoritative multiplayer state and is the only application layer that coordinates state mutations.
- `internal/battle` owns the three-Monster combat engine. `internal/capture`, `internal/dojo`, `internal/expedition`, and `internal/queue` own their gameplay policies rather than the TUI.
- `internal/tui` owns Bubble Tea presentation and input routing. Keep game rules in the domain packages so non-TUI harnesses use the same behavior.
- `internal/store` defines persistence operations, the SQLite adapter, and `store.MemoryStore`. Keep storage mechanisms behind this interface; the durability contract is in `docs/design/durability.md`.
- `internal/content` loads and validates the read-only JSON packs under `content/`. One file represents one entity, and its filename equals its slug.
- `internal/balance`, `internal/loadtest`, and `internal/sshload` back reproducible operator harnesses. Run the Balance Run with `go run ./cmd/balancerun -content ./content`; CI adds `-fail-gates -capture`.

## Context pointers

- Before introducing or renaming domain terms, read `CONTEXT.md`; its Trainer, Battle, Queue, Dojo, and related vocabulary is canonical in code and docs.
- For gameplay behavior or persistence changes, start at `docs/README.md` and read the linked contract. Update the contract and its TERM ticket when a decision changes.
- For content or balance changes, read `docs/design/balance-methodology.md`; malformed content must fail server startup, and combat-affecting edits require a passing Balance Run.
- For sprite work, follow `docs/design/sprite-pipeline.md` and use `cmd/artimport`, `cmd/artsheet`, and `cmd/sheetextract` rather than editing generated art grids by hand.
- For SSH listeners, deployment, host keys, or dependency security, read `docs/operations.md`. When changing Wish or `golang.org/x/crypto`, compare Wish's x/crypto floor and the selected version against current Go SSH advisories because minimum version selection may retain a vulnerable direct dependency.
- For capacity changes, reproduce the relevant procedure in `docs/load-baseline.md` with `cmd/termon-load` or `cmd/termon-ssh-load`.

All monsters, Moves, art, and text must be original. Do not add copyrighted Pokémon names, art, or prose.
