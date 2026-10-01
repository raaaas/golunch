package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/instance"
)

// CmdNew wraps an existing host binary into an isolated instance.
//
//	golunch new kilo-work --agent kilo --proxy http://127.0.0.1:7890
//
// It takes the context because `--install` runs a vendor installer over the
// network: without it a Ctrl-C could not reach that child, and the whole point of
// execd is that nothing an instance started can outlive the command.
func (a *App) CmdNew(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	var (
		agentName   = fs.String("agent", "", "agent from the registry ("+agentList()+")")
		binary      = fs.String("binary", "", "path to the host executable to wrap (default: located on PATH)")
		proxySpec   = fs.String("proxy", "", "proxy URL, profile name, or \"none\"")
		noProxy     stringList
		shell       = fs.String("shell", "", "value exported as $SHELL inside the instance")
		link        = fs.Bool("link", true, "expose the alias as a command in ~/.local/bin")
		native      = fs.Bool("native", true, "pass the agent's own isolation flags on the headless path")
		force       = fs.Bool("force", false, "regenerate the launcher of an existing instance")
		version     = fs.String("version", "", "record this version string instead of probing the binary")
		doInstall   = fs.Bool("install", false, "download a missing agent CLI into the new instance's own bin (needs --url or --script)")
		instYes     = fs.Bool("yes", false, "run the fetched installer without asking")
		instURL     = fs.String("url", "", "vendor installer script URL to fetch when --install is used")
		instScript  = fs.String("script", "", "installer script on disk to run when --install is used")
		instSHA     = fs.String("sha256", "", "refuse the installer unless it hashes to this hex digest")
		instTimeout = fs.Duration("timeout", defaultInstallTimeout, "kill the installer after this long")
		keep        stringList
		root        = fs.String("root", "", "override the instances directory")
	)
	fs.Var(&noProxy, "noproxy", "NO_PROXY exception, repeatable")
	fs.Var(&keep, "keep", "host variable to copy into this instance, repeatable")
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: golunch new <alias> --agent <name> [flags]")
	}
	alias := rest[0]

	instDir := a.InstancesDir()
	if root != nil && *root != "" {
		instDir = *root
	}

	inst, err := instance.New(instDir, alias)
	if err != nil {
		return err
	}
	if inst.Exists() && !*force {
		return CommandError{Code: ExitUsage, Err: fmt.Errorf("instance %q already exists at %s (use --force to rewrite its launcher)", alias, inst.Root)}
	}

	meta := instance.Metadata{}
	meta.Instance.Alias = alias
	meta.Instance.Shell = firstNonEmpty(*shell, a.Cfg.Defaults.Shell, "/bin/bash")
	meta.Proxy.Spec = *proxySpec
	meta.Proxy.NoProxy = commaSplit(noProxy)
	meta.Launch.KeepVars = commaSplit(keep)

	// --install is opt-in downloading, and it reorders this command: the tree
	// and its environment must exist before an installer can be run into them,
	// so this branch creates the instance from the provisional meta above and
	// hands off to installInto, which fills in the launch command. Everything
	// below this branch is the original wrap-a-host-binary path, untouched.
	if *doInstall {
		if *agentName == "" {
			return usageErr("--install needs --agent: the registry row names the binary the installer must produce (with --binary there is nothing to download)")
		}
		if *binary != "" {
			return usageErr("--install and --binary are alternatives: --install downloads the agent into the instance's own bin, --binary wraps a host copy")
		}
		entry, ok := agent.Lookup(*agentName)
		if !ok {
			return notFoundErr("unknown agent %q; known agents: %s", *agentName, agentList())
		}
		if hostBin, err := agent.Locate(entry); err == nil {
			// Downloading something that already exists is never the quiet
			// answer; the user gets the host wrapper and says so in the report.
			a.warnf("%q is already installed on this host at %s; --install downloaded nothing and %s wraps that copy.\n",
				entry.Name, hostBin, alias)
		} else {
			return a.newWithInstall(ctx, inst, &meta, entry, *native, *link, installOpts{
				url: *instURL, script: *instScript, sha256: *instSHA,
				version: *version, timeout: *instTimeout, yes: *instYes,
				proxySpec: *proxySpec,
			})
		}
	}

	if *agentName != "" {
		entry, ok := agent.Lookup(*agentName)
		if !ok {
			return notFoundErr("unknown agent %q; known agents: %s", *agentName, agentList())
		}
		meta.Instance.Agent = entry.Name
		bin, err := a.resolveBinary(entry, *binary)
		if err != nil {
			return err
		}
		meta.Launch.Command = []string{bin}
		meta.Launch.Binary = filepath.Base(bin)
		meta.Launch.NativeFlags = *native && entry.SupportNative
		if *version != "" {
			meta.Instance.Version = *version
		} else {
			meta.Instance.Version = agent.Version(bin, entry.VersionArgs)
		}
	} else {
		if *binary == "" {
			return usageErr("an instance without --agent needs --binary so golunch knows what to exec")
		}
		bin, err := filepath.Abs(*binary)
		if err != nil {
			return err
		}
		meta.Launch.Command = []string{bin}
		meta.Launch.Binary = filepath.Base(bin)
		if *version != "" {
			meta.Instance.Version = *version
		}
	}

	if err := inst.Create(); err != nil {
		return err
	}
	meta.Paths.Root = inst.Root
	meta.Paths.Bin = inst.BinDir()
	meta.Paths.Launcher = inst.LauncherPath()
	if *link {
		meta.Paths.LinkPath = filepath.Join(a.BinDir(), alias)
	}

	if err := a.writeLauncher(inst, &meta, ""); err != nil {
		return err
	}
	meta.Touch()
	if err := inst.SaveMetadata(&meta); err != nil {
		return err
	}

	a.printf("created %s\n  root:     %s\n  command:  %s\n", alias, inst.Root, meta.Launch.Command[0])
	if meta.Instance.Agent != "" {
		v := meta.Instance.Version
		if v == "" {
			v = "unknown"
		}
		a.printf("  agent:    %s (%s)\n", meta.Instance.Agent, v)
	}
	if meta.Paths.LinkPath != "" {
		a.printf("  command:  %s\n", meta.Paths.LinkPath)
		if dir := filepath.Dir(meta.Paths.LinkPath); !inPath(dir, a.Host.Path) {
			a.warnf("note: %s is not on your PATH; add it or invoke the launcher by path\n", dir)
		}
	}
	if meta.Instance.Agent != "" {
		a.printf("\nThis instance starts logged out. Run %q to sign in without touching your other %s setup.\n",
			alias, meta.Instance.Agent)
	}
	return nil
}

// newWithInstall is the --install half of CmdNew: create the tree from
// provisional metadata first (installInto needs the instance's own
// directories and env), download the agent into it, and let installInto save
// the metadata and write the launcher it has filled in. Without --install
// CmdNew never comes here.
func (a *App) newWithInstall(ctx context.Context, inst *instance.Instance, meta *instance.Metadata, entry agent.Entry, native, link bool, opts installOpts) error {
	created := !inst.Exists()
	meta.Instance.Agent = entry.Name
	meta.Launch.NativeFlags = native && entry.SupportNative
	if opts.version != "" {
		meta.Instance.Version = opts.version
	}
	if err := inst.Create(); err != nil {
		return err
	}
	meta.Paths.Root = inst.Root
	meta.Paths.Bin = inst.BinDir()
	meta.Paths.Launcher = inst.LauncherPath()
	if link {
		meta.Paths.LinkPath = filepath.Join(a.BinDir(), inst.Alias)
	}
	if err := a.installInto(ctx, entry, inst, meta, opts); err != nil {
		return err
	}
	if meta.Install == nil {
		// Declined at consent, or a dry run: nothing was installed. The dirs
		// this call mkdir'd have no metadata and no command behind them, so
		// "nothing happened" means removing exactly what did not exist before.
		if created {
			if err := inst.Destroy(); err != nil {
				return err
			}
			if !opts.dryRun {
				a.printf("%s not created: the installer was not approved.\n", inst.Alias)
			}
		} else if !opts.dryRun {
			a.printf("%s unchanged: the installer was not approved.\n", inst.Alias)
		}
		return nil
	}
	a.printf("created %s\n  root:     %s\n  command:  %s\n", inst.Alias, inst.Root, meta.Launch.Command[0])
	v := meta.Instance.Version
	if v == "" {
		v = "unknown"
	}
	a.printf("  agent:    %s (%s)\n", meta.Instance.Agent, v)
	if meta.Paths.LinkPath != "" {
		a.printf("  command:  %s\n", meta.Paths.LinkPath)
		if dir := filepath.Dir(meta.Paths.LinkPath); !inPath(dir, a.Host.Path) {
			a.warnf("note: %s is not on your PATH; add it or invoke the launcher by path\n", dir)
		}
	}
	a.printf("\nThis instance starts logged out. Run %q to sign in without touching your other %s setup.\n",
		inst.Alias, meta.Instance.Agent)
	return nil
}

// writeLauncher renders and installs the launcher for one instance. Shared with
// `refresh` and `clone` so a generated script is always produced by this one
// call path.
func (a *App) writeLauncher(inst *instance.Instance, meta *instance.Metadata, flagProxy string) error {
	res, err := a.Resolve(*meta, flagProxy, nil)
	if err != nil {
		return err
	}
	e, err := a.BuildEnv(inst, meta, res, nil, a.KeepVarsFor(meta))
	if err != nil {
		return err
	}
	body := instance.RenderLauncher(inst.Alias, e, inst, meta.Launch.Command, res)
	return instance.WriteLauncher(inst.LauncherPath(), meta.Paths.LinkPath, body)
}

// keepVarsFor is the union of what every candidate driver needs and what the
// user configured. A missing CA bundle is the difference between an instance
// that works and one that mysteriously cannot reach any endpoint.
func (a *App) keepVarsFor(meta *instance.Metadata) []string {
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
	add(a.Cfg.Defaults.KeepVars)
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

func (a *App) resolveBinary(entry agent.Entry, override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", notFoundErr("binary %s: %v", override, err)
		}
		return abs, nil
	}
	bin, err := agent.Locate(entry)
	if err != nil {
		return "", notFoundErr("could not find %q on this host. Install it, or point --binary at it.\n  searched: %s",
			entry.Binary, searchNote(entry, a.Host.Path))
	}
	return bin, nil
}

func searchNote(entry agent.Entry, path string) string {
	return fmt.Sprintf("PATH=%s plus %v", path, entry.KnownDirs)
}

func agentList() string { return strings.Join(agent.Names(), ", ") }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func inPath(dir, path string) bool {
	if dir == "" {
		return false
	}
	for _, p := range filepath.SplitList(path) {
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}
