# Termon documentation

Use this index to find the current gameplay contracts, operating procedures, and implementation records. Player-facing behavior uses the canonical terms defined in [`CONTEXT.md`](../CONTEXT.md).

## Run and operate Termon

- [Local multiplayer walkthrough](local-mvp.md) — start an isolated server and test onboarding, matchmaking, battle, persistence, and reconnects.
- [Deployment](deployment.md) — provision Dokploy, deploy the production Compose service, and verify backup, restore, upgrade, and rollback.
- [Operations](operations.md) — configure runtime topology, rate limits, metrics, SQLite durability, and recovery behavior.
- [Load baseline](load-baseline.md) — reproduce the measured 4 CPU / 4 GiB capacity tests.

## Gameplay design

- [Lobby](design/lobby.md) and [matchmaking](design/matchmaking.md) define movement, challenges, the Queue, and session lifecycle.
- [Onboarding storyboard](design/onboarding-storyboard.md) defines starter selection and the required Capture Lessons.
- [Collection and Party](design/collection-party.md), [progression](design/progression.md), [persistence](design/progression-persistence.md), [Evolution](design/evolution.md), and [XP](design/xp-progression.md) define Trainer-owned state.
- [Combat](design/combat.md), [three-Monster Battles](design/party-battles.md), and the [Battle view](design/party-battle-view.md) define battle rules and presentation.
- [Capture Gauge](design/capture-gauge.md), [capture catalog](design/capture-catalog.md), [Expeditions](design/expeditions.md), and the [Expedition view](design/expedition-view.md) define capture activities.
- [Dojo Master](design/dojo-master.md), [Dojo policy and teams](design/dojo-policy.md), and [balance methodology](design/balance-methodology.md) define solo battles and balance gates.
- [Roster](design/roster.md) and [data model](design/data-model.md) define shipped content.

## Engineering design

- [Durability](design/durability.md) defines persistence boundaries and migration rules.
- [Telemetry and player statistics](design/telemetry.md) defines correlation identifiers, PostHog delivery, support references, and authoritative Stats queries.
- [Implementation rollout](design/implementation-rollout.md) records the completed migration from the original 1v1 prototype.
- [Render caching](design/render-caching.md) records the TUI performance design.
- [Sprite pipeline](design/sprite-pipeline.md) explains how source art becomes terminal sprites.
