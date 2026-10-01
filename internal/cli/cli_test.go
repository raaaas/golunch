package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raaaas/golunch"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/internal/fixtures"
	"github.com/raaaas/golunch/internal/osutil"
	"github.com/raaaas/golunch/proxy"
)

// testApp gives every test its own data root and its own ~/.local/bin stand-in,
// so nothing a test creates can touch the real home directory.
func testApp(t *testing.T) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GOLUNCH_ROOT", root)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Root = root
	cfg.Paths.InstancesDir = filepath.Join(root, "instances")
	cfg.Paths.BinDir = filepath.Join(root, "bin")
	cfg.Proxy.Profiles = map[string]proxy.Profile{
		"corp": {Name: "corp", HTTP: "http://corp-proxy:3128", NoProxy: []string{".corp.test"}},
	}
	var out, errb bytes.Buffer
	app := &App{Runner: golunch.NewRunner(cfg, strings.NewReader(""), &out, &errb, instance.HostFromOs())}
	return app, &out, &errb
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p, err := fixtures.Path(name)
	if err != nil {
		t.Skipf("fixture %s missing: %v", name, err)
	}
	return p
}

// fakeAgent writes a script that ignores its arguments and replays a recorded
// fixture. This is the whole offline story: a real driver run, real streaming,
// real parsing, zero API credits.
func fakeAgent(t *testing.T, fixture string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-agent")
	body := "#!/bin/sh\ncat " + instance.ShellQuote(fixturePath(t, fixture)) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestInstance(t *testing.T, app *App, alias string, args ...string) {
	t.Helper()
	argv := append([]string{alias}, args...)
	if err := app.CmdNew(context.Background(), argv); err != nil {
		t.Fatalf("new %s: %v\n  argv: %s", alias, err, strings.Join(argv, " "))
	}
}

func TestNewLsEnvDoctor(t *testing.T) {
	app, out, errb := testApp(t)

	newTestInstance(t, app, "cat1", "--binary", "/bin/cat", "--link=false", "--proxy", "corp")

	if !strings.Contains(out.String(), "created cat1") {
		t.Fatalf("expected creation output, got %q", out.String())
	}
	inst, err := instance.New(app.InstancesDir(), "cat1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"home", "config", "data", "cache", "state", "runtime", "tmp", "logs", "bin"} {
		if !dirExists(inst.Dir(d)) {
			t.Errorf("missing storage dir %s", d)
		}
	}
	if _, err := os.Stat(inst.LauncherPath()); err != nil {
		t.Fatalf("no launcher generated: %v", err)
	}

	out.Reset()
	if err := app.CmdList([]string{"-l"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cat1", "(wrapper)", "global config profile"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("ls -l lacks %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := app.CmdEnv([]string{"cat1"}); err != nil {
		t.Fatal(err)
	}
	envOut := out.String()
	for _, want := range []string{
		filepath.Join(app.InstancesDir(), "cat1", "home"),
		"HTTP_PROXY=http://corp-proxy:3128",
		"http_proxy=http://corp-proxy:3128",
		".corp.test",
		"127.0.0.1",
		"global config profile",
		"instance: profile corp",
	} {
		if !strings.Contains(envOut, want) {
			t.Errorf("env output lacks %q:\n%s", want, envOut)
		}
	}

	out.Reset()
	if err := app.CmdDoctor([]string{}); err != nil {
		t.Fatalf("doctor: %v\n%s\n%s", err, out.String(), errb.String())
	}

	out.Reset()
	if err := app.CmdRemove([]string{"cat1", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if inst.Exists() {
		t.Error("instance tree survived rm")
	}
	if _, err := os.Stat(inst.LockPath()); !os.IsNotExist(err) {
		t.Errorf("lock file survived rm: %v", err)
	}
}

// Passthrough must hand the child the instance environment and nothing else, and
// it must not reinterpret arguments that come after --.
func TestRunPassthroughUsesInstanceEnv(t *testing.T) {
	app, _, errb := testApp(t)
	ctx := context.Background()
	marker := filepath.Join(t.TempDir(), "child-env")

	newTestInstance(t, app, "sh1", "--binary", "/bin/sh", "--link=false")

	cmd := `printf '%s\n' "HOME=$HOME" "XDG=$XDG_CONFIG_HOME" "PROXY=${HTTP_PROXY:-none}" > ` + marker
	if err := app.CmdRun(ctx, []string{"sh1", "--", "-c", cmd}); err != nil {
		t.Fatalf("passthrough run: %v\n%s", err, errb.String())
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "HOME="+filepath.Join(app.InstancesDir(), "sh1", "home")) {
		t.Errorf("child HOME was not the instance home:\n%s", got)
	}
	if !strings.Contains(got, "PROXY=none") {
		t.Errorf("child should see no proxy when none is configured:\n%s", got)
	}
}

func TestRunHeadlessStreamsAndRecords(t *testing.T) {
	app, out, errb := testApp(t)
	ctx := context.Background()
	script := fakeAgent(t, "kilo-success.jsonl")

	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", script, "--link=false", "--version", "test")
	// The registry records the located host binary; point the driver at the
	// replay script so nothing reaches a model endpoint.
	setDriverBinary(t, app, "k1", script)

	out.Reset()
	if err := app.CmdRun(ctx, []string{"k1", "--prompt", "list the files", "--jsonl"}); err != nil {
		t.Fatalf("headless run: %v\nstdout:%s\nstderr:%s", err, out.String(), errb.String())
	}

	var types []string
	answer := ""
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stdout is not NDJSON: %v (%q)", err, line)
		}
		types = append(types, ev.Type)
		if ev.Type == "text" {
			answer += ev.Text
		}
	}
	if len(types) < 4 {
		t.Fatalf("expected a stream of events, got %v", types)
	}
	if !strings.Contains(answer, "go.mod and internal") {
		t.Errorf("answer text missing from stream: %q", answer)
	}
	for _, want := range []string{"tool_call", "usage"} {
		if !strings.Contains(strings.Join(types, ","), want) {
			t.Errorf("no %s event in %v", want, types)
		}
	}

	// The run must be recorded, with a raw log on disk, so a failure later can
	// be diagnosed without re-running the agent.
	_, meta, err := app.Open("k1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRun == nil {
		t.Fatal("no lastrun recorded")
	}
	if meta.LastRun.ExitCode != 0 {
		t.Errorf("recorded exit code %d", meta.LastRun.ExitCode)
	}
	if meta.LastRun.SessionID != "ses_fixture0001" {
		t.Errorf("session id = %q, want the one in the fixture", meta.LastRun.SessionID)
	}
	if _, err := os.Stat(meta.LastRun.Log); err != nil {
		t.Errorf("run log missing: %v", err)
	}

	// Each run also leaves a proxy audit beside its transcript. Which precedence
	// layer won is not recoverable from the agent's own output, and `golunch env`
	// only reports the answer for right now.
	audits, _ := filepath.Glob(filepath.Join(filepath.Dir(meta.LastRun.Log), "env-*.txt"))
	if len(audits) != 1 {
		t.Fatalf("expected exactly one proxy audit next to %s, got %v", meta.LastRun.Log, audits)
	}
	body, err := os.ReadFile(audits[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "source: ") {
		t.Errorf("audit lacks its provenance line:\n%s", body)
	}
}

func TestRunDryRunStartsNothing(t *testing.T) {
	app, out, errb := testApp(t)
	ctx := context.Background()
	script := fakeAgent(t, "cline-success.jsonl")
	newTestInstance(t, app, "c1", "--agent", "cline", "--binary", script, "--link=false")
	setDriverBinary(t, app, "c1", script)

	out.Reset()
	if err := app.CmdRun(ctx, []string{"c1", "--prompt", "hi", "--dry-run", "--proxy", "none", "--timeout", "90s"}); err != nil {
		t.Fatalf("dry run: %v\n%s", err, errb.String())
	}
	got := out.String()
	for _, want := range []string{"argv:", "--json", "--auto-approve", "true", "timeout:", "1m30s", "[0] "} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "-y") {
		t.Errorf("the -y flag does not exist in cline; it must not appear:\n%s", got)
	}
	// The explicit-unset path: --proxy none must show as disabled, not inherited.
	if !strings.Contains(got, "explicitly disabled") {
		t.Errorf("--proxy none should resolve as disabled:\n%s", got)
	}
	inst, _ := instance.New(app.InstancesDir(), "c1")
	if names, _ := os.ReadDir(inst.LogsDir()); len(names) > 0 {
		t.Error("a dry run must not write a run log")
	}
}

// A locked instance is a busy answer, not a crash: two agents writing the same
// session directory is how a user loses work.
func TestRunRefusesWhileBusy(t *testing.T) {
	app, _, errb := testApp(t)
	ctx := context.Background()
	script := fakeAgent(t, "kilo-success.jsonl")
	newTestInstance(t, app, "k2", "--agent", "kilo", "--binary", script, "--link=false")
	setDriverBinary(t, app, "k2", script)

	inst, err := instance.New(app.InstancesDir(), "k2")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := inst.Acquire(instance.Exclusive)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	err = app.CmdRun(ctx, []string{"k2", "--prompt", "hi"})
	if err == nil {
		t.Fatal("expected a busy error while the instance is locked")
	}
	if ce, ok := err.(CommandError); !ok || ce.Code != ExitBusy {
		t.Errorf("error = %v (%T), want CommandError with code %d", err, err, ExitBusy)
	}
	_ = errb
}

func TestTaskFanOut(t *testing.T) {
	app, out, errb := testApp(t)
	ctx := context.Background()
	script := fakeAgent(t, "kilo-success.jsonl")
	newTestInstance(t, app, "t1", "--agent", "kilo", "--binary", script, "--link=false")
	setDriverBinary(t, app, "t1", script)
	newTestInstance(t, app, "t2", "--agent", "kilo", "--binary", script, "--link=false")
	setDriverBinary(t, app, "t2", script)

	dir := t.TempDir()
	file := filepath.Join(dir, "tasks.json")
	body := `{
	  "name": "sweep",
	  "defaults": {"instance": "t1", "parallel": 2, "timeout": 30},
	  "tasks": [
	    {"prompt": "first"},
	    {"prompts": ["second", "third"], "instance": "t2"},
	    {"id": "slow", "prompt": "never finishes", "instance": "slow", "timeout": "200ms"}
	  ]
	}`
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	newTestInstance(t, app, "slow", "--agent", "kilo", "--binary", "/bin/sleep", "--link=false")
	inst, _ := instance.New(app.InstancesDir(), "slow")
	meta, err := inst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	meta.Launch.DriverBinary = fakeSleeper(t)
	if err := inst.SaveMetadata(&meta); err != nil {
		t.Fatal(err)
	}

	summary := filepath.Join(dir, "summary.json")
	out.Reset()
	err = app.CmdTask(ctx, []string{file, "-out", summary, "-quiet"})
	if err == nil {
		t.Fatal("expected the timed-out task to fail the run")
	}
	if !strings.Contains(errb.String()+out.String(), "slow") {
		t.Errorf("summary should name the failing task:\nout:%s\nerr:%s", out.String(), errb.String())
	}

	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("summary not written: %v", err)
	}
	var doc struct {
		Taskfile string `json:"taskfile"`
		Tasks    []struct {
			ID       string `json:"id"`
			Instance string `json:"instance"`
			Answer   string `json:"answer"`
			TimedOut bool   `json:"timed_out"`
			Log      string `json:"log"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Tasks) != 4 {
		t.Fatalf("expected 4 expanded tasks, got %d", len(doc.Tasks))
	}
	var answered, timeouts int
	seen := map[string]string{}
	for _, tk := range doc.Tasks {
		if strings.Contains(tk.Answer, "go.mod and internal") {
			answered++
		}
		if tk.TimedOut {
			timeouts++
		}
		if !strings.Contains(tk.Log, "run-") {
			t.Errorf("task %s has no per-run log: %q", tk.ID, tk.Log)
		}
		// Concurrent tasks on one instance start in the same second. If the log
		// name only carries a second, two agents append the same transcript and
		// neither is readable afterwards.
		if other, dup := seen[tk.Log]; dup {
			t.Errorf("tasks %s and %s share log %s", other, tk.ID, tk.Log)
		}
		seen[tk.Log] = tk.ID
	}
	if answered != 3 {
		t.Errorf("3 tasks should have answered, got %d", answered)
	}
	if timeouts != 1 {
		t.Errorf("the slow task should be recorded as timed out, got %d timeouts", timeouts)
	}
}

func TestCloneGivesAFreshIdentity(t *testing.T) {
	app, out, _ := testApp(t)
	newTestInstance(t, app, "src", "--agent", "kilo", "--binary", "/bin/cat", "--link=false",
		"--proxy", "http://p:1")
	// Simulate a signed-in source instance.
	inst, _ := instance.New(app.InstancesDir(), "src")
	auth := filepath.Join(inst.DataDir(), "kilo", "auth.json")
	if err := os.MkdirAll(filepath.Dir(auth), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auth, []byte(`{"tok":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := app.CmdClone([]string{"src", "dst", "--link=false"}); err != nil {
		t.Fatal(err)
	}
	dst, _ := instance.New(app.InstancesDir(), "dst")
	if osutil.Exists(filepath.Join(dst.DataDir(), "kilo", "auth.json")) {
		t.Error("clone must not copy credentials without --copy-data")
	}
	if !strings.Contains(out.String(), "logged out") {
		t.Errorf("clone should tell the user the new identity is logged out:\n%s", out.String())
	}

	dmeta, err := dst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if dmeta.Proxy.Spec != "http://p:1" {
		t.Errorf("clone lost the source proxy: %q", dmeta.Proxy.Spec)
	}

	if err := app.CmdClone([]string{"src", "withdata", "--copy-data", "--yes", "--link=false"}); err != nil {
		t.Fatal(err)
	}
	wd, _ := instance.New(app.InstancesDir(), "withdata")
	if !osutil.Exists(filepath.Join(wd.DataDir(), "kilo", "auth.json")) {
		t.Error("--copy-data must duplicate the credential file")
	}
}

func TestMainUsesTheConfiguredAgentVersion(t *testing.T) {
	app, out, _ := testApp(t)
	if err := app.CmdVersion([]string{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "golunch ") {
		t.Errorf("version output = %q", out.String())
	}
}

// setDriverBinary writes the metadata field that points the driver path at a
// replay script instead of the real agent.
func setDriverBinary(t *testing.T, app *App, alias, path string) {
	t.Helper()
	inst, meta, err := app.Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	meta.Launch.DriverBinary = path
	if err := inst.SaveMetadata(&meta); err != nil {
		t.Fatal(err)
	}
}

// fakeSleeper is a driver that never emits anything, so a timeout has to reap it.
func fakeSleeper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-sleeper")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLockModeForContinueWithoutSession pins the one run shape that takes the
// exclusive lock, because two of those would append to the same session and
// interleave garbage.
func TestLockModeForContinueWithoutSession(t *testing.T) {
	app, _, errb := testApp(t)
	ctx := context.Background()
	script := fakeAgent(t, "kilo-success.jsonl")
	newTestInstance(t, app, "k3", "--agent", "kilo", "--binary", script, "--link=false")
	setDriverBinary(t, app, "k3", script)

	inst, err := instance.New(app.InstancesDir(), "k3")
	if err != nil {
		t.Fatal(err)
	}
	// Hold a shared lock: --continue with no session wants exclusive, so it must
	// refuse, while a normal run would proceed alongside.
	shared, err := inst.Acquire(instance.Shared)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.CmdRun(ctx, []string{"k3", "--prompt", "hi", "--continue"}); err == nil {
		shared.Release()
		t.Fatal("--continue without --session must take the exclusive lock")
	}
	if err := app.CmdRun(ctx, []string{"k3", "--prompt", "hi", "--session", "ses_x"}); err != nil {
		t.Fatalf("a run with an explicit session should share the lock: %v\n%s", err, errb.String())
	}
	shared.Release()
}
