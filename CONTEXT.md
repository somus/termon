# Termon — Domain Glossary

Canonical vocabulary for termon.sh. Implementation-free by design.

## Terms

**Trainer** — A player with a stable identity independent of any connection credential. Owns a Collection, an ordered Party, and a win/loss record.

**SSH Credential** — A hashed SSH public key fingerprint that authenticates a Trainer. A Trainer initially has one SSH Credential; the identity model permits key rotation or multiple credentials later.

**Species** — The template for a kind of monster: base stats, Type(s), art, and Movepool. One original species per concept; there are no copyrighted species.

**Monster** — An individual creature owned by a Trainer, instantiated from a Species. Carries its own identity, progression, nickname, Move Library, and Battle Loadout. Two Monsters of the same Species are distinct individuals.

**Move** — A single combat action a Monster can take: has a Type, a damage category, power, and accuracy. (Not called "attack" or "technique".)

**Type** — An element category (e.g. thermal, coolant). Effectiveness between Types is defined by a chart consulted attacker-side.

**Movepool** — The set of Moves available to a Species, with the level or condition at which each Move becomes eligible to unlock. Launch Families share six unique same-Type Moves across all three stages.

**Move Library** — The Moves an individual Monster has permanently unlocked from its current or predecessor Species' Movepools through leveling. Unlocking a Move is never reversed by changing the Battle Loadout or accepting an Evolution.

**Battle Loadout** — Up to four Moves selected from an individual Monster's Move Library for use in Battle. A Trainer may change a Battle Loadout outside a Battle.

**Level** — An individual Monster's persistent progression rank from 1 through 50. Levels govern natural Battle strength, Move unlocks, and Evolution eligibility; global Queue Battles normalize Levels.

**Evolution** — A permanent change from one Species into the next Species in the same Evolution Family. Reaching the required Level makes a Monster eligible; the Trainer may accept or defer the change.

**Evolution Family** — A linear sequence of one to three Species connected by Evolution rules. A one-stage family is a stable standalone Species.

**Collection** — The uncapped set of individual Monsters a Trainer owns. A Trainer manages the Collection outside Battles and selects Monsters from it for the Party.

**Party** — An ordered three-slot selection of Monsters from a Trainer's Collection. A slot may be empty. One occupied Monster is active at a time during a Battle.

**Full Party** — A Party containing three battle-ready Monsters. A Full Party is required to enter the global Queue.

**Battle** — A turn-based duel between two Trainers, each fielding an ordered Party of up to three Monsters with one active Monster at a time. Trainers simultaneously select an action, including a Move or a switch, before the turn resolves.

**Battle Action** — A hidden, immutable selection for the current turn: either use one Move from the active Monster's Battle Loadout or voluntarily switch to a healthy reserve Monster.

**Switch** — A Battle Action that replaces the active Monster with a healthy reserve Monster before any Moves resolve. Damage remains with a Monster when it leaves the field.

**Replacement** — The mandatory selection of a healthy reserve Monster after the active Monster faints. Only the affected Trainer selects; the next turn begins after the replacement enters.

**Decision Clock** — A Trainer-specific time bank that runs while the Battle is waiting for that Trainer's Battle Action or Replacement. Exhausting the bank forfeits the Battle.

**Normalized Battle** — A Trainer-versus-Trainer Battle in which each Monster uses a standard level and stat budget plus a temporary Queue Move Set, while preserving its Species, Evolution stage, and role identity. Queue Battles and direct Challenge Battles are Normalized Battles; persistent XP and natural Levels govern progression in solo modes.

**Queue Move Set** — The temporary four-Move selection used by a Monster in a Normalized Battle. It is chosen from the current Species' legal level-bounded Movepool and inherited Moves without changing the persistent Move Library or Battle Loadout.

**XP Threshold** — The cumulative XP value at which an individual Monster reaches a Level. Thresholds are monotonic from Level 1 through Level 50; reaching one can unlock Moves and make an Evolution eligible.

**Reward Packet** — The idempotent XP outcome attached to one completed activity or Battle Result. It contains a base reward and any active-participant, winner, or completion bonuses before each Party Monster receives its share.

**Reserve Share** — The smaller Reward Packet share paid to a selected Party Monster that never becomes active during an activity. A Monster that enters and resolves a turn receives the active share instead.

**Repeated-Opponent Decay** — A rolling 24-hour reduction applied symmetrically to PvP Reward Packets when the same two Trainers complete repeated Battles. It limits rematch farming without imposing a global daily XP cap.

**Battle Result** — The immutable outcome of a completed multiplayer Battle: its identity, participants, winner, reason, and completion time. Dojo Master activities use solo Activity Results instead of multiplayer Battle Results.

**Activity Result** — The idempotent outcome of a completed solo encounter, Lesson, Sparring attempt, or Daily Challenge. It records participation and any capture, XP, first-clear, or Mastery outcome without changing multiplayer W/L totals.

**Queue** — The matchmaking holding pen that pairs waiting Trainers into Battles.

**Lobby** — The shared spatial hub Trainers inhabit between Battles: walk around, see who else is online, issue Challenges.

**Challenge** — A direct Battle invitation from one Trainer to another, issued in the Lobby.

**Dojo Opponent** — A server-controlled Battle participant used by Lessons, Sparring, and Daily Challenges. It follows the selected mode's published policy and may inspect only public Battle state plus its own private Party state.

**Expedition** — A short PvE activity launched from the Dojo in which a Trainer encounters wild Monsters and may acquire one. Expeditions are separate from the shared Lobby and do not form an explorable overworld.

**Signal Board**: The Dojo surface showing three target Evolution Families for the current server day. The board follows a deterministic eight-day rotation that presents all 24 Families once per cycle; selecting a target snapshots the Expedition's target and support pool for that run.

**Preparation Encounter**: Either of the first two single-Monster Battles in an Expedition. It uses a distinct non-target base-stage Wild Monster from the selected target's curated support pool and never offers capture.

**Target Encounter**: The final single-Monster Battle in an Expedition. It is the only encounter with a Capture Gauge and the only encounter that can add a captured Monster to the Collection.

**Wild Monster** — An unowned Monster encountered during an Expedition. Only the base Species of an Evolution Family appears as a Wild Monster.

**Capture Gauge** — Visible, encounter-scoped progress earned through Battle actions against a Wild Monster. Filling the gauge guarantees acquisition without a consumable item or random capture check.

**Capture Objective**: A visible, encounter-scoped condition in a Target Encounter that awards Capture Gauge progress when the Trainer satisfies it through Battle actions. Objectives are deterministic and explainable. Launch IDs, Family profiles, and the Target Encounter PvE band are in the Capture Objective catalog.

**Lesson** — A guided Dojo Master activity with an authored teaching scenario and explainable behavior that teaches one Battle concept. A Lesson may reveal intent before resolution when that supports the concept being taught.

**Capture Lesson** — One of two required onboarding Lessons that ends in a guaranteed capture. Starter-aware targets round out the first Party. The first teaches Moves, matchups, and capture; the second teaches switching and Replacement. Failure fully restores and retries the current Lesson with no penalty; hints escalate from intent, to the relevant matchup, to one recommended legal action without auto-playing it. Exact targets and authored objectives are in the Dojo Master policy specification.

**Sparring** — A repeatable Dojo Master Battle using the Trainer's natural Party against a Party-aware opponent intended to approximate multiplayer play. The opponent matches each Party slot's Level and Evolution stage and balances favorable, neutral, and unfavorable matchups. The ordered Party Types and Server Day select a rotating roster shared by every Sparring Tier; after a tier's first clear, no-reward practice remixes rotate that roster again. Before entry, the Trainer sees the complete opposing roster and selected Tier rules. Every Tier may be replayed freely; only its first clear during a Server Day grants XP. The Dojo Master explains decisions only after they resolve.

**Sparring Tier** — One of three published difficulty contracts selected before Sparring. Apprentice uses simple weighted heuristics, Rival evaluates immediate outcomes and switches, and Master uses bounded two-turn reasoning. No tier may inspect the Trainer's hidden Battle Action.

**Decision Explanation** — The Dojo Master's post-resolution account of a bot choice. The Battle view shows the primary reason, while the Battle Log may expose scored legal alternatives; a Lesson may additionally explain intent before resolution.

**Daily Challenge** — One of seven authored standardized tactical puzzles selected by the Server Day: Type Read, Safe Switch, Full Rotation, Tempo, Preservation, Limited Toolkit, or Master Trial. Every Trainer receives the same loaned teams, visible objective and par, and random seed. Each loaned Party slot maps its participation to the same owned Party slot for XP. The first objective clear grants XP; meeting par records a Mastery Mark instead of granting more power. Bot decisions are explained only after they resolve.

**Server Day** — The UTC calendar day snapshotted when a daily activity starts. Rotation, first-clear eligibility, and Mastery Marks belong to that snapshot even when the activity finishes after UTC midnight.

**Mastery Mark** — A non-power record that shows a Trainer met a Daily Challenge's published par. It may be earned on an unlimited replay after the first-clear XP has already been claimed.

**Balance Run** — A reproducible evaluation of one snapshotted rules and content revision against versioned scenarios, policies, paired side and Party-order swaps, and a fixed random-seed corpus. A Balance Run produces replayable results and acceptance-gate outcomes before balance content changes.

**Reference Team** — A versioned Full Party and deterministic loadout fixture used by Balance Runs. The complete reference set covers every Evolution Family, major Party role, lead position, and a deliberate counterplay scenario.

**Balance Gate** — A measurable acceptance threshold applied to a complete Balance Run. A failed gate identifies a reproducible scenario for tuning; an isolated one-on-one Type counter is diagnostic unless it also causes a team-level gate failure.

**Species Index** — The complete public catalog of Species arranged by Evolution Family, including Type, stage, thresholds, and how many distinct individuals the Trainer owns. It describes Species; the Collection contains Monsters.

**Progression Summary** — The post-activity account of one persisted Reward Packet across every affected Party Monster: XP, Levels, Move unlocks, Evolution eligibility, and active or Reserve Shares. It is shown as one batch before optional per-Monster review.

**Progression Notice** — A durable, unreviewed progression event that asks the Trainer to inspect a newly unlocked Move, a newly captured Monster, or other actionable change. Acknowledgement removes the notice. A pending Evolution remains visible from Monster state even after its reward notice is deferred.

**Handle** — A Trainer's public display name, shown in Battles. How handles are assigned (random vs chosen) is decided by the first-run flow.

**Save** — A Trainer's persistent game state: Collection, Party, individual Monster progression, nicknames, and battle record.

**Trainer Reset** — A fresh start for a Trainer's mutable game state. The Trainer identity and SSH Credentials remain intact while onboarding chooses a new Handle and Party.
