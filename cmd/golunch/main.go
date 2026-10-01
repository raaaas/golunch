package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/raaaas/golunch/internal/cli"
)

// golunch is a static launcher: one binary, no daemon, nothing listening. The
// dispatch table lives in package cli, which takes its streams as arguments so
// the whole surface is testable without exec'ing this file.

// version is set with -ldflags "-X main.version=..." for builds from a git
// checkout; otherwise the version comes from the module, which is what makes
// `go install github.com/raaaas/golunch/cmd/golunch@v1.0.0` report v1.0.0
// without anyone passing a flag.
var version = "dev"

func main() {
	// Ctrl-C cancels the context, which is how execd reaches the child's process
	// group. Without this, SIGINT lands only on golunch and the agent keeps
	// running against a deleted lock.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, buildVersion()))
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}
