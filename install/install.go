package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/execd"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/proxy"
)

const (
	// defaultScriptTimeout is a network install of a Node or Bun CLI, so it is
	// measured in minutes. It is not inherited from cfg.Defaults.Timeout, which
	// is 0 (unbounded) because that is right for a `make` the user chose to run
	// and wrong for a child process downloading from somebody else's server.
	defaultScriptTimeout = 10 * time.Minute

	// versionProbeTimeout bounds `binary --version` after a completed install. A
	// CLI that hangs on its own version string must not turn a good install into
	// a hung command, and its version is never worth waiting for.
	versionProbeTimeout = 5 * time.Second
)

// Spec is one install transaction: a script, an instance to run it in, and the
// environment that makes "run it in" mean something.
type Spec struct {
	// Entry is the registry row being installed. Its Install table supplies the
	// fixed argv, the version-pin variable name and the child PATH prereqs; nil
	// means an agent with no verified installer, which is exactly the case
	// --script exists for.
	Entry agent.Entry
	// Inst is the instance whose tree receives the binary. Its directories, never
	// the caller's, are where the script lands.
	Inst *instance.Instance
	// Env is the assembled child environment from instance.BuildEnv, carrying the
	// redirected HOME and XDG dirs, TMPDIR, the prepended bin/ and the proxy
	// decision. It is not built here because BuildEnv is the only place that
	// decision is made, and the script's own curl has to be told what golunch
	// decided rather than guess.
	Env []string

	// ScriptURL is the installer to download. Ignored when ScriptPath is set.
	ScriptURL string
	// ScriptPath is a script that is already on disk. It skips Fetch entirely,
	// which is why this package is fully usable with ScriptPath alone: the
	// registry ships no verified URLs, so today this is the normal path.
	ScriptPath string
	// ScriptSHA256 pins the script. Empty means no pin, and the hash that was
	// computed is recorded in Result.SHA256 for doctor to compare later.
	ScriptSHA256 string
	// PinVersion is the version the user asked for. It goes to the script
	// through Entry.Install.PinEnv when that names a variable, and as a
	// positional when it does not.
	PinVersion string
	// Shell is the interpreter to run the script with and must be an absolute
	// path, e.g. "/bin/bash". execd resolves argv[0] against golunch's own PATH
	// (execd/execd.go:101), so anything relative here would be resolved by the
	// host, not the instance.
	Shell string
	// Args is extra argv for the script, appended after the registry's fixed
	// args, mirroring RunSpec.Extra's "verbatim argv, appended last".
	Args []string

	// Proxy is the instance's resolved proxy decision, used only for the
	// Go-side download of ScriptURL. The script's own network access is already
	// decided by Env: Proxy is not a second opinion about the child.
	Proxy proxy.Resolution

	// MaxScriptBytes bounds the download. 0 means 1 MiB.
	MaxScriptBytes int64
	// Timeout bounds the script's run, killing its whole process group. 0 means
	// ten minutes.
	Timeout time.Duration
}

// Result is what one install produced, and it is returned alongside the error on
// most failure paths: the exit code and the audit copy are the two things an
// operator needs exactly when the install failed.
type Result struct {
	// ScriptPath is the audit copy kept inside the instance, at
	// <root>/logs/install-<stamp>.sh. It is the bytes that ran.
	ScriptPath string
	// SHA256 is the hash of those bytes, pinned or observed.
	SHA256 string
	// FetchedAt is when the script was staged inside the instance. For a local
	// --script that is the install time rather than a download time, and it is
	// still the honest value for metadata's fetched_at.
	FetchedAt time.Time
	// BinaryPath is the real file installed at <root>/bin/<binary>.
	BinaryPath string
	// Version is what the instance's own copy reported, or "" when it would not
	// say. An agent that installs but will not print a version is a warning, not
	// a failed install.
	Version string
	// Escaped lists host paths that appeared because of this run. It must
	// normally be empty; anything in it means the installer wrote to the host
	// anyway and the caller should say so loudly.
	Escaped  []string
	ExitCode int
	TimedOut bool
}

// Install fetches if needed, then runs the vendor installer inside the instance
// and links the binary it produced into <root>/bin.
//
// The order is deliberate and load-bearing: the script is downloaded, hashed,
// checked against the pin and copied to the instance's logs directory before
// anything is executed, so a caller that needs consent can do the same thing in
// two calls with Fetch and a ScriptPath. Prerequisites are checked before any
// network traffic, because a missing curl discovered after a download is a
// mystery rather than an error.
func Install(ctx context.Context, s Spec) (Result, error) {
	var out Result
	switch {
	case s.Inst == nil:
		return out, errors.New("install: Spec.Inst is required; the installer needs a tree to write into")
	case len(s.Env) == 0:
		return out, errors.New("install: Spec.Env is required; build it with instance.BuildEnv so the installer inherits the redirected HOME, XDG dirs and PATH")
	case s.Entry.Binary == "":
		return out, errors.New("install: Spec.Entry.Binary is required; it names the file to link into the instance's bin")
	}
	shell, err := checkShell(s.Shell)
	if err != nil {
		return out, err
	}
	path := childPATH(s.Env)
	if err := checkPrereqs(s.Entry, path); err != nil {
		return out, err
	}

	scriptPath, digest, stagedAt, err := stage(ctx, s)
	if err != nil {
		return out, err
	}
	out.ScriptPath = scriptPath
	out.SHA256 = digest
	out.FetchedAt = stagedAt

	// Snapshot before the run, so "did this install touch the host" is answered
	// with evidence rather than with a guess afterwards.
	before := snapshotHost(s.Entry, s.Entry.Binary)

	// The deadline is owned here rather than left to execd because the failure
	// has to name the duration that was exceeded and point at the audit copy.
	limit := scriptTimeout(s.Timeout)
	scriptCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	run := execd.Run(scriptCtx, execd.Spec{
		Argv:         append([]string{shell, scriptPath}, scriptArgs(s)...),
		Env:          runEnv(s),
		InheritStdio: true,
		StderrTail:   4096,
		Timeout:      limit,
	})
	out.ExitCode = run.ExitCode
	out.TimedOut = run.TimedOut
	switch {
	case out.TimedOut:
		return out, fmt.Errorf("install: the installer did not finish within %s and was killed with its whole process group; audit copy at %s",
			limit, scriptPath)
	case run.ExitCode != 0:
		return out, fmt.Errorf("install: the installer exited with code %d; audit copy at %s\n%s",
			run.ExitCode, scriptPath, strings.TrimSpace(run.Stderr))
	}

	// BuildEnv set HOME to this directory, so a $HOME-respecting installer put the
	// binary here and nowhere else. LocateIn is the only lookup that can prove it:
	// it never consults PATH and never falls back to the system dirs, so a binary
	// that landed on the host cannot pass as a success.
	found, err := agent.LocateIn(s.Entry, s.Inst.HomeDir())
	if err != nil {
		return out, fmt.Errorf(
			"install: %s ran without error but produced no %q inside the instance (%v). "+
				"The installer either wrote to an absolute host path such as /usr/local/bin or "+
				"needed root, both of which this instance is deliberately denied. Install the agent "+
				"on the host yourself and wrap it with \"golunch new --binary\", or use a version "+
				"manager whose directory you can point the instance at",
			scriptPath, s.Entry.Binary, err)
	}
	escaped, err := Verify(s.Entry, s.Inst, found, before)
	if err != nil {
		return out, err
	}
	out.Escaped = escaped

	dst := filepath.Join(s.Inst.BinDir(), s.Entry.Binary)
	if err := Link(found, dst, s.Entry.Binary); err != nil {
		return out, err
	}
	out.BinaryPath = dst
	// A missing version does not fail the install: the binary is in place and
	// runs, and plenty of CLIs have no --version at all.
	out.Version = probeVersion(ctx, s, dst)
	return out, nil
}

// Verify answers what one install actually left behind: it refuses a located
// binary that is not inside the instance, and reports the host paths that did not
// exist before the run and do now.
//
// The host side is a fixed set of stats, not a walk. agent.LocateIn's own
// candidate list is the definition of what could be mistaken for "installed", so
// those are exactly the paths compared against the snapshot; walking the host
// would be slow, non-deterministic and would report files the run had nothing to
// do with.
func Verify(e agent.Entry, inst *instance.Instance, found string, before map[string]bool) ([]string, error) {
	if inst == nil {
		return nil, errors.New("install: Verify: instance is required")
	}
	if !pathUnder(inst.Root, found) {
		return nil, fmt.Errorf(
			"install: %q is outside the instance root %s; an installer that writes to absolute "+
				"system paths cannot be sandboxed, and golunch will not pretend it did",
			found, inst.Root)
	}
	// LocateIn only ever returns <dir>/<binary>, so a name that disagrees with the
	// registry row means the lookup changed underneath this function. Cheap enough
	// to assert rather than assume.
	if e.Binary != "" && filepath.Base(found) != e.Binary {
		return nil, fmt.Errorf("install: agent %q installs %q, but the lookup returned %q",
			e.Name, e.Binary, found)
	}
	var escaped []string
	for candidate, existed := range before {
		if existed {
			// It was already there, so this install did not create it. A host
			// that was modified in place is a different problem and one a stat
			// cannot answer.
			continue
		}
		// A KnownDirs pattern can gain matches: cline declares
		// "~/nvm/versions/node/*/bin/cline", and an installer that creates a new
		// node version dir on the host has escaped into exactly the place
		// LocateIn would later be satisfied by.
		if strings.Contains(candidate, "*") {
			matches, _ := filepath.Glob(candidate)
			for _, m := range matches {
				if !before[m] && exists(m) {
					escaped = append(escaped, m)
				}
			}
			continue
		}
		if exists(candidate) {
			escaped = append(escaped, candidate)
		}
	}
	// Map iteration order is random and this list gets printed.
	sort.Strings(escaped)
	return escaped, nil
}

// Link puts the located binary at <root>/bin/<binary> as a real file.
//
// A hardlink is tried first, because it is free and keeps one copy of the bytes;
// the fallback is a byte copy that preserves the executable mode, the same
// link-then-copy shape instance.WriteLauncher uses (instance/launcher.go:100-116)
// for the same reason: a link another filesystem refuses is not a reason to lose
// the install.
//
// What it never does is create a symlink. `golunch clone` copies bin/ always but
// home/ only with --copy-data (internal/cli/clone.go:104-116), so a symlink into
// home/ would dangle in the clone, and a dangling launcher in the user's PATH is
// worse than a duplicated file. That is also why a symlinked source is
// materialised by copy rather than linked: os.Link on a symlink would produce a
// second symlink.
func Link(found, dst, binary string) error {
	switch {
	case found == "":
		return errors.New("install: Link: no source binary")
	case dst == "":
		return errors.New("install: Link: no destination")
	case binary == "":
		return errors.New("install: Link: no binary name")
	}
	if filepath.Base(dst) != binary {
		return fmt.Errorf("install: Link: destination %q is not the bin entry for %q", dst, binary)
	}
	// The containment rule Link can state without being handed a root: a
	// destination is always <root>/bin/<binary>, so its grandparent is the tree
	// the source has to come from. Install checks against the authoritative
	// inst.Root in Verify first; this guards every other caller.
	root := filepath.Dir(filepath.Dir(dst))
	if !pathUnder(root, found) {
		return fmt.Errorf("install: refusing to link %q into %s: the source is outside the instance tree %s",
			found, dst, root)
	}
	if filepath.Clean(found) == filepath.Clean(dst) {
		return nil
	}
	st, err := os.Stat(found)
	if err != nil {
		return fmt.Errorf("install: %s is not usable: %w", found, err)
	}
	if st.IsDir() {
		return fmt.Errorf("install: %s is a directory, not a binary", found)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("install: create %s: %w", filepath.Dir(dst), err)
	}
	// Replace whatever is there: a stale copy from an earlier install of a
	// different version, or the hardlink this function made last time.
	_ = os.Remove(dst)
	perm := st.Mode().Perm()
	if lst, err := os.Lstat(found); err == nil && lst.Mode()&os.ModeSymlink == 0 {
		if err := os.Link(found, dst); err == nil {
			return nil
		}
	}
	data, err := os.ReadFile(found)
	if err != nil {
		return fmt.Errorf("install: read %s: %w", found, err)
	}
	if err := os.WriteFile(dst, data, perm); err != nil {
		return fmt.Errorf("install: copy %s to %s: %w", found, dst, err)
	}
	return nil
}

// stage resolves the script into the audit copy that will be executed, and
// returns that path, the sha256 of its bytes and the moment it was staged.
func stage(ctx context.Context, s Spec) (string, string, time.Time, error) {
	maxBytes := s.MaxScriptBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxScriptBytes
	}
	now := time.Now().UTC()
	var src string
	switch {
	case s.ScriptPath != "":
		// First-class, not a fallback: the registry ships no verified URLs, so
		// this is how every install works until the vendor docs are checked.
		src = s.ScriptPath
	case s.ScriptURL != "":
		scratch := filepath.Join(s.Inst.TmpDir(), "install-script-"+stampOf(now)+".sh")
		// Fetch's return value is the hash of the bytes it served; the one below
		// is the hash of the file as it now exists on disk, and since it is the
		// file that gets executed it is the value reported and pinned against.
		if _, err := Fetch(ctx, s.ScriptURL, scratch, maxBytes, s.Proxy); err != nil {
			return "", "", now, err
		}
		src = scratch
	default:
		return "", "", now, errors.New("install: either ScriptPath or ScriptURL is required")
	}
	digest, err := hashFile(src, maxBytes)
	if err != nil {
		return "", "", now, err
	}
	if s.ScriptSHA256 != "" && !strings.EqualFold(s.ScriptSHA256, digest) {
		return "", "", now, fmt.Errorf(
			"install: script %s has sha256 %s, not the pinned %s; refusing to run it. "+
				"The installer changed upstream, or this is not the file that was approved",
			src, digest, s.ScriptSHA256)
	}
	dst, err := auditScriptPath(s.Inst.LogsDir(), now)
	if err != nil {
		return "", "", now, err
	}
	if filepath.Clean(src) != dst {
		// Promote a scratch download that is already inside the tree with a
		// rename, and copy anything else: a --script that lives in the caller's
		// own directory has to stay where the user put it.
		if pathUnder(s.Inst.TmpDir(), src) {
			if err := os.Rename(src, dst); err != nil {
				return "", "", now, fmt.Errorf("install: keep audit copy of script at %s: %w", dst, err)
			}
		} else if err := copyFile(src, dst); err != nil {
			return "", "", now, fmt.Errorf("install: keep audit copy of script at %s: %w", dst, err)
		}
	}
	return dst, digest, now, nil
}

// auditScriptPath names the retained copy of the script and makes sure the
// directory it lives in exists. No new top-level instance directory is created
// for this: storageDirs (instance/instance.go:16-18) is fixed, and clone, doctor
// and the launcher all assume exactly that set.
func auditScriptPath(dir string, at time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("install: create %s: %w", dir, err)
	}
	base := "install-" + stampOf(at)
	for i := 0; i <= 99; i++ {
		name := base + ".sh"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.sh", base, i)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p, nil
		}
	}
	return "", fmt.Errorf("install: %s already holds a hundred install scripts for this second", dir)
}

// stampOf is the RFC3339 instant with the punctuation that would complicate a
// filename removed, so the audit copies sort chronologically by name.
func stampOf(t time.Time) string {
	return t.Format("20060102T150405Z")
}

func scriptTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return defaultScriptTimeout
}

// scriptArgs assembles the argv handed to the interpreter after the script path.
func scriptArgs(s Spec) []string {
	var out []string
	var pinEnv string
	if info := s.Entry.Install; info != nil {
		out = append(out, info.Args...)
		pinEnv = info.PinEnv
	}
	// A requested version needs somewhere to go. If the registry names the
	// variable the script reads, runEnv sets it; otherwise the only honest
	// remaining channel is the positional, and silently dropping the user's
	// --version is worse than passing an argument the script may ignore.
	if pinEnv == "" && s.PinVersion != "" {
		out = append(out, s.PinVersion)
	}
	return append(out, s.Args...)
}

// runEnv is Spec.Env plus the pinned version, if the script takes one through
// the environment. The caller's slice is copied rather than appended to, so a
// reused Spec does not grow its environment on every call.
func runEnv(s Spec) []string {
	env := append([]string(nil), s.Env...)
	if info := s.Entry.Install; info != nil && info.PinEnv != "" && s.PinVersion != "" {
		env = append(env, info.PinEnv+"="+s.PinVersion)
	}
	return env
}

// childPATH returns the PATH the installer will see. The last assignment wins
// for a raw environment slice, which is why the loop does not break early:
// BuildEnv guarantees one entry, but a caller that appended a --env override
// means it.
func childPATH(env []string) string {
	path := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == "PATH" {
			path = v
		}
	}
	return path
}

// checkPrereqs stats each program the registry claims the script needs on the
// *child* PATH.
//
// The split from the interpreter is the trap this guards: execd resolves argv[0]
// against golunch's own PATH (execd/execd.go:101) while the script resolves curl,
// git and npm against the instance's, so the two views disagree more often than
// either admits. A developer's /usr/bin/curl proves nothing about a sandbox whose
// PATH is the instance's bin/ plus whatever was inherited, and the failure mode
// otherwise is a script that downloads a tarball and then dies on line 40.
func checkPrereqs(e agent.Entry, path string) error {
	if e.Install == nil || len(e.Install.Prereqs) == 0 {
		return nil
	}
	if path == "" {
		return errors.New("install: the environment carries no PATH, so the installer's prerequisites " +
			"(" + strings.Join(e.Install.Prereqs, ", ") + ") cannot be checked; build Env with instance.BuildEnv")
	}
	dirs := filepath.SplitList(path)
	var missing []string
	for _, name := range e.Install.Prereqs {
		if name == "" || lookInDirs(dirs, name) {
			continue
		}
		missing = append(missing, fmt.Sprintf("%q", name))
	}
	if len(missing) > 0 {
		return fmt.Errorf("install: the installer for %q needs %s on the instance's PATH, which is %q; "+
			"put those in the instance first, or wrap an existing copy with \"golunch new --binary\"",
			e.Name, strings.Join(missing, ", "), path)
	}
	return nil
}

func lookInDirs(dirs []string, name string) bool {
	if strings.ContainsRune(name, filepath.Separator) {
		return executable(name)
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if executable(filepath.Join(d, name)) {
			return true
		}
	}
	return false
}

func executable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// checkShell insists on an absolute interpreter path. execd resolves argv[0]
// against the parent's PATH (execd/execd.go:101), which is golunch's environment
// and not the instance's, so a bare "bash" would run whichever bash the host
// happens to have first and a relative one would depend on the caller's working
// directory. Neither is the redirection this package exists to provide.
func checkShell(shell string) (string, error) {
	if shell == "" {
		return "", errors.New(`install: Spec.Shell is required and must be an absolute interpreter path such as "/bin/bash"`)
	}
	if !filepath.IsAbs(shell) {
		return "", fmt.Errorf("install: Spec.Shell %q must be absolute: the interpreter is resolved against golunch's own PATH, not the instance's", shell)
	}
	st, err := os.Stat(shell)
	if err != nil {
		return "", fmt.Errorf("install: interpreter %s is not usable: %w", shell, err)
	}
	if st.IsDir() || st.Mode()&0o111 == 0 {
		return "", fmt.Errorf("install: interpreter %s is not an executable file", shell)
	}
	return shell, nil
}

// probeVersion runs the instance's own linked copy under the instance's
// environment.
//
// agent.Version is deliberately not used: it execs with the parent environment
// (agent/agent.go:314-323), which would report a version without proving the
// isolated copy can start at all. InheritStdio is set with explicit buffers
// because that is the only branch of execd.Run that honours Spec.Stdout and
// Spec.Stderr (execd/execd.go:158-171); the parsing branch pipes stdout into its
// own scanner and drops it, so the documented "buffers, InheritStdio false"
// combination would silently capture nothing.
func probeVersion(ctx context.Context, s Spec, bin string) string {
	args := s.Entry.VersionArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	var stdout, stderr bytes.Buffer
	probeCtx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	run := execd.Run(probeCtx, execd.Spec{
		Argv:         append([]string{bin}, args...),
		Env:          runEnv(s),
		InheritStdio: true,
		Stdout:       &stdout,
		Stderr:       &stderr,
		Timeout:      versionProbeTimeout,
	})
	if run.ExitCode != 0 {
		return ""
	}
	if v := agent.ExtractVersion(stdout.String()); v != "" {
		return v
	}
	// A handful of CLIs write their version banner to stderr. Only fall back when
	// stdout had no version in it, so a warning on stderr cannot become the
	// recorded version.
	return agent.ExtractVersion(stderr.String())
}

// snapshotHost records the handful of host paths that agent.LocateIn would consult
// for this entry, and whether each already existed.
//
// It stats against config.RealHome, never $HOME: golunch runs inside environments
// where HOME has already been redirected by an outer tool, and using $HOME here
// would snapshot the instance and report the host as untouched no matter what the
// installer did. The list is LocateIn's own candidate list plus the two places an
// installer with absolute paths writes to, and nothing else, because this runs on
// every install and the host filesystem is far too big to walk.
func snapshotHost(e agent.Entry, binary string) map[string]bool {
	out := map[string]bool{}
	home := config.RealHome()
	add := func(p string) { out[p] = exists(p) }
	if home != "" {
		for _, d := range e.KnownDirs {
			base := filepath.Join(home, strings.TrimPrefix(d, "~/"))
			if strings.Contains(base, "*") {
				// Record the pattern as not-satisfied plus each current match, so
				// Verify can tell a new match from an old one.
				matches, _ := filepath.Glob(base)
				if len(matches) == 0 {
					out[base] = false
				}
				for _, m := range matches {
					out[m] = true
				}
				continue
			}
			add(base)
		}
		add(filepath.Join(home, ".local", "bin", binary))
	}
	// The one absolute path the registry's notes promise installers will fail on,
	// checked so a failure that somehow succeeded is still reported.
	add(filepath.Join("/usr/local/bin", binary))
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// pathUnder is the containment test, written here rather than imported: the
// equivalents in internal/cli (clone.go's within, seed.go's pathUnder) are
// unexported, and a downloader that executed a path outside the instance is the
// incident this one guards against.
func pathUnder(root, p string) bool {
	r := filepath.Clean(root)
	c := filepath.Clean(p)
	return c == r || strings.HasPrefix(c, r+string(filepath.Separator))
}

// hashFile returns the hex sha256 of a file, refusing anything larger than
// maxBytes. The same bound applies to a local script and a downloaded one: the
// limit is about what gets executed, not about where it came from.
func hashFile(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("install: open script: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	// One byte past the limit, for the same reason Fetch reads maxBytes+1.
	n, err := io.Copy(h, io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("install: read %s: %w", path, err)
	}
	if n > maxBytes {
		return "", fmt.Errorf("install: script %s is larger than the %s limit",
			path, instance.FormatBytes(uint64(maxBytes)))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// 0755: this is the retained copy of a script that was just executed by
	// handing the path to an interpreter, and making it directly runnable is what
	// lets a human re-read and re-run it after the fact.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
