#!/usr/bin/env bash
# golunch profile smoke test.
#
# Discovery is delegated entirely to golunch — it works for EVERY agent in the
# driver registry (currently cline, kilo, opencode), not just qoder:
#   golunch ls --json            -> every instance: alias, agent, version, root
#   golunch doctor <a> --json    -> login check via the driver's declared
#                                   credential paths (existence only, never read)
#   golunch run <a> --prompt ... -> one headless message, normalized events
#
# Usage:
#   ./golunch-profile-smoke.sh                 # all instances
#   ./golunch-profile-smoke.sh --list          # inventory, no model requests
#   ./golunch-profile-smoke.sh alias1 alias2   # only these
#   MODEL=anthropic/claude-sonnet-4-5 ./golunch-profile-smoke.sh   # pin a model
set -uo pipefail

PROMPT="Reply with exactly: PROFILE OK"
MARKER="PROFILE OK"
# A fresh instance has no default model; qodercli exits 1 without -m. Override
# with MODEL=... — other agents want provider/model form (e.g. ollama/qwen2.5).
MODEL="${MODEL:-Qwen3.8-Flash}"
LIST_ONLY=0
if [ "${1:-}" = "--list" ]; then LIST_ONLY=1; shift; fi

command -v golunch >/dev/null || { echo "error: golunch not in PATH"; exit 1; }

rc=0
n=0
while IFS=$'\t' read -r alias agent version root; do
  n=$((n+1))
  echo "== $alias (agent: $agent $version) =="
  echo "   root: $root"

  # login state straight from the driver registry's credential paths
  auth=$(golunch doctor "$alias" --json 2>/dev/null |
    python3 -c 'import json,sys
for line in sys.stdin:
    d = json.loads(line)
    if d.get("check") == "login":
        detail = d.get("detail", "")
        print("logged-out" if "logged out" in detail else "authed")
        break')
  echo "   auth: ${auth:-unknown}"

  if [ $LIST_ONLY -eq 1 ]; then continue; fi

  if [ "${auth:-unknown}" != "authed" ]; then
    echo "FAIL  $alias: not logged in — run '$alias' interactively to sign in"
    rc=1
    continue
  fi

  model_args=()
  [ -n "${MODEL:-}" ] && model_args=(--model "$MODEL")
  out=$(cd /tmp && golunch run "$alias" --prompt "$PROMPT" "${model_args[@]}" \
        --timeout 3m --quiet < /dev/null 2>&1)
  status=$?
  echo "   exit=$status output: $out"
  if [ $status -ne 0 ] || [[ "$out" != *"$MARKER"* ]]; then
    echo "FAIL  $alias"
    rc=1
  else
    echo "PASS  $alias"
  fi
done < <(golunch ls --json | python3 -c 'import json,sys
want = set(sys.argv[1:])
for line in sys.stdin:
    d = json.loads(line)
    if not want or d["alias"] in want:
        print("\t".join([d["alias"], d["agent"], d.get("version", "?"), d["root"]]))
' "$@")

[ $n -eq 0 ] && { echo "no golunch instances discovered"; exit 1; }
exit $rc
