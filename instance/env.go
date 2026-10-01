package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/raaaas/golunch/proxy"
)

// Env is the assembled child environment. It is an ordered set of
// key=value pairs plus the audit trail explaining why each one is there,
// because "what environment did this agent actually get" is the question
// `golunch env` exists to answer.
type Env struct {
	values map[string]string
	// origin records the layer that decided each key, for annotated output.
	origin map[string]string
	order  []string
}

func newEnv() *Env {
	return &Env{values: map[string]string{}, origin: map[string]string{}}
}

func (e *Env) Set(key, val, layer string) {
	if _, seen := e.values[key]; !seen {
		e.order = append(e.order, key)
	}
	e.values[key] = val
	e.origin[key] = layer
}

func (e *Env) Del(key string) {
	if _, ok := e.values[key]; !ok {
		return
	}
	delete(e.values, key)
	delete(e.origin, key)
	for i, k := range e.order {
		if k == key {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
}

func (e *Env) Get(key string) (string, bool) { v, ok := e.values[key]; return v, ok }

// Slice returns the environment in the order keys were set. exec.Cmd wants a
// slice of assignments; duplicate keys would be resolved by the kernel taking
// the last, so ordering plus the delete-then-set discipline below is what
// guarantees no key appears twice.
func (e *Env) Slice() []string {
	out := make([]string, 0, len(e.order))
	for _, k := range e.order {
		out = append(out, k+"="+e.values[k])
	}
	return out
}

// Map returns a copy keyed by variable name.
func (e *Env) Map() map[string]string {
	out := make(map[string]string, len(e.values))
	for k, v := range e.values {
		out[k] = v
	}
	return out
}

func (e *Env) Origin(key string) string { return e.origin[key] }

// isInherited reports whether a variable was only copied in from the host, in
// which case a launcher that runs inside the user's shell already has it.
func (e *Env) isInherited(key string) bool {
	o := e.origin[key]
	return o == LayerHost || o == string(proxy.SourceInherit)
}

// Baked returns the variables a launcher must export explicitly: the isolation
// block, the decided proxy and anything a flag pinned. Keys stay in build
// order so the generated script is byte-stable and diffable.
func (e *Env) Baked() []string {
	var out []string
	for _, k := range e.order {
		if v, ok := e.values[k]; ok && v != "" && !e.isInherited(k) {
			out = append(out, k)
		}
	}
	return out
}

// ProxyLayer reports which precedence layer won the proxy decision, or "" when
// nothing was decided and the host's own setting is inherited untouched. Used
// for the provenance comment in the launcher and by `golunch env`.
func (e *Env) ProxyLayer() string {
	for _, pair := range proxy.CaseVars {
		for _, name := range []string{pair.Upper, pair.Lower} {
			if _, ok := e.values[name]; !ok {
				continue
			}
			if o := e.origin[name]; o != "" && !e.isInherited(name) {
				return o
			}
		}
	}
	return ""
}

// ProxyKeys returns the proxy variables this Env actually sets, in build order.
func (e *Env) ProxyKeys() []string {
	var out []string
	for _, k := range e.order {
		for _, pair := range proxy.CaseVars {
			if (k == pair.Upper || k == pair.Lower) && e.values[k] != "" && !e.isInherited(k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// RenderKeys emits export lines for exactly the named keys, in the order given.
func (e *Env) RenderKeys(keys []string) string {
	var b strings.Builder
	for _, k := range keys {
		v, ok := e.values[k]
		if !ok || v == "" {
			continue
		}
		b.WriteString("export " + k + "=" + ShellQuote(v) + "\n")
	}
	return b.String()
}

// KeysSorted is for stable golden-file comparison.
func (e *Env) KeysSorted() []string {
	out := append([]string(nil), e.order...)
	sort.Strings(out)
	return out
}

// HostEnv is the environment golunch itself was invoked with. Captured once
// so the child's view never depends on reading os.Getenv after we have started
// mutating our own view.
type HostEnv struct {
	Raw     []string
	Home    string
	Path    string
	User    string
	TmpDir  string
	Runtime string
	Extra   map[string]string
}

func HostFromOs() HostEnv {
	raw := os.Environ()
	h := HostEnv{Raw: raw, Extra: map[string]string{}}
	get := func(k string) string { return h.Extra[k] }
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		h.Extra[k] = v
	}
	h.Home = firstNonEmpty(get("HOME"), RealHomeFallback())
	h.Path = get("PATH")
	h.User = firstNonEmpty(get("USER"), os.Getenv("USER"))
	h.TmpDir = get("TMPDIR")
	h.Runtime = get("XDG_RUNTIME_DIR")
	return h
}

// RealHomeFallback is duplicated here to keep package instance free of a
// dependency on package config, which imports proxy and would otherwise
// create a cycle as the registry grows.
func RealHomeFallback() string {
	if u, err := os.UserHomeDir(); err == nil {
		return u
	}
	return "/"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// LayerHost marks a variable copied unchanged from the host environment. The
// launcher deliberately does not bake these — it runs inside the user's own
// shell, which already exports them — while `golunch run` passes them to the
// child explicitly. Same effective environment, without freezing a session's
// PATH or SSH_AUTH_SOCK into a permanent script.
const LayerHost = "host"

// Layer names the isolation block, so callers can tell it apart from a
// variable that merely happened to be inherited.
const LayerIsolation = "isolation"

// Request describes one environment build.
type Request struct {
	Host HostEnv
	Inst *Instance
	Meta *Metadata
	// Proxy is the already-resolved precedence winner.
	Proxy proxy.Resolution
	// NoProxyHostAdditions carries exception entries found on the host
	// NO_PROXY, which must survive when we override the proxy itself.
	KeepVars []string
	// Extra is per-run --env K=V, applied last.
	Extra map[string]string
}

// BuildEnv is the only place a child environment is assembled. Both
// `golunch run` and the generated launcher render from it, so the two cannot
// drift — the invariant warren states in commands/run.rs ("Mirrors the
// generated launcher script so `warren run` and the `~/.local/bin` launcher
// behave identically").
func BuildEnv(r Request) (*Env, error) {
	if r.Inst == nil {
		return nil, fmt.Errorf("BuildEnv: instance is required")
	}
	e := newEnv()

	// Layer 0: start from the host environment minus everything we are about
	// to decide. We copy rather than start empty because agents need TERM,
	// LANG, USER, SSH_AUTH_SOCK and dozens of variables no one wants to
	// enumerate — but the isolation, proxy and nesting-marker variables must
	// not leak in from whatever sandbox golunch itself happens to be running
	// inside.
	skip := map[string]bool{}
	for _, k := range isolationKeys {
		skip[k] = true
	}
	for _, pair := range proxy.CaseVars {
		skip[pair.Upper], skip[pair.Lower] = true, true
	}
	for _, kv := range r.Host.Raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || skip[k] || strings.HasPrefix(k, "WARREN_") || strings.HasPrefix(k, "GOLUNCH_INSTANCE") {
			continue
		}
		e.Set(k, v, LayerHost)
	}

	// Layer 1: the isolation block. HOME plus every XDG dir plus TMPDIR, all
	// pointing inside the instance, and bin/ prepended to PATH so an
	// instance-local binary wins over the host one.
	inst := r.Inst
	e.Set("HOME", inst.HomeDir(), LayerIsolation)
	e.Set("XDG_CONFIG_HOME", inst.ConfigDir(), LayerIsolation)
	e.Set("XDG_CACHE_HOME", inst.CacheDir(), LayerIsolation)
	e.Set("XDG_DATA_HOME", inst.DataDir(), LayerIsolation)
	e.Set("XDG_STATE_HOME", inst.StateDir(), LayerIsolation)
	e.Set("XDG_RUNTIME_DIR", inst.RuntimeDir(), LayerIsolation)
	e.Set("TMPDIR", inst.TmpDir(), LayerIsolation)
	// Instance data first, host share dirs still readable: this is what lets
	// a wrapped program find system mime types and icons while writing only
	// into its own tree.
	e.Set("XDG_DATA_DIRS", strings.Join([]string{
		inst.DataDir(), "/usr/local/share", "/usr/share",
	}, ":"), LayerIsolation)
	e.Set("XDG_CONFIG_DIRS", "/etc/xdg", LayerIsolation)

	path := r.Host.Path
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	e.Set("PATH", inst.BinDir()+":"+path, LayerIsolation)

	// Markers let a script inside the instance discover which instance it is.
	e.Set("GOLUNCH_INSTANCE", inst.Alias, LayerIsolation)
	e.Set("GOLUNCH_INSTANCE_DIR", inst.Root, LayerIsolation)

	if r.Meta != nil {
		if sh := r.Meta.Instance.Shell; sh != "" {
			e.Set("SHELL", sh, LayerIsolation)
		}
	}

	// Layer 2: proxy.
	applyProxy(e, r)

	// Layer 3: driver-mandated passthrough (things like NODE_EXTRA_CA_CERTS
	// that an agent needs from the host to talk to any endpoint at all).
	for _, k := range r.KeepVars {
		if v, ok := r.Host.Extra[k]; ok && v != "" {
			e.Set(k, v, "driver")
		}
	}

	// Layer 4: per-run --env, last so an operator can override anything above
	// without editing config.
	keys := make([]string, 0, len(r.Extra))
	for k := range r.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if r.Extra[k] == "" {
			e.Del(k)
			continue
		}
		e.Set(k, r.Extra[k], "flag --env")
	}

	return e, nil
}

var isolationKeys = []string{
	"HOME", "TMPDIR", "TEMP", "TMP",
	"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
	"XDG_RUNTIME_DIR", "XDG_DATA_DIRS", "XDG_CONFIG_DIRS",
	"PATH",
}

// applyProxy installs the resolved proxy, or removes all eight variables when
// the winner is the "none" sentinel.
//
// Deleting every case variant before setting any is what makes this correct:
// a host that exports both http_proxy=A and HTTP_PROXY=B would otherwise let
// one survive while we set the other, and the child would then see two
// disagreeing proxies with precedence decided by whichever library reads
// which name.
func applyProxy(e *Env, r Request) {
	for _, pair := range proxy.CaseVars {
		e.Del(pair.Upper)
		e.Del(pair.Lower)
	}
	res := r.Proxy

	if res.Profile.Disabled() {
		return
	}

	// If nothing was configured anywhere, leave the host's own proxy settings
	// alone. Silent stripping would break setups that work today.
	if !res.Profile.Any() && res.Source == proxy.SourceInherit {
		for _, pair := range proxy.CaseVars {
			for _, name := range []string{pair.Upper, pair.Lower} {
				if v, ok := r.Host.Extra[name]; ok && v != "" {
					e.Set(name, v, string(proxy.SourceInherit))
				}
			}
		}
		return
	}

	hostNoProxy := splitEntries(firstNonEmpty(
		r.Host.Extra["NO_PROXY"], r.Host.Extra["no_proxy"]))
	instanceExtra := []string(nil)
	if r.Meta != nil {
		instanceExtra = r.Meta.Proxy.NoProxy
	}
	// The host's own exceptions are legitimate knowledge about the network the
	// user is on; keep them when we take over the proxy decision.
	merged := proxy.MergeEntries(
		res.NoProxyExtra, instanceExtra, hostNoProxy, res.Profile.NoProxy)
	vals := res.Profile.Values(merged)

	layer := string(res.Source)
	for _, pair := range proxy.CaseVars {
		for _, name := range []string{pair.Upper, pair.Lower} {
			v, ok := vals[name]
			if !ok {
				// HTTPS unset but HTTP set is a real configuration; fall back
				// to the HTTP URL, which is what most tools expect.
				if (name == "HTTPS_PROXY" || name == "https_proxy") && vals["HTTP_PROXY"] != "" {
					e.Set(name, vals["HTTP_PROXY"], layer)
				}
				continue
			}
			e.Set(name, v, layer)
		}
	}
}

func splitEntries(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

// ProxyBlock renders just the proxy-related variables, for `golunch env`.
func (e *Env) ProxyBlock() []string {
	var out []string
	for _, pair := range proxy.CaseVars {
		for _, name := range []string{pair.Upper, pair.Lower} {
			if v, ok := e.Get(name); ok {
				out = append(out, fmt.Sprintf("%s=%s   # %s", name, v, e.Origin(name)))
			}
		}
	}
	return out
}

// ShellQuote leaves benign values bare and single-quotes everything else,
// escaping embedded single quotes as the '\” idiom. Ported from warren's
// launcher.rs::shell_quote so generated launchers stay readable where possible.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '.', r == '/', r == ':', r == ',':
		default:
			safe = false
		}
		if !safe {
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// LogPath returns the NDJSON log file for one run of this instance.
func (i *Instance) LogPath(stamp string) string {
	return filepath.Join(i.LogsDir(), "run-"+stamp+".ndjson")
}

// AuditPath is where one run records the proxy it was actually given. The
// transcript says what the agent did; this says where its traffic was told to
// go, which is the question asked when a run misbehaves on a networked machine.
func (i *Instance) AuditPath(stamp string) string {
	return filepath.Join(i.LogsDir(), "env-"+stamp+".txt")
}
