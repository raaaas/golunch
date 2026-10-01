package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/raaaas/golunch"
)

// CmdRun has two modes that share everything up to the argv, which is the point:
//
//	golunch run kilo-work -- ls -la          passthrough, stdio inherited
//	golunch run kilo-work --prompt "hi"      headless, normalized event stream
//
// Both hand off to package golunch; this function only turns flags into its
// options struct.
func (a *App) CmdRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	var (
		prompt    = fs.String("prompt", "", "run headless with this prompt instead of passing args through")
		stdin     = fs.Bool("stdin", false, "read the prompt from stdin (headless mode)")
		model     = fs.String("model", "", "model id, e.g. anthropic/claude-sonnet-4-5")
		provider  = fs.String("provider", "", "provider name (cline -P)")
		session   = fs.String("session", "", "session id to continue")
		cont      = fs.Bool("continue", false, "continue the most recent session")
		fork      = fs.Bool("fork", false, "fork the session before continuing")
		plan      = fs.Bool("plan", false, "plan/act mode where the agent supports it")
		approve   = fs.Bool("auto-approve", true, "approve tool use without asking (cline --auto-approve)")
		thinking  = fs.String("thinking", "", "none|low|medium|high|xhigh")
		title     = fs.String("title", "", "session title")
		attach    = fs.String("attach", "", "attach to a running server URL")
		workdir   = fs.String("cwd", "", "working directory for the agent")
		timeout   = fs.Duration("timeout", 0, "kill the whole process group after this long, e.g. 90s")
		proxySpec = fs.String("proxy", "", "proxy URL, profile name, or \"none\"")
		key       = fs.String("key", "", "API key injected through the driver's env var, never argv")
		jsonl     = fs.Bool("jsonl", false, "emit normalized events as NDJSON on stdout")
		quiet     = fs.Bool("quiet", false, "suppress progress on stderr, print only the answer")
		dryRun    = fs.Bool("dry-run", false, "print the argv and environment, start nothing")
		noProxy   stringList
		envPairs  stringList
		files     stringList
		extra     stringList
	)
	fs.Var(&noProxy, "noproxy", "extra NO_PROXY entry, repeatable")
	fs.Var(&envPairs, "env", "K=V for the child, repeatable; K= unsets")
	fs.Var(&files, "file", "attachment path, repeatable")
	fs.Var(&extra, "arg", "verbatim argv for the agent, repeatable")

	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 1 {
		return usageErr("usage: golunch run <alias> [flags] [-- passthrough-args...]")
	}
	alias := rest[0]
	passthrough := rest[1:]
	headless := *prompt != "" || *stdin
	if !headless && len(passthrough) == 0 {
		return usageErr("golunch run %s needs either --prompt/--stdin or arguments after --", alias)
	}

	inst, meta, err := a.Open(alias)
	if err != nil {
		return err
	}
	extraEnv, err := envMap(envPairs)
	if err != nil {
		return err
	}
	res, err := a.Resolve(meta, *proxySpec, commaSplit(noProxy))
	if err != nil {
		return err
	}
	e, err := a.BuildEnv(inst, &meta, res, extraEnv, a.KeepVarsFor(&meta))
	if err != nil {
		return err
	}
	inst.EnsureStorage()

	if !headless {
		return a.RunPassthrough(ctx, inst, &meta, res, e, passthrough, *dryRun)
	}

	text := *prompt
	if *stdin {
		b, err := io.ReadAll(a.In)
		if err != nil {
			return fmt.Errorf("read prompt from stdin: %w", err)
		}
		text = strings.TrimRight(string(b), "\n")
	}

	_, err = a.Prompt(ctx, golunch.PromptOptions{
		Alias: alias, Prompt: text, Model: *model, Provider: *provider,
		Thinking: *thinking, Session: *session, Title: *title, Attach: *attach,
		WorkDir: *workdir, Files: commaSplit(files), Extra: []string(extra),
		NoProxy: commaSplit(noProxy), Env: extraEnv, Proxy: *proxySpec, Key: *key,
		Continue: *cont, Fork: *fork, Plan: *plan, AutoApprove: *approve,
		Timeout: *timeout, Stream: true, JSONL: *jsonl, Quiet: *quiet, DryRun: *dryRun,
	})
	return err
}
