// Package golunch runs the same agent CLI as many times as you want, each with
// its own private config, its own login, and its own proxy — without
// containers, namespaces, or a daemon.
//
// An instance is a directory plus a set of environment variables: a private
// HOME, every XDG base directory, TMPDIR, and a bin/ on PATH. Nothing is
// namespaced and nothing runs in the background; the isolation is real because
// the agent is told, correctly, where its files live.
//
// This package is that mechanism as a library. Runner is one handle over the
// instances under a data root: it resolves an instance, decides the child
// environment for it, and runs a headless prompt to completion while handing you
// each normalized event. The `golunch` command in internal/cli is a thin shell
// over these same methods, which is why an alias invoked directly and
// `golunch run <alias> --` cannot disagree about the environment they provide.
//
// # Use as a library
//
//	cfg, err := config.Load()
//	r := golunch.NewRunner(cfg, os.Stdin, os.Stdout, os.Stderr, instance.HostFromOs())
//
//	res, err := r.Prompt(ctx, golunch.PromptOptions{
//	    Alias:   "work",
//	    Prompt:  "what tests are failing?",
//	    Timeout: 5 * time.Minute,
//	    OnEvent: func(ev agent.Event) bool { return true },
//	})
//
// Nothing here reads the environment or opens a terminal on its own: the data
// root comes from the config.Config you pass in, and the streams from the
// writers. That is what makes a run testable in-process, and the suite does it
// against recorded transcripts rather than a live model.
//
// Proxy support is environment-variable injection only. This package does not
// intercept traffic, rewrite base URLs, or run a proxy server.
package golunch

import (
	"fmt"
	"io"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/proxy"
)

// Runner is one handle on the instance store: a data root, the resolved global
// configuration, and the streams a run writes to. It holds no lock, opens no
// port, and keeps no state between calls, so one Runner is as safe to share as
// the configuration it was built from.
type Runner struct {
	Cfg  config.Config
	Out  io.Writer
	Err  io.Writer
	In   io.Reader
	Host instance.HostEnv
}

// NewRunner builds a Runner from already-resolved inputs. It deliberately reads
// no environment variable and touches no descriptor of its own, so a caller that
// wants a run somewhere other than ~/.golunch says so in cfg rather than in the
// ambient environment.
func NewRunner(cfg config.Config, in io.Reader, out, errw io.Writer, host instance.HostEnv) *Runner {
	return &Runner{Cfg: cfg, Out: out, Err: errw, In: in, Host: host}
}

func (r *Runner) print(s string) { fmt.Fprint(r.Out, s) }
func (r *Runner) printf(f string, a ...any) {
	fmt.Fprintf(r.Out, f, a...)
}
func (r *Runner) warnf(f string, a ...any) {
	fmt.Fprintf(r.Err, f, a...)
}

// InstancesDir and BinDir honour config overrides while defaulting under the
// resolved data root.
func (r *Runner) InstancesDir() string {
	if r.Cfg.Paths.InstancesDir != "" {
		return r.Cfg.Paths.InstancesDir
	}
	return r.Cfg.Root + "/instances"
}

func (r *Runner) BinDir() string {
	if r.Cfg.Paths.BinDir != "" {
		return r.Cfg.Paths.BinDir
	}
	return config.RealHome() + "/.local/bin"
}

// Open loads an existing instance and its metadata. A missing instance is an
// error with exit code ExitNotFound, which is how a caller tells "you did not
// create that yet" from "the run failed".
func (r *Runner) Open(alias string) (*instance.Instance, instance.Metadata, error) {
	inst, err := instance.New(r.InstancesDir(), alias)
	if err != nil {
		return nil, instance.Metadata{}, err
	}
	if !inst.Exists() {
		return nil, instance.Metadata{}, NotFoundErr("no instance %q (created with `golunch new %s --agent <name>`)",
			alias, alias)
	}
	meta, err := inst.LoadMetadata()
	if err != nil {
		return nil, instance.Metadata{}, err
	}
	return inst, meta, nil
}

// Resolve applies the proxy precedence chain for one run: flag over instance
// over global over inheritance.
func (r *Runner) Resolve(meta instance.Metadata, flagSpec string, flagNoProxy []string) (proxy.Resolution, error) {
	globalExtra := r.Cfg.Proxy.NoProxyExtra
	// NO_PROXY exceptions given on the command line belong to the winning
	// layer, but they are additive at every layer, so they ride along whatever
	// profile ends up chosen.
	res, err := proxy.Resolve(flagSpec, meta.Proxy.Spec, r.Cfg.Proxy.Default,
		r.Cfg.Proxy.Profiles, globalExtra)
	if err != nil {
		return res, err
	}
	res.NoProxyExtra = proxy.MergeEntries(res.NoProxyExtra, flagNoProxy)
	return res, nil
}

// ResolveSource names the precedence layer that would win for this instance, for
// the PROXY column of `golunch ls -l` and for `golunch doctor`.
func (r *Runner) ResolveSource(meta instance.Metadata) proxy.Source {
	res, err := r.Resolve(meta, "", nil)
	if err != nil {
		// An unresolvable profile name is reported by doctor as its own
		// finding; for a column we show the failure inline rather than
		// dropping the row.
		return proxy.Source("invalid proxy: " + err.Error())
	}
	return res.Source
}

// BuildEnv assembles the child environment for one instance, shared by run,
// shell, task and the launcher generator so no path invents its own.
func (r *Runner) BuildEnv(inst *instance.Instance, meta *instance.Metadata, res proxy.Resolution, extra map[string]string, keep []string) (*instance.Env, error) {
	return instance.BuildEnv(instance.Request{
		Host:     r.Host,
		Inst:     inst,
		Meta:     meta,
		Proxy:    res,
		KeepVars: keep,
		Extra:    extra,
	})
}

// KeepVarsFor is the union of what every candidate driver needs and what the
// user configured. A missing CA bundle is the difference between an instance
// that works and one that mysteriously cannot reach any endpoint.
func (r *Runner) KeepVarsFor(meta *instance.Metadata) []string {
	seen := map[string]bool{}
	var out []string
	add := func(vs []string) {
		for _, v := range vs {
			if v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	add(r.Cfg.Defaults.KeepVars)
	if meta != nil {
		add(meta.Launch.KeepVars)
	}
	if meta != nil && meta.Instance.Agent != "" {
		if e, ok := agent.Lookup(meta.Instance.Agent); ok {
			add(e.Env.KeepVars)
		}
	}
	return out
}
