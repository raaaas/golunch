package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/internal/osutil"
)

// CmdClone makes a second identity out of an existing instance.
//
// The default copies the command wiring and nothing else, so the new alias
// starts logged out and holds no session history. That is the useful case —
// two parallel agents that cannot see each other's context. --copy-data is the
// other case: a second live login for the same account, which the plan treats
// as worth a warning because it duplicates credentials.
func (a *App) CmdClone(args []string) error {
	fs := flag.NewFlagSet("clone", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	copyData := fs.Bool("copy-data", false, "also copy config and data, i.e. duplicate the login and sessions")
	proxySpec := fs.String("proxy", "", "proxy for the new instance (default: inherit the source's)")
	noProxy := fs.Bool("no-proxy", false, "start the clone with no proxy even if the source has one")
	link := fs.Bool("link", true, "expose the new alias as a command")
	yes := fs.Bool("yes", false, "do not ask for confirmation when copying credentials")
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return usageErr("usage: golunch clone <source> <new-alias> [--copy-data]")
	}
	srcAlias, dstAlias := rest[0], rest[1]

	src, srcMeta, err := a.Open(srcAlias)
	if err != nil {
		return err
	}
	dst, err := instance.New(a.InstancesDir(), dstAlias)
	if err != nil {
		return err
	}
	if dst.Exists() {
		return usageErr("%q already exists; remove it first with golunch rm %s", dstAlias, dstAlias)
	}
	if srcAlias == dstAlias {
		return usageErr("source and destination are the same alias")
	}

	dstMeta := instance.Metadata{}
	dstMeta.Instance = srcMeta.Instance
	dstMeta.Instance.Alias = dstAlias
	dstMeta.Instance.CreatedAt = time.Time{}
	dstMeta.Instance.UpdatedAt = time.Time{}
	dstMeta.Launch = srcMeta.Launch
	dstMeta.Proxy.Spec = srcMeta.Proxy.Spec
	dstMeta.Proxy.NoProxy = append([]string(nil), srcMeta.Proxy.NoProxy...)
	if *proxySpec != "" {
		dstMeta.Proxy.Spec = *proxySpec
	}
	if *noProxy {
		dstMeta.Proxy.Spec = "none"
	}

	if *copyData {
		if entry, ok := agent.Lookup(srcMeta.Instance.Agent); ok {
			secrets := entry.Env.Secrets(agent.Paths{
				Root: src.Root, Home: src.HomeDir(), Config: src.ConfigDir(),
				Data: src.DataDir(), State: src.StateDir(), Cache: src.CacheDir(),
				Tmp: src.TmpDir(), Bin: src.BinDir(),
			})
			var live []string
			for _, s := range secrets {
				if osutil.Exists(s) {
					live = append(live, s)
				}
			}
			if len(live) > 0 {
				a.warnf("--copy-data duplicates credentials for agent %q:\n  %s\n",
					srcMeta.Instance.Agent, strings.Join(live, "\n  "))
				a.warnf("both instances will then act as the same account, with the same quota and the " +
					"same audit trail in the provider's logs.\n")
				if !*yes && !a.confirm("copy the credentials anyway") {
					return nil
				}
			}
		}
	}

	lock, err := dst.Acquire(instance.Exclusive)
	if err != nil {
		return busyErr("%s is busy", dstAlias)
	}
	defer lock.Release()

	if err := dst.Create(); err != nil {
		return err
	}
	// bin/ first and always: this is what makes the clone able to run the same
	// program, including an instance-local override the source installed.
	if err := copyTree(src.BinDir(), dst.BinDir()); err != nil {
		return fmt.Errorf("copy bin: %w", err)
	}
	if *copyData {
		for _, d := range []string{"home", "config", "cache", "data", "state"} {
			if err := copyTree(src.Dir(d), dst.Dir(d)); err != nil {
				return fmt.Errorf("copy %s: %w", d, err)
			}
		}
	}

	dstMeta.Paths.Root = dst.Root
	dstMeta.Paths.Bin = dst.BinDir()
	dstMeta.Paths.Launcher = dst.LauncherPath()
	if *link {
		dstMeta.Paths.LinkPath = filepath.Join(a.BinDir(), dstAlias)
	} else {
		dstMeta.Paths.LinkPath = ""
	}

	// An instance that downloaded its own agent records absolute paths into the
	// source tree. bin/ was just copied, so the file itself is already here and
	// only the strings have to follow it; leaving them pointing at the source
	// would make the clone exec the source's binary, which is the isolation
	// guarantee quietly voided. home/ is the exception, because the default
	// clone does not copy it and a reprefixed path there describes a file that
	// was never carried.
	if in := srcMeta.Install; in != nil {
		cp := *in
		cp.Dir = reprefixSrc(cp.Dir, src.Root, dst.Root)
		cp.Binary = reprefixSrc(cp.Binary, src.Root, dst.Root)
		cp.ScriptPath = reprefixSrc(cp.ScriptPath, src.Root, dst.Root)
		// bin/ is copied and logs/ is not, so the audit copy of the installer
		// would be stranded in the source while [install] still named the
		// clone's path for it. It is small, it is the evidence behind
		// Install.SHA256, and doctor's drift check only means anything if the
		// file travels with the record that hashes it.
		if cp.ScriptPath != "" {
			if st, err := os.Stat(in.ScriptPath); err == nil && st.Mode().IsRegular() && within(src.Root, in.ScriptPath) {
				if err := copyFile(in.ScriptPath, cp.ScriptPath, st.Mode().Perm()); err != nil {
					a.warnf("could not carry the installer audit copy into %s: %v\n", dstAlias, err)
				}
			}
			if _, err := os.Stat(cp.ScriptPath); err != nil {
				cp.ScriptPath = ""
			}
		}
		dstMeta.Install = &cp
		srcCmd := ""
		if len(dstMeta.Launch.Command) > 0 {
			srcCmd = dstMeta.Launch.Command[0]
			cmd := append([]string(nil), dstMeta.Launch.Command...)
			cmd[0] = reprefixSrc(srcCmd, src.Root, dst.Root)
			dstMeta.Launch.Command = cmd
		}
		if !*copyData {
			// Which of the two recorded paths is stranded in the source's home/,
			// stated as the clone's own path so the user can see what is missing.
			stranded, want := "", ""
			if underHome(srcCmd, src.HomeDir()) {
				stranded, want = srcCmd, dstMeta.Launch.Command[0]
			} else if underHome(in.Binary, src.HomeDir()) {
				stranded, want = in.Binary, cp.Binary
			}
			if stranded != "" {
				a.warnf("%s keeps its downloaded binary under home/, which the default clone does not "+
					"copy: %s.\n", srcAlias, stranded)
				a.warnf("%s has nothing at %s; re-run golunch install %s, or clone again with "+
					"--copy-data.\n", dstAlias, want, dstAlias)
			}
		}
	}

	dstMeta.LastRun = nil
	dstMeta.Touch()

	if err := a.writeLauncher(dst, &dstMeta, ""); err != nil {
		return err
	}
	if err := dst.SaveMetadata(&dstMeta); err != nil {
		return err
	}

	a.printf("cloned %s -> %s\n  root: %s\n  data: %s\n", srcAlias, dstAlias, dst.Root,
		map[bool]string{true: "copied (same account)", false: "not copied (fresh login)"}[*copyData])
	if dstMeta.Paths.LinkPath != "" {
		a.printf("  command: %s\n", dstMeta.Paths.LinkPath)
	}
	if !*copyData && dstMeta.Instance.Agent != "" {
		a.printf("\n%s starts logged out. Run it to sign in as a separate identity.\n", dstAlias)
	}
	return nil
}

// copyTree walks a directory. Symlinks are recreated as symlinks rather than
// followed, because a node_modules tree full of package symlinks would
// otherwise be duplicated as real files — and a dangling link must not abort the
// copy, since an instance often holds one pointing at the host.
func copyTree(src, dst string) error {
	if !dirExists(src) {
		return nil
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if !within(dst, target) {
			return fmt.Errorf("clone target %s escapes %s", target, dst)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return nil
		}
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// within is the traversal guard: metadata is hand-editable, and a clone that
// followed a recorded path out of the instance tree could overwrite anything the
// user can read. The check is on the target itself, not its parent, so the root
// directory of the copy — whose relative name is "." — is not rejected.
func within(root, target string) bool {
	rp, err := filepath.EvalSymlinks(root)
	if err != nil {
		rp = filepath.Clean(root)
	}
	tp := filepath.Clean(target)
	if tp == rp {
		return true
	}
	return strings.HasPrefix(tp, rp+string(filepath.Separator))
}

// reprefixSrc moves one path recorded by the installer out of the source tree
// and into the clone's. A path that is not under the source root is returned
// untouched: metadata.toml is hand-editable, and rewriting the prefix of a host
// path such as /usr/local/bin/cline would convert an escape doctor can name into
// a fabricated in-tree path that looks legitimate.
func reprefixSrc(p, srcRoot, dstRoot string) string {
	if p == "" || !pathUnder(srcRoot, p) {
		return p
	}
	rel, err := filepath.Rel(srcRoot, p)
	if err != nil {
		return p
	}
	return filepath.Join(dstRoot, rel)
}

// underHome names the one installed location clone cannot follow by rewriting:
// a curl installer honoring $HOME drops the binary under home/, and the default
// clone deliberately does not copy home/, so the path moves but the file does
// not.
func underHome(p, home string) bool {
	return p != "" && pathUnder(home, p)
}

func refreshAll(a *App) (int, error) {
	names, err := instance.List(a.InstancesDir())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names {
		inst, err := instance.New(a.InstancesDir(), name)
		if err != nil {
			a.warnf("%s: %v\n", name, err)
			continue
		}
		meta, err := inst.LoadMetadata()
		if err != nil {
			a.warnf("%s: %v\n", name, err)
			continue
		}
		lock, err := inst.Acquire(instance.Exclusive)
		if err != nil {
			a.warnf("%s is busy, skipped\n", name)
			continue
		}
		err = a.writeLauncher(inst, &meta, "")
		lock.Release()
		if err != nil {
			return n, fmt.Errorf("%s: %w", name, err)
		}
		n++
	}
	return n, nil
}

// CmdRefresh regenerates every launcher, which is the fix after editing
// metadata.toml or upgrading golunch itself.
func (a *App) CmdRefresh(args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	alias := ""
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 1 {
		alias = rest[0]
	} else if len(rest) > 1 {
		return usageErr("usage: golunch refresh [alias]")
	}
	if alias != "" {
		inst, meta, err := a.Open(alias)
		if err != nil {
			return err
		}
		lock, err := inst.Acquire(instance.Exclusive)
		if err != nil {
			return busyErr("%s is busy", alias)
		}
		defer lock.Release()
		if err := a.writeLauncher(inst, &meta, ""); err != nil {
			return err
		}
		a.printf("regenerated %s\n", inst.LauncherPath())
		return nil
	}
	n, err := refreshAll(a)
	if err != nil {
		return err
	}
	a.printf("regenerated %d launcher(s)\n", n)
	return nil
}
