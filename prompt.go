package golunch

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/execd"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/internal/osutil"
	"github.com/raaaas/golunch/proxy"
)

// PromptOptions is one headless execution: a prompt against an instance, with
// every knob the driver path exposes. `golunch run --prompt` builds one from
// flags and `golunch task` builds one per task, so both share the lock, run log,
// event normalization and metadata recording. Two implementations of that flow
// would be two chances for them to disagree.
type PromptOptions struct {
	Alias       string
	Prompt      string
	Model       string
	Provider    string
	Thinking    string
	Session     string
	Title       string
	Attach      string
	WorkDir     string
	Files       []string
	Extra       []string
	NoProxy     []string
	Env         map[string]string
	Proxy       string
	Key         string
	Continue    bool
	Fork        bool
	Plan        bool
	AutoApprove bool
	Timeout     time.Duration

	// OnEvent sees each normalized event before golunch prints it. Returning
	// false stops the run, which is how a caller takes only the first sentence.
	OnEvent func(agent.Event) bool
	// Stream renders human-readable progress; the task runner turns it off and
	// prints its own summary instead.
	Stream bool
	JSONL  bool
	Quiet  bool
	DryRun bool
}

// PromptResult is what a headless run produced.
type PromptResult struct {
	Result    execd.Result
	Accum     agent.Accumulator
	SessionID string
	LogPath   string
	Proxy     proxy.Source
}

// Prompt runs one headless invocation. It returns the partial result alongside
// an error: a run that timed out halfway still has the transcript and the events
// that did arrive, and a caller that only looks at the error loses them.
func (r *Runner) Prompt(ctx context.Context, pr PromptOptions) (PromptResult, error) {
	var out PromptResult
	inst, meta, err := r.Open(pr.Alias)
	if err != nil {
		return out, err
	}
	if meta.Instance.Agent == "" {
		return out, UsageErr("instance %q wraps a plain binary and has no agent driver, so a prompt has no meaning.\n"+
			"Recreate it with --agent, or pass the command after --.", pr.Alias)
	}
	entry, ok := agent.Lookup(meta.Instance.Agent)
	if !ok {
		return out, NotFoundErr("instance %q names agent %q, which is not in the registry", pr.Alias, meta.Instance.Agent)
	}
	if pr.Prompt == "" {
		return out, UsageErr("empty prompt")
	}

	to := pr.Timeout
	if to == 0 && r.Cfg.Defaults.Timeout > 0 {
		to = time.Duration(r.Cfg.Defaults.Timeout) * time.Second
	}

	// Attaching to a server means the agent will talk to it constantly. If that
	// host goes through the proxy, the attach fails with a confusing timeout, so
	// its hostname joins NO_PROXY for this run only.
	noProxy := pr.NoProxy
	if h := hostOf(pr.Attach); h != "" {
		noProxy = append(append([]string(nil), noProxy...), h)
	}

	res, err := r.Resolve(meta, pr.Proxy, noProxy)
	if err != nil {
		return out, err
	}
	e, err := r.BuildEnv(inst, &meta, res, pr.Env, r.KeepVarsFor(&meta))
	if err != nil {
		return out, err
	}
	inst.EnsureStorage()

	spec := agent.RunSpec{
		Prompt: pr.Prompt, Model: pr.Model, Provider: pr.Provider,
		SessionID: pr.Session, Continue: pr.Continue, Fork: pr.Fork, Plan: pr.Plan,
		AutoApprove: pr.AutoApprove, Thinking: pr.Thinking, Title: pr.Title,
		Attach: pr.Attach, WorkDir: pr.WorkDir, Files: pr.Files,
		Timeout: to, Extra: pr.Extra,
	}
	if entry.SupportNative && meta.Launch.NativeFlags {
		spec.Native = agent.NativePaths{ConfigDir: inst.ConfigDir(), DataDir: inst.DataDir()}
	}

	envSlice := e.Slice()
	if pr.Key != "" {
		if len(entry.Env.APIKeyVars) == 0 {
			return out, UsageErr("agent %q documents no API key variable; sign in inside the instance instead", entry.Name)
		}
		envSlice = setEnv(envSlice, entry.Env.APIKeyVars[0], pr.Key)
	}

	argv := []string{r.childBinary(entry, inst, meta)}
	argv = append(argv, entry.Subcmd...)
	argv = append(argv, entry.BuildArgs(spec)...)

	childDir := pr.WorkDir
	if childDir == "" {
		childDir = osutil.Cwd()
	}

	spec2 := execd.Spec{
		Dir: childDir, Argv: argv, Env: envSlice, Timeout: to,
		ParseLine: entry.ParseLine, StderrTail: 16 << 10,
		Decided: e.Baked(),
	}
	out.Proxy = res.Source

	if pr.DryRun {
		r.print(execd.DryRun(spec2))
		r.printf("\nproxy: %s\n", res.Source)
		for _, line := range e.ProxyBlock() {
			// Values already appear in the env block above; this adds the layer
			// each one came from, which is the thing --dry-run cannot show.
			r.printf("  %s\n", line)
		}
		r.printf("\nnote: nothing was started; --dry-run neither takes the lock nor writes a run log.\n")
		return out, nil
	}

	// --continue without --session resolves to "whatever the newest session is"
	// at start time. Two such runs would both append to it and interleave, so
	// that one shape takes the exclusive lock.
	mode := instance.Shared
	if pr.Continue && pr.Session == "" {
		mode = instance.Exclusive
	}
	lock, err := inst.Acquire(mode)
	if err != nil {
		return out, CommandError{Code: ExitBusy,
			Err: fmt.Errorf("%s is busy (%s)", inst.Alias, describeMode(mode)), Cause: err}
	}
	defer lock.Release()

	now := time.Now().UTC()
	name := stamp(now)
	logPath := inst.LogPath(name)
	spec2.RawLog = logPath
	out.LogPath = logPath
	r.auditProxy(inst.AuditPath(name), e, res)

	printer := r.eventPrinter(pr, entry.Name)
	spec2.OnEvent = func(ev agent.Event) bool {
		if !printer.p.f(ev) {
			return false
		}
		if pr.OnEvent != nil {
			return pr.OnEvent(ev)
		}
		return true
	}

	r2 := execd.Run(ctx, spec2)
	out.Result = r2
	out.Accum = printer.p.accum
	out.SessionID = printer.p.accum.SessionID()
	if pr.Stream || pr.JSONL {
		printer.finish(r2)
	}

	meta.LastRun = &instance.RunRecord{
		StartedAt: now, ExitCode: r2.ExitCode, SessionID: out.SessionID,
		ProxySource: string(res.Source), Log: logPath, Model: pr.Model,
	}
	if err := inst.SaveMetadata(&meta); err != nil {
		r.warnf("could not record the run in metadata: %v\n", err)
	}

	if r2.TimedOut {
		return out, CommandError{Code: ExitTimeout, Err: fmt.Errorf("timed out after %s and killed the process group", to)}
	}
	if r2.Err != nil {
		return out, r2.Err
	}
	if r2.ExitCode != 0 {
		return out, CommandError{Code: r2.ExitCode, Err: fmt.Errorf("%s exited with code %d", entry.Name, r2.ExitCode)}
	}
	if r2.Stats.Events == 0 && !pr.JSONL && pr.Stream {
		// The failure mode the original sample code had: a wrong struct shape
		// yields an empty answer, exit 0, and looks like success.
		r.warnf("warning: %s produced %d parseable events from %d lines (%s holds the raw output)\n",
			entry.Name, r2.Stats.Events, r2.Stats.Lines, logPath)
	}
	return out, nil
}

// childBinary prefers an instance-local copy, then the recorded host path, then
// the registry's search — in that order, because an instance with its own
// bin/<name> has opted into a specific version.
func (r *Runner) childBinary(entry agent.Entry, inst *instance.Instance, meta instance.Metadata) string {
	if meta.Launch.DriverBinary != "" {
		return meta.Launch.DriverBinary
	}
	if meta.Launch.Binary != "" {
		if p := filepath.Join(inst.BinDir(), meta.Launch.Binary); osutil.Exists(p) {
			return p
		}
	}
	if len(meta.Launch.Command) > 0 && osutil.Exists(meta.Launch.Command[0]) {
		return meta.Launch.Command[0]
	}
	if p, err := agent.Locate(entry); err == nil {
		return p
	}
	if len(meta.Launch.Command) > 0 {
		return meta.Launch.Command[0]
	}
	return entry.Binary
}

func setEnv(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	prefix := key + "="
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return append(out, prefix+val)
}

// hostOf extracts the bare hostname from a URL, tolerating a value that is only
// a host:port. Returns "" for anything unparseable, which is correct: an attach
// value that is not a URL must not silently widen NO_PROXY.
func hostOf(u string) string {
	if u == "" {
		return ""
	}
	if !strings.Contains(u, "://") {
		u = "//" + u
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return ""
	}
	h := parsed.Host
	if i := strings.LastIndex(h, ":"); i > strings.LastIndex(h, "]") {
		h = h[:i]
	}
	return strings.Trim(h, "[]")
}

func describeMode(m instance.LockMode) string {
	if m == instance.Exclusive {
		return "exclusive: --continue without --session"
	}
	return "another run or admin command holds it"
}

// logSeq distinguishes runs inside one process: a fan-out starts several tasks
// in the same second against the same instance, and two agents appending one
// NDJSON file produce a transcript that is unreadable by a human and by jq
// alike. The pid covers the other half, since two live golunch processes cannot
// share one.
var logSeq atomic.Uint32

// stamp is the run-log filename: sortable, unique per run rather than per
// second, and the same string the metadata records so `ls -l` can point at the
// file.
func stamp(t time.Time) string {
	return fmt.Sprintf("%s-%d-%d", t.UTC().Format("20060102T150405Z"), os.Getpid(), logSeq.Add(1))
}
