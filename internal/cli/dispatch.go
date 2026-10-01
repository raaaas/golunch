package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// versionText is reported by `golunch version`; the real value is injected by
// cmd/golunch through Main so the ldflags stamp lives in one place.
var versionText = "dev"

// command is one entry in the dispatch table.
type command struct {
	name string
	// aliases covers the shapes people type from memory: `remove` for `rm`.
	aliases []string
	summary string
	// run takes the args after the command name. Commands that start a child
	// process take the context so a signal can reach its process group; an
	// installer run by `new --install` is as much a child as an agent is.
	run func(ctx context.Context, a *App, args []string) error
	// needsCtx documents which commands are interactive; the table stays one
	// shape rather than two.
	needsCtx bool
}

var commands = []command{
	{"new", []string{"add"}, "wrap a host CLI in a fresh isolated instance", func(ctx context.Context, a *App, args []string) error { return a.CmdNew(ctx, args) }, false},
	{"install", nil, "download a missing agent CLI into the instance's own bin", func(ctx context.Context, a *App, args []string) error { return a.CmdInstall(ctx, args) }, true},
	{"ls", []string{"list"}, "show every instance", func(_ context.Context, a *App, args []string) error { return a.CmdList(args) }, false},
	{"rm", []string{"remove", "delete"}, "delete an instance and its command link", func(_ context.Context, a *App, args []string) error { return a.CmdRemove(args) }, false},
	{"run", []string{"exec"}, "run an instance: passthrough after --, or headless with --prompt", func(ctx context.Context, a *App, args []string) error { return a.CmdRun(ctx, args) }, true},
	{"task", []string{}, "fan out a taskfile across instances", func(ctx context.Context, a *App, args []string) error { return a.CmdTask(ctx, args) }, true},
	{"clone", []string{"duplicate"}, "make a second identity from an existing instance", func(_ context.Context, a *App, args []string) error { return a.CmdClone(args) }, false},
	{"seed", []string{}, "copy the host's MCP servers, skills and plugin config into an instance, never its login", func(_ context.Context, a *App, args []string) error { return a.CmdSeed(args) }, false},
	{"shell", []string{}, "open a login shell inside an instance", func(ctx context.Context, a *App, args []string) error { return a.CmdShell(ctx, args) }, true},
	{"env", []string{}, "print the resolved child environment and where each proxy came from", func(_ context.Context, a *App, args []string) error { return a.CmdEnv(args) }, false},
	{"doctor", []string{"check"}, "report whether instances are healthy, isolated and free of drift", func(_ context.Context, a *App, args []string) error { return a.CmdDoctor(args) }, false},
	{"refresh", []string{}, "regenerate launcher scripts from current configuration", func(_ context.Context, a *App, args []string) error { return a.CmdRefresh(args) }, false},
	{"config", []string{}, "show or initialize the global configuration", func(_ context.Context, a *App, args []string) error { return a.CmdConfig(args) }, false},
	{"version", []string{}, "print the version", func(_ context.Context, a *App, args []string) error { return a.CmdVersion(args) }, false},
}

func lookup(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
		for _, al := range c.aliases {
			if al == name {
				return c, true
			}
		}
	}
	return command{}, false
}

// Main runs one invocation and returns the process exit code. It takes its args
// and streams rather than reaching for them, so a test can drive the entire
// command surface in-process against a buffer; cmd/golunch is the one place that
// knows os.Args and os.Stdout exist. It returns rather than calling os.Exit.
func Main(ctx context.Context, args []string, in io.Reader, out, errw io.Writer, version string) int {
	if version != "" {
		versionText = version
	}

	if len(args) == 0 {
		usage(errw)
		return ExitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(out)
		return ExitOK
	}

	cmd, ok := lookup(args[0])
	if !ok {
		fmt.Fprintf(errw, "golunch: unknown command %q\n\n", args[0])
		usage(errw)
		return ExitUsage
	}

	// Refusing root is not decoration: every file this tool creates is owned by
	// the invoking user, and a root-created instance tree leaves the agent
	// unable to write its own config afterwards. warren makes the same refusal.
	// It stays here, at the process boundary, rather than moving into the
	// library: a caller that embeds package golunch must not be able to skip it
	// by forgetting to.
	if os.Geteuid() == 0 && os.Getenv("GOLUNCH_ALLOW_ROOT") == "" {
		fmt.Fprintln(errw, "golunch: refusing to run as root; instance files would end up owned by root "+
			"and unusable by the agent that reads them. Set GOLUNCH_ALLOW_ROOT=1 to override.")
		return ExitUsage
	}

	app, err := New(in, out, errw)
	if err != nil {
		fmt.Fprintf(errw, "golunch: %v\n", err)
		return ExitError
	}
	if app.Cfg.NestingWarning != "" && cmd.needsCtx {
		fmt.Fprintf(errw, "golunch: %s\n", app.Cfg.NestingWarning)
	}

	err = cmd.run(ctx, app, args[1:])
	return finish(err, errw)
}

// finish maps an error onto an exit code, and prints only the message. A child
// exit code is returned unchanged so `golunch run k -- make` is usable in a
// shell that checks $?.
func finish(err error, w io.Writer) int {
	if err == nil {
		return ExitOK
	}
	var ce CommandError
	if errors.As(err, &ce) {
		if ce.Err != nil && ce.Err.Error() != "" {
			fmt.Fprintf(w, "golunch: %v\n", ce.Err)
		}
		return ce.Code
	}
	if errors.Is(err, flag.ErrHelp) {
		return ExitUsage
	}
	fmt.Fprintf(w, "golunch: %v\n", err)
	return ExitError
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `golunch %s — run any agent CLI as many times as you want, each with its own private config and login.

Usage:
  golunch new <alias> --agent <name> [--proxy <url|profile|none>] [--binary <path>]
  golunch install <alias> [--url <https-installer> | --script <path>] [--sha256 <hex>] [--yes]
  golunch ls [-l]
  golunch run <alias> [--prompt "..." --model m --session id --timeout 90s --jsonl --dry-run]
  golunch run <alias> [-- passthrough args...]
  golunch task <taskfile.json> [--parallel n]
  golunch clone <src> <dst> [--copy-data]
  golunch seed <alias> [mcp|skills|config|deps] [--all] [--dry-run]
  golunch shell <alias> [-c "command"]
  golunch env <alias> [--all | --shell]
  golunch doctor [alias]
  golunch refresh [alias]
  golunch rm <alias> [--yes]
  golunch config | version

An instance is a directory plus environment variables: HOME, every XDG base dir,
TMPDIR and a private bin/ on PATH. No namespaces, no containers, no daemon, and
nothing runs unless you ask for it.

Proxy support is env-var injection only: HTTP_PROXY/HTTPS_PROXY/ALL_PROXY/NO_PROXY
(both cases) are set on the child. Precedence is --proxy flag, then the
instance's metadata.toml, then the global profile, then the host environment
untouched. The loopback range is always in NO_PROXY.
`, versionText)
}
