package golunch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raaaas/golunch"
	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/internal/fixtures"
)

// These tests are in package golunch_test on purpose: they can only reach the
// exported API, so they fail if the library surface is too small to drive an
// instance without exec'ing the CLI. That is the whole point of the package.

func runner(t *testing.T) (*golunch.Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		Root: root,
		Paths: config.Paths{
			InstancesDir: filepath.Join(root, "instances"),
			BinDir:       filepath.Join(root, "bin"),
		},
		Defaults: config.Defaults{Shell: "/bin/sh"},
	}
	var out, errb bytes.Buffer
	return golunch.NewRunner(cfg, strings.NewReader(""), &out, &errb, instance.HostFromOs()), &out, &errb
}

// replay writes a driver that ignores its argv and cats a recorded transcript,
// so a run exercises streaming, parsing, locking and log writing without
// touching a model or spending a credit.
func replay(t *testing.T, fixture string) string {
	t.Helper()
	path, err := fixtures.Path(fixture)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "driver")
	body := "#!/bin/sh\ncat " + instance.ShellQuote(path) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o644|0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func newRunnerInstance(t *testing.T, r *golunch.Runner, alias, driver string) *instance.Instance {
	t.Helper()
	inst, err := instance.New(r.InstancesDir(), alias)
	if err != nil {
		t.Fatal(err)
	}
	inst.EnsureStorage()
	meta := instance.Metadata{}
	meta.Instance.Alias = alias
	meta.Instance.Agent = "cline"
	meta.Instance.Shell = "/bin/sh"
	meta.Launch.Command = []string{driver}
	// The registry would otherwise resolve a real installed cline; this is the
	// same escape hatch the CLI tests use.
	meta.Launch.DriverBinary = driver
	meta.Touch()
	if err := inst.SaveMetadata(&meta); err != nil {
		t.Fatal(err)
	}
	return inst
}

func TestLibraryPromptStreamsAndRecords(t *testing.T) {
	r, out, errb := runner(t)
	newRunnerInstance(t, r, "lib1", replay(t, "cline-success.jsonl"))

	var seen []string
	res, err := r.Prompt(context.Background(), golunch.PromptOptions{
		Alias:  "lib1",
		Prompt: "list the files here",
		JSONL:  true,
		OnEvent: func(ev agent.Event) bool {
			seen = append(seen, string(ev.Type))
			return true
		},
	})
	if err != nil {
		t.Fatalf("Prompt: %v\nstderr: %s", err, errb.String())
	}

	if len(seen) < 4 {
		t.Errorf("OnEvent saw %d events, expected a stream: %v", len(seen), seen)
	}
	for _, want := range []string{"tool_call", "usage"} {
		if !strings.Contains(strings.Join(seen, ","), want) {
			t.Errorf("stream missing %q: %v", want, seen)
		}
	}

	// The JSONL rendering and the hook must agree, otherwise a library caller
	// that reads events off stdout and one that reads them off OnEvent are not
	// watching the same run.
	var lines int
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stdout is not NDJSON: %v (%q)", err, line)
		}
		lines++
	}
	if lines != len(seen) {
		t.Errorf("OnEvent got %d events, stdout carried %d", len(seen), lines)
	}

	if res.Accum.Text == "" {
		t.Error("accumulated answer is empty")
	}
	if res.SessionID == "" {
		t.Error("no session id recovered from the transcript")
	}

	// The transcript and the metadata record are how a headless run is
	// debuggable after it returns.
	if _, err := os.Stat(res.LogPath); err != nil {
		t.Fatalf("run log not written: %v", err)
	}
	_, meta, err := r.Open("lib1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRun == nil || meta.LastRun.Log != res.LogPath {
		t.Errorf("metadata did not record this run: %+v (log %s)", meta.LastRun, res.LogPath)
	}
	if meta.LastRun.ExitCode != 0 {
		t.Errorf("recorded exit code %d, want 0", meta.LastRun.ExitCode)
	}
}

func TestLibraryPromptOnEventFalseStopsRun(t *testing.T) {
	r, _, errb := runner(t)
	newRunnerInstance(t, r, "stop1", replay(t, "cline-success.jsonl"))

	var count int
	_, err := r.Prompt(context.Background(), golunch.PromptOptions{
		Alias:  "stop1",
		Prompt: "hi",
		OnEvent: func(agent.Event) bool {
			count++
			return false
		},
	})
	if count != 1 {
		t.Errorf("hook fired %d times after returning false, want 1", count)
	}
	// Stopping the stream is a caller's choice, not a failure of the run.
	if golunch.ErrKind(err) == golunch.KindTimeout {
		t.Fatalf("early stop reported as timeout: %v\n%s", err, errb.String())
	}
}

func TestLibraryPromptTimeout(t *testing.T) {
	r, _, _ := runner(t)
	sleeper := filepath.Join(t.TempDir(), "sleeper")
	if err := os.WriteFile(sleeper, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newRunnerInstance(t, r, "slow1", sleeper)

	start := time.Now()
	_, err := r.Prompt(context.Background(), golunch.PromptOptions{
		Alias:   "slow1",
		Prompt:  "never answers",
		Timeout: 200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if code := golunch.ErrCode(err); code != golunch.ExitTimeout {
		t.Errorf("exit code %d, want %d", code, golunch.ExitTimeout)
	}
	if k := golunch.ErrKind(err); k != golunch.KindTimeout {
		t.Errorf("kind %v, want timeout", k)
	}
	// The point of the timeout is that it actually reaps the process group; a
	// check that only reads the error would pass on a run that hung forever.
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("timeout took %s; the child was not killed", d)
	}
}

func TestLibraryBusyAndMissing(t *testing.T) {
	r, _, _ := runner(t)
	inst := newRunnerInstance(t, r, "busy1", replay(t, "cline-success.jsonl"))

	lock, err := inst.Acquire(instance.Exclusive)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	_, err = r.Prompt(context.Background(), golunch.PromptOptions{Alias: "busy1", Prompt: "hi"})
	if golunch.ErrCode(err) != golunch.ExitBusy {
		t.Fatalf("busy run returned %v (code %d), want %d", err, golunch.ErrCode(err), golunch.ExitBusy)
	}
	if !golunch.IsBusy(err) {
		t.Errorf("IsBusy false for %v", err)
	}
	if k := golunch.ErrKind(err); k != golunch.KindBusy {
		t.Errorf("kind %v, want busy", k)
	}

	if _, err := r.Prompt(context.Background(), golunch.PromptOptions{Alias: "absent", Prompt: "hi"}); golunch.ErrCode(err) != golunch.ExitNotFound {
		t.Errorf("missing instance code %d, want %d", golunch.ErrCode(err), golunch.ExitNotFound)
	} else if k := golunch.ErrKind(err); k != golunch.KindNotFound {
		t.Errorf("kind %v, want not found", k)
	}
}

func TestErrCodePassesThroughChildExit(t *testing.T) {
	// A passthrough child's own status must survive: `golunch run k -- make` is
	// only usable in a script if $? is make's.
	if got := golunch.ErrCode(golunch.CommandError{Code: 2, Err: os.ErrInvalid}); got != 2 {
		t.Errorf("child code not preserved: %d", got)
	}
	if got := golunch.ErrCode(nil); got != golunch.ExitOK {
		t.Errorf("nil error mapped to %d, want 0", got)
	}
	if got := golunch.ErrCode(os.ErrNotExist); got != golunch.ExitError {
		t.Errorf("foreign error mapped to %d, want %d", got, golunch.ExitError)
	}
	if got := golunch.ErrKind(os.ErrNotExist); got != golunch.KindUnknown {
		t.Errorf("foreign error classified as %v, want unknown", got)
	}
}

func TestDryRunStartsNothing(t *testing.T) {
	r, out, _ := runner(t)
	inst := newRunnerInstance(t, r, "dry1", replay(t, "cline-success.jsonl"))

	res, err := r.Prompt(context.Background(), golunch.PromptOptions{
		Alias: "dry1", Prompt: "hi", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.LogPath != "" {
		t.Errorf("--dry-run wrote a run log: %s", res.LogPath)
	}
	if _, err := os.Stat(inst.LogPath("anything")); err == nil {
		t.Error("dry run left a log directory entry behind")
	}
	for _, want := range []string{"argv:", "HOME=", "proxy:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
}
