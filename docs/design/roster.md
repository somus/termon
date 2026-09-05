# Roster & Type Chart v1 — decided (TERM-12), terminal-naming pass applied

Full terminal/computing naming is adopted across types, species, and moves. Every Species' Movepool draws from its own Type and thematically matches its concept. This document freezes the original 24 base Species; their 48 new evolutions are canonical in [evolution.md](evolution.md).

## Types

Six types, two interlocked triangles, every type exactly 2 strengths and 2 weaknesses.

**Starter triangle**: Thermal > Organic > Coolant > Thermal
**Tech triangle**: Current > Silicon > Virus > Current
**Cross edges**: Thermal > Virus · Coolant > Silicon · Organic > Current · Current > Coolant · Virus > Organic · Silicon > Thermal

| Attacker ↓ | Thermal | Coolant | Organic | Current | Virus | Silicon |
|-----------|---------|---------|---------|---------|-------|---------|
| **Thermal** | – | | 2× | | 2× | |
| **Coolant** | 2× | – | | | | 2× |
| **Organic** | | 2× | – | 2× | | |
| **Current** | | 2× | | – | | 2× |
| **Virus**   | | | 2× | 2× | – | |
| **Silicon** | 2× | | | | 2× | – |

(Read row = attacker, column = defender. Blank = 1×.)

## Moves — 144, six unique STAB per Evolution Family

Each Family owns six same-Type Moves. All three stages list the same slugs. Unlock levels are 1, 1, 1, 1, first Evolution, final Evolution. Power/accuracy rungs are 40/100, 55/100, 65/95, 75/90, 90/85, 100/80. Wilds and starters use the first four as the default Battle Loadout.

Move slugs are lowercase snake_case of the display name (`kill -9` → `kill_9`, `tail -f` → `tail_f`, `git fsck` → `git_fsck`).

A Move name must express its Family's body or role rather than serve as a Type-wide synonym. Evolution unlocks a new slug instead of silently increasing an existing Move's power: the first four Moves remain available while the 90-power and 100-power Moves arrive at the Family's Evolution levels. Keep one Type per Move and no off-Type coverage. Physical and special categories should follow the Family's attacking-stat profile without reducing its useful Loadout choices to one dominant Move.

The 100-power finisher stays below the 120-power benchmark until the Balance Run proves it preserves the 3–5-hit KO gate and avoids non-critical one-hit KOs. Content validation also requires four Level-1 Moves, four Queue-eligible Moves by Level 30, three distinct default slugs for Capture objectives, and the same six slugs across all three Family stages.

| Family | 40/100 | 55/100 | 65/95 | 75/90 | 90/85 (evo 1) | 100/80 (final) |
| --- | --- | --- | --- | --- | --- | --- |
| **Aquabit** | Ping Flood (phys) | Hop Count (spec) | Checksum (spec) | Jumbo Frame (spec) | Flood Fill (spec) | Packet Storm (spec) |
| **Flowcell** | Enqueue (spec) | Flush (spec) | Drain (spec) | Watermark (spec) | Backpressure (spec) | Buffer Bloat (spec) |
| **Gushkit** | Pipe (phys) | Redirect (phys) | pipefail (spec) | FIFO (phys) | splice (spec) | Named Pipe (phys) |
| **Mistcache** | Incognito (spec) | Memoize (spec) | Cache Stampede (spec) | Write-Through (spec) | LRU Evict (spec) | Sealed Secret (spec) |
| **Splashscreen** | Boot Splash (spec) | Cold Boot (spec) | PXE Boot (spec) | kexec (spec) | Soft Reset (phys) | reboot (phys) |
| **Amperent** | Short Circuit (phys) | Stack Smash (phys) | Buffer Wrap (spec) | Ring Buffer (spec) | Livelock (spec) | Circular Wait (phys) |
| **Joulpup** | Hotkey (phys) | Keymash (phys) | Macro (spec) | Keybind (phys) | Input Buffer (spec) | Sticky Keys (phys) |
| **Surgetail** | SIGFPE (spec) | SIGTERM (phys) | SIGHUP (spec) | SIGKILL (phys) | SIGBUS (spec) | Abort (phys) |
| **Zaplet** | Floating Pin (spec) | Debounce (spec) | Interrupt (spec) | IRQ Handler (spec) | NMI (spec) | Kernel Trap (spec) |
| **Mossmuff** | Compile Time (spec) | Memory Leak (spec) | Busy-Wait (phys) | Defrag (phys) | Checkpoint (spec) | Uptime (spec) |
| **Rootanami** | Traceroute (phys) | Handshake (phys) | BGP Peer (spec) | Merkle Root (phys) | git fsck (spec) | Root Zone (phys) |
| **Rootkit** | Root Access (spec) | chmod (phys) | sudo (spec) | setuid (phys) | chroot (spec) | Kernel Mode (spec) |
| **Sproutware** | Fork Bomb (spec) | Bind Mount (phys) | Symlink (phys) | Autoload (spec) | Overlay FS (spec) | Spanning Tree (spec) |
| **Thornpatch** | Hot Patch (phys) | Hotfix (phys) | Breaking Change (spec) | Force Push (phys) | Merge Conflict (spec) | Lockfile (phys) |
| **Chippunk** | Punch Card (phys) | malloc (spec) | memcpy (spec) | realloc (spec) | Bin Pack (spec) | Slab Alloc (spec) |
| **Coghound** | grep (phys) | strace (spec) | tail -f (spec) | inotify (spec) | crontab (phys) | watchdogd (phys) |
| **Servoboar** | Hard Reset (phys) | kill -9 (spec) | OOM Kill (spec) | drop_caches (spec) | fsync (phys) | mkfs (phys) |
| **Cindernode** | Overclock (phys) | Busy Loop (phys) | Cron Job (spec) | Watchdog Timer (spec) | Thermal Runaway (phys) | Core Dump (spec) |
| **Emberbyte** | Burn-in (spec) | XOR Fold (spec) | CRC32 (spec) | Salted Hash (spec) | Avalanche (spec) | Rainbow Table (phys) |
| **Scorchip** | Bit Flip (spec) | Reflow (spec) | Latch-Up (spec) | Bus Error (spec) | Brownout (spec) | Cascade Fail (spec) |
| **Wickware** | Boot Up (spec) | Daemonize (spec) | nohup (spec) | Double Fork (spec) | PID File (spec) | execve (spec) |
| **Bloatware** | Feature Creep (phys) | Scope Creep (phys) | Heap Spray (spec) | Use-After-Free (spec) | Heap Overflow (phys) | GC Thrash (spec) |
| **Spamlet** | Flame Mail (spec) | CC Bomb (spec) | Spoofed From (spec) | Tracking Pixel (spec) | Clickjack (spec) | Spearphish (spec) |
| **Wormate** | Bit Rot (phys) | Self-Replicate (spec) | Polymorphic (spec) | Dropper (phys) | Persist (spec) | Morph (phys) |

## Base roster - 24 species, names/stats/pools frozen

Distribution: Organic 5 · Thermal 4 · Coolant 5 · Current 4 · Virus 3 · Silicon 3.

Progression: every base Species starts its own three-stage family. [Evolution families, thresholds, complete stats, and stories](evolution.md) are canonical.

### Organic (5)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 001 | **Rootkit** | STARTER sturdy — superuser sapling; root = plant root AND root access | 55/45/60/48/42 | Root Access, chmod, sudo, setuid, chroot, Kernel Mode |
| 002 | Sproutware | creeping vine that installs itself anywhere | 46/52/44/54/62 | Fork Bomb, Bind Mount, Symlink, Autoload, Overlay FS, Spanning Tree |
| 003 | Thornpatch | hostile hedge; a patch you do NOT want applied | 58/48/66/40/36 | Hot Patch, Hotfix, Breaking Change, Force Push, Merge Conflict, Lockfile |
| 004 | Mossmuff | damp legacy-system puffball; slow but never crashes | 60/42/56/50/30 | Compile Time, Memory Leak, Busy-Wait, Defrag, Checkpoint, Uptime |
| 005 | Rootanami | ancient taproot, the Dojo guardian | 68/58/62/44/28 | Traceroute, Handshake, BGP Peer, Merkle Root, git fsck, Root Zone |

### Thermal (4)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 006 | **Emberbyte** | STARTER spicy — a coal that corrupted its own shell | 44/50/38/66/52 | Burn-in, XOR Fold, CRC32, Salted Hash, Avalanche, Rainbow Table |
| 007 | Cindernode | smoldering reactor node | 54/64/48/62/34 | Overclock, Busy Loop, Cron Job, Watchdog Timer, Thermal Runaway, Core Dump |
| 008 | Scorchip | swarm of burnt microchips in a husk | 40/44/34/70/64 | Bit Flip, Reflow, Latch-Up, Bus Error, Brownout, Cascade Fail |
| 009 | Wickware | candle-flame daemon; lights itself on boot | 44/40/40/62/66 | Boot Up, Daemonize, nohup, Double Fork, PID File, execve |

### Coolant (5)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 010 | **Aquabit** | STARTER speedy — quicksilver packet-hopper of the shallows | 42/46/40/50/68 | Ping Flood, Hop Count, Checksum, Jumbo Frame, Flood Fill, Packet Storm |
| 011 | Flowcell | tidal battery storing wave power | 58/46/54/52/44 | Enqueue, Flush, Drain, Watermark, Backpressure, Buffer Bloat |
| 012 | Gushkit | hose-tailed kitten, chaotic throughput | 44/56/38/50/64 | Pipe, Redirect, pipefail, FIFO, splice, Named Pipe |
| 013 | Mistcache | fog that caches secrets nobody asked it to keep | 48/42/46/58/56 | Incognito, Memoize, Cache Stampede, Write-Through, LRU Evict, Sealed Secret |
| 014 | Splashscreen | axolotl stuck on its own boot splash | 50/48/46/50/54 | Boot Splash, Cold Boot, PXE Boot, kexec, Soft Reset, reboot |

### Current (4)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 015 | Zaplet | static-charged hatchling | 44/50/40/52/62 | Floating Pin, Debounce, Interrupt, IRQ Handler, NMI, Kernel Trap |
| 016 | Joulpup | puppy that sheds sparks and presses hotkeys by accident | 46/58/40/40/66 | Hotkey, Keymash, Macro, Keybind, Input Buffer, Sticky Keys |
| 017 | Amperent | constrictor of live wire | 52/60/50/42/48 | Short Circuit, Stack Smash, Buffer Wrap, Ring Buffer, Livelock, Circular Wait |
| 018 | Surgetail | carp that rides thunderheads and surges | 60/54/50/54/40 | SIGFPE, SIGTERM, SIGHUP, SIGKILL, SIGBUS, Abort |

### Virus (3)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 019 | Spamlet | hamlet of hoarded garbage mail; "to GC or not to GC" | 46/48/42/50/60 | Flame Mail, CC Bomb, Spoofed From, Tracking Pixel, Clickjack, Spearphish |
| 020 | Bloatware | bubbling vat of unused features; never garbage-collects | 66/50/58/46/26 | Feature Creep, Scope Creep, Heap Spray, Use-After-Free, Heap Overflow, GC Thrash |
| 021 | Wormate | a computer worm that is, literally, a worm | 48/56/52/38/54 | Bit Rot, Self-Replicate, Polymorphic, Dropper, Persist, Morph |

### Silicon (3)

| # | Name | Concept | hp/atk/def/spa/spe | Movepool |
|---|------|---------|--------------------|----------|
| 022 | Chippunk | rodent assembled from loose components | 42/50/40/52/68 | Punch Card, malloc, memcpy, realloc, Bin Pack, Slab Alloc |
| 023 | Coghound | loyal clockwork tracker; always closes its tickets | 50/60/48/46/50 | grep, strace, tail -f, inotify, crontab, watchdogd |
| 024 | Servoboar | freight-hauling machine boar; hard resets everything in its path | 70/62/58/36/24 | Hard Reset, kill -9, OOM Kill, drop_caches, fsync, mkfs |

## Starter default loadouts (first 4 by level)

- **Rootkit**: Root Access, chmod, sudo, setuid
- **Emberbyte**: Burn-in, XOR Fold, CRC32, Salted Hash
- **Aquabit**: Ping Flood, Hop Count, Checksum, Jumbo Frame

## Bench (cut for count parity; revive when roster grows)

Heatmap (Thermal lizard with glowing skin), Streamlet (live-streaming minnow), Ohmlet (ohm + omelet egg), Trojanfin (gift-wrapped predator fish), Daemoseed (daemon + seed).

## Authoring notes (execution, post-bootstrap)

- Keep the 24 base designs and source-sheet order aligned with this roster; keep the 48 evolution designs aligned with [evolution.md](evolution.md).
- Follow [sprite-pipeline.md](./sprite-pipeline.md) to generate, review, and import monster art.
- Store the imported runtime grid at `content/art/<slug>.json`.
