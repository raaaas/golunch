# Tutorial: Discovering and Smoke-Testing golunch Instances

Step-by-step guide to discover every instance golunch manages, detect
logins, and prove the tool works with one real prompt. Discovery is fully
general: it asks golunch itself, so it covers all agents in the driver
registry — currently `cline`, `kilo`, `opencode`, `qoder` — and every alias
you create in the future, without editing the script.

## Verified working node (live, end-to-end)

A qoder instance was built and confirmed answering through Qwen3.8-Flash:

```bash
golunch new qtest --agent qoder                       # binary auto-found in ~/.qoder/entry
# give the instance its own qodercli + login (import an existing profile):
mkdir -p ~/.golunch/instances/qtest/home/.qoder/bin/qodercli ~/.golunch/instances/qtest/home/.qoder/.auth
ln -s ~/.qoder/bin/qodercli/qodercli-<ver> ~/.golunch/instances/qtest/home/.qoder/bin/qodercli/qodercli
cp <source-profile>/.auth/user <source-profile>/.auth/machine_id ~/.golunch/instances/qtest/home/.qoder/.auth/
chmod 600 ~/.golunch/instances/qtest/home/.qoder/.auth/*
golunch doctor qtest                                  # "signed in; credentials live only in ..."
golunch run qtest --prompt "Reply with exactly: PROFILE OK" --model "Qwen3.8-Flash" --timeout 2m
```

Two measured facts about a fresh qoder instance: it starts logged out until
the two `.auth` files are present, and it has **no default model** — omitting
`--model` makes qodercli exit 1 with an empty error. The smoke script now
passes `--model` by default for this reason.

## Concepts

- **Instance** = a directory + an environment (`~/.golunch/instances/<alias>/`
  with private HOME/XDG dirs and a `bin/`), launched through its alias command.
- **Driver registry** (`agent/` package) — one data row per supported agent:
  argv builder, output parser, and **credential paths**. This is where
  discovery and login detection come from; never hardcode one agent.
- **Login detection** — `golunch doctor` checks whether the driver's credential
  files exist inside the instance (existence only; contents are never read).

## Step 1 — Discover all instances (any agent)

```bash
golunch ls          # table: ALIAS, AGENT, BINARY, LAST RUN
golunch ls --json   # one JSON object per instance, includes root + version
```

This lists every instance of every agent — one qoder alias is not the whole
fleet, and neither is one agent family.

## Step 2 — Check which instances are logged in

```bash
golunch doctor            # all instances
golunch doctor <alias>    # one instance
golunch doctor --json     # machine-readable; "check":"login" lines
```

`logged out: none of the N credential path(s) exist` is normal for a fresh
instance — that proves the isolation works. Sign an instance in once:
`<alias>` interactively, or seed config with `golunch seed <alias> --all`.

## Step 3 — Send one headless test message

```bash
golunch run <alias> --prompt "Reply with exactly: PROFILE OK" --timeout 3m
golunch run <alias> --prompt "..." --model <provider/model>   # pin a model
golunch run <alias> --dry-run --prompt "..."                  # show argv/env, no request
```

`run` builds the right argv for the instance's agent (the registry does this —
`opencode`/`kilo` become `<binary> run --format json <prompt>`). Success =
exit 0 and the answer contains your marker.

## Step 4 — The script: batch-test everything

`tests/golunch-profile-smoke.sh` chains Steps 1–3:

```bash
tests/golunch-profile-smoke.sh --list                  # inventory + auth, free
tests/golunch-profile-smoke.sh                         # test all instances
tests/golunch-profile-smoke.sh node1 node2             # only these
MODEL=openrouter/qwen3.8-flash tests/golunch-profile-smoke.sh   # pin model
```

Per instance it prints agent, version, root, auth state, then `PASS`/`FAIL`.
Logged-out instances fail with the login hint and are **not** prompted.
Exit code is non-zero if anything failed.

## Adding a new agent

1. Add a driver row in the `agent` package (argv, parser, credential paths).
2. `golunch new <alias> --agent <name>`.
3. The smoke script picks it up automatically — it iterates `golunch ls
   --json` and reads login state from `golunch doctor --json`. No changes
   needed in the script itself.

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `no instances discovered` | Create one: `golunch new <alias> --agent <name>` |
| `auth: logged-out` everywhere | Expected for fresh instances — sign in interactively once per alias |
| Run exits 3 | Instance busy — another run holds its flock; wait or stop it |
| Run exits 124 | `--timeout` hit; raise it or check the agent's `logs/run-*.ndjson` |
| Marker missing but exit 0 | Agent returned unexpected text — inspect the run log in the instance's `logs/` |

## Notes

- One model request per authed instance — use `--list` for a free inventory.
- Credentials stay where the agent put them; the script only records that the
  registry-named paths exist.
- `--key` injects API keys via the driver's env var, never argv (`ps` leakage).
