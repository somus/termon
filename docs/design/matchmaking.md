# Matchmaking & Session Lifecycle v1 — decided (TERM-10)

## Two roads into a Battle

1. **Queue** — global FIFO random pairing ("Find Battle"). For people who just want a match.
2. **Lobby challenge** — Trainers walk around a shared spatial Lobby and challenge whoever's nearby. The social path, late.sh-style. Lobby world design: separate ticket.

## Session lifecycle

The live session flow uses an SSH Credential to authenticate a stable Trainer. See [durability and production persistence](durability.md) for the restart boundary.

```
ssh in → SSH Credential → Trainer exists?
  no  → first-run flow (handle, starter, Master Sable, two Capture Lessons)
  yes → Lobby
Lobby: walk · challenge adjacent trainer · Find Battle (queue) · Dojo Master · Party · Exit
battle end → results screen → Lobby
connection drop during Battle → 60s same-process reconnect grace
process restart → active Battle abandoned, then Lobby on reconnect
```

## Queue rules

- Single global FIFO. Join alone → waiting screen with cancel, showing wait count and your position. Second trainer arrives → instant pair, both transition to battle.
- No ratings, no matchmaking windows.
- Every Queue pairing and Lobby Challenge uses three owned Monsters with their earned Levels, achieved Species, natural stats, and equipped unlocked Moves. Preparation changes Party membership and order through Workbench. It does not grant stages, Levels or Moves. Completed Battles award XP under [progression rules](xp-progression.md).
- Queue and Challenge stay closed until Capture Lessons produce a Full Party. They reopen as three-Monster Normalized Battles. See [Implementation rollout](implementation-rollout.md).

## Connection displacement (reconnect)

An SSH Credential authenticates one Trainer, and a Trainer owns at most one live session. A new SSH connection with the same credential displaces the old connection: mid-battle it reattaches to the Battle view via event-log replay; otherwise it takes over at the same screen. The 60-second grace timer governs the opponent's view while the same termond process remains alive: "Opponent lost connection - reconnecting...", then auto-forfeit.

Active Battles do not survive a process restart or deployment. Restart abandons the Battle without changing either record; authenticated Trainers return to the Lobby.

## Dojo Master

Standing beside Master Sable opens Capture Lessons, three-tier Sparring, and the Daily Challenge. These solo modes never touch multiplayer W/L; their teams, rewards, and replay rules are defined in [Dojo Master modes and bot behavior](dojo-master.md).

## Monster management

Outside Battle, `p` opens the selected [Collection and Party Workbench](collection-party.md). It browses owned individuals and the Species Index, edits the ordered Party and persistent Battle Loadouts, and reviews pending progression. The old Practice shortcut is removed; Master Sable owns solo-mode entry.

## After battle

Results screen → back to the Lobby. Rematch = turn around and challenge again; no explicit rematch prompt in v1.
