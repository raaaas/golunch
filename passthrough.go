package golunch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raaaas/golunch/execd"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/internal/osutil"
	"github.com/raaaas/golunch/proxy"
)

// RunPassthrough execs the instance's recorded command with the instance
// environment and this terminal attached. This is the `golunch run <alias> --
// <args>` path, and the reason a Makefile inside an instance needs no golunch
// knowledge at all.
func (r *Runner) RunPassthrough(ctx context.Context, inst *instance.Instance, meta *instance.Metadata,
	res proxy.Resolution, e *instance.Env, argv []string, dryRun bool) error {

	target := append([]string(nil), meta.Launch.Command...)
	if meta.Launch.Binary != "" {
		if p := filepath.Join(inst.BinDir(), meta.Launch.Binary); osutil.Exists(p) {
			target = []string{p}
		}
	}
	if len(target) == 0 {
		return UsageErr("instance %q records no command to run", inst.Alias)
	}
	full := append(target, argv...)

	if dryRun {
		r.print(execd.DryRun(execd.Spec{Argv: full, Env: e.Slice(), Decided: e.Baked()}))
		r.printf("\nproxy: %s\n", res.Source)
		return nil
	}

	lock, err := inst.Acquire(instance.Shared)
	if err != nil {
		return CommandError{Code: ExitBusy, Err: fmt.Errorf("%s is busy", inst.Alias), Cause: err}
	}
	defer lock.Release()

	r.auditProxy(inst.AuditPath(stamp(time.Now().UTC())), e, res)

	run := execd.Run(ctx, execd.Spec{Argv: full, Env: e.Slice(), InheritStdio: true})
	if run.TimedOut {
		return CommandError{Code: ExitTimeout, Err: errors.New("timed out")}
	}
	if run.Err != nil {
		return run.Err
	}
	if run.ExitCode != 0 {
		return CommandError{Code: run.ExitCode,
			Err: fmt.Errorf("%s exited with code %d", filepath.Base(full[0]), run.ExitCode)}
	}
	return nil
}

// auditProxy persists which proxy a run was handed, next to its transcript.
// Reading the resolved value out of `golunch env` only tells you what it is
// now; a run that misbehaved on a network months ago needs what it was then,
// including which precedence layer won.
func (r *Runner) auditProxy(path string, e *instance.Env, res proxy.Resolution) {
	lines := e.ProxyBlock()
	var b strings.Builder
	b.WriteString("# golunch proxy audit\n")
	fmt.Fprintf(&b, "source: %s\n", res.Source)
	for _, l := range res.Layers {
		fmt.Fprintf(&b, "considered: %s\n", l)
	}
	if len(lines) == 0 {
		b.WriteString("(no proxy variables set by golunch; the child sees whatever the calling shell had)\n")
	}
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		r.warnf("could not write the proxy audit: %v\n", err)
	}
}
