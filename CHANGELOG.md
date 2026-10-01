# Changelog

## v1.1.0 (2026-10-01)

The downloader. Everything before this release could only *wrap* a binary that
already existed on the host; `golunch install` fetches the agent into the
instance instead. Because this is the one place golunch executes remote bytes,
the whole design is bent around proving, hashing and containing that step rather
than around convenience.

- A new `install` package owns the fetch: an HTTPS download bounded in size
  (default 1 MiB), routed through the instance's *resolved* proxy (never the
  ambient host `HTTPS_PROXY`), written to disk and hashed before it is run —
  never `curl | bash` in a shell string. It executes under the existing `execd`
  child runner, so the install runs in its own process group, is killed on
  `--timeout`, and cannot outlive the command, and it holds the instance's
  exclusive flock, so installing onto a running agent exits `3` (busy) like `rm`.
- New command `golunch install <alias>`, with `--url`, `--script`, `--sha256`,
  `--version`, `--timeout`, `--yes`, `--proxy` and `--dry-run`. `golunch new
  <alias> --agent <name> --install` creates the instance and downloads in one
  step, and wraps the host binary instead if the host already has it. Consent is
  explicit: `install` is a verb you type, the URL and script hash are printed, and
  you confirm on the terminal unless you pass `--yes` — with no terminal the
  confirmation is refused rather than answered by a script. What you approve is
  what runs: the fetched bytes are pinned to the hash that was shown, so a swap
  between the print and the exec fails the check instead of passing consent.
  Nothing is ever fetched automatically by `run`, `task` or `shell`.
- `agent.InstallInfo` is a new registry field describing each agent's installer
  as *data* — script URL, optional pinned hash, fixed args, the version-pin env
  var, the child-PATH prerequisites and a limitation note — so the CLI can print,
  hash and confirm the transaction without per-agent code. `ScriptURL` is
  **deliberately empty in every shipped row**: the vendor URLs could not be
  verified when these rows were written, and inventing one is worse than
  requiring `--url`/`--script`. So today `golunch install <alias>` with no flag
  tells you that rather than guessing; a later commit fills the rows.
- `agent.LocateIn` answers only "did this land inside the instance": it walks the
  agent's `KnownDirs` and each redirected home's `$HOME/.local/bin`, never `PATH`
  and never the system dirs `Locate` falls back to, so a download that escaped the
  tree cannot pass as a success. As part of this work `KnownDirs` entries
  containing `*` are now glob-expanded. **This is an intentional behavior
  change**, and it cuts against v1.0.0's "behavior unchanged" line: `Locate`
  historically joined and stat'd each entry literally, so cline's
  `~/nvm/versions/node/*/bin/cline` template could never match. `golunch new
  --agent cline` may therefore now find an nvm-installed host cline it previously
  missed.
- `metadata.toml` gains an `[install]` table (`url`, `sha256`, the audit copy of
  the script under `logs/`, `version`, `dir`, `binary`, `fetched_at`), and
  `golunch doctor` gains three checks for it: `install` (info: provenance),
  `install integrity` (error: the recorded binary is missing or resolves outside
  the instance root), and `install drift` (warn: the kept audit script no longer
  hashes to the recorded `sha256`).
- `golunch clone` carries an installed binary — it always copies `bin/` — and the
  audit copy of the installer with it, reprefixing every path in `[install]` into
  the destination tree so the clone's provenance points at its own files. A
  binary that lives under the source's `home/` is named as stranded instead of
  being quietly reprefixed onto a path the clone does not have. `golunch rm`
  deletes the copy with the rest of the tree and says that it is doing so.
- `execd` now maps its own deadline onto `Result.TimedOut` on the inherited-stdio
  branch as well as the streaming one. That branch returned before the check, so
  a timed-out interactive child reported only `Killed` and exit `143` and no caller
  of `golunch shell -c` or a passthrough could ever hand back the `124` that
  `errors.go` says a timeout costs.
- `golunch new --install` runs the installer under the command's own context, so
  Ctrl-C reaches it the way it reaches `run` and `shell`: a vendor script that
  hangs dies with the command rather than outliving it, and the tree it had made
  is left with no `[install]` table and no launcher, for `golunch rm` to take back.

`rm`, `run`, `task` and `shell` are otherwise unchanged, and the properties that
made the isolation trustworthy still hold: golunch still never updates itself,
still runs no daemon, and still refuses `uid 0` — precisely so a downloaded
script can never own the instance tree. An installer that hardcodes an absolute
host path or calls `sudo` will not land in the instance and is failed loudly with
a pointer to `--binary`, which is the designed outcome, not an escape: there is
still no filesystem barrier.

## v1.0.0

First release as a fetchable module. The binary is the same one; what changed is
that it can be installed and embedded.

- Module path is now `github.com/raaaas/golunch`. A bare module name could not be
  fetched, so nothing here could be installed or imported from outside the
  directory.
- `agent`, `instance`, `proxy`, `config`, `execd`, `task` and `atomicfile` moved
  out of `internal/` and are importable.
- New root package `golunch` with `Runner`: the headless execution layer that was
  already in `internal/cli` (`Prompt`, the proxy precedence chain, `BuildEnv`,
  event rendering), now reachable without exec'ing the CLI. `internal/cli` keeps
  flags, usage and dispatch.
- `CommandError` carries the underlying cause alongside its message, so a caller
  can still distinguish a lock collision from any other failure (`IsBusy`).
- Timeout is `ExitTimeout` (124) instead of a literal repeated in three places.
- `golunch version` reports the module version from build info, so
  `go install github.com/raaaas/golunch/cmd/golunch@v1.0.0` needs no `-ldflags`.
- Tests are in `package golunch_test` where they assert the library surface, so a
  future narrowing of the API fails a test instead of quietly requiring the CLI.

Behavior, the CLI flag set, the exit codes and the on-disk layout under
`~/.golunch` are unchanged.
