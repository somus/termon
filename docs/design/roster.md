# Roster & Type Chart v1 - decided (TERM-12), creature-first naming pass applied

Species and Move names put the creature first, retaining computing wordplay when it naturally fits the body, element, or action. Every Species' Movepool draws from its own Type and thematically matches its concept. This document defines the 24 base Species; their 48 evolutions are canonical in [evolution.md](evolution.md).

The naming pass changes names and identifiers together without changing artwork, stats, Move categories, power, accuracy, or unlock levels. A stable Move ordering value preserves the original generated loadouts and seeded choices across these identifier changes. Existing saves containing renamed identifiers are unsupported; this change includes no save migration, compatibility aliases, or automatic resets.

## Types

Six types, two interlocked triangles, every type exactly 2 strengths and 2 weaknesses.

**Starter triangle**: Thermal > Organic > Coolant > Thermal
**Tech triangle**: Current > Silicon > Virus > Current
**Cross edges**: Thermal > Virus · Coolant > Silicon · Organic > Current · Current > Coolant · Virus > Organic · Silicon > Thermal

| Attacker ↓ | Thermal | Coolant | Organic | Current | Virus | Silicon |
|-----------|---------|---------|---------|---------|-------|---------|
| **Thermal** | – | | 1.5× | | 1.5× | |
| **Coolant** | 1.5× | – | | | | 1.5× |
| **Organic** | | 1.5× | – | 1.5× | | |
| **Current** | | 1.5× | | – | | 1.5× |
| **Virus**   | | | 1.5× | 1.5× | – | |
| **Silicon** | 1.5× | | | | 1.5× | – |

(Read row = attacker, column = defender. Blank = 1×.)

## Moves — 144, six unique STAB per Evolution Family

Each Family owns six same-Type Moves. All three stages list the same slugs. Unlock levels are 1, 1, 1, 1, first Evolution, final Evolution. Power/accuracy rungs are 40/100, 55/100, 65/95, 75/90, 90/85, 100/80. Wilds and starters use the first four as the default Battle Loadout.

Move slugs are lowercase snake_case of the display name (`Torque Blast` → `torque_blast`, `Signal Bark` → `signal_bark`, `Ring Broadcast` → `ring_broadcast`).

Each Move has a positive, globally unique `order` value for deterministic tie-breaking. Lower values come first when gameplay selection criteria tie. Preserve this value when renaming a Move; new Moves receive unused values. The naming pass freezes the previous ordering without retaining old names or aliases. This ordering does not change the authored Movepool or player-selected Battle Loadout order.

A Move name must express its Family's body or role rather than serve as a Type-wide synonym. Evolution unlocks a new slug instead of silently increasing an existing Move's power: the first four Moves remain available while the 90-power and 100-power Moves arrive at the Family's Evolution levels. Keep one Type per Move and no off-Type coverage. Physical and special categories should follow the Family's attacking-stat profile without reducing its useful Loadout choices to one dominant Move.

Moves currently deal damage only. Their names should describe an attack, match its physical or special category, and make later unlocks sound larger without promising healing, shielding, or status effects.

The 100-power finisher stays below the 120-power benchmark until the Balance Run proves it preserves the 3–5-hit KO gate and avoids non-critical one-hit KOs. Content validation also requires four baseline Moves, three distinct default slugs for Capture objectives, and the same six slugs across all three Family stages. PvP uses equipped Moves unlocked through earned progression; owning a Family does not grant its later Moves or stages.

| Family | 40/100 | 55/100 | 65/95 | 75/90 | 90/85 (evo 1) | 100/80 (final) |
| --- | --- | --- | --- | --- | --- | --- |
| **Aquabit** | Packet Bump (phys) | Ripple Ping (spec) | Stream Pulse (spec) | Jumbo Wave (spec) | Flood Fill (spec) | Packet Storm (spec) |
| **Flowcell** | Queue Jet (spec) | Flush (spec) | Drain Jet (spec) | Spillway Burst (spec) | Backpressure (spec) | Reservoir Dump (spec) |
| **Gushkit** | Pipe Swipe (phys) | Pressure Pounce (phys) | Pipe Burst (spec) | Hose Lash (phys) | Split Jet (spec) | Torrent Tackle (phys) |
| **Mistcache** | Mist Ping (spec) | Vapor Echo (spec) | Cache Cloudburst (spec) | Vapor Stream (spec) | Pressure Evict (spec) | Vault Deluge (spec) |
| **Splashlotl** | Boot Splash (spec) | Cold Boot (spec) | Gill Pulse (spec) | Reboot Rush (spec) | Reset Slap (phys) | Restart Crash (phys) |
| **Ampcoil** | Short Circuit (phys) | Coil Smash (phys) | Induction Wave (spec) | Ring Discharge (spec) | Livewire Pulse (spec) | Grid Constrict (phys) |
| **Joulepup** | Hotkey (phys) | Keymash (phys) | Spark Bark (spec) | Livewire Bite (phys) | Capacitor Howl (spec) | Grid Maul (phys) |
| **Surgetail** | Surge Spray (spec) | Static Slap (phys) | Thunderwake (spec) | Surge Slam (phys) | Tempest Discharge (spec) | Storm Crash (phys) |
| **Zaplet** | Static Pin (spec) | Pulse Chirp (spec) | Arc Interrupt (spec) | Feather Relay (spec) | Thunder Broadcast (spec) | Kernel Storm (spec) |
| **Mossmuff** | Spore Cache (spec) | Mold Leak (spec) | Moss Bump (phys) | Chassis Crunch (phys) | Spore Dump (spec) | Bog Overflow (spec) |
| **Taproot** | Root Trace (phys) | Branch Clasp (phys) | Root Signal (spec) | Taproot Hammer (phys) | Ring Broadcast (spec) | Rootquake (phys) |
| **Rootkit** | Root Pulse (spec) | Bark Bash (phys) | Sudo Surge (spec) | Branch Breach (phys) | Root Override (spec) | Kernel Bloom (spec) |
| **Sproutware** | Fork Bomb (spec) | Vine Mount (phys) | Cable Lash (phys) | Sprout Burst (spec) | Leaf Overflow (spec) | Canopy Broadcast (spec) |
| **Thornpatch** | Thorn Patch (phys) | Briar Jab (phys) | Splinter Fault (spec) | Bramble Shove (phys) | Splinter Merge (spec) | Deadbolt Thorns (phys) |
| **Chippunk** | Punch Card (phys) | Chip Volley (spec) | Solder Spatter (spec) | Scrap Scatter (spec) | Component Burst (spec) | Scrap Cannon (spec) |
| **Coghound** | Trace Bite (phys) | Sensor Pulse (spec) | Signal Bark (spec) | Alarm Blast (spec) | Clockwork Crunch (phys) | Watchdog Rush (phys) |
| **Servoboar** | Hard Reset (phys) | Torque Blast (spec) | Exhaust Burst (spec) | Cache Eject (spec) | Ram Commit (phys) | Rackbreaker (phys) |
| **Cindernode** | Overclock (phys) | Furnace Bash (phys) | Vent Burst (spec) | Reactor Pulse (spec) | Thermal Runaway (phys) | Core Eruption (spec) |
| **Emberbyte** | Burn-in (spec) | Ember Fold (spec) | Cinder Pulse (spec) | Hash Flare (spec) | Cinder Overflow (spec) | Flare Crash (phys) |
| **Scorchip** | Bit Flare (spec) | Reflow (spec) | Trace Burn (spec) | Bus Flare (spec) | Circuit Melt (spec) | Cascade Burn (spec) |
| **Wickware** | Boot Flame (spec) | Daemon Spark (spec) | Wick Jet (spec) | Forked Flame (spec) | Threadfire (spec) | Daemon Inferno (spec) |
| **Bloatware** | Bloat Bump (phys) | Module Mash (phys) | Heap Spray (spec) | Garbage Burst (spec) | Heap Overflow (phys) | Garbage Storm (spec) |
| **Spamlet** | Junkmail Shot (spec) | CC Bomb (spec) | Spoof Pulse (spec) | Pixel Barrage (spec) | Clickburst (spec) | Spearphish (spec) |
| **Wormate** | Bit Gnaw (phys) | Replica Burst (spec) | Hex Spit (spec) | Payload Bite (phys) | Fault Pulse (spec) | Segment Crush (phys) |

## Base roster - 24 species, names/stats/pools frozen

Distribution: Organic 5 · Thermal 4 · Coolant 5 · Current 4 · Virus 3 · Silicon 3.

Progression: every base Species starts its own three-stage family. [Evolution families, thresholds, complete stats, and stories](evolution.md) are canonical.

### Organic (5)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 001 | **Rootkit** | STARTER sturdy — superuser sapling; root = plant root AND root access | 55/45/60/48/42 | Root Pulse, Bark Bash, Sudo Surge, Branch Breach, Root Override, Kernel Bloom |
| 002 | Sproutware | creeping vine that installs itself anywhere | 46/52/44/54/62 | Fork Bomb, Vine Mount, Cable Lash, Sprout Burst, Leaf Overflow, Canopy Broadcast |
| 003 | Thornpatch | hostile hedge; a patch you do NOT want applied | 58/48/66/40/36 | Thorn Patch, Briar Jab, Splinter Fault, Bramble Shove, Splinter Merge, Deadbolt Thorns |
| 004 | Mossmuff | damp legacy-system puffball; slow but never crashes | 60/42/56/50/30 | Spore Cache, Mold Leak, Moss Bump, Chassis Crunch, Spore Dump, Bog Overflow |
| 005 | Taproot | ancient taproot, the Dojo guardian | 68/58/62/44/28 | Root Trace, Branch Clasp, Root Signal, Taproot Hammer, Ring Broadcast, Rootquake |

### Thermal (4)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 006 | **Emberbyte** | STARTER spicy — a coal that corrupted its own shell | 44/50/38/66/52 | Burn-in, Ember Fold, Cinder Pulse, Hash Flare, Cinder Overflow, Flare Crash |
| 007 | Cindernode | smoldering reactor node | 54/64/48/62/34 | Overclock, Furnace Bash, Vent Burst, Reactor Pulse, Thermal Runaway, Core Eruption |
| 008 | Scorchip | swarm of burnt microchips in a husk | 40/44/34/70/64 | Bit Flare, Reflow, Trace Burn, Bus Flare, Circuit Melt, Cascade Burn |
| 009 | Wickware | candle-flame daemon; lights itself on boot | 44/40/40/62/66 | Boot Flame, Daemon Spark, Wick Jet, Forked Flame, Threadfire, Daemon Inferno |

### Coolant (5)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 010 | **Aquabit** | STARTER speedy — quicksilver packet-hopper of the shallows | 42/46/40/50/68 | Packet Bump, Ripple Ping, Stream Pulse, Jumbo Wave, Flood Fill, Packet Storm |
| 011 | Flowcell | tidal battery storing wave power | 58/46/54/52/44 | Queue Jet, Flush, Drain Jet, Spillway Burst, Backpressure, Reservoir Dump |
| 012 | Gushkit | hose-tailed kitten, chaotic throughput | 44/56/38/50/64 | Pipe Swipe, Pressure Pounce, Pipe Burst, Hose Lash, Split Jet, Torrent Tackle |
| 013 | Mistcache | fog that caches secrets nobody asked it to keep | 48/42/46/58/56 | Mist Ping, Vapor Echo, Cache Cloudburst, Vapor Stream, Pressure Evict, Vault Deluge |
| 014 | Splashlotl | axolotl stuck on its own boot splash | 50/48/46/50/54 | Boot Splash, Cold Boot, Gill Pulse, Reboot Rush, Reset Slap, Restart Crash |

### Current (4)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 015 | Zaplet | static-charged hatchling | 44/50/40/52/62 | Static Pin, Pulse Chirp, Arc Interrupt, Feather Relay, Thunder Broadcast, Kernel Storm |
| 016 | Joulepup | puppy that sheds sparks and presses hotkeys by accident | 46/58/40/40/66 | Hotkey, Keymash, Spark Bark, Livewire Bite, Capacitor Howl, Grid Maul |
| 017 | Ampcoil | constrictor of live wire | 52/60/50/42/48 | Short Circuit, Coil Smash, Induction Wave, Ring Discharge, Livewire Pulse, Grid Constrict |
| 018 | Surgetail | carp that rides thunderheads and surges | 60/54/50/54/40 | Surge Spray, Static Slap, Thunderwake, Surge Slam, Tempest Discharge, Storm Crash |

### Virus (3)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 019 | Spamlet | hook-tailed scavenger hiding bait in bright scraps of mail | 46/48/42/50/60 | Junkmail Shot, CC Bomb, Spoof Pulse, Pixel Barrage, Clickburst, Spearphish |
| 020 | Bloatware | bubbling vat of unused features; never garbage-collects | 66/50/58/46/26 | Bloat Bump, Module Mash, Heap Spray, Garbage Burst, Heap Overflow, Garbage Storm |
| 021 | Wormate | a computer worm that is, literally, a worm | 48/56/52/38/54 | Bit Gnaw, Replica Burst, Hex Spit, Payload Bite, Fault Pulse, Segment Crush |

### Silicon (3)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 022 | Chippunk | rodent assembled from loose components | 42/50/40/52/68 | Punch Card, Chip Volley, Solder Spatter, Scrap Scatter, Component Burst, Scrap Cannon |
| 023 | Coghound | loyal clockwork tracker; always closes its tickets | 50/60/48/46/50 | Trace Bite, Sensor Pulse, Signal Bark, Alarm Blast, Clockwork Crunch, Watchdog Rush |
| 024 | Servoboar | freight-hauling machine boar; hard resets everything in its path | 70/62/58/36/24 | Hard Reset, Torque Blast, Exhaust Burst, Cache Eject, Ram Commit, Rackbreaker |

## Starter default loadouts (first 4 by level)

- **Rootkit**: Root Pulse, Bark Bash, Sudo Surge, Branch Breach
- **Emberbyte**: Burn-in, Ember Fold, Cinder Pulse, Hash Flare
- **Aquabit**: Packet Bump, Ripple Ping, Stream Pulse, Jumbo Wave

## Bench (cut for count parity; revive when roster grows)

Heatmap (Thermal lizard with glowing skin), Streamlet (live-streaming minnow), Ohmlet (ohm + omelet egg), Trojanfin (gift-wrapped predator fish), Daemoseed (daemon + seed).

## Authoring notes (execution, post-bootstrap)

- Keep the 24 base designs and source-sheet order aligned with this roster; keep the 48 evolution designs aligned with [evolution.md](evolution.md).
- Follow [sprite-pipeline.md](./sprite-pipeline.md) to generate, review, and import monster art.
- Store the imported runtime grid at `content/art/<slug>.json`.
