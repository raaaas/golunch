---
name: golunch-parallel-tasks
description: Split work into parallel launches across isolated agent-CLI nodes managed by golunch. Use when asked to run tasks in parallel across agents/accounts, fan out prompts over multiple logins, or use golunch instances as worker nodes. Preflight checks node inventory and login state, and bootstraps auth with the user before any task is launched.
---

# golunch: parallel task fan-out across isolated agent nodes

You are driving **golunch** (`golunch` on PATH), a launcher that wraps agent
CLIs (`qoder`, `kilo`, `opencode`, `cline`) into isolated instances. Each
instance = one node: private login, config, tmp. Your job: verify nodes are
signed in, create/auth what's missing **with the user**, then split the work
into a taskfile, run it in parallel, and collect results.

## Step 0 — Preflight (never skip)

```bash
command -v golunch || echo MISSING
golunch ls --json      # one row per node: alias, agent, version, root
golunch doctor --json  # per-node checks; "check":"login" says signed in / logged out
```

- `MISSING` → tell the user golunch is not installed and stop
  (`go install github.com/raaaas/golunch/cmd/golunch@latest` or a release
  binary; do not run installers without the user's consent).
- Zero nodes → go to Step 1.

## Step 1 — Create nodes (only with the user's consent)

Ask the user how many parallel workers they want and which agent(s). One node
per concurrent worker is the default; more nodes of the same agent give more
independent logins.

```bash
golunch new <alias> --agent qoder      # or kilo | opencode | cline
```

Never reuse or delete existing aliases without asking.

## Step 2 — Auth gate: check, then bootstrap with the user

Every node you launch a task on must report **signed in**:

```bash
golunch doctor <alias> --json | grep '"login"'
# logged out → detail says "logged out: none of the N credential path(s) exist"
```

If logged out, you cannot proceed silently. Do this:

1. **Interactive login (the normal path).** Tell the user: run `<alias>` in a
   terminal — it opens the agent's own login (browser OAuth for qoder,
   provider config for opencode/kilo, etc.). You cannot complete browser
   OAuth for them; wait until they say it's done, then re-run doctor.
2. **API key path.** If the user volunteers a key, pass it as an environment
   value the agent reads, or via `golunch run <alias> --key <KEY>` on a test
   prompt — keys go through env, never argv echo. Do not store keys yourself.
3. **Import an existing profile (only if the user explicitly asks).** If the
   user points you at an already-authenticated profile directory of the same
   agent, copy ONLY its declared credential files into the instance's own
   state dir (for qoder: `user` and `machine_id` from `.auth/`, into
   `<instance>/home/.qoder/.auth/`, chmod 600). Never print or log file
   contents. Verify with doctor afterward.

Gate rule: launch tasks only on nodes doctor reports signed in. Logged-out
nodes burn no model calls but produce dead tasks — leave them out of the
sweep and tell the user which ones still need login.

## Step 3 — Sanity-check one node before fanning out

```bash
golunch run <alias> --prompt "Reply with exactly: NODE OK" --model <valid-model> --timeout 2m
```

- qoder nodes have **no default model** — you must pass `--model` (e.g. a
  name from `golunch run <alias> -- --list-models`).
- kilo/opencode want `provider/model` form.
- Nonzero exit or missing marker → do not fan out; fix auth/model first.

## Step 4 — Split the work

Decompose into independent units. One task per unit; units that need shared
conversation context stay on the same node and chain sessions (Step 6).

Taskfile schema (JSON):

```json
{
  "name": "sweep",
  "defaults": { "instance": "<signed-in alias>", "timeout": "2m", "parallel": 3 },
  "tasks": [
    { "id": "unit-1", "prompt": "…", "model": "…", "instance": "node-a" },
    { "id": "unit-2", "prompt": "…", "instance": "node-b" },
    { "prompts": ["sub-a", "sub-b"], "instance": "node-a" }
  ]
}
```

Rules:
- `instance` required (per task or defaults); `prompts[]` expands into runs;
  `timeout` = `"90s"`, `"2m"` or bare seconds; omitted `id` becomes
  `<instance>-<index>/<total>`.
- Set `model` **per task** when nodes run different agents — `defaults.model`
  leaks a wrong name to the other agent family.
- Round-robin tasks across the signed-in nodes from Step 2.

## Step 5 — Launch and retrieve

```bash
golunch task sweep.json --dry-run                       # validates, starts nothing
golunch task sweep.json --parallel <nodes> --out summary.json --quiet
```

Retrieve: `summary.json` rows carry `id`, `instance`, `answer`, `exit_code`,
`timed_out`, `error`, `session_id`, `log`, `duration_ms`, `usage`. The full
transcript (thinking/tool_call/usage events) is the NDJSON file at `log`.
Present `answer` per task to the user; report failures with their `error`.

Exit codes: `3` node busy (another run holds its lock — wait, don't spam
retry), `124` timeout, `1` errors. One failed task does not abort the sweep.

## Step 6 — Continue or branch a unit

Reuse `session_id` from the results:

```bash
golunch run <alias> --session <id> --prompt "follow-up…"
golunch run <alias> --session <id> --fork --prompt "branch…"
```

## Hard rules

- Never read, print, or copy credential file contents; existence checks only.
- Never invent model names: read them from the node (`-- --list-models` for
  qoder) or ask the user.
- Never run a sweep over nodes that failed Step 2/3 — report which need login.
- Respect quota: every task is a real model request; dry-run first, and
  confirm with the user before launching large fan-outs (say how many
  requests, on how many nodes).
