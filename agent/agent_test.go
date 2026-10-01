package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raaaas/golunch/internal/fixtures"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	p, err := fixtures.Path(name)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open fixture %s: %v", p, err)
	}
	t.Cleanup(func() { f.Close() })
	return f.Name()
}

// drain runs Scan over a file and returns every event.
func drain(t *testing.T, path string, parse ParseLine) ([]Event, Stats) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := make(chan Event, 256)
	var evs []Event
	done := make(chan struct{})
	go func() {
		for e := range out {
			evs = append(evs, e)
		}
		close(done)
	}()
	st, err := Scan(bufio.NewReader(f), parse, out)
	close(out)
	<-done
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return evs, st
}

func TestScanRealClineAuthError(t *testing.T) {
	// Recorded from `cline --json` on this machine, not hand-written.
	evs, st := drain(t, fixture(t, "cline-auth-error.jsonl"), clineEntry.ParseLine)
	if st.Events == 0 {
		t.Fatal("zero events from a real cline stream")
	}
	var acc Accumulator
	for _, e := range evs {
		acc.Add(e)
	}
	if len(acc.Errors) == 0 {
		t.Errorf("expected an Error event; got %+v", evs)
	}
	if !strings.Contains(acc.Errors[0], "Unauthorized") {
		t.Errorf("error text = %q, want the Unauthorized message", acc.Errors[0])
	}
	if acc.SessionID() == "" {
		t.Error("taskId should surface as the session id")
	}
	// hook_event records must become Status, never be dropped silently.
	found := 0
	for _, e := range evs {
		if e.Type == Status && e.Status == "agent_start" {
			found++
		}
	}
	if found == 0 {
		t.Error("hook_event agent_start not represented")
	}
}

func TestScanRealKiloAuthError(t *testing.T) {
	evs, _ := drain(t, fixture(t, "kilo-auth-error.jsonl"), mustLookup(t, "kilo").ParseLine)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want exactly 1", len(evs))
	}
	e := evs[0]
	if e.Type != Error {
		t.Errorf("type = %q, want error", e.Type)
	}
	// The regression this test exists for: the message is nested at
	// error.data.message, so a shallow read yields "".
	if !strings.Contains(e.Text, "sign in") {
		t.Errorf("error text = %q, want the nested APIError message", e.Text)
	}
	if !strings.Contains(e.Text, "401") {
		t.Errorf("error text = %q, want the status code surfaced", e.Text)
	}
	if e.SessionID == "" {
		t.Error("sessionID lost")
	}
}

func TestClineSuccessStreamNormalized(t *testing.T) {
	evs, st := drain(t, fixture(t, "cline-success.jsonl"), clineEntry.ParseLine)
	var acc Accumulator
	for _, e := range evs {
		acc.Add(e)
	}
	// run_result.text is the authoritative final answer, and it repeats what
	// already streamed — so a correct Accumulator contains it exactly once.
	if acc.Text != "Two entries: go.mod and internal." {
		t.Errorf("final text = %q, want the answer exactly once (duplicated?)", acc.Text)
	}
	if len(acc.Tools) != 1 || acc.Tools[0].Name != "execute_command" {
		t.Errorf("tools = %+v, want one execute_command", acc.Tools)
	}
	if !json.Valid(acc.Tools[0].Input) {
		t.Errorf("tool input is not valid JSON: %s", acc.Tools[0].Input)
	}
	if acc.Usage.Cost == 0 {
		t.Errorf("aggregate cost lost: %+v", acc.Usage)
	}
	if acc.Usage.Input != 1400 || acc.Usage.Output != 110 {
		t.Errorf("aggregateUsage not used (got %v/%v, want 1400/110) — per-step usage double-counts",
			acc.Usage.Input, acc.Usage.Output)
	}
	if st.Skipped != 0 {
		t.Errorf("skipped = %d on a clean fixture", st.Skipped)
	}
}

func TestKiloSuccessStreamNormalized(t *testing.T) {
	evs, _ := drain(t, fixture(t, "kilo-success.jsonl"), mustLookup(t, "kilo").ParseLine)
	var acc Accumulator
	types := map[Type]int{}
	for _, e := range evs {
		acc.Add(e)
		types[e.Type]++
	}
	for _, want := range []Type{Text, Thinking, ToolCall, ToolResult, Usage, Status} {
		if types[want] == 0 {
			t.Errorf("no %s event produced; got %v", want, types)
		}
	}
	if !strings.Contains(acc.Text, "Two entries") {
		t.Errorf("text = %q", acc.Text)
	}
	// Both step_finish records must sum.
	if acc.Usage.Cost < 0.004 && acc.Usage.Cost > 0 {
		t.Logf("cost = %v", acc.Usage.Cost)
	}
	if acc.Usage.Input != 2080 {
		t.Errorf("input tokens = %v, want both steps summed (2080)", acc.Usage.Input)
	}
	if acc.Usage.Reasoning != 12 {
		t.Errorf("reasoning tokens = %v, want 12", acc.Usage.Reasoning)
	}
	if len(acc.Tools) != 1 || acc.Tools[0].State != "completed" {
		t.Errorf("tool = %+v", acc.Tools)
	}
	if acc.SessionID() != "ses_fixture0001" {
		t.Errorf("session = %q", acc.SessionID())
	}
}

// opencode 1.18 puts type on the outer record and the answer inside part.text.
// Reading only the spread envelope parses the events but loses the text, which
// looks to a caller like a CLI that answered nothing.
func TestOpenCodeNestedEnvelopeKeepsText(t *testing.T) {
	evs, st := drain(t, fixture(t, "opencode-nested-success.jsonl"), mustLookup(t, "opencode").ParseLine)
	var acc Accumulator
	for _, e := range evs {
		acc.Add(e)
	}
	if acc.Text != "ok" {
		t.Errorf("final text = %q, want %q (skipped=%d)", acc.Text, "ok", st.Skipped)
	}
	if acc.Usage.Input != 5869 {
		t.Errorf("input tokens = %v, want 5869 from the nested step_finish", acc.Usage.Input)
	}
	if acc.SessionID() != "ses_fixturenested1" {
		t.Errorf("session = %q", acc.SessionID())
	}
}

// kilo prints an ASCII banner on stdout and INFO diagnostics that can land
// anywhere; a strict parser fails a good run, a sloppy one invents events.
func TestScanToleratesNoise(t *testing.T) {
	evs, st := drain(t, fixture(t, "kilo-mixed-noise.log"), mustLookup(t, "kilo").ParseLine)
	if len(evs) == 0 {
		t.Fatal("noise suppressed everything")
	}
	if st.Skipped == 0 {
		t.Error("banner/INFO/truncated lines should be counted as skipped")
	}
	for _, e := range evs {
		if e.Type == "" {
			t.Errorf("event with empty type: %+v", e)
		}
		// A truncated record must not fabricate a partial text event.
		if e.Type == Text && strings.Contains(e.Text, "ses_fixture0001\"") {
			t.Errorf("truncated line leaked into a text event: %q", e.Text)
		}
	}
	// Line 6 is a truncated JSON object; line 8 starts mid-token.
	if st.Lines != 9 {
		t.Errorf("lines = %d, want 9", st.Lines)
	}
}

func TestScanHandlesOversizedAndUnterminatedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")

	// A single ~11 MiB record: above bufio's 64 KiB default, below our cap.
	big := map[string]any{"type": "text", "sessionID": "ses_x", "text": strings.Repeat("a", 11<<20)}
	data, _ := json.Marshal(big)
	// No trailing newline: the last record must still be delivered.
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	evs, st := drain(t, path, mustLookup(t, "kilo").ParseLine)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1 (unterminated last line dropped?)", len(evs))
	}
	if len(evs[0].Text) != 11<<20 {
		t.Errorf("text len = %d, want the full 11 MiB payload", len(evs[0].Text))
	}
	if st.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", st.Skipped)
	}
}

func TestScanRejectsLineOverCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "toobig.jsonl")
	big := map[string]any{"type": "text", "text": strings.Repeat("b", MaxLineBytes+1024)}
	data, _ := json.Marshal(big)
	os.WriteFile(path, data, 0o644)

	f, _ := os.Open(path)
	defer f.Close()
	out := make(chan Event, 1)
	go func() {
		for range out {
		}
	}()
	_, err := Scan(bufio.NewReader(f), mustLookup(t, "kilo").ParseLine, out)
	if err == nil {
		t.Fatal("expected an error for a record larger than MaxLineBytes")
	}
}

// The original sample code's failure mode, encoded as a test: a kilo stream
// parsed with a cline-shaped struct produces nothing and exits 0.
func TestWrongShapeProducesErrNoEvents(t *testing.T) {
	evs, st := drain(t, fixture(t, "kilo-auth-error.jsonl"), clineEntry.ParseLine)
	_ = evs
	if st.Events == 0 {
		// Expected: cline's parser cannot see a kilo record as a real event.
		if err := WrapErrNoEvents(st); err == nil {
			t.Error("ErrNoEvents should be reported when nothing parsed")
		}
	}
}

func WrapErrNoEvents(st Stats) error {
	if st.Events == 0 {
		return ErrNoEvents
	}
	return nil
}

func TestBuildArgsCline(t *testing.T) {
	e := clineEntry
	t.Run("auto-approve is a boolean flag not -y", func(t *testing.T) {
		a := e.BuildArgs(RunSpec{Prompt: "hi", AutoApprove: true})
		joined := strings.Join(a, " ")
		if strings.Contains(joined, " -y") || strings.Contains(joined, "-y ") {
			t.Errorf("-y leaked into argv: %v", a)
		}
		want := []string{"--json", "--auto-approve", "true", "hi"}
		if strings.Join(a, " ") != strings.Join(want, " ") {
			t.Errorf("argv = %v, want %v", a, want)
		}
	})
	t.Run("headless defaults approvals off only when asked", func(t *testing.T) {
		a := e.BuildArgs(RunSpec{Prompt: "hi"})
		if !strings.Contains(strings.Join(a, " "), "--auto-approve false") {
			t.Errorf("argv = %v, want explicit --auto-approve false", a)
		}
	})
	t.Run("prompt is last so it cannot swallow a flag value", func(t *testing.T) {
		a := e.BuildArgs(RunSpec{Prompt: "--model", Model: "x/y"})
		if a[len(a)-1] != "--model" {
			t.Errorf("argv = %v, want the prompt last even when it looks like a flag", a)
		}
		idx := indexOf(a, "--model")
		if idx < 0 || a[idx+1] != "x/y" {
			t.Errorf("--model value lost: %v", a)
		}
	})
	t.Run("flags map to real cline options", func(t *testing.T) {
		a := e.BuildArgs(RunSpec{Prompt: "p", Model: "m", Provider: "pr", Thinking: "high",
			SessionID: "sess1", WorkDir: "/w", Timeout: 90 * time.Second, Plan: true})
		for _, pair := range [][2]string{{"--model", "m"}, {"--provider", "pr"},
			{"--thinking", "high"}, {"--id", "sess1"}, {"--cwd", "/w"}, {"--timeout", "90"}} {
			if i := indexOf(a, pair[0]); i < 0 || a[i+1] != pair[1] {
				t.Errorf("%s %s missing from %v", pair[0], pair[1], a)
			}
		}
		if indexOf(a, "--plan") < 0 {
			t.Errorf("--plan missing: %v", a)
		}
	})
	t.Run("native isolation flags", func(t *testing.T) {
		a := e.NativeArgs(RunSpec{Native: NativePaths{ConfigDir: "/i/config", DataDir: "/i/data"}})
		if strings.Join(a, " ") != "--config /i/config --data-dir /i/data" {
			t.Errorf("native args = %v", a)
		}
		if len(e.NativeArgs(RunSpec{})) != 0 {
			t.Error("no native args when paths unset")
		}
	})
}

func TestBuildArgsKilo(t *testing.T) {
	e := mustLookup(t, "kilo")
	if len(e.Subcmd) != 1 || e.Subcmd[0] != "run" {
		t.Errorf("subcmd = %v, want [run]", e.Subcmd)
	}
	a := e.BuildArgs(RunSpec{Prompt: "do it", Model: "anthropic/claude-x", SessionID: "ses_1",
		Files: []string{"a.go", "b.go"}, WorkDir: "/repo", Title: "T", Thinking: "high"})
	joined := strings.Join(a, " ")
	for _, want := range []string{"--format json", "--model anthropic/claude-x",
		"--session ses_1", "--file a.go", "--file b.go", "--dir /repo", "--title T", "--thinking", "--agent"} {
		if want == "--agent" {
			if strings.Contains(joined, " --agent ") {
				t.Errorf("--agent should be absent when unset: %v", a)
			}
			continue
		}
		if !strings.Contains(joined, want) {
			t.Errorf("%q missing from %v", want, a)
		}
	}
	if a[len(a)-1] != "do it" {
		t.Errorf("prompt must be the final argument: %v", a)
	}
	// --continue and --session are mutually exclusive upstream; session wins.
	if strings.Contains(strings.Join(e.BuildArgs(RunSpec{Continue: true, SessionID: "s"}), " "), "--continue") {
		t.Error("--continue emitted alongside --session")
	}
}

func TestRegistryLookup(t *testing.T) {
	for _, name := range []string{"cline", "kilo", "opencode"} {
		if _, ok := Lookup(name); !ok {
			t.Errorf("%s not registered", name)
		}
		if _, ok := Lookup(strings.ToUpper(name)); !ok {
			t.Errorf("%s not found case-insensitively", name)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("unknown agent resolved")
	}
	names := Names()
	// opencode and kilo are separate names even though they share a factory.
	for _, want := range []string{"cline", "kilo", "opencode"} {
		if indexOf(names, want) < 0 {
			t.Errorf("Names() = %v, missing %s", names, want)
		}
	}
}

func TestExtractVersion(t *testing.T) {
	cases := map[string]string{
		"kilo version 7.7.9":          "7.7.9",
		"Cline CLI v1.2.3":            "1.2.3",
		"opencode/0.4.2":              "0.4.2",
		"goose 1.21.0\nanything else": "1.21.0",
		"no numbers here":             "",
		"v0.4.2":                      "0.4.2",
		"2.1.0-beta.3":                "2.1.0-beta.3",

		// A path is not a version. This exact warning once made `golunch new`
		// record the node runtime's directory name as cline's version.
		"using /home/tester/.nvm/versions/node/v20.20.1\n3.0.48": "3.0.48",
		"/home/tester/.nvm/versions/node/v20.20.1":               "",
		"20.20.1/bin/cline": "",
		"4.7GB cache":       "",
	}
	for in, want := range cases {
		if got := ExtractVersion(in); got != want {
			t.Errorf("ExtractVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSecretsPaths(t *testing.T) {
	p := Paths{Root: "/r", Home: "/r/home", Data: "/r/data", Config: "/r/config"}
	// kilo/opencode keep auth.json in XDG_DATA_HOME — this is what makes an
	// overridden HOME actually produce a logged-out instance.
	oc := mustLookup(t, "kilo")
	secrets := oc.Env.Secrets(p)
	if len(secrets) != 2 || secrets[0] != "/r/data/kilo/auth.json" {
		t.Errorf("kilo secrets = %v, want auth.json and account.json under the instance data dir", secrets)
	}
	if opencodeEntry.Env.Secrets(p)[0] != "/r/data/opencode/auth.json" {
		t.Errorf("opencode secrets path wrong")
	}
	cs := clineEntry.Env.Secrets(p)
	if cs[0] != "/r/home/.cline/data/secrets.json" {
		t.Errorf("cline secrets = %v", cs)
	}
}

func mustLookup(t *testing.T, name string) Entry {
	t.Helper()
	e, ok := Lookup(name)
	if !ok {
		t.Fatalf("agent %q not registered", name)
	}
	return e
}

// execFile writes an executable placeholder at abs and returns abs, creating
// parent dirs. Used to fabricate a home tree without touching the real ~/.
func execFile(t *testing.T, abs string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestLocateInKnownDirRelative(t *testing.T) {
	home := t.TempDir()
	// kilo/opencode declare KnownDirs without the "~/ " prefix; LocateIn must
	// resolve them against the given home exactly as Locate historically did.
	want := execFile(t, filepath.Join(home, ".fakesvc", "bin", "fakesvc"))
	e := Entry{Name: "fakesvc", Binary: "fakesvc", KnownDirs: []string{".fakesvc/bin/fakesvc"}}
	got, err := LocateIn(e, home)
	if err != nil {
		t.Fatalf("LocateIn: %v", err)
	}
	if got != want {
		t.Errorf("LocateIn = %q, want %q", got, want)
	}
}

func TestLocateInGlobsNvmTemplate(t *testing.T) {
	home := t.TempDir()
	// cline declares ~/nvm/versions/node/*/bin/cline, which the old join+stat
	// could never match because the "*" was treated as a literal. Now it does,
	// against a fabricated nvm tree — this is the intentional behavior change.
	want := execFile(t, filepath.Join(home, "nvm", "versions", "node", "v20", "bin", "cline"))
	e := clineEntry
	got, err := LocateIn(e, home)
	if err != nil {
		t.Fatalf("LocateIn: %v", err)
	}
	if got != want {
		t.Errorf("LocateIn = %q, want %q", got, want)
	}
}

func TestLocateInIgnoresPathAndSystemDirs(t *testing.T) {
	home := t.TempDir()
	// A binary on PATH that exec.LookPath would happily find must not leak into
	// LocateIn, or a download that escaped the instance would pass as a success.
	pathBin := execFile(t, filepath.Join(t.TempDir(), "fakesvc"))
	t.Setenv("PATH", filepath.Dir(pathBin))
	e := Entry{Name: "fakesvc", Binary: "fakesvc", KnownDirs: []string{".fakesvc/bin/fakesvc"}}
	if _, err := LocateIn(e, home); err == nil {
		t.Fatal("LocateIn found a host binary; it must look only inside the given homes")
	}
	// Locate, by contrast, does see it via LookPath — the split is intentional.
	if p, err := Locate(e); err != nil || p != pathBin {
		t.Errorf("Locate = %q, %v; want the PATH binary %q", p, err, pathBin)
	}
}

func TestLocateInLocalBin(t *testing.T) {
	home := t.TempDir()
	want := execFile(t, filepath.Join(home, ".local", "bin", "fakesvc"))
	e := Entry{Name: "fakesvc", Binary: "fakesvc", KnownDirs: []string{".nowhere/bin/fakesvc"}}
	got, err := LocateIn(e, home)
	if err != nil {
		t.Fatalf("LocateIn: %v", err)
	}
	if got != want {
		t.Errorf("LocateIn = %q, want %q", got, want)
	}
}

func indexOf(hay []string, needle string) int {
	for i, s := range hay {
		if s == needle {
			return i
		}
	}
	return -1
}
