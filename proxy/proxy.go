package proxy

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

// CaseVars are the eight environment variables a proxy setting actually has
// to touch. C-style libraries read the uppercase forms, most Node HTTP agents
// read only the lowercase ones, and a few read both and disagree. Setting one
// and leaving the other inherited from the host is the classic "why is only
// half my traffic proxied" bug, so we always delete all eight and re-add pairs.
var CaseVars = []struct{ Upper, Lower string }{
	{"HTTP_PROXY", "http_proxy"},
	{"HTTPS_PROXY", "https_proxy"},
	{"ALL_PROXY", "all_proxy"},
	{"NO_PROXY", "no_proxy"},
}

// Sentinel disables proxying for an instance even when the host environment
// already exports a proxy, which plain "unset in config" cannot do because
// unset means "inherit".
const Sentinel = "none"

// Profile is a resolved proxy configuration. Empty fields mean "do not set
// this variable", which is distinct from Sentinel's "actively unset".
type Profile struct {
	Name    string   `toml:"-"`
	HTTP    string   `toml:"http"`
	HTTPS   string   `toml:"https"`
	SOCKS   string   `toml:"socks"`
	NoProxy []string `toml:"no_proxy"`
}

// FromSpec turns user input into a Profile. Accepts a bare URL (applied to
// every scheme) or one of the named forms. Returns a Profile with only Name
// set for the "none" sentinel.
func FromSpec(spec string) (Profile, error) {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "":
		return Profile{}, nil
	case strings.EqualFold(spec, Sentinel):
		return Profile{Name: Sentinel}, nil
	}
	if !strings.Contains(spec, "://") {
		return Profile{}, fmt.Errorf("proxy %q must include a scheme, e.g. http://127.0.0.1:7890 or socks5://host:1080", spec)
	}
	u, err := url.Parse(spec)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid proxy URL %q: %w", spec, err)
	}
	if u.Host == "" {
		return Profile{}, fmt.Errorf("proxy URL %q has no host", spec)
	}
	p := Profile{Name: spec}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h", "socks4", "socks":
		p.SOCKS = spec
	default:
		p.HTTP = spec
		p.HTTPS = spec
	}
	return p, nil
}

func (p Profile) Disabled() bool { return p.Name == Sentinel }

// Any reports whether the profile would set at least one variable.
func (p Profile) Any() bool {
	return p.HTTP != "" || p.HTTPS != "" || p.SOCKS != "" || len(p.NoProxy) > 0
}

// Floor is never proxied, whatever the user configured. Without loopback in
// NO_PROXY, a CLI attaching to a local server (`kilo run --attach
// http://127.0.0.1:4096`) or talking to a local model runner (ollama on
// :11434) gets routed out to the upstream proxy and fails in confusing ways.
func Floor() []string {
	// Both the literal address and the range: CIDR is understood by Go's
	// httpproxy and Node's proxy-agent, but curl — and therefore every CLI that
	// shells out to it — matches NO_PROXY entries as plain suffixes, so
	// "127.0.0.0/8" alone would not bypass a request to http://127.0.0.1:4096.
	entries := []string{"localhost", "127.0.0.1", "127.0.0.0/8", "::1", ".localhost", "169.254.169.254"}
	if hn, err := os.Hostname(); err == nil && hn != "" {
		entries = append(entries, hn)
	}
	return entries
}

// MergeEntries unions proxy exception lists preserving first-seen order, so
// output is stable and diffable. User entries come before the floor.
func MergeEntries(groups ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range groups {
		for _, raw := range g {
			for _, e := range strings.Split(raw, ",") {
				e = strings.TrimSpace(e)
				if e == "" || seen[strings.ToLower(e)] {
					continue
				}
				seen[strings.ToLower(e)] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// Values returns the variables this profile wants, in both case variants.
// A disabled profile returns an empty map meaning "unset everything".
func (p Profile) Values(extraNoProxy []string) map[string]string {
	if p.Disabled() {
		return map[string]string{}
	}
	set := func(m map[string]string, name, v string) {
		if v == "" {
			return
		}
		m[name] = v
		m[strings.ToLower(name)] = v
	}
	vals := map[string]string{}
	set(vals, "HTTP_PROXY", p.HTTP)
	set(vals, "HTTPS_PROXY", p.HTTPS)
	set(vals, "ALL_PROXY", p.SOCKS)
	// A socks-only upstream still needs NO_PROXY computed, but if nothing at
	// all is being proxied there is no point asserting exceptions.
	if p.Any() {
		np := MergeEntries(extraNoProxy, p.NoProxy, Floor())
		set(vals, "NO_PROXY", strings.Join(np, ","))
	}
	return vals
}

// Source labels which precedence layer produced the effective profile, so
// `golunch env` can explain itself instead of just dumping values.
type Source string

const (
	SourceInherit  Source = "inherited from host environment"
	SourceGlobal   Source = "global config default"
	SourceProfile  Source = "global config profile"
	SourceInstance Source = "instance metadata override"
	SourceFlag     Source = "command-line flag"
	SourceNone     Source = "explicitly disabled"
)

// Resolution is the winner of the precedence contest plus provenance.
type Resolution struct {
	Profile Profile
	Source  Source
	// NoProxyExtra is merged in from global config regardless of winner.
	NoProxyExtra []string
	// Layers records every candidate considered, lowest precedence first.
	Layers []string
}

// Resolve applies precedence: flag > instance > global. An empty spec at any
// layer means "no opinion, fall through". Candidates are examined in ascending
// precedence so the last non-empty one wins outright.
func Resolve(flagSpec, instanceSpec, globalSpec string, profiles map[string]Profile, globalNoProxy []string) (Resolution, error) {
	res := Resolution{Source: SourceInherit, NoProxyExtra: globalNoProxy}
	candidates := []struct {
		spec   string
		source Source
		label  string
	}{
		{globalSpec, SourceGlobal, "global"},
		{instanceSpec, SourceInstance, "instance"},
		{flagSpec, SourceFlag, "flag"},
	}
	for _, c := range candidates {
		spec := strings.TrimSpace(c.spec)
		if spec == "" {
			continue
		}
		// A name without a scheme is a profile reference, except on the
		// command line where typing a full URL is the documented interface and
		// a bare word is far more likely a mistake than a profile name.
		if !strings.Contains(spec, "://") && !strings.EqualFold(spec, Sentinel) && c.source != SourceFlag {
			p, ok := profiles[strings.ToLower(spec)]
			if !ok {
				var keys []string
				for k := range profiles {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				return res, fmt.Errorf("unknown proxy profile %q (configured profiles: %v)", spec, keys)
			}
			p.Name = strings.ToLower(spec)
			res.Profile = p
			res.Source = SourceProfile
			res.Layers = append(res.Layers, fmt.Sprintf("%s: profile %s", c.label, p.Name))
			continue
		}
		p, err := FromSpec(spec)
		if err != nil {
			return res, fmt.Errorf("%s proxy: %w", c.label, err)
		}
		res.Profile = p
		if p.Disabled() {
			res.Source = SourceNone
		} else {
			res.Source = c.source
		}
		res.Layers = append(res.Layers, fmt.Sprintf("%s: %s", c.label, spec))
	}
	return res, nil
}
