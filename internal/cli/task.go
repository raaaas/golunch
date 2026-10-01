package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/raaaas/golunch"
	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/internal/osutil"
	"github.com/raaaas/golunch/task"
)

// CmdTask fans N prompts out over instances. Every task goes through the same
// Prompt path as `golunch run --prompt`, so a taskfile run produces the same run
// logs, the same metadata record and the same lock discipline as an interactive
// one.
func (a *App) CmdTask(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("task", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	parallel := fs.Int("parallel", 0, "how many tasks may run at once (default: defaults.parallel or 2)")
	outPath := fs.String("out", "", "write the summary as JSON to this file")
	dryRun := fs.Bool("dry-run", false, "resolve every task and print what would run, starting nothing")
	quiet := fs.Bool("quiet", false, "no per-task progress, only the final summary")
	maxRing := fs.Int("ring", 0, "events kept in memory per task (full NDJSON always goes to the instance log)")
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	// Anything left over after the taskfile means a flag was spelled wrong and
	// its value was taken for a positional; saying so is better than quietly
	// running the file and ignoring the rest.
	if len(rest) != 1 {
		return usageErr("usage: golunch task <taskfile.json> [--parallel n] [--out summary.json] (got %d positionals: %v)", len(rest), rest)
	}
	f, err := task.UnmarshalFile(rest[0])
	if err != nil {
		return err
	}

	ring := *maxRing
	if ring == 0 {
		ring = a.Cfg.UI.MaxRingEvents
	}
	if ring == 0 {
		ring = 200
	}

	if *dryRun {
		for i, t := range f.Parsed {
			a.printf("[%d/%d] %s  instance=%s model=%s timeout=%s\n",
				i+1, len(f.Parsed), t.ID, t.Instance, dash(t.Model), t.Timeout.Go())
			a.printf("      prompt: %s\n", osutil.FirstLine(t.Prompt))
		}
		a.printf("\n%d task(s) resolved; nothing started.\n", len(f.Parsed))
		return nil
	}

	progress := !*quiet

	opt := task.Options{
		Parallel: *parallel,
		MaxRing:  ring,
		OnStart: func(req task.Request) {
			if !progress {
				return
			}
			a.warnf("… %-*s %s\n", 24, req.Task.ID, req.Task.Instance)
		},
		OnDone: func(o task.Outcome, done, total int) {
			if !progress {
				return
			}
			status := "ok"
			switch {
			case o.Err != nil:
				status = "error: " + o.Err.Error()
			case o.TimedOut:
				status = "timeout"
			case o.ExitCode != 0:
				status = fmt.Sprintf("exit %d", o.ExitCode)
			}
			a.warnf("  %d/%d %-*s %s  %s  (%d events)\n",
				done, total, 24, o.Task.ID, o.Instance, truncate(status, 40), len(o.Events))
		},
	}

	run := func(ctx context.Context, req task.Request) task.Outcome {
		t := req.Task
		rr := task.NewRing(opt.MaxRing)
		pr := golunch.PromptOptions{
			Alias: t.Instance, Prompt: t.Prompt, Model: req.Model, Provider: req.Provider,
			Thinking: req.Thinking, Session: t.Session, Continue: t.Continue, Title: t.Title,
			Files: t.Files, Extra: t.Extra, Timeout: req.Timeout, AutoApprove: req.AutoApprove,
			OnEvent: func(ev agent.Event) bool {
				if b, err := json.Marshal(ev); err == nil {
					rr.Add(b)
				}
				return true
			},
		}
		res, err := a.Prompt(ctx, pr)
		out := task.Outcome{
			Task: t, Instance: t.Instance, Answer: res.Accum.Text,
			Err: err, ExitCode: res.Result.ExitCode, TimedOut: res.Result.TimedOut,
			Log: res.LogPath, SessionID: res.SessionID, Duration: res.Result.Duration,
			Events: eventsOf(rr), RingDropped: droppedOf(rr),
		}
		if res.Accum.Usage.Any() {
			out.Usage = map[string]float64{
				"input": res.Accum.Usage.Input, "output": res.Accum.Usage.Output,
				"reasoning": res.Accum.Usage.Reasoning, "cost": res.Accum.Usage.Cost,
			}
		}
		return out
	}

	name := filepath.Base(rest[0])
	a.warnf("task %s: %d task(s), parallel %d\n", name, len(f.Parsed), effectiveParallel(*parallel, f, len(f.Parsed)))
	outcomes, err := task.Run(ctx, f, run, opt)

	failed := 0
	for _, o := range outcomes {
		if o.Err != nil || o.ExitCode != 0 {
			failed++
		}
	}
	a.printf("\nsummary\n")
	for _, o := range outcomes {
		status := "ok"
		switch {
		case o.Err != nil:
			status = "FAIL " + truncate(o.Err.Error(), 60)
		case o.TimedOut:
			status = "TIMEOUT"
		case o.ExitCode != 0:
			status = fmt.Sprintf("exit %d", o.ExitCode)
		}
		a.printf("  %-*s  %-*s  %8s  %s\n", 28, o.Task.ID, 16, o.Instance,
			o.Duration.Truncate(time.Millisecond), status)
		if o.Answer != "" {
			a.printf("      %s\n", osutil.FirstLine(o.Answer))
		}
		if o.Log != "" {
			a.printf("      log: %s\n", o.Log)
		}
	}

	if *outPath != "" {
		if err := writeSummary(*outPath, name, outcomes); err != nil {
			return err
		}
		a.warnf("summary written to %s\n", *outPath)
	}
	if err != nil {
		return err
	}
	if failed > 0 {
		return CommandError{Code: ExitError, Err: fmt.Errorf("%d of %d task(s) did not succeed", failed, len(outcomes))}
	}
	return nil
}

func effectiveParallel(flagVal int, f *task.File, total int) int {
	p := flagVal
	if p == 0 {
		p = f.Defaults.Parallel
	}
	if p <= 0 {
		p = 2
	}
	if p > total {
		p = total
	}
	return p
}

func eventsOf(r *task.Ring) []json.RawMessage {
	items, _ := r.Slice()
	return items
}

func droppedOf(r *task.Ring) int {
	_, d := r.Slice()
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// writeSummary persists the fan-out result. Events are the bounded ring, not the
// whole transcript — that is already in each task's log file.
func writeSummary(path, file string, outcomes []task.Outcome) error {
	type row struct {
		ID          string             `json:"id"`
		Instance    string             `json:"instance"`
		Prompt      string             `json:"prompt"`
		Answer      string             `json:"answer"`
		ExitCode    int                `json:"exit_code"`
		TimedOut    bool               `json:"timed_out"`
		Error       string             `json:"error,omitempty"`
		SessionID   string             `json:"session_id,omitempty"`
		Log         string             `json:"log,omitempty"`
		DurationMS  int64              `json:"duration_ms"`
		Usage       map[string]float64 `json:"usage,omitempty"`
		Events      []json.RawMessage  `json:"tail_events,omitempty"`
		RingDropped int                `json:"ring_dropped"`
	}
	doc := struct {
		Taskfile string `json:"taskfile"`
		Rows     []row  `json:"tasks"`
	}{Taskfile: file}
	for _, o := range outcomes {
		r := row{
			ID: o.Task.ID, Instance: o.Instance, Prompt: o.Task.Prompt, Answer: o.Answer,
			ExitCode: o.ExitCode, TimedOut: o.TimedOut, SessionID: o.SessionID, Log: o.Log,
			DurationMS: o.Duration.Milliseconds(), Usage: o.Usage, Events: o.Events,
			RingDropped: o.RingDropped,
		}
		if o.Err != nil {
			r.Error = o.Err.Error()
		}
		doc.Rows = append(doc.Rows, r)
	}
	enc, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	enc = append(enc, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, enc, 0o644)
}
