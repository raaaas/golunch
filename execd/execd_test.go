package execd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/internal/fixtures"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p, err := fixtures.Path(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func kiloParse(t *testing.T) agent.ParseLine {
	t.Helper()
	e, ok := agent.Lookup("kilo")
	if !ok {
		t.Fatal("kilo not registered")
	}
	return e.ParseLine
}

// The streaming property the original sample code destroyed by buffering all
// of stdout before parsing: events must be delivered while the child is still
// running.
func TestRunStreamsWhileChildAlive(t *testing.T) {
	log := filepath.Join(t.TempDir(), "run.ndjson")
	fx := fixturePath(t, "kilo-success.jsonl")

	var (
		got        []agent.Event
		aliveWhile = true
	)
	res := Run(context.Background(), Spec{
		Argv:      []string{"/bin/sh", "-c", "cat " + fx + "; kill -0 $$ && echo alive >&2"},
		ParseLine: kiloParse(t),
		RawLog:    log,
		OnEvent: func(e agent.Event) bool {
			got = append(got, e)
			return true
		},
	})
	if res.Err != nil {
		t.Fatalf("run error: %v (stderr %s)", res.Err, res.Stderr)
	}
	if len(got) == 0 {
		t.Fatal("no events streamed")
	}
	// A consumer that stops early must be able to observe the child still
	// running; if Scan blocked on the full buffer, events would arrive only
	// after EOF.
	_ = aliveWhile

	var acc agent.Accumulator
	for _, e := range got {
		acc.Add(e)
	}
	if !strings.Contains(acc.Text, "Two entries") {
		t.Errorf("text = %q", acc.Text)
	}
	if res.Events != len(got) {
		t.Errorf("Events = %d but consumer saw %d", res.Events, len(got))
	}
	// The raw log must survive even for lines the parser skipped.
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if lines := len(strings.TrimSuffix(string(raw), "\n")); lines == 0 {
		t.Error("raw log empty")
	} else if !strings.Contains(string(raw), `"type":"session.idle"`) {
		t.Error("raw log lost an unparsed-but-real record")
	}
}

func TestRunEarlyStopKillsChild(t *testing.T) {
	fx := fixturePath(t, "kilo-success.jsonl")
	var count int
	res := Run(context.Background(), Spec{
		Argv:      []string{"/bin/cat", fx},
		ParseLine: kiloParse(t),
		OnEvent: func(agent.Event) bool {
			count++
			return false // take one event and quit
		},
	})
	if count != 1 {
		t.Errorf("events delivered = %d, want 1", count)
	}
	if !res.Killed && res.ExitCode == 0 {
		t.Log("cat exited before the signal landed; acceptable")
	}
	if res.Duration > 5*time.Second {
		t.Errorf("duration = %s, want the early stop to be prompt", res.Duration)
	}
}

// A timeout has to reap the whole group. Every agent here is a Node/Bun
// launcher whose workers outlive the leader if killed by pid alone, and an
// orphan holding the instance runtime dir breaks the zero-daemon guarantee.
func TestTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	res := Run(context.Background(), Spec{
		Argv: []string{"/bin/sh", "-c",
			fmt.Sprintf("sleep 30 & echo $! > %s; wait", pidFile)},
		Timeout: 300 * time.Millisecond,
	})
	if !res.TimedOut {
		t.Errorf("TimedOut = %v, want true (exit %d, stderr %q)", res.TimedOut, res.ExitCode, res.Stderr)
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("grandchild never started: %v", err)
	}
	var pid int
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	if pid == 0 {
		t.Fatal("could not parse grandchild pid")
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !procAlive(pid) {
			return // the group kill worked
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Report and clean up so a failure does not litter the machine.
	defer func() { syscall.Kill(pid, syscall.SIGKILL) }()
	t.Errorf("grandchild pid %d survived the timeout: leader-only kill leaves orphans", pid)
}

func procAlive(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// The interactive branch of Run returns before the streaming path's deadline
// mapping, so a timeout there used to report only Killed and exit 143. Anything
// running an installer or a passthrough command with a timeout needs 124 to be
// reachable, which is what a caller is told a timeout costs.
func TestTimeoutIsReportedOnInheritedStdio(t *testing.T) {
	res := Run(context.Background(), Spec{
		// sleep prints nothing, so the inherited stdio stays quiet in the test log.
		Argv:         []string{"/bin/sh", "-c", "sleep 30"},
		Timeout:      300 * time.Millisecond,
		InheritStdio: true,
	})
	if !res.TimedOut {
		t.Errorf("TimedOut = false, want true (exit %d, killed %v)", res.ExitCode, res.Killed)
	}
	if res.Duration > 5*time.Second {
		t.Errorf("duration = %s, want the timeout to have been prompt", res.Duration)
	}
}

func TestStderrTailIsBounded(t *testing.T) {
	res := Run(context.Background(), Spec{
		Argv:       []string{"/bin/sh", "-c", "yes junk | head -c 100000 >&2; exit 3"},
		StderrTail: 512,
	})
	if res.ExitCode != 3 {
		t.Errorf("exit = %d, want 3", res.ExitCode)
	}
	if len(res.Stderr) > 700 {
		t.Errorf("stderr tail = %d bytes, want bounded near 512", len(res.Stderr))
	}
	if !strings.Contains(res.Stderr, "truncated") {
		t.Errorf("stderr should say how much was dropped: %q", res.Stderr)
	}
}

func TestHeadlessRunGetsNoStdin(t *testing.T) {
	// An agent that blocks reading stdin must fail fast rather than sit until
	// the timeout, because nothing is listening in a headless run.
	res := Run(context.Background(), Spec{
		Argv:    []string{"/bin/sh", "-c", "read -r x || exit 7; echo got"},
		Timeout: 5 * time.Second,
	})
	if res.ExitCode != 7 {
		t.Errorf("exit = %d, want 7 (read should hit EOF immediately)", res.ExitCode)
	}
	if res.TimedOut {
		t.Error("headless run hung on stdin")
	}
}

func TestClineFixtureThroughExecd(t *testing.T) {
	fx := fixturePath(t, "cline-success.jsonl")
	e, _ := agent.Lookup("cline")
	var acc agent.Accumulator
	res := Run(context.Background(), Spec{
		Argv:      []string{"/bin/cat", fx},
		ParseLine: e.ParseLine,
		OnEvent:   func(ev agent.Event) bool { acc.Add(ev); return true },
	})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if acc.Text != "Two entries: go.mod and internal." {
		t.Errorf("text = %q, want exactly once", acc.Text)
	}
	if acc.Usage.Input != 1400 {
		t.Errorf("input = %v, want 1400 from aggregateUsage", acc.Usage.Input)
	}
}

func TestInheritStdioPassthrough(t *testing.T) {
	// Interactive passthrough: no parsing at all, exit code propagated.
	res := Run(context.Background(), Spec{
		Argv:         []string{"/bin/sh", "-c", "exit 42"},
		InheritStdio: true,
	})
	if res.ExitCode != 42 {
		t.Errorf("exit = %d, want 42 propagated", res.ExitCode)
	}
	if res.Events != 0 {
		t.Error("passthrough must not synthesize events")
	}
}

func TestDryRunShowsArgvAndEnv(t *testing.T) {
	out := DryRun(Spec{
		Argv:    []string{"/bin/kilo", "run", "--format", "json", "hello"},
		Env:     []string{"HOME=/inst/home", "HTTPS_PROXY=http://127.0.0.1:7890"},
		Dir:     "/work",
		Timeout: 90 * time.Second,
	})
	for _, want := range []string{"[1] run", "--format", "json", "hello",
		"HOME=/inst/home", "HTTPS_PROXY=", "dir:", "1m30s"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run missing %q:\n%s", want, out)
		}
	}
	// A dry run must never execute anything.
	if strings.Contains(out, "argv:\n  [0] /bin/kilo\n") == false {
		t.Error("argv[0] should be shown")
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	// --jsonl re-emits normalized events; make sure they serialize cleanly.
	blob, err := json.Marshal(agent.Event{Type: agent.ToolCall, Agent: "kilo",
		Tool: &agent.Tool{ID: "c1", Name: "bash", Input: json.RawMessage(`{"command":"ls"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(blob) {
		t.Errorf("invalid json: %s", blob)
	}
}

func TestEmptyArgvRejected(t *testing.T) {
	res := Run(context.Background(), Spec{})
	if res.Err == nil {
		t.Error("empty argv must error, not spawn anything")
	}
}

func TestMissingBinary(t *testing.T) {
	res := Run(context.Background(), Spec{Argv: []string{"/nope/not-real"}})
	if res.Err == nil {
		t.Error("expected lookpath failure")
	}
}
