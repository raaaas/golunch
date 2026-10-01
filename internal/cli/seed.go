package cli

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/instance"
)

// CmdSeed hands an instance the host agent's MCP servers, skills and plugin
// configuration without handing it the login.
//
// Isolation and this command are the same mechanism seen from opposite sides:
// redirecting HOME and the XDG dirs is what makes a new instance start logged
// out, and it is also why that instance sees no MCP server, no skill and no
// plugin. Rather than weaken isolation, seed copies the specific files the
// driver declares. Everything comes from agent.Entry.Seed, so a new agent
// contributes data here and never copy logic.
//
// Credential files are refused by name. cline keeps
// data/settings/providers.json one directory away from its MCP settings and it
// holds apiKeys and accessTokens in plaintext, so "copy the settings dir" is not
// a safe shorthand for "copy the MCP config".
func (a *App) CmdSeed(args []string) error {
	fl := flag.NewFlagSet("seed", flag.ContinueOnError)
	fl.SetOutput(a.Err)
	all := fl.Bool("all", false, "seed every declared group except dependency trees")
	withDeps := fl.Bool("with-deps", false, "also copy dependency trees such as node_modules (large)")
	dryRun := fl.Bool("dry-run", false, "print what would be copied and change nothing")
	force := fl.Bool("force", false, "overwrite files the instance already has")
	hostHome := fl.String("host-home", "", "read host configuration from this home instead of the real one")
	rest, err := splitArgs(fl, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageErr("usage: golunch seed <alias> [group...] [--all] [--dry-run]")
	}
	alias, want := rest[0], rest[1:]

	inst, meta, err := a.Open(alias)
	if err != nil {
		return err
	}
	entry, ok := agent.Lookup(meta.Instance.Agent)
	if !ok {
		return notFoundErr("instance %q names agent %q, which is not in the registry", alias, meta.Instance.Agent)
	}
	if entry.Seed == nil {
		return usageErr("agent %q declares no seedable paths; configure it inside the instance with `golunch shell %s`",
			entry.Name, alias)
	}

	host := hostPaths()
	if *hostHome != "" {
		if host, err = hostPathsUnder(*hostHome); err != nil {
			return err
		}
	}

	groups := want
	if *all {
		groups = append(groups, seedGroups(entry, host, false)...)
	}
	if *withDeps {
		groups = appendUnique(groups, "deps")
	}
	if len(groups) == 0 {
		return usageErr("say what to seed: %s, or --all", strings.Join(seedGroups(entry, host, true), "|"))
	}

	lock, err := inst.Acquire(instance.Exclusive)
	if err != nil {
		return busyErr("%s is busy", alias)
	}
	defer lock.Release()

	plan, note := buildSeedPlan(entry, host, inst, groups)
	for _, n := range note {
		a.printf("  %s\n", n)
	}
	if len(plan) == 0 {
		a.printf("\nnothing to seed: the host has none of the files these groups name.\n")
		return nil
	}
	var copied, kept int
	var warns []string
	for _, p := range plan {
		if p.exists && !*force {
			kept++
			a.printf("  keep    %s (already in the instance; --force overwrites)\n", relOf(inst.Root, p.dst))
			continue
		}
		if *dryRun {
			a.printf("  would   %s -> %s\n", shorten(p.src, host), relOf(inst.Root, p.dst))
			continue
		}
		if p.item.Tree {
			n, w, err := seedTree(p.src, p.dst, *force, host)
			if err != nil {
				return fmt.Errorf("seed %s: %w", p.item.Group, err)
			}
			copied += int(n)
			warns = append(warns, w...)
			a.printf("  tree    %s -> %s (%d files)\n", shorten(p.src, host), relOf(inst.Root, p.dst), n)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p.dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(p.src, p.dst, p.mode); err != nil {
			return fmt.Errorf("seed %s: %w", p.item.Group, err)
		}
		copied++
		if p.size < 1<<20 {
			if leak, found := secretsInFile(p.dst); found {
				// Tightened after the write rather than before, so what is on
				// disk is what was scanned.
				if err := os.Chmod(p.dst, 0o600); err != nil {
					return err
				}
				warns = append(warns, fmt.Sprintf("%s holds %s; written 0600", relOf(inst.Root, p.dst), leak))
			}
			warns = append(warns, hostPathRefs(p.dst, host)...)
		}
		a.printf("  copy    %s -> %s\n", shorten(p.src, host), relOf(inst.Root, p.dst))
	}
	if *dryRun {
		a.printf("\ndry run: %d item(s) would be seeded, nothing was written.\n", len(plan))
		return nil
	}

	summary := fmt.Sprintf("seeded %s (%d file%s copied", alias, copied, pluralGo(copied))
	if kept > 0 {
		summary += fmt.Sprintf(", %d left as the instance had them", kept)
	}
	a.printf("\n%s)\n", summary)
	for _, w := range dedupe(warns) {
		a.warnf("warn: %s\n", w)
	}
	if len(hostRefWarn(warns)) > 0 {
		a.warnf("\nAn absolute host path inside a seeded config file is not a hole in isolation that an " +
			"environment variable can plug: golunch redirects env, and a program told to open /home/... will " +
			"still reach the host copy. Repoint those entries at the instance bin/ if the instance must not " +
			"touch host state.\n")
	}
	a.printf("next: golunch doctor %s\n", alias)
	return nil
}

// seedAction is one file or tree chosen for copying, with the destination
// already resolved and guarded.
type seedAction struct {
	item   agent.SeedItem
	src    string
	dst    string
	size   int64
	mode   os.FileMode
	exists bool
}

// buildSeedPlan turns the driver's declarations into concrete copies, dropping
// what does not exist on the host, what belongs to an unasked group, anything
// the driver calls a credential, and any destination that would land outside the
// instance.
func buildSeedPlan(entry agent.Entry, host agent.HostPaths, inst *instance.Instance, groups []string) ([]seedAction, []string) {
	want := map[string]bool{}
	for _, g := range groups {
		want[g] = true
	}
	secrets := map[string]bool{}
	if entry.Env.Secrets != nil {
		for _, s := range entry.Env.Secrets(agent.Paths{
			Root: host.Home, Home: host.Home, Config: host.Config,
			Data: host.Data, State: host.State, Cache: host.Cache,
		}) {
			secrets[filepath.Clean(s)] = true
		}
	}

	var plan []seedAction
	var note []string
	var absent []string
	for _, item := range entry.Seed(host) {
		if !want[item.Group] {
			continue
		}
		src := filepath.Clean(item.Src)
		if secrets[src] {
			note = append(note, fmt.Sprintf("skip    %s — %s names it a credential file and seed never copies one",
				shorten(src, host), entry.Name))
			continue
		}
		info, err := os.Stat(src)
		if err != nil {
			// One line for all of them: a driver declares every spelling it
			// knows about, so a host missing six of them is the normal case,
			// not six lines of noise.
			absent = append(absent, filepath.Base(src))
			continue
		}
		dst := filepath.Join(inst.Root, filepath.FromSlash(item.Dst))
		if !within(inst.Root, dst) {
			note = append(note, fmt.Sprintf("skip    %s — destination %s is outside the instance", item.Group, item.Dst))
			continue
		}
		if item.Tree != info.IsDir() {
			note = append(note, fmt.Sprintf("skip    %s — declared as %s but the host has %s",
				shorten(src, host), map[bool]string{true: "a tree", false: "a file"}[item.Tree],
				map[bool]string{true: "a directory", false: "a file"}[info.IsDir()]))
			continue
		}
		plan = append(plan, seedAction{
			item: item, src: src, dst: dst, size: info.Size(),
			mode: info.Mode().Perm(), exists: pathExists(dst),
		})
	}
	if len(absent) > 0 {
		note = append(note, "absent  "+strings.Join(dedupe(absent), ", ")+" (host has none)")
	}
	return plan, note
}

// seedGroups lists the groups a driver declares, optionally including the
// dependency trees that are off by default.
func seedGroups(entry agent.Entry, host agent.HostPaths, includeDeps bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range entry.Seed(host) {
		if item.Group == "deps" && !includeDeps {
			continue
		}
		if !seen[item.Group] {
			seen[item.Group] = true
			out = append(out, item.Group)
		}
	}
	return out
}

// seedTree copies a directory without clobbering files the instance already has.
// Symlinks are recreated rather than followed, so a host node_modules tree does
// not become a second full copy of itself. A link pointing into the host home is
// reported for the same reason hostPathRefs reports it: it is a path out of the
// instance that no environment variable controls.
func seedTree(src, dst string, force bool, host agent.HostPaths) (int64, []string, error) {
	var n int64
	var warns []string
	err := filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if !within(dst, target) {
			return fmt.Errorf("%s escapes %s", target, dst)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(link) && pathUnder(host.Home, link) {
				warns = append(warns, fmt.Sprintf("%s is a symlink into the host at %s", relOf(dst, target), link))
			}
			if pathExists(target) && !force {
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			if pathExists(target) && !force {
				return nil
			}
			if err := copyFile(path, target, info.Mode().Perm()); err != nil {
				return err
			}
			n++
			return nil
		}
		return nil
	})
	return n, warns, err
}

// hostPathRefs finds absolute references to the host home inside a seeded file.
// Such a reference still resolves inside an instance — that is the problem: the
// agent reads and writes the host's copy of it, and never notices the instance
// exists.
func hostPathRefs(file string, host agent.HostPaths) []string {
	b := readSmall(file)
	if b == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range hostPathRe.FindAllString(string(b), -1) {
		if !pathUnder(host.Home, m) || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, fmt.Sprintf("%s references %s", filepath.Base(file), m))
	}
	return out
}

// hostPathRe is deliberately coarse: it matches any absolute path of two or
// more segments, and the caller keeps only those under the real home. A tighter
// pattern would need to know each agent's escaping rules, and the whole point of
// this check is to report what the file says rather than what golunch expects.
var hostPathRe = regexp.MustCompile(`(?:/[A-Za-z0-9._~+@-]+){2,}`)

// secretsInFile reports the credential-looking keys in a file, so a copied
// config that carries a key can be written 0600 and named out loud.
func secretsInFile(file string) (string, bool) {
	b := readSmall(file)
	if b == nil {
		return "", false
	}
	var found []string
	for _, m := range credKeyRe.FindAllStringSubmatch(string(b), -1) {
		found = append(found, m[1])
	}
	if len(found) == 0 {
		return "", false
	}
	return strings.Join(dedupe(found), ", "), true
}

var credKeyRe = regexp.MustCompile(`(?i)"(api[_-]?key|access[_-]?token|secret[_-]?key|password|token)"\s*:`)

func readSmall(file string) []byte {
	st, err := os.Stat(file)
	if err != nil || st.Size() > 1<<20 {
		return nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	return b
}

// hostPaths resolves the real user's config roots. Inherited environment
// variables are trusted only when they sit under the real home and this process
// is not itself inside an isolation tool: running golunch from a warren instance
// would otherwise make "the host config" mean that instance's nearly empty tree,
// and seed would report success having copied nothing.
func hostPaths() agent.HostPaths {
	h, err := hostPathsUnder(config.RealHome())
	if err != nil {
		return agent.HostPaths{Home: config.RealHome()}
	}
	return h
}

func hostPathsUnder(home string) (agent.HostPaths, error) {
	abs, err := filepath.Abs(home)
	if err != nil {
		return agent.HostPaths{}, fmt.Errorf("--host-home %q: %w", home, err)
	}
	h := agent.HostPaths{
		Home:   abs,
		Config: filepath.Join(abs, ".config"),
		Data:   filepath.Join(abs, ".local", "share"),
		State:  filepath.Join(abs, ".local", "state"),
		Cache:  filepath.Join(abs, ".cache"),
	}
	if nested() {
		return h, nil
	}
	for _, p := range []struct {
		env  string
		dest *string
	}{
		{"XDG_CONFIG_HOME", &h.Config},
		{"XDG_DATA_HOME", &h.Data},
		{"XDG_STATE_HOME", &h.State},
		{"XDG_CACHE_HOME", &h.Cache},
	} {
		v := os.Getenv(p.env)
		if v == "" || !filepath.IsAbs(v) || !pathUnder(abs, v) {
			continue
		}
		*p.dest = filepath.Clean(v)
	}
	return h, nil
}

// nested reports whether this golunch process is already inside an isolation
// tool's instance, in which case the inherited XDG variables describe that
// instance rather than the host.
func nested() bool {
	if os.Getenv("WARREN_INSTANCE") != "" || os.Getenv("GOLUNCH_INSTANCE") != "" {
		return true
	}
	for _, frag := range []string{"/.warren/instances/", "/.golunch/instances/"} {
		if strings.Contains(os.Getenv("HOME"), frag) || strings.Contains(os.Getenv("XDG_CONFIG_HOME"), frag) {
			return true
		}
	}
	return false
}

func pathUnder(root, p string) bool {
	r := filepath.Clean(root)
	c := filepath.Clean(p)
	return c == r || strings.HasPrefix(c, r+string(filepath.Separator))
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// relOf renders a path relative to the instance root, which is how the layout is
// documented and the only form worth printing: the absolute prefix is long and
// identical on every line.
func relOf(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

func shorten(p string, host agent.HostPaths) string {
	if r, err := filepath.Rel(host.Home, p); err == nil && !strings.HasPrefix(r, "..") {
		return "~/" + filepath.ToSlash(r)
	}
	return p
}

func pluralGo(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func hostRefWarn(warns []string) []string {
	var out []string
	for _, w := range warns {
		if strings.Contains(w, " references ") || strings.Contains(w, "symlink into the host") {
			out = append(out, w)
		}
	}
	return out
}

func appendUnique(s []string, v string) []string {
	for _, e := range s {
		if e == v {
			return s
		}
	}
	return append(s, v)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
