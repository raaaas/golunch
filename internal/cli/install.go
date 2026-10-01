package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/install"
	"github.com/raaaas/golunch/instance"
)

// defaultInstallTimeout bounds the vendor installer's run. It is deliberately
// not cfg.Defaults.Timeout: that is 0 (unbounded) because a `make` the user
// chose to run may take as long as it takes, and wrong for a child process
// downloading from somebody else's server. package install applies the same
// floor when Spec.Timeout is 0; naming it on the flag is what lets `golunch
// install --help` tell the user the real number.
const defaultInstallTimeout = 10 * time.Minute

// installOpts is one install request with the flags already resolved. It is a
// struct rather than a long parameter list because both `golunch install` and
// `new --install` build it from their own FlagSets and hand it to the same
// core.
type installOpts struct {
	url       string
	script    string
	sha256    string
	version   string
	timeout   time.Duration
	yes       bool
	dryRun    bool
	proxySpec string
}

// CmdInstall downloads a missing agent CLI into an existing instance's own
// tree and points the instance at it.
//
//	golunch install work --url https://example.invalid/install.sh
//
// Until now golunch could only wrap a binary the host already had; this is the
// other half. The fetch is explicit (a named verb, an explicit --url/--script,
// a printed sha256 and a terminal confirmation), and the installer runs with
// the instance's redirected environment, so a $HOME-respecting script lands
// inside the tree and `golunch rm` deletes the download with it.
func (a *App) CmdInstall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	var (
		url       = fs.String("url", "", "vendor installer script to fetch over HTTPS")
		script    = fs.String("script", "", "path to an installer script already on disk (no network)")
		sha256Pin = fs.String("sha256", "", "refuse to run the script unless it hashes to this hex digest")
		version   = fs.String("version", "", "version to pin, handed to the installer")
		timeout   = fs.Duration("timeout", defaultInstallTimeout, "kill the installer and its process group after this long")
		yes       = fs.Bool("yes", false, "run the installer without asking")
		dryRun    = fs.Bool("dry-run", false, "print the resolved plan and fetch, run or write nothing")
		proxySpec = fs.String("proxy", "", "proxy URL, profile name, or \"none\" for this install")
	)
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: golunch install <alias> [--url <https-url> | --script <path>] [--sha256 <hex>] [--version <v>] [--timeout 10m] [--yes] [--dry-run] [--proxy <url|profile|none>]")
	}
	alias := rest[0]

	inst, meta, err := a.Open(alias)
	if err != nil {
		return err
	}
	if meta.Instance.Agent == "" {
		return usageErr("instance %q wraps a bare binary and names no agent, so there is nothing golunch could install for it.\n  recreate it with: golunch new %s --agent <name> --install --url <installer>", alias, alias)
	}
	entry, ok := agent.Lookup(meta.Instance.Agent)
	if !ok {
		return notFoundErr("instance %q names agent %q, which is not in the registry (known: %s)",
			alias, meta.Instance.Agent, agentList())
	}
	return a.installInto(ctx, entry, inst, &meta, installOpts{
		url: *url, script: *script, sha256: *sha256Pin, version: *version,
		timeout: *timeout, yes: *yes, dryRun: *dryRun, proxySpec: *proxySpec,
	})
}

// installInto is the whole install transaction with the agent entry as a
// parameter, which is the seam buildSeedPlan set (seed.go): the offline tests
// hand it a fabricated entry instead of a registry row, and `new --install`
// and `golunch install` share everything else. The caller supplies an opened
// instance and its loaded metadata; on success the metadata is updated,
// saved, and the launcher regenerated.
func (a *App) installInto(ctx context.Context, entry agent.Entry, inst *instance.Instance, meta *instance.Metadata, opts installOpts) error {
	url, script := opts.url, opts.script
	if url == "" && script == "" && entry.Install != nil {
		// A registry row that has grown a verified URL makes the plain
		// `golunch install <alias>` form work; until then every row ships an
		// empty ScriptURL and the flags below are the only way in.
		url = entry.Install.ScriptURL
	}
	switch {
	case url != "" && script != "":
		return usageErr("give either --url or --script, not both: one downloads the installer, the other points at a file you already approved")
	case script != "":
		if _, err := os.Stat(script); err != nil {
			return notFoundErr("--script %s: %v", script, err)
		}
	case url != "":
		// The fetched-by-URL path; install.Fetch validates the scheme later,
		// and validation before the lock is what keeps a typo from becoming a
		// half-acquired install.
	default:
		return usageErr("golunch has no verified installer URL for %q: every registry row ships an empty ScriptURL until the vendor docs are checked, so install names the script itself.\n"+
			"  --url <https-url>   fetch the vendor installer over HTTPS\n"+
			"  --script <path>     run an installer script that is already on disk\n"+
			"  (or install the agent on the host yourself and wrap it: golunch new %s --agent %s --binary <path>)",
			entry.Name, inst.Alias, entry.Name)
	}

	res, err := a.Resolve(*meta, opts.proxySpec, nil)
	if err != nil {
		return err
	}
	e, err := a.BuildEnv(inst, meta, res, nil, a.KeepVarsFor(meta))
	if err != nil {
		return err
	}
	shell := firstNonEmpty(meta.Instance.Shell, a.Cfg.Defaults.Shell, "/bin/bash")
	target := filepath.Join(inst.BinDir(), entry.Binary)

	if opts.dryRun {
		source := "local script " + script
		if url != "" {
			source = "fetch " + url
		}
		a.printf("install (dry run) %s\n  root:     %s\n  script:   %s\n", inst.Alias, inst.Root, source)
		switch {
		case opts.sha256 != "":
			a.printf("  sha256:   %s (pinned; any other digest aborts before anything runs)\n", opts.sha256)
		case url != "":
			a.printf("  sha256:   unknown until fetched (pass --sha256 to pin it)\n")
		}
		a.printf("  proxy:    %s\n  target:   %s\n\nnothing was fetched, run, or written.\n", res.Source, target)
		return nil
	}

	// BuildEnv has already written the instance paths into Env, but the
	// directories themselves must exist: Install reads PATH= out of that slice
	// to stat the entry's prereqs, and the audit copy needs logs/ the moment
	// the script is staged.
	inst.EnsureStorage()

	// Exclusive, like rm and seed: an installer replacing bin/<binary> under a
	// running agent is the one thing the lock exists to stop, and a busy
	// instance must cost ExitBusy rather than a half-written tree.
	lock, err := inst.Acquire(instance.Exclusive)
	if err != nil {
		return busyErr("%s is busy: an agent is running in it, or another admin command holds it", inst.Alias)
	}
	defer lock.Release()

	spec := install.Spec{
		Entry:        entry,
		Inst:         inst,
		Env:          e.Slice(),
		ScriptSHA256: opts.sha256,
		PinVersion:   opts.version,
		Shell:        shell,
		Proxy:        res,
		Timeout:      opts.timeout,
	}

	// Consent needs the two-call path: fetch to scratch, print the URL and the
	// hash of exactly the bytes that will run, ask, and only then hand the
	// saved file to Install with the pin attached. Install promotes that file
	// into the logs/ audit copy and executes that copy, so what ran is what
	// was shown. The single-call ScriptURL path has no hook between fetch and
	// exec, which is why nothing here takes that shortcut.
	pending := ""
	if url != "" {
		pending = filepath.Join(inst.TmpDir(), "pending.sh")
		fetched, err := install.Fetch(ctx, url, pending, 0, res)
		if err != nil {
			_ = os.Remove(pending)
			return err
		}
		spec.ScriptPath = pending
		if spec.ScriptSHA256 == "" {
			// Pin what the human is about to be shown: a file swapped between
			// this print and the exec must fail the hash, not pass consent.
			spec.ScriptSHA256 = fetched
		}
		a.printf("installer: %s\n  sha256:    %s\n", url, fetched)
	} else {
		spec.ScriptPath = script
		a.printf("installer: %s (local file, nothing fetched)\n", script)
		if opts.sha256 != "" {
			a.printf("  sha256:    %s (pinned)\n", opts.sha256)
		}
	}
	if entry.Install != nil && entry.Install.Note != "" {
		a.printf("  note:      %s\n", entry.Install.Note)
	}
	if !opts.yes && !a.confirm(fmt.Sprintf("run this installer in instance %q", inst.Alias)) {
		if pending != "" {
			_ = os.Remove(pending)
		}
		a.warnf("left untouched.\n")
		return nil
	}

	result, err := install.Install(ctx, spec)
	if pending != "" {
		// Success renamed it into logs/; this is the refusal, pin-mismatch and
		// crash sweep so tmp/ never keeps an executable scratch copy.
		_ = os.Remove(pending)
	}
	if err != nil {
		// Install returns a populated partial Result on most failure paths, so
		// the audit copy inside logs/ is named by the error itself. A killed
		// or nonzero installer maps to the child's code, not a bare failure:
		// a scripted caller checking $? must be able to tell timeout from a
		// bug in the vendor script.
		switch {
		case result.TimedOut:
			return CommandError{Code: ExitTimeout, Err: err}
		case result.ExitCode != 0:
			return CommandError{Code: result.ExitCode, Err: err}
		}
		return err
	}

	if len(result.Escaped) > 0 {
		a.warnf("WARNING: the installer for %q created paths on the host, outside the instance: %s\n",
			entry.Name, strings.Join(result.Escaped, ", "))
		a.warnf("This install is not private to %s. Those host paths are shared state now; golunch rm %s deletes the instance copy but not what leaked.\n",
			inst.Alias, inst.Alias)
	}

	// Result has no Dir field, so compute the honest one: LocateIn is the same
	// deterministic walk the install used a moment ago, and its answer is the
	// directory inside the tree that actually holds the installed file.
	dir := inst.HomeDir()
	if found, lerr := agent.LocateIn(entry, inst.HomeDir()); lerr == nil {
		dir = filepath.Dir(found)
	}

	meta.Launch.Command = []string{result.BinaryPath}
	meta.Launch.Binary = filepath.Base(result.BinaryPath)
	if result.Version != "" {
		meta.Instance.Version = result.Version
	}
	meta.Install = &instance.InstallInfo{
		Agent:      entry.Name,
		URL:        url,
		SHA256:     result.SHA256,
		ScriptPath: result.ScriptPath,
		Version:    result.Version,
		Dir:        dir,
		Binary:     result.BinaryPath,
		FetchedAt:  result.FetchedAt,
	}
	meta.Touch()
	if err := inst.SaveMetadata(meta); err != nil {
		return err
	}
	// The single render path shared by new/refresh/clone, so the launcher
	// cannot drift from the metadata that was just saved (and doctor's drift
	// check, which re-renders with no flag proxy, sees exactly this file).
	if err := a.writeLauncher(inst, meta, ""); err != nil {
		return err
	}

	version := result.Version
	if version == "" {
		version = "version unreported"
	}
	a.printf("installed %s\n  root:     %s\n  binary:   %s\n  version:  %s\n  script:   %s (sha256 %s)\n",
		entry.Binary, inst.Root, result.BinaryPath, version, relOf(inst.Root, result.ScriptPath), result.SHA256)
	a.printf("\n%s now runs its own copy of %q; a host install of the same name, if any, is untouched and unused. golunch rm %s deletes this copy with the tree.\n",
		inst.Alias, entry.Binary, inst.Alias)
	return nil
}
