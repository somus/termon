# Test multiplayer locally

This walkthrough starts an isolated Termon server and uses two disposable SSH identities to test onboarding, matchmaking, a three-Monster Battle, persistence, and reconnects.

## Prerequisites

Install Go and OpenSSH, then open three terminals at least 100 columns by 32 rows. The helper uses port 2222 by default so it can run without elevated privileges; set `TERMON_PORT` if that port is busy.

## Start the server

From the repository root, run:

```sh
./scripts/local-duel.sh
```

The helper copies the content pack into a temporary directory, creates two client keys, prints a connection command for each Trainer, and starts termond. Keep this terminal open.

## Prepare two Trainers

1. Run the printed **Terminal A** command in your second terminal and the **Terminal B** command in your third.
2. Complete onboarding in both clients. Each Trainer chooses a starter and completes two Capture Lessons, producing the Full Party required for multiplayer.
3. Follow the on-screen key hints to return both Trainers to the Dojo.

## Complete a multiplayer Battle

1. Press `f` in both clients, configure each Queue Move Set, and confirm entry. The Queue pairs the Trainers and opens the same Battle.
2. Follow the battle prompts to choose Moves, Switch active Monsters, and select replacements after faints.
3. Continue until one Party has no healthy Monsters. To test forfeit instead, choose **RUN** and confirm it.
4. Press `Enter` after the result hold to return to the Dojo.
5. Confirm the winner and loser records in the header changed.

## Verify persistence

Stop the server, then restart it with the state path printed at startup:

```sh
TERMON_RUN_DIR=/tmp/termon-mvp.ABC123 ./scripts/local-duel.sh
```

Run the same two SSH commands again. Both Trainers should retain their handles, Collections, Parties, progression, and multiplayer records.

## Verify battle reconnects

1. Start another paired Battle and resolve at least one turn.
2. Close Terminal A with `Ctrl+C`. Terminal B should report that its opponent is reconnecting.
3. Within 60 seconds, rerun Terminal A's original SSH command.
4. Confirm both clients return to the active Battle with the prior HP, Party state, and event log.

Repeat the disconnect and wait longer than 60 seconds to test timeout behavior. Terminal B should receive a disconnect-timeout win, and both records should update.

## Stop the server

Press `Ctrl+C` in the server terminal. The helper retains its temporary directory so you can inspect or reuse the database, keys, and copied content.
