package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// Finding is one doctor result. Severity decides the exit code: warnings leave
// the command successful, errors do not.
type Finding struct {
	Severity string `json:"severity"`
	Instance string `json:"instance,omitempty"`
	Check    string `json:"check"`
	Detail   string `json:"detail"`
	Hint     string `json:"hint,omitempty"`
}

// CmdDoctor answers the two questions that matter before trusting an instance:
// can golunch actually start this agent, and is it isolated (i.e. logged out)?
// It also reports drift between a baked launcher and the current configuration,
// which is the failure mode of editing metadata.toml by hand.
func (a *App) CmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	jsonOut := fs.Bool("json", false, "machine-readable findings")
	if _, err := splitArgs(fs, args); err != nil {
		return err
	}

	var findings []Finding
	add := func(sev, inst, check, detail, hint string) {
		findings = append(findings, Finding{sev, inst, check, detail, hint})
	}

	// Global checks first: without a writable data root nothing else matters.
	root, nesting := a.nestingNote()
	if nesting != "" {
		add("warn", "", "nesting", nesting, "export GOLUNCH_ROOT or run golunch outside the wrapping instance")
	}
	if err := probeWrite(root); err != nil {
		add("error", "", "data root", fmt.Sprintf("%s is not writable: %v", root, err),
			"fix the permissions or set paths.instances_dir in "+filepath.Join(root, "config.toml"))
	}

	names, err := instance.List(a.InstancesDir())
	if err != nil {
		add("error", "", "instances dir", err.Error(), "")
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if _, err := instance.New(a.InstancesDir(), args[0]); err != nil {
			return err
		}
		names = []string{args[0]}
	}

	for _, name := range names {
		inst, err := instance.New(a.InstancesDir(), name)
		if err != nil {
			add("error", name, "alias", err.Error(), "")
			continue
		}
		meta, err := inst.LoadMetadata()
		if err != nil {
			add("error", name, "metadata", err.Error(), "the file is hand-editable; compare it with a working instance")
			continue
		}
		a.checkInstance(inst, &meta, add)
	}

	if *jsonOut {
		for _, f := range findings {
			a.printf("%s\n", mustJSON(f))
		}
	} else {
		a.printf("golunch %s\n  data root %s\n\n", versionText, root)
		if len(findings) == 0 {
			a.printf("no findings: %d instance(s) look healthy\n", len(names))
		}
		for _, f := range findings {
			label := strings.ToUpper(f.Severity)
			if f.Instance != "" {
				label += " [" + f.Instance + "]"
			}
			a.printf("%s %s: %s\n", label, f.Check, f.Detail)
			if f.Hint != "" {
				a.printf("  -> %s\n", f.Hint)
			}
		}
	}

	for _, f := range findings {
		if f.Severity == "error" {
			return CommandError{Code: ExitError, Err: fmt.Errorf("%d error(s) found", countSeverity(findings, "error"))}
		}
	}
	return nil
}

func (a *App) checkInstance(inst *instance.Instance, meta *instance.Metadata, add func(string, string, string, string, string)) {
	name := inst.Alias
	report := func(sev, check, detail, hint string) { add(sev, name, check, detail, hint) }

	for _, d := range []string{"home", "config", "data", "cache", "state", "runtime", "tmp", "logs", "bin"} {
		if p := inst.Dir(d); !dirExists(p) {
			report("error", "layout", p+" is missing", "run: golunch new "+name+" --force")
		}
	}

	if len(meta.Launch.Command) == 0 {
		report("error", "command", "metadata records no command to exec", "recreate with --binary or --agent")
	} else if _, err := os.Stat(meta.Launch.Command[0]); err != nil {
		report("error", "command", meta.Launch.Command[0]+" is no longer present on this host",
			"the agent was upgraded or moved; run golunch new "+name+" --agent "+meta.Instance.Agent+" --force")
	}

	entry, hasDriver := agent.Lookup(meta.Instance.Agent)
	if meta.Instance.Agent != "" && !hasDriver {
		report("error", "agent", fmt.Sprintf("agent %q is not in the registry (known: %s)",
			meta.Instance.Agent, strings.Join(agent.Names(), ", ")), "this build of golunch cannot drive it")
	}

	if hasDriver {
		// The isolation claim, stated as a fact a user can verify: if the
		// credential files are absent inside the instance, the agent really is
		// logged out here, and signing in cannot touch the host copy.
		secrets := entry.Env.Secrets(agent.Paths{
			Root: inst.Root, Home: inst.HomeDir(), Config: inst.ConfigDir(),
			Data: inst.DataDir(), State: inst.StateDir(), Cache: inst.CacheDir(),
			Tmp: inst.TmpDir(), Bin: inst.BinDir(),
		})
		var present []string
		for _, s := range secrets {
			if osutil.Exists(s) {
				present = append(present, s)
			}
		}
		if len(present) == 0 {
			report("info", "login", fmt.Sprintf("logged out: none of the %d credential path(s) exist under %s",
				len(secrets), inst.Root), "run "+name+" interactively to sign in")
		} else {
			report("info", "login", "signed in; credentials live only in "+strings.Join(present, ", "), "")
		}

		if emptyDir(inst.ConfigDir()) && !hasSecrets(present) {
			report("warn", "config", "instance config dir is empty, so plugins/agents/skills from your host "+
				"configuration are deliberately not loaded here",
				"copy what you need in, or expect a bare agent")
		}
	}

	// An instance that downloaded its own agent binary is a different animal
	// from one that wraps the host's: it carries a file, and it carries a claim
	// about where that file came from. Both are checked here.
	if meta.Install != nil {
		checkInstall(inst, meta.Install, report)
	}

	// Launcher drift: the script is a snapshot of the environment as it was
	// when it was written. Comparing it to a fresh render is how doctor knows
	// an edit to metadata.toml has not taken effect yet.
	if _, err := os.Stat(inst.LauncherPath()); err != nil {
		report("error", "launcher", inst.LauncherPath()+" is missing", "golunch new "+name+" --force")
	} else if drift := a.launcherDrift(inst, meta); drift != "" {
		report("warn", "launcher", drift, "regenerate with: golunch new "+name+" --force")
	}

	if meta.Paths.LinkPath != "" {
		if _, err := os.Lstat(meta.Paths.LinkPath); err != nil {
			report("warn", "command link", meta.Paths.LinkPath+" is missing", "golunch new "+name+" --force")
		} else if !instance.LinkIsOurs(meta.Paths.LinkPath, inst.LauncherPath()) {
			report("warn", "command link", meta.Paths.LinkPath+" exists but is not golunch's launcher",
				"something else owns that name; the launcher at "+inst.LauncherPath()+" still works")
		}
	}

	if instance.Held(inst.LockPath()) {
		report("info", "lock", "an agent or admin command currently holds this instance", "")
	}

	if _, total, err := inst.DiskUsage(); err == nil && total > (512<<20) {
		report("info", "size", instance.FormatBytes(total)+" used under "+inst.Root,
			"golunch rm "+name+" frees it")
	}
}

func hasSecrets(paths []string) bool { return len(paths) > 0 }

// checkInstall is the isolation promise made checkable: a downloaded agent
// binary has to live inside the instance tree, still be executable, and still
// match the installer script that produced it. Every path it reads was recorded
// by the installer and is hand-editable afterwards, which is precisely why the
// honest answer needs a stat and a hash rather than trust.
func checkInstall(inst *instance.Instance, in *instance.InstallInfo, report func(string, string, string, string)) {
	fix := "re-download with: golunch install " + inst.Alias + ", or discard the copy with: golunch rm " + inst.Alias

	where := in.Dir
	if where == "" && in.Binary != "" {
		where = filepath.Dir(in.Binary)
	}
	switch {
	case where == "":
		where = "directory unrecorded"
	case insideRoot(inst.Root, where):
		where = relOf(inst.Root, where)
	}
	when := "time unrecorded"
	if !in.FetchedAt.IsZero() {
		when = in.FetchedAt.UTC().Format(time.RFC3339)
	}
	// The registry name is what a reader recognizes; a hand-edited table can
	// leave it out, and then the file it points at is the only name worth showing.
	what := in.Agent
	if what == "" {
		what = filepath.Base(in.Binary)
	}
	if what == "" || what == "." || what == string(filepath.Separator) {
		what = "agent"
	}
	report("info", "install", fmt.Sprintf("carries its own %s copy: installed into %s from %s (script sha256 %s, %s)",
		what, where, firstNonEmpty(in.URL, "url unrecorded"), shortSHA(in.SHA256), when), "")

	switch {
	case in.Binary == "":
		report("error", "install integrity", "the [install] table records no binary path", fix)
	case !insideRoot(inst.Root, in.Binary):
		report("error", "install integrity", "[install] binary "+in.Binary+" is outside the instance root "+inst.Root, fix)
	default:
		if st, err := os.Stat(in.Binary); err != nil {
			report("error", "install integrity", in.Binary+" is no longer present", fix)
		} else if st.IsDir() || st.Mode()&0o111 == 0 {
			report("error", "install integrity", in.Binary+" is not executable", fix)
		} else if rp, err := filepath.EvalSymlinks(in.Binary); err == nil && !within(inst.Root, rp) {
			// A recorded path inside the tree that links out of it is the same
			// escape wearing a friendlier name.
			report("error", "install integrity", in.Binary+" resolves to "+rp+", outside the instance root", fix)
		}
	}
	if in.Dir != "" && !insideRoot(inst.Root, in.Dir) {
		report("error", "install integrity", "[install] dir "+in.Dir+" is outside the instance root "+inst.Root, fix)
	}

	// A missing audit copy is not drift: logs/ is the one directory a user is
	// expected to prune, and nagging about it would train them to ignore the
	// finding that does matter.
	if in.ScriptPath != "" && in.SHA256 != "" {
		if _, err := os.Stat(in.ScriptPath); err == nil {
			if got, err := fileSHA256(in.ScriptPath); err == nil && !strings.EqualFold(got, in.SHA256) {
				report("warn", "install drift", "the retained installer "+relOf(inst.Root, in.ScriptPath)+
					" hashes to "+shortSHA(got)+", not the recorded "+shortSHA(in.SHA256),
					"the audit script was edited or replaced; fetch a fresh copy with: golunch install "+inst.Alias)
			}
		}
	}
}

// insideRoot accepts a recorded path as belonging to the instance under either
// reading of the tree: within resolves the root through symlinks and pathUnder
// is purely lexical, so an absolute path the installer wrote and the same path
// edited by hand can differ in that prefix without meaning anything. A path
// matching neither is nowhere near the instance, which is the only case worth
// accusing.
func insideRoot(root, p string) bool { return within(root, p) || pathUnder(root, p) }

// shortSHA keeps a 64-character digest out of a one-line finding; the prefix is
// enough for a human to compare against the drift warning next to it.
func shortSHA(s string) string {
	switch {
	case s == "":
		return "unpinned"
	case len(s) > 12:
		return s[:12]
	default:
		return s
	}
}

// fileSHA256 is cheap enough to run on every doctor pass because the only file
// it is pointed at is a vendor install script, which is kilobytes, not the
// binary.
func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// launcherDrift re-renders the launcher from current configuration and reports
// the first line that differs, without writing anything.
func (a *App) launcherDrift(inst *instance.Instance, meta *instance.Metadata) string {
	res, err := a.Resolve(*meta, "", nil)
	if err != nil {
		return "proxy configuration is invalid: " + err.Error()
	}
	e, err := a.BuildEnv(inst, meta, res, nil, a.KeepVarsFor(meta))
	if err != nil {
		return "environment cannot be built: " + err.Error()
	}
	want := instance.RenderLauncher(inst.Alias, e, inst, meta.Launch.Command, res)
	have, err := os.ReadFile(inst.LauncherPath())
	if err != nil {
		return "launcher is unreadable: " + err.Error()
	}
	if string(have) == want {
		return ""
	}
	wl, hl := strings.Split(want, "\n"), strings.Split(string(have), "\n")
	for i := range wl {
		if i >= len(hl) || wl[i] != hl[i] {
			return fmt.Sprintf("baked launcher differs from current config at line %d\n     have: %s\n     want: %s",
				i+1, trim(hl, i), wl[i])
		}
	}
	return ""
}

func trim(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(end of file)"
}

func (a *App) nestingNote() (string, string) {
	if a.Cfg.NestingWarning != "" {
		return a.Cfg.Root, a.Cfg.NestingWarning
	}
	return a.Cfg.Root, ""
}

// probeWrite reports whether dir can hold files, creating it first: a data
// root that does not exist yet is a first run, not a failure — every other
// command creates it lazily, and doctor must not be the command that
// contradicts them.
func probeWrite(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".golunch-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// emptyDir reports a directory that exists and holds nothing. Readdirnames
// returns io.EOF along with the empty slice when it hits the end before its
// count, so treating that as an error would make every real empty directory look
// non-empty — which is how this function's only caller, the doctor warning about
// an instance that loads no plugins, silently never fired.
func emptyDir(p string) bool {
	d, err := os.Open(p)
	if err != nil {
		return false
	}
	defer d.Close()
	names, err := d.Readdirnames(1)
	return len(names) == 0 && (err == nil || errors.Is(err, io.EOF))
}

func countSeverity(fs []Finding, sev string) int {
	n := 0
	for _, f := range fs {
		if f.Severity == sev {
			n++
		}
	}
	return n
}
