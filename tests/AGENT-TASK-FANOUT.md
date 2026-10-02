# Tutorial: An Agent's Guide to Splitting Work Across golunch Nodes

How an orchestrating agent uses golunch to **fan a task out over several
isolated CLI nodes in parallel and get the results back** — programmatically.

## Mental model

| idea | golunch thing |
|---|---|
| worker node | an **instance** (`golunch ls`): an agent CLI with its own login, config, tmp |
| launch one job | `golunch run <node> --prompt "..."` |
| launch many jobs | a **taskfile** + `golunch task sweep.json --out summary.json` |
| get a result | stdout (human) · `--jsonl` events (streaming) · `summary.json` rows · per-run NDJSON log |
| continue a job | the `session_id` echoed by every run |

Nodes of **different aliases run truly concurrently** — that is the point of
isolation: no shared config, no session-state collision.

## Step 1 — Inventory the nodes

```bash
golunch ls --json      # {"alias","agent","binary","root","version",...} per line
golunch doctor --json  # per-instance health; the `"check":"login"` line says signed in / logged out
```

Pick only `signed in` nodes for the sweep. Logged-out nodes fail every task
with no model call — that wastes nothing but time.

## Step 2 — Split the work into a taskfile

A taskfile is JSON with `defaults` and `tasks[]`; every task inherits
defaults and may override anything. `prompts[]` expands into separate runs.

```json
{
  "name": "sweep",
  "defaults": {
    "instance": "node1",
    "model": "Qwen3.8-Flash",
    "timeout": "2m",
    "parallel": 3
  },
  "tasks": [
    { "id": "summarize", "prompt": "Summarize the changes in CHANGELOG.md in 5 bullets" },
    { "id": "audit",     "instance": "node2",
      "prompt": "List every TODO and FIXME in this repo with file:line" },
    { "prompts": ["explain the build target", "explain the test target"],
      "instance": "node1" },
    { "id": "long",      "prompt": "deep review of package X", "timeout": "10m",
      "thinking": "enabled" }
  ]
}
```

Rules (from `task/task.go`):
- `instance` is required somewhere — per task or in defaults.
- `timeout` accepts `"90s"`, `"2m"`, or bare seconds (`120`).
- Omitted `id` defaults to `<instance>-<index>/<total>` (e.g. `node1-3/1`).
- Per-task fields: `id, instance, prompt(s), model, provider, thinking,
  session, continue, title, files, timeout, auto_approve, extra`.

Splitting heuristic for the orchestrating agent: independent sub-questions →
one task each; sub-questions that need the *same conversation* → same
`instance` and `session` chaining (Step 5); volume → more aliases of the same
agent (`golunch clone node1 node1-b`).

## Step 3 — Launch the sweep

```bash
golunch task sweep.json --dry-run          # resolves every task, starts nothing
golunch task sweep.json --parallel 4 --out summary.json
golunch task sweep.json --quiet --out summary.json --ring 200
```

- `--parallel n` overrides `defaults.parallel`.
- `--out summary.json` writes machine-readable results (Step 4).
- `--ring n` keeps only n events per task in memory; the **full transcript
  always** lands in the instance's `logs/run-*.ndjson`.
- One failing task does not abort the sweep; the command exits nonzero if any
  task failed. Exit codes: `3` busy, `124` timeout, `1` error.

## Step 4 — Retrieve the results

`summary.json` shape (exactly, per `internal/cli/task.go`):

```json
{
  "taskfile": "sweep",
  "tasks": [
    {
      "id": "summarize",
      "instance": "node1",
      "prompt": "...",
      "answer": "the assistant's final text",
      "exit_code": 0,
      "timed_out": false,
      "error": "",
      "session_id": "...",         // keep this for follow-ups
      "log": "/.../logs/run-....ndjson",
      "duration_ms": 3089,
      "usage": {"input": 0, "output": 0, "cost": 0},
      "tail_events": [],           // present with --ring
      "ring_dropped": 0
    }
  ]
}
```

The answer alone: `jq '.tasks[] | {id, answer}' summary.json`.
The full transcript (every thinking/tool_call/usage event, normalized, with
the agent's raw line preserved in `raw`): read the `log` path — one JSON
object per line, `type` ∈ `text|thinking|tool_call|tool_result|usage|status|error|log`.

Streaming alternative for a single job instead of a sweep:

```bash
golunch run node1 --prompt "..." --jsonl   # events stream to stdout as they happen
```

## Step 5 — Follow-ups and continuation

Every run echoes `session_id` (stderr line or `summary.json`). To continue a
job on the same node:

```bash
golunch run node1 --session <id> --prompt "now expand bullet 3"
golunch run node1 --continue --prompt "..."        # newest session
golunch run node1 --session <id> --fork --prompt "..."  # branch, keep original
```

In a taskfile: `{"id":"stage2","session":"<stage1 session_id>","prompt":"..."}`.

## Step 6 — Reap and clean up

```bash
golunch doctor              # node health after the sweep
golunch ls -l               # sizes incl. last run per instance
golunch rm <alias> --yes    # when a scratch node is done
```

## Gotchas measured on this tool

- **qoder nodes have no default model** — every task needs `model` in
  defaults or the agent exits 1 with an empty error.
- **`defaults.model` applies to every task** — in a mixed-agent sweep, a
  qoder model name also gets handed to kilo/opencode nodes (which expect
  `provider/model` names). Set `model` per task when agents differ, not in
  defaults.
- A node is locked **shared** while running; admin ops (`rm`, `clone`,
  `install`) exit `3` busy until the run finishes — do not retry in a tight
  loop, wait on the run.
- `--dry-run` is free and validates the whole taskfile (instance names,
  timeouts, defaults). Run it before every sweep.
- Results never live in golunch itself: they are in `summary.json`, stdout,
  and the per-instance `logs/`. The `log` path in each row is the durable copy.
