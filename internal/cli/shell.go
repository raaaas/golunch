package cli

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/raaaas/golunch/execd"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/internal/osutil"
)

// CmdShell opens a login shell inside an instance's environment. This is how an
// agent gets signed in: golunch does not know any agent's OAuth flow, it only
// guarantees that whatever the shell writes lands inside the instance tree.
func (a *App) CmdShell(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	cmdStr := fs.String("c", "", "run this with the instance shell instead of an interactive session")
	proxySpec := fs.String("proxy", "", "override the instance proxy for this shell")
	var envPairs stringList
	fs.Var(&envPairs, "env", "K=V for the child, repeatable; K= unsets")
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: golunch shell <alias> [-c \"command\"]")
	}
	inst, meta, err := a.Open(rest[0])
	if err != nil {
		return err
	}
	extra, err := envMap(envPairs)
	if err != nil {
		return err
	}
	res, err := a.Resolve(meta, *proxySpec, nil)
	if err != nil {
		return err
	}
	e, err := a.BuildEnv(inst, &meta, res, extra, a.KeepVarsFor(&meta))
	if err != nil {
		return err
	}
	inst.EnsureStorage()

	sh := firstNonEmpty(meta.Instance.Shell, a.Cfg.Defaults.Shell, "/bin/bash")
	argv := []string{sh}
	if *cmdStr != "" {
		argv = append(argv, "-c", *cmdStr)
	} else {
		// A login shell, so the instance's own rc files are read the way an
		// interactive session would read them.
		argv = append(argv, "-l")
	}

	lock, err := inst.Acquire(instance.Shared)
	if err != nil {
		return busyErr("%s is busy", inst.Alias)
	}
	defer lock.Release()

	r := execd.Run(ctx, execd.Spec{Argv: argv, Env: e.Slice(), InheritStdio: true, Dir: osutil.Cwd()})
	return childError(filepath.Base(sh), r)
}

// CmdEnv prints the resolved child environment, annotating which precedence
// layer decided each proxy variable. This is the verification surface for the
// whole proxy feature: what appears here is exactly what the child receives.
func (a *App) CmdEnv(args []string) error {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	all := fs.Bool("all", false, "print every variable, not only the proxy block")
	sh := fs.Bool("shell", false, "emit eval-able export lines for the calling shell")
	proxySpec := fs.String("proxy", "", "resolve as if --proxy were given at run")
	var envPairs, noProxy stringList
	fs.Var(&envPairs, "env", "K=V, repeatable")
	fs.Var(&noProxy, "noproxy", "extra NO_PROXY entry, repeatable")
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: golunch env <alias> [--all | --shell]")
	}
	inst, meta, err := a.Open(rest[0])
	if err != nil {
		return err
	}
	extra, err := envMap(envPairs)
	if err != nil {
		return err
	}
	res, err := a.Resolve(meta, *proxySpec, commaSplit(noProxy))
	if err != nil {
		return err
	}
	e, err := a.BuildEnv(inst, &meta, res, extra, a.KeepVarsFor(&meta))
	if err != nil {
		return err
	}

	if *sh {
		for _, k := range e.KeysSorted() {
			if v, ok := e.Get(k); ok && v != "" {
				a.printf("export %s=%s\n", k, instance.ShellQuote(v))
			}
		}
		return nil
	}

	a.printf("instance %s\nroot      %s\n\n", inst.Alias, inst.Root)
	a.printf("isolation:\n")
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
		"XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "XDG_DATA_DIRS", "XDG_CONFIG_DIRS", "TMPDIR", "PATH"} {
		if v, ok := e.Get(k); ok {
			a.printf("  %-17s %s\n", k, v)
		}
	}

	a.printf("\nproxy candidates considered (last wins):\n")
	if len(res.Layers) == 0 {
		a.printf("  none configured at any layer\n")
	} else {
		for _, l := range res.Layers {
			a.printf("  %s\n", l)
		}
	}

	a.printf("\neffective proxy — source: %s\n", res.Source)
	block := e.ProxyBlock()
	if len(block) == 0 {
		a.printf("  (nothing set: the child inherits no proxy decision from golunch)\n")
	}
	for _, line := range block {
		a.printf("  %s\n", line)
	}
	if res.Profile.Disabled() {
		a.printf("  all eight proxy variables are removed from the child\n")
	}
	if np, ok := e.Get("NO_PROXY"); ok {
		a.printf("\nNO_PROXY floor and merge: %s\n", np)
	}

	if *all {
		a.printf("\ncomplete child environment:\n")
		for _, kv := range e.Slice() {
			k, v, _ := strings.Cut(kv, "=")
			a.printf("  %s=%s\n", k, v)
		}
	}
	return nil
}

// childError turns an execd result into a command error carrying the child's own
// exit code, so `golunch run k -- make` reports make's status.
func childError(name string, r execd.Result) error {
	if r.Err != nil {
		return r.Err
	}
	if r.TimedOut {
		return CommandError{Code: ExitTimeout, Err: fmt.Errorf("%s timed out", name)}
	}
	if r.ExitCode != 0 {
		return CommandError{Code: r.ExitCode, Err: fmt.Errorf("%s exited with code %d", name, r.ExitCode)}
	}
	return nil
}
