# Lobby World v1 - decided (TERM-15, TERM-28)

The shared spatial hub between Battles. Theme: **the Dojo**, with an original retro training-hall aesthetic using a tatami-style floor grid, pillars, and wall banners as ASCII decor. All assets and terminology are original.

## Dojos

Each logical Dojo is a 48×14 tile grid capped at 32 reserved Trainers. The terminal shows a camera-following window over that world so each visible tile can use a 9×4 cell render block. Walls, pillars, the Dojo Master, and explicitly solid landmarks are collision tiles. Floor decor is passable and remains on the scenery layer beneath Trainers.

The Hub fills an existing Dojo before creating another. A reconnecting Trainer returns to the previous Dojo while that Dojo still exists. The Hub removes an empty secondary Dojo; Dojo 1 remains available for new arrivals.

Trainers spawn across 32 entrance tiles. A Trainer in a Battle keeps a Dojo reservation so the return path cannot fail after the Battle.

## Movement

- Grid-step movement: arrow keys, WASD, and hjkl all bound.
- Collision: other Trainers plus landmarks explicitly marked impassable.
- Layering: passable floor objects render first; a Trainer standing on that tile replaces the object's visible cells.

The TUI submits movement to the Hub in input order and paints the returned
snapshot in the same update. `Hub.MoveAndSnapshot` performs only in-memory
validation, mutation, and capture under one lock; it must not add persistence,
broadcast delivery, or callbacks to that synchronous path. Running movement as
independent Bubble Tea commands can reorder intents at collisions, even when
snapshot delivery itself is sequenced. Other asynchronous gameplay operations
keep their existing paths.

## Presence

Each visible Trainer renders as a compact multi-line model with an attached, width-bounded Handle card. The local Trainer has a distinct marker. Trainers mid-Battle and in the Queue keep visible state markers; Trainers mid-Battle cannot be challenged.

The camera follows the local Trainer and clamps at the world boundary. It renders only Trainers and landmarks inside the visible tile window, while the Dojo population remains in the page header.

## Dojo Master and landmarks

Master Sable occupies a fixed, impassable tile near the entrance. First-run dialogue is spoken by Master Sable; the two required Capture Lessons launch from that briefing before free Lobby play. Later, stand orthogonally adjacent and press Enter to open the Dojo Master menu for Capture Lessons, Sparring tiers, and the Daily Challenge. See [Dojo Master modes and bot behavior](dojo-master.md). Menu wiring follows [Implementation rollout](implementation-rollout.md).

The notice board is the Signal Board landmark. Opening it uses the [Expedition terminal view](expedition-view.md): Family cards in the arena, and every prompt in the same four-row Battle chrome.

The Dojo contains both solid landmarks and passable details. Selected passable details reveal short contextual lines when a Trainer steps onto them. These discoveries are cosmetic and do not change persistent Trainer state.

The formal hall centers on a bordered sparring court with opposing start marks and a floor crest. Its red perimeter remains visible but is fully passable, so Trainers can enter from any side. Benches, plants, pillars, training equipment, water, recovery supplies, storage, records, and discoveries fill the outer zones without entering the court or blocking the 32-tile south entrance.

The room has four rendered boundaries, but only the north wall has architectural depth. It uses an additional blocked row for banners, teaching scrolls, lanterns, a central crest, and wood paneling; the side and south walls stay visually thin. Alternating warm square tiles form the outer floor, while the fighting mat remains a uniform lighter surface inside its red border.

## Challenges

- Stand orthogonally adjacent to another Trainer and press `C` → Challenge modal opens on their screen: accept / decline.
- Auto-decline after 30s of silence. Decline dismisses quietly, no penalty, no cooldown in v1.
- Challenges stay inside one Dojo.

## Matchmaking

The Queue is global across every Dojo in the process. Trainers from different Dojos can enter a Battle together, while their Lobby presence remains in the assigned Dojo.

## Emotes

Preset quick phrases only (fixed list: "gl hf", "gg", "well fought", "rematch?", "hello!"), bound to `E` → picker → an anchored speech bubble above the Trainer model for a few seconds. No free-text chat in v1; full chat is fog pending moderation design.

## Sync

The server owns all positions. It validates movement intents and broadcasts dirty updates only to viewers in the same Dojo at a modest tick rate. This bounds presence fan-out at 32 Trainers per room while keeping Battle matchmaking global.

Snapshots carry a process-local sequence allocated under the Hub lock. Delivery
happens after unlocking, so broadcasts and immediate movement replies can arrive
out of capture order. The TUI rejects older or duplicate sequenced snapshots before
updating positions, camera state, presence, or offers. The sequence spans Dojos and
recipients in the same Hub; it is not a persistence revision and resets with the
server process, whose SSH sessions also end. See TERM-71 and the
[movement investigation](../dojo-movement-latency.md).
