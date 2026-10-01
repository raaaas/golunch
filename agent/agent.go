package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// RunSpec is everything a driver needs to build a headless invocation.
type RunSpec struct {
	Prompt      string
	Model       string
	Provider    string
	APIKey      string
	SessionID   string
	Thinking    string
	Agent       string
	Continue    bool
	Fork        bool
	Plan        bool
	AutoApprove bool
	Timeout     time.Duration
	WorkDir     string
	Files       []string
	Attach      string
	Title       string
	// Extra is verbatim argv, appended last. The escape hatch for any flag a
	// driver does not model, and the reason a missing mapping is never a
	// dead end.
	Extra []string
	// Native points the agent's own isolation flags at instance dirs. Only
	// drivers that advertise SupportNative honor this.
	Native NativePaths
}

type NativePaths struct {
	ConfigDir string
	DataDir   string
}

func (n NativePaths) Active() bool { return n.ConfigDir != "" || n.DataDir != "" }

// Paths are an instance's directory roots, passed to drivers that need to
// name their own state locations without importing the instance package.
type Paths struct {
	Root   string
	Home   string
	Config string
	Data   string
	State  string
	Cache  string
	Tmp    string
	Bin    string
}

// HostPaths are the *real* user's config roots, resolved outside any instance.
// A driver uses them to name where the host keeps the files an instance might
// want to inherit; package cli computes them, because only cli knows whether
// $XDG_* has already been redirected by an outer isolation tool.
type HostPaths struct {
	Home   string
	Config string
	Data   string
	State  string
	Cache  string
}

// SeedItem is one host path that `golunch seed` may hand to an instance, and
// where it lands. The registry states both sides rather than letting the copier
// guess: agents disagree about whether MCP servers live in the config file
// (kilo, opencode) or in a separate file under the data dir (cline), and a
// heuristic mapping would silently copy the wrong thing.
//
// Dst is relative to the instance root, mirroring OwnPaths. A Src that does not
// exist on the host is skipped without error, which is what lets one entry list
// both kilo.jsonc and opencode.json for a fork family.
type SeedItem struct {
	// Group is what the user names on the command line: "mcp", "skills",
	// "config", or "deps" for node_modules-sized trees that are never copied
	// unless asked.
	Group string
	Src   string
	Dst   string
	// Tree marks a directory rather than a single file.
	Tree bool
}

// EnvProfile records the environment an agent cares about beyond the
// isolation block.
type EnvProfile struct {
	// KeepVars must be copied from the host or the agent cannot talk to
	// anything: a corporate CA bundle, a machine-scoped credential helper
	// path, an ssh agent socket.
	KeepVars []string
	// Secrets reports absolute paths whose existence means "this instance is
	// logged in". `golunch doctor` reads them and `golunch clone` uses them to
	// warn before copying credentials into a new alias. They are computed per
	// instance because agents disagree wildly: kilo puts auth.json in
	// XDG_DATA_HOME while cline puts secrets.json under $HOME/.cline/data.
	Secrets func(Paths) []string
	// BaseURLVars are the variables that would redirect the agent's model
	// endpoint. Declared for future gateway support; the current proxy scope
	// only injects *_PROXY and never rewrites these.
	BaseURLVars []string
	// APIKeyVars are names the agent reads a key from, so a --key flag can be
	// injected without each driver reimplementing env plumbing.
	APIKeyVars []string
}

// InstallInfo declares what a downloader needs to fetch and run this agent's
// upstream installer inside a redirected instance. It is data rather than
// code because the download is the one place golunch executes remote bytes:
// the fields describe the transaction so the CLI can print, hash and confirm
// it before running anything. ScriptURL is deliberately empty in the shipped
// registry — the vendor URLs were not verified when these rows were written,
// and inventing one is worse than requiring `install --url/--script`. A later
// commit fills the rows once the docs are checked; nothing else changes.
type InstallInfo struct {
	// ScriptURL is the upstream installer to fetch. Empty until verified.
	ScriptURL string
	// ScriptSHA256 optionally pins the fetched script so a changed installer
	// is rejected rather than silently executed.
	ScriptSHA256 string
	// Args are fixed argv handed to the script, not the user's prompt.
	Args []string
	// PinEnv names the variable the script reads a pinned version from, so a
	// `--version` can be injected without each caller learning the name.
	PinEnv string
	// Prereqs are programs the script needs on the *child* PATH (sh, curl,
	// git, node, npm). They are checked by stat'ing the instance PATH, which
	// differs from the parent's, so a name here is a claim about the sandbox.
	Prereqs []string
	// Note is a limitation printed before executing fetched remote code, e.g.
	// that the installer cannot sudo or write to system paths.
	Note string
}

// Entry is one agent's complete knowledge. Adding claude/codex/gemini later
// means writing one of these literals, not touching the launcher, the
// environment builder or the task runner.
type Entry struct {
	Name    string
	Aliases []string
	// Binary is the program name looked up on PATH and in KnownDirs.
	Binary string
	// KnownDirs are non-PATH locations this agent commonly installs to, which
	// matters because a curl installer's location is not on PATH until the
	// user's shell rc is re-read.
	KnownDirs []string
	// Subcmd is prepended for a headless run (kilo: "run"; cline: none).
	Subcmd []string
	// VersionArgs defaults to {"--version"}.
	VersionArgs []string

	BuildArgs func(RunSpec) []string
	ParseLine ParseLine
	OwnPaths  func(root string) []string
	// Seed declares what the host has that an instance may want without
	// becoming a copy of the host login. Nil means `golunch seed` has nothing
	// to offer for this agent and says so instead of guessing.
	Seed func(HostPaths) []SeedItem
	Env  EnvProfile

	// Install describes how a downloader fetches and runs this agent's upstream
	// installer inside an instance. Nil means golunch has no verified installer
	// for this agent and `install` needs an explicit --url/--script.
	Install *InstallInfo

	// SupportNative reports whether the agent has its own isolation flags that
	// complement the HOME/XDG override.
	SupportNative bool
	// NativeArgs renders those flags, given the spec.
	NativeArgs func(RunSpec) []string
	// PromptAsArgv reports whether the prompt is a positional argument
	// (cline, kilo) rather than read from stdin.
	PromptAsArgv bool
}

var registry = map[string]Entry{}
var order []string

func register(e Entry) {
	registry[e.Name] = e
	order = append(order, e.Name)
	for _, a := range e.Aliases {
		registry[a] = e
	}
}

// Names returns the canonical agent names.
func Names() []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range order {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func Lookup(name string) (Entry, bool) {
	e, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	return e, ok
}

// Locate finds the host executable for an agent. It checks PATH first, then
// the agent's well-known install dirs, then the user's personal bin. Node and
// bun installers routinely drop binaries in places that are only on PATH from
// an interactive shell rc, which a non-interactive exec does not read.
func Locate(e Entry) (string, error) {
	if p, err := exec.LookPath(e.Binary); err == nil {
		return p, nil
	}
	var homes []string
	if h := os.Getenv("HOME"); h != "" {
		homes = append(homes, h)
	}
	if rh := realHome(); rh != "" && rh != os.Getenv("HOME") {
		homes = append(homes, rh)
	}
	p, tried := locateInDirs(e, homes)
	if p != "" {
		return p, nil
	}
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("agent %q: binary %q not found (searched PATH and %v)", e.Name, e.Binary, tried)
	}
	// Absolute system locations are worth a last look for package installs.
	for _, d := range []string{"/usr/local/bin", "/usr/bin", "/opt/homebrew/bin"} {
		sp := filepath.Join(d, e.Binary)
		if st, err := os.Stat(sp); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return sp, nil
		}
	}
	return "", fmt.Errorf("agent %q: %q not found on PATH or in %v", e.Name, e.Binary, tried)
}

// LocateIn answers "did this land inside the instance" and nothing else. It
// walks only the agent's KnownDirs and each given home's `$HOME/.local/bin` —
// never PATH, never the system dirs Locate falls back to — because those two
// look at the *host* and would let a download that escaped the instance pass
// as a success. The installer flow runs a script under a redirected HOME and
// then calls this to prove the binary it produced lives inside the tree.
//
// KnownDirs templates are glob-expanded. Locate historically only joined and
// stat'd each entry, so a pattern like cline's `~/nvm/versions/node/*/bin/cline`
// (agent/cline.go) could never match: the `*` was treated as a literal. Now an
// entry containing `*` is run through filepath.Glob, which matches a single
// path segment and so is exactly what `node/*/bin/cline` needs. This is a
// deliberate behavior change: `golunch new --agent cline` may now find a host
// nvm install the old join+stat silently missed.
func LocateIn(e Entry, homes ...string) (string, error) {
	p, tried := locateInDirs(e, homes)
	if p != "" {
		return p, nil
	}
	return "", fmt.Errorf("agent %q: %q not found in %v", e.Name, e.Binary, tried)
}

// locateInDirs is the shared KnownDirs + `$HOME/.local/bin` walk. It returns
// the first executable it finds and, when nothing matched, the candidate paths
// it tried so the caller can name them in an error. Both Locate and LocateIn
// build on it; the split keeps Locate's historical error text and `tried`
// list while giving LocateIn the same walk without any host probes.
func locateInDirs(e Entry, homes []string) (found string, tried []string) {
	for _, h := range homes {
		if h == "" {
			continue
		}
		for _, d := range e.KnownDirs {
			// Normalize both shapes: cline writes "~/nvm/…/bin/cline" while
			// kilo/opencode write ".opencode/bin/opencode" with no prefix.
			base := filepath.Join(h, strings.TrimPrefix(d, "~/"))
			candidates := []string{base}
			if strings.Contains(base, "*") {
				if g, err := filepath.Glob(base); err == nil {
					candidates = g
				}
			}
			for _, p := range candidates {
				tried = append(tried, p)
				if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
					return p, tried
				}
			}
		}
		p := filepath.Join(h, ".local", "bin", e.Binary)
		tried = append(tried, p)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, tried
		}
	}
	return "", tried
}

func realHome() string {
	if u, err := os.UserHomeDir(); err == nil {
		return u
	}
	return ""
}

// Version runs the agent's --version and extracts the first version-looking
// token, mirroring warren's install/detect.rs probing which accepts --version,
// -v and -V because CLI authors cannot agree.
func Version(binPath string, args []string) string {
	if len(args) == 0 {
		args = []string{"--version"}
	}
	out, err := exec.Command(binPath, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return ExtractVersion(string(out))
}

// ExtractVersion picks the first plausible version number out of a --version
// report. Authors disagree on the shape ("7.7.9", "v1.2", "opencode/0.4.2",
// "kilo version 7.7.9"), so this scans tokens instead of matching one format.
// It must not mistake a filesystem path for a version: a warning that named
// /home/…/node/v20.20.1 once got recorded as the agent's version, because the
// digits after the last slash looked exactly like one.
func ExtractVersion(s string) string {
	for _, field := range strings.Fields(s) {
		i := strings.IndexFunc(field, isDigit)
		if i < 0 {
			continue
		}
		prefix, digits := field[:i], field[i:]
		// More than one separator, or a leading one, means this is a path rather
		// than a tool name attached to a number — "opencode/0.4.2" is legal and
		// has a single interior separator.
		if strings.HasPrefix(prefix, "/") || strings.Count(prefix, "/") > 1 {
			continue
		}
		if t := strings.TrimPrefix(digits, "v"); isVersionToken(t) {
			return t
		}
	}
	return ""
}

// isVersionToken accepts "3.0.48", "1.2", "2.1.0-beta.3" and rejects a path
// remainder ("20.20.1/bin/cline") or a unit ("4.7GB").
func isVersionToken(t string) bool {
	if len(t) < 3 || t[0] < '0' || t[0] > '9' {
		return false
	}
	dots := 0
	for i := 0; i < len(t); i++ {
		c := t[i]
		last := i == len(t)-1
		switch {
		case c >= '0' && c <= '9':
		case c == '.':
			if last {
				return false
			}
			dots++
		case c == '-' || c == '+' || c == '_':
			if last {
				return false
			}
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			// Letters belong to a pre-release tag, so they follow a separator or
			// another letter: 1.2.3-rc1 yes, 4.7GB no.
			if i == 0 {
				return false
			}
			if p := t[i-1]; !isLetterByte(p) && p != '-' && p != '+' && p != '_' {
				return false
			}
		default:
			return false
		}
	}
	return dots > 0
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isLetterByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
