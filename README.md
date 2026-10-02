# golunch

[![ci](https://github.com/raaaas/golunch/actions/workflows/ci.yml/badge.svg)](https://github.com/raaaas/golunch/actions/workflows/ci.yml)
[![pages](https://img.shields.io/badge/website-golunch-blue)](https://raaaas.github.io/golunch/)
[![release](https://github.com/raaaas/golunch/actions/workflows/release.yml/badge.svg)](https://github.com/raaaas/golunch/actions/workflows/release.yml) 
![golunch — multiple agent CLIs, private configs, real isolation, no containers, no daemon](docs/golunch-social.webp)

Run the same agent CLI as many times as you want, each time with its own private
config, its own login, and its own proxy — without containers, namespaces, or a
daemon.

`golunch` is a launcher. An **instance** is a directory plus a set of environment
variables: a private `HOME`, every XDG base directory, `TMPDIR`, and a `bin/` on
`PATH`. Nothing is namespaced and nothing runs in the background; the isolation is
real because the agent is told, correctly, where its files live.

```console
$ golunch new work --agent cline --proxy http://127.0.0.1:7890
$ golunch doctor work                # logged out — that is the isolation, proven
$ golunch seed work --all            # optionally: bring your MCP servers and skills in
$ work                               # the alias is now an ordinary command; sign in
$ golunch run work --prompt "list the Go packages here" --jsonl
```

## What it does / does not do

| | |
|---|---|
| isolated config, data, cache, state, runtime, tmp per instance | no proxy **server**: proxy support is env-var injection only |
| unlimited instances of the same binary, each its own identity | no traffic interception, no base-URL rewriting, no MITM |
| headless prompt runs with normalized streaming events | no namespaces, bind mounts, or containers |
| task fan-out across instances with per-task timeouts | no daemon, no self-update **of the tool itself**, no GUI |
| `flock`-based busy protection that cannot leak | not a secret manager — credentials stay where the agent puts them |
| `seed` copies MCP servers, skills and plugins in on request | no filesystem isolation — an absolute host path in a config still reaches the host |
| an agent CLI can be downloaded into the instance's own `bin/`, opt-in | installers that hardcode host paths or call `sudo` fail here instead of escaping — there is still no filesystem barrier |

Requires Go 1.27+ and a non-root user (it refuses `uid 0`, because a
root-owned instance tree leaves the agent unable to write its own config).

## Install

```console
$ go install github.com/raaaas/golunch/cmd/golunch@latest
$ golunch version                     # the module version, no -ldflags needed
$ golunch config --init               # optional: writes ~/.golunch/config.toml
```

Prebuilt binaries for Linux and macOS are attached to each
[release](https://github.com/raaaas/golunch/releases), with `checksums.txt`.

From a checkout, to get the git describe string instead of a module version:

```console
$ ver=$(git -C . describe --tags --always 2>/dev/null || echo dev)
$ go build -ldflags "-X main.version=$ver" -o ~/.local/bin/golunch ./cmd/golunch
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`, and that `~/.local/bin` is
too — that is where an instance's alias is linked.

State lives in `~/.golunch`:

```
~/.golunch/config.toml                    global defaults and named proxy profiles
~/.golunch/instances/<alias>/
    bin/                                  first on the instance PATH; may hold a downloaded agent
    home/  config/  cache/  data/  state/
    runtime/  tmp/  logs/                 logs/ also keeps the audit copy of an install script
    launcher                              the generated bash script
    metadata.toml                         what this instance is, and its proxy
~/.golunch/instances/.<alias>.lock        advisory flock, kernel-released on death
~/.local/bin/<alias> -> .../launcher      so the alias is a command
```

Override the location with `GOLUNCH_ROOT`.

## Concepts

**One environment builder.** `golunch run`, `golunch shell`, `golunch task` and the
generated launcher all render from the same function. The launcher exports exactly
the variables golunch decided, and a test asserts that in both directions, so
`work` and `golunch run work --` cannot drift apart. Values inherited from your
shell are deliberately *not* baked into a permanent script.

**A driver registry, not per-agent code.** `internal/agent` holds one data literal
per agent — argv builder, line parser, credential paths, env profile. Adding
claude or codex is a new row. Ships with `cline`, `kilo`, `opencode` (the
latter two share a factory: kilo is a fork of opencode) and `qoder`.

**Normalized events.** Headless runs decode the agent's NDJSON into one `Event`
type — `text`, `thinking`, `tool_call`, `usage`, `status`, `error`, `log` — and keep
the original line in `raw`, so an unknown schema degrades to a log line instead of
vanishing. Output is streamed line-by-line, never buffered.

**Proxy precedence,** highest wins:

```
--proxy / --noproxy on the command line
  → [proxy] in the instance's metadata.toml
    → global profile from config.toml
      → the host environment, untouched
```

Both cases of every variable are always written (`HTTP_PROXY`/`http_proxy`,
`HTTPS_PROXY`/`https_proxy`, `ALL_PROXY`/`all_proxy`, `NO_PROXY`/`no_proxy`) after
deleting all eight, because some Node libraries read only lowercase and divergent
duplicates produce half-proxied traffic. `proxy = "none"` **force-unsets** all eight
— the only way to beat a proxy inherited from the host. `NO_PROXY` always contains
the loopback floor (`localhost`, `127.0.0.1`, `127.0.0.0/8`, `::1`, `.localhost`,
`169.254.169.254`, your hostname) plus the host of `--attach`, so an agent talking
to a local server is not sent through the tunnel.

Every run writes its transcript to `logs/run-<ts>-<pid>-<n>.ndjson` and a proxy
audit to `logs/env-<ts>-<pid>-<n>.txt` recording which layer won.

**Locking.** Agent runs take a shared lock; admin operations take an exclusive one.
`--continue` without `--session` also takes exclusive, because two runs would both
append to "whatever the newest session is" and interleave. Locks are `flock`, so a
crash or `SIGKILL` cannot leave a stale lock that blocks every future operation.

## Commands

| command | what it does |
|---|---|
| `new <alias> --agent <name>` | wrap an existing host binary in a fresh instance (`--install` to fetch it instead) |
| `install <alias> [--url \| --script]` | download the agent into the instance's own `bin/`, on request — nothing else fetches |
| `ls [-l \| --json]` | list instances; `-l` adds size, version, proxy source, last run |
| `run <alias> [flags]` | headless prompt with `--prompt`/`--stdin`, else passthrough |
| `run <alias> -- <args>` | hand the args to the agent with the instance environment |
| `task <taskfile.json>` | fan N prompts out across instances |
| `clone <src> <dst> [--copy-data]` | second identity: copies the binary, not the login |
| `seed <alias> [group...] [--all]` | give the instance the host's MCP servers, skills and plugins — not its login |
| `shell <alias> [-c "cmd"]` | login shell inside the instance |
| `env <alias> [--all \| --shell]` | resolved child environment, with proxy provenance |
| `doctor [alias] [--json]` | isolation, missing binaries, launcher drift, locks, install provenance/integrity/drift |
| `refresh [alias]` | regenerate launchers from current configuration |
| `rm <alias> [--yes]` | delete the instance and its command link |
| `config [--init \| --path]`, `version` | global configuration |

`new` — `--binary --proxy --noproxy --shell --link --native --force --version --install --keep --root`
`install` — `--url --script --sha256 --version --timeout --yes --proxy --dry-run`
`run` — `--prompt --stdin --model --provider --session --continue --fork --plan
--auto-approve --thinking --title --attach --cwd --timeout --proxy --noproxy --key
--env --file --arg --jsonl --quiet --dry-run`
`task` — `--parallel --out --dry-run --quiet --ring`
`clone` — `--copy-data --proxy --no-proxy --link --yes`
`shell` — `-c --proxy --env --noproxy` · `env` — `--all --shell --proxy --env --noproxy`
`rm` — `--yes --keep-link` · `ls` — `-l --json` · `doctor` — `--json`
`seed` — `--all --with-deps --dry-run --force --host-home`

### MCP servers, skills and plugins

Isolation and this question are the same mechanism seen from two sides. Redirecting
`HOME` and the XDG dirs is what makes a new instance start logged out — and it is
also why that instance sees no MCP server, no skill and no plugin, because every
agent keeps them in exactly those directories:

| agent | MCP servers | skills / plugins / agents |
|---|---|---|
| kilo | `kilo.jsonc` → `"mcp"` | `agents/`, npm packages in `node_modules/` |
| opencode | `opencode.json` → `"mcp"` | `skill/`, `plugin/`, `agent/`, `command/` |
| cline | `~/.cline/data/settings/cline_mcp_settings.json` | `~/.cline/skills/` |

`golunch seed` copies the host's into one instance, group by group, and refuses to
copy credentials while doing it:

```console
$ golunch seed work --all
  absent  opencode.jsonc, agents, skills, plugins, commands (host has none)
  copy    ~/.config/opencode/opencode.json -> config/opencode/opencode.json
  copy    ~/.config/opencode/package.json -> config/opencode/package.json
  tree    ~/.config/opencode/agent -> config/opencode/agent (7 files)
  tree    ~/.config/opencode/skill -> config/opencode/skill (7 files)
  tree    ~/.config/opencode/plugin -> config/opencode/plugin (5 files)
  tree    ~/.config/opencode/command -> config/opencode/command (12 files)
  tree    ~/.config/opencode/scripts -> config/opencode/scripts (2 files)

seeded work (35 files copied)
warn: config/opencode/opencode.json holds apiKey; written 0600
warn: opencode.json references /home/you/.config/opencode/scripts/cf_mcp_proxy.py

An absolute host path inside a seeded config file is not a hole in isolation that an
environment variable can plug: golunch redirects env, and a program told to open
/home/... will still reach the host copy.

next: golunch doctor work
$ golunch seed work mcp --dry-run     # look first; nothing is written
$ golunch seed work skills --force    # re-copy after you changed the host's
```

- Groups come from the driver registry, not a heuristic: `mcp`, `skills`, `config`,
  and `deps`. `deps` is `node_modules` — 58 MB on the machine this was built on —
  so it is never copied unless you name it or pass `--with-deps`. Without it an
  instance loads no npm-installed plugins, which is the honest default.
- Credentials are refused by name. cline keeps
  `data/settings/providers.json` one directory away from its MCP settings, and that
  file holds `apiKey` and `accessToken` in plaintext; `seed` names it the same way
  `doctor` does and will not copy it. If a copied file turns out to contain a
  key-looking field anyway, it is written `0600` and said out loud.
- A seeded config with an **absolute host path** still resolves inside the instance
  — that is the point of the warning, not a bug in it. golunch redirects environment
  variables and launches the agent's own child processes with them, so a stdio MCP
  server starts fine and `uv`, `npx` and `node` are found on the inherited `PATH`.
  But a program told to open `/home/you/...` reaches the host copy and writes
  there. To keep an MCP server inside the instance, put its binary in
  `<instance>/bin/` (which is first on `PATH`) and name it without an absolute path.
- `--host-home <dir>` seeds from a different home, which is also how you seed from a
  mounted profile or a dotfiles checkout.
- Remote (HTTP) MCP servers are proxied like any other traffic; `127.0.0.1`, `::1`
  and `.localhost` are always in `NO_PROXY`, so a local MCP server stays direct.

Both dash styles work (`-out f.json` and `--out f.json`). A flag golunch does not
recognize is passed through to the agent; after `--`, everything is verbatim.

`--key` is injected through the driver's environment variable and never into argv,
because argv is visible in `ps` and in every run log.

### Installing an agent into an instance

Until now `golunch` could only wrap a binary that already existed on the host.
`golunch install` goes one step further and fetches the agent **into** the
instance. It is opt-in and a deliberate verb: `run`, `task` and `shell` never
download anything.

```console
$ golunch install work --url <installer> --sha256 <hex>
  fetch   via the instance's resolved proxy (never the ambient host HTTPS_PROXY)
  sha256  printed before anything runs
  run     the saved script, inside work, under its redirected HOME
  link    -> bin/<binary>, already first on work's PATH
```

Step by step: golunch fetches the vendor's install script over HTTPS into the
instance (bounded size, default 1 MiB) using the instance's **resolved profile**,
prints its **sha256**, and only then executes the saved file. It is never
`curl | bash` in a shell string — the bytes are on disk and hashed before
anything runs. Execution uses the existing `execd` child runner (own process
group, killed on `--timeout`, so nothing outlives the run) while holding the
instance's **exclusive** flock, so installing onto a running agent exits `3`
(busy) just like `rm`.

Because `HOME` is the instance's `home/`, an installer that respects `$HOME`
drops the binary inside the tree. golunch locates it and puts a **real file**
(hardlink, byte-copy fallback) at `bin/<binary>` — the first entry on that
instance's `PATH` — so the alias runs its own private copy. `golunch clone`
carries the binary (clone always copies `bin/`) and the audit script, reprefixing
every recorded path onto the clone's own tree; `golunch rm` deletes the copy with
the tree and says so, because what it removes is a download and not a link. The
instance's proxy is honored on both sides: Go's own download uses the
resolved profile, and the script's `curl`/`wget`/`git` inherit the same eight
`*_PROXY`/`NO_PROXY` variables the isolation engine already injects; loopback
stays unbypassed-by-proxy. Provenance (url, sha256, the kept audit script,
version, dir, binary, fetched_at) is recorded in `metadata.toml` under
`[install]`. A failed install records nothing there: an `[install]` table without
a binary is an integrity error by definition, so a half-done download is not
booked as an install.

Consent: the URL and script hash are printed and you confirm on the terminal,
unless you pass `--yes`; with no terminal available the confirmation is refused
rather than answered by a script, so pass `--yes` deliberately. What you approve
is what runs — the fetched bytes are pinned to the hash that was shown, so a file
swapped in between fails the check instead of passing consent. Flags are
`--url <installer>`, `--script <local file>`, `--sha256 <hex>`, `--version <v>`,
`--timeout 10m`, `--yes`, `--proxy <url|profile|none>` and `--dry-run`; and
`golunch new <alias> --agent <name> --install` creates the instance and fetches
in one step, taking the same source flags. If the host already has that agent,
`--install` says so and wraps the host copy instead of downloading a duplicate.

`doctor` then shows three things for that instance: `install` (info: the
recorded provenance), `install integrity` (error: the recorded binary is missing,
or resolves outside the instance root), and `install drift` (warn: the kept audit
script no longer hashes to the recorded sha256).

Two honesty notes, because both are the point:

- **No verified upstream URL is recorded yet.** The registry's install rows ship
  with an empty `ScriptURL`: the vendor URLs could not be checked when the code
  was written, and inventing one is worse than asking. So today `install` requires
  `--url` or `--script`, and bare `golunch install <alias>` says so rather than
  guessing. A later commit fills the per-agent rows.
- **Some installers cannot be sandboxed, and that is the designed outcome.** An
  installer that hardcodes `/usr/local/bin` or calls `sudo` will not land in the
  instance. golunch refuses `uid 0` precisely so a downloaded script can never own
  the tree, and there is still no filesystem barrier. When that happens golunch
  fails loudly and points you at `--binary` or a version manager — the isolation
  holding, not a hole to grumble about.

Why do it at all: two aliases, two versions of the same CLI, one host untouched.

### Example run

```console
$ golunch run work --prompt "what tests are failing?" -m anthropic/claude-sonnet-4-5 \
      --timeout 5m --jsonl
{"type":"tool_call","agent":"cline","tool":{"name":"list_files","input":{...}}}
{"type":"text","agent":"cline","text":"Two suites fail ..."}
{"type":"usage","agent":"cline","usage":{"input_tokens":41203,"output_tokens":812,"cost":0.19}}
```

In human mode (`--prompt` without `--jsonl`) stdout carries only the answer and
stderr carries progress, so `golunch run work --prompt "..." > answer.txt` gives you
the text and nothing else. `--dry-run` prints the argv, the variables golunch
decided, and the resolved proxy, then exits without starting anything — no lock, no
log.

### Taskfile

```json
{
  "name": "sweep",
  "defaults": { "instance": "work", "model": "ollama/qwen2.5", "parallel": 3, "timeout": "120s" },
  "tasks": [
    { "id": "arith",     "prompt": "What is 17 plus 25?" },
    { "prompts": ["a", "b", "c"] },
    { "id": "slow",      "prompt": "never finishes", "instance": "other", "timeout": 20 }
  ]
}
```

`prompts` expands into separate runs. `timeout` accepts `"90s"`, `"2m"`, or bare
seconds. One failing task is reported and the sweep continues — a bad prompt in a
batch of fifty is not a reason to lose the other forty-nine — but the command exits
nonzero. `-parallel n`, `-out summary.json`, `-quiet`, `-ring n` (events held in
RAM per task; the full transcript always goes to the log).

## Use as a library

The root package is the same mechanism without the argv. `Runner.Prompt` is what
`golunch run --prompt` calls, so a Go program gets the same lock, the same proxy
precedence, the same run log and the same normalized events.

```go
cfg, err := config.Load()
if err != nil { return err }
r := golunch.NewRunner(cfg, os.Stdin, os.Stdout, os.Stderr, instance.HostFromOs())

res, err := r.Prompt(ctx, golunch.PromptOptions{
    Alias:   "work",
    Prompt:  "what tests are failing?",
    Timeout: 5 * time.Minute,
    OnEvent: func(ev agent.Event) bool {
        fmt.Println(ev.Type, ev.Text)
        return true // false stops the run
    },
})
switch golunch.ErrKind(err) {
case golunch.KindBusy: // retry: another run holds the instance
case golunch.KindTimeout:
case golunch.KindNotFound:
}
```

`NewRunner` reads no environment variable and opens no descriptor of its own: the
data root comes from the `config.Config` you hand it and the streams from the
writers you pass, which is why the test suite runs the real code path against a
temp directory. Instance *creation* stays on the CLI — `golunch new` is a human,
one-time action, and a library that silently made instances would be a library
that silently made them somewhere unexpected.

Everything else is importable too: `agent` for the driver registry, `instance` for
`BuildEnv` and the launcher renderer, `proxy` for the precedence resolution.

## Configuration

`golunch config --init` writes a commented file. Named profiles are referenced by
name from anywhere:

```toml
[defaults]
shell   = "/bin/bash"
timeout_seconds = 0
proxy   = "corp"          # a profile name, a URL, or "none"

[proxy]
default          = "corp"
no_proxy_extra   = ["..internal"]

[proxy.profiles.corp]
http     = "http://proxy.corp:8888"
https    = "http://proxy.corp:8888"
socks    = ""              # sets ALL_PROXY when present
no_proxy = [".corp.test"]
```

Per-instance overrides live in `~/.golunch/instances/<alias>/metadata.toml` and beat
the global config; `--proxy` beats both.

## Exit codes

`0` success · `1` general failure · `2` usage error · `3` instance busy ·
`4` unknown instance or agent · `124` timed out. A passthrough child's own exit code
is returned unchanged, so `golunch run k -- make` works in a shell that checks `$?`.

## Development

```console
$ gofmt -l . && go vet ./... && go test ./...
```

Layout: the root package is the library (`Runner`), the packages next to it are the
mechanism (`agent`, `instance`, `proxy`, `config`, `execd`, `task`, `atomicfile`),
`internal/cli` is flags and dispatch, and `cmd/golunch` is the only file that names
`os.Args`, `os.Std*` or `os.Exit`. `internal/fixtures` and `internal/osutil` are
shared plumbing that no caller should have opinions about.

The suite is fully offline and costs no API credits: `GOLUNCH_ROOT` points the whole
run at a temp directory, `testdata/fixtures/` holds recorded cline and kilo
transcripts (including a real 401 and a banner-noise log), and `driver_binary_override`
in an instance's metadata can point a driver at `/bin/sh` replaying a fixture, which
exercises streaming, parsing, locks, timeouts, and process-group kills without
touching a model. `golunch run --dry-run` and a sleeping fake agent cover the kill
path; launcher/`BuildEnv` parity is asserted by test, not by comment.

The library tests are in `package golunch_test`, so they can only reach exported
names: narrowing the public surface fails a test rather than quietly making people
shell out to the binary.

## Known agent quirks (measured, not assumed)

- `cline` has **no `-y` flag**; auto-approval is `--auto-approve <boolean>`.
- `kilo run --format json` emits **flat** records (`{type,sessionID,part}`), not the
  `agent_event` wrapper `cline` uses. Parsing kilo with cline's schema yields zero
  events and exit 0.
- `kilo` and `opencode` keep credentials in `XDG_DATA_HOME`, and `kilo.db` is
  ~1.2 GB — which is why `ls` never walks instance trees unless you ask with `-l`,
  and why `clone` does not copy data by default.
- Overriding `XDG_CONFIG_HOME` means a kilo instance starts with no plugins or
  agents, because those live in `~/.config/kilo/node_modules`. `doctor` warns when an
  instance config directory is empty, and `golunch seed` is the way to fill it.
- `cline` keeps `~/.cline/data/settings/providers.json` next to its MCP settings
  file, holding `apiKey` and `accessToken` in plaintext. golunch treats it as a
  credential: it is in cline's secret list, which means `doctor` counts it as a
  sign of being logged in, `clone --copy-data` warns about it, and `seed` refuses it.
- This kilo build returns no assistant text part for custom OpenAI-compatible local
  providers (verified in the agent's own raw output, not in golunch). Prompt a real
  gateway model, or check `logs/run-*.ndjson` before blaming the launcher.
- `freebuff` (npm, a manicode-derived TUI) has **no headless mode**: `-p`,
  `--headless`, subcommands and piped stdin all either error or render the
  interactive UI as raw escape codes. It is therefore not a registry driver;
  wrap it for isolated interactive use with `golunch new <alias> --binary
  <freebuff>` and reach it through passthrough (`golunch run <alias> --
  --version` works, and the agent's own installer download lands inside the
  instance tree). `run --prompt` and task fan-out are impossible until the
  vendor emits parseable output.

## Agent skill

`skills/golunch-parallel-tasks/SKILL.md` is a portable Agent Skills file: it
teaches any skill-capable agent (Claude Code, Qoder, opencode/kilo, Codex
with a skills loader) to preflight nodes, gate on login (bootstrapping auth
with the user when a node is logged out), split work into a taskfile, fan it
out in parallel, and collect `summary.json` results. Install by symlinking
the skill directory into the agent's skills root, e.g.:

```bash
ln -s "$PWD/skills/golunch-parallel-tasks" ~/.claude/skills/golunch-parallel-tasks
ln -s "$PWD/skills/golunch-parallel-tasks" ~/.qoder/skills/golunch-parallel-tasks   # Qoder
ln -s "$PWD/skills/golunch-parallel-tasks" ~/.config/opencode/skill/golunch-parallel-tasks
```

## Nesting

If `$HOME` points inside another isolated instance, golunch stores its data under
the **real** home (resolved from the passwd database) and warns, because otherwise
`~/.golunch` would silently live inside that instance and be deleted with it. It
strips `WARREN_*` and `GOLUNCH_INSTANCE*` from a child's environment so an instance
cannot mistake itself for its parent.

## Support

If golunch saves you the twelve `--config-dir` flags, [sponsor the
project](https://github.com/sponsors/raaaas).
