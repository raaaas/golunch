package instance

import (
	"strings"
	"testing"

	"github.com/raaaas/golunch/proxy"
)

func testHost(extra ...string) HostEnv {
	raw := append([]string{
		"HOME=/home/tester",
		"PATH=/usr/bin:/bin",
		"USER=tester",
		"TERM=xterm-256color",
		"XDG_CONFIG_HOME=/home/tester/.config",
		"XDG_DATA_HOME=/home/tester/.local/share",
		"TMPDIR=/host/tmp",
		"WARREN_INSTANCE=qoder-1",
		"WARREN_INSTANCE_DIR=/home/tester/.warren/instances/qoder-1",
		"DISPLAY=:0",
	}, extra...)
	h := HostEnv{Raw: raw, Home: "/home/tester", Path: "/usr/bin:/bin", User: "tester",
		TmpDir: "/host/tmp", Runtime: "/run/user/1000", Extra: map[string]string{}}
	for _, kv := range raw {
		k, v, _ := strings.Cut(kv, "=")
		h.Extra[k] = v
	}
	return h
}

func setup(t *testing.T) *Instance {
	t.Helper()
	inst, err := New(t.TempDir(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Create(); err != nil {
		t.Fatal(err)
	}
	return inst
}

func TestBuildEnvIsolationGolden(t *testing.T) {
	inst := setup(t)
	e, err := BuildEnv(Request{Host: testHost(), Inst: inst})
	if err != nil {
		t.Fatal(err)
	}

	wantPath := map[string]string{
		"HOME":             inst.HomeDir(),
		"XDG_CONFIG_HOME":  inst.ConfigDir(),
		"XDG_CACHE_HOME":   inst.CacheDir(),
		"XDG_DATA_HOME":    inst.DataDir(),
		"XDG_STATE_HOME":   inst.StateDir(),
		"XDG_RUNTIME_DIR":  inst.RuntimeDir(),
		"TMPDIR":           inst.TmpDir(),
		"GOLUNCH_INSTANCE": inst.Alias,
	}
	for k, want := range wantPath {
		got, _ := e.Get(k)
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	// PATH must put the instance bin first.
	p, _ := e.Get("PATH")
	if !strings.HasPrefix(p, inst.BinDir()+":") {
		t.Errorf("PATH = %q, want instance bin first", p)
	}
	if !strings.HasSuffix(p, "/usr/bin:/bin") {
		t.Errorf("PATH = %q, want host PATH preserved at the end", p)
	}

	// XDG_DATA_DIRS: private data first, system shares still readable.
	dd, _ := e.Get("XDG_DATA_DIRS")
	if !strings.HasPrefix(dd, inst.DataDir()+":") || !strings.Contains(dd, "/usr/share") {
		t.Errorf("XDG_DATA_DIRS = %q, want instance data then system dirs", dd)
	}
}

// Nested-hazard guard: this project is developed inside a warren instance, so
// WARREN_* and the outer instance's markers must never reach the child — the
// child would otherwise believe it is a different instance than it is.
func TestBuildEnvStripsNestingMarkers(t *testing.T) {
	e, err := BuildEnv(Request{Host: testHost(), Inst: setup(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range e.Slice() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "WARREN_") {
			t.Errorf("%s leaked into the child environment: %s", k, kv)
		}
	}
	if v, ok := e.Get("GOLUNCH_INSTANCE"); !ok || v != "env1" {
		t.Errorf("GOLUNCH_INSTANCE = %q, want env1", v)
	}
}

// Host passthrough is the other half of isolation: TERM/USER/DISPLAY must
// survive or interactive agents misbehave.
func TestBuildEnvKeepsBenignHostVars(t *testing.T) {
	e, _ := BuildEnv(Request{Host: testHost(), Inst: setup(t)})
	for _, k := range []string{"TERM", "USER", "DISPLAY"} {
		if _, ok := e.Get(k); !ok {
			t.Errorf("%s should be inherited from the host", k)
		}
	}
}

func TestBuildEnvNoDuplicateKeys(t *testing.T) {
	cases := []struct {
		name string
		req  func(*Instance) Request
	}{
		{"plain", func(i *Instance) Request { return Request{Host: testHost(), Inst: i} }},
		{"proxy-flag", func(i *Instance) Request {
			p, _ := proxy.FromSpec("http://127.0.0.1:7890")
			return Request{Host: testHost("HTTP_PROXY=http://old:1", "http_proxy=http://old:1"), Inst: i,
				Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceFlag}}
		}},
		{"proxy-none", func(i *Instance) Request {
			p, _ := proxy.FromSpec("none")
			return Request{Host: testHost("HTTPS_PROXY=http://old:1", "https_proxy=http://old:2"), Inst: i,
				Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceNone}}
		}},
		{"inherit", func(i *Instance) Request {
			return Request{Host: testHost("HTTP_PROXY=http://host:1", "NO_PROXY=keepme"), Inst: i,
				Proxy: proxy.Resolution{Source: proxy.SourceInherit}}
		}},
		{"env-override", func(i *Instance) Request {
			p, _ := proxy.FromSpec("http://127.0.0.1:7890")
			return Request{Host: testHost(), Inst: i, Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceFlag},
				Extra: map[string]string{"HTTP_PROXY": "http://explicit:9", "TERM": ""}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, err := BuildEnv(c.req(setup(t)))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, kv := range e.Slice() {
				k, _, _ := strings.Cut(kv, "=")
				if seen[k] {
					t.Errorf("duplicate key %s in child EnvS; exec would resolve it by last-wins and hide the conflict", k)
				}
				seen[k] = true
			}
		})
	}
}

func TestBuildEnvProxyLayers(t *testing.T) {
	inst := setup(t)

	t.Run("flag proxy sets all case pairs", func(t *testing.T) {
		p, _ := proxy.FromSpec("http://127.0.0.1:7890")
		e, _ := BuildEnv(Request{Host: testHost(), Inst: inst,
			Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceFlag}})
		for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
			v, ok := e.Get(name)
			if !ok || v != "http://127.0.0.1:7890" {
				t.Errorf("%s = %q/%v, want the URL", name, v, ok)
			}
			if got := e.Origin(name); got != string(proxy.SourceFlag) {
				t.Errorf("%s origin = %q, want %q so `env` can explain the layer", name, got, proxy.SourceFlag)
			}
		}
	})

	t.Run("none sentinel removes host proxy entirely", func(t *testing.T) {
		p, _ := proxy.FromSpec("none")
		e, _ := BuildEnv(Request{
			Host:  testHost("HTTP_PROXY=http://host:1", "http_proxy=http://host:1", "NO_PROXY=host"),
			Inst:  inst,
			Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceNone},
		})
		for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy",
			"ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
			if v, ok := e.Get(name); ok {
				t.Errorf("%s = %q survived the none sentinel", name, v)
			}
		}
	})

	t.Run("inherit leaves host proxy untouched", func(t *testing.T) {
		e, _ := BuildEnv(Request{
			Host:  testHost("HTTP_PROXY=http://host:8888", "NO_PROXY=.internal"),
			Inst:  inst,
			Proxy: proxy.Resolution{Source: proxy.SourceInherit},
		})
		if v, _ := e.Get("HTTP_PROXY"); v != "http://host:8888" {
			t.Errorf("HTTP_PROXY = %q, want the host value untouched", v)
		}
	})

	// The invariant that matters most operationally.
	t.Run("loopback floor survives every input", func(t *testing.T) {
		for _, extra := range [][]string{nil, {".corp"}, {"att.local"}} {
			p, _ := proxy.FromSpec("http://127.0.0.1:7890")
			e, _ := BuildEnv(Request{
				Host:  testHost("NO_PROXY=hostentry"),
				Inst:  inst,
				Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceFlag, NoProxyExtra: extra},
			})
			np, ok := e.Get("NO_PROXY")
			if !ok {
				t.Fatal("NO_PROXY missing")
			}
			for _, must := range []string{"localhost", "127.0.0.0/8", "::1"} {
				if !strings.Contains(np, must) {
					t.Errorf("NO_PROXY=%q lost floor entry %q", np, must)
				}
			}
			if !strings.Contains(np, "hostentry") {
				t.Errorf("NO_PROXY=%q dropped the host's own exception", np)
			}
		}
	})

	t.Run("https falls back to http when only http configured", func(t *testing.T) {
		// A profile hand-edited in TOML may set only `http`. Every case name
		// must still end up populated, or the agent's https traffic quietly
		// bypasses the proxy.
		p := proxy.Profile{Name: "partial", HTTP: "http://only-http:1"}
		e, _ := BuildEnv(Request{Host: testHost(), Inst: inst,
			Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceInstance}})
		if v, _ := e.Get("HTTPS_PROXY"); v != "http://only-http:1" {
			t.Errorf("HTTPS_PROXY = %q, want fallback to the HTTP URL", v)
		}
		if v, _ := e.Get("https_proxy"); v != "http://only-http:1" {
			t.Errorf("https_proxy = %q, want fallback in both cases", v)
		}
	})
}

func TestBuildEnvExtraOverridesEverything(t *testing.T) {
	p, _ := proxy.FromSpec("http://proxy:1")
	e, _ := BuildEnv(Request{
		Host:  testHost(),
		Inst:  setup(t),
		Proxy: proxy.Resolution{Profile: p, Source: proxy.SourceInstance},
		Extra: map[string]string{"HTTP_PROXY": "http://forced:2"},
	})
	if v, _ := e.Get("HTTP_PROXY"); v != "http://forced:2" {
		t.Errorf("HTTP_PROXY = %q, want the --env value to win", v)
	}
	// An --env with an empty value means unset.
	e2, _ := BuildEnv(Request{Host: testHost(), Inst: setup(t), Extra: map[string]string{"TERM": ""}})
	if _, ok := e2.Get("TERM"); ok {
		t.Error("empty --env value should remove the variable")
	}
}

func TestBuildEnvKeepVars(t *testing.T) {
	e, _ := BuildEnv(Request{
		Host:     testHost("NODE_EXTRA_CA_CERTS=/host/ca.pem"),
		Inst:     setup(t),
		KeepVars: []string{"NODE_EXTRA_CA_CERTS"},
	})
	if v, _ := e.Get("NODE_EXTRA_CA_CERTS"); v != "/host/ca.pem" {
		t.Errorf("NODE_EXTRA_CA_CERTS = %q, want driver keep-var honored", v)
	}
}

func TestBakedKeysAreExecutable(t *testing.T) {
	e, _ := BuildEnv(Request{Host: testHost("NODE_EXTRA_CA_CERTS=/host/ca.pem"), Inst: setup(t),
		KeepVars: []string{"NODE_EXTRA_CA_CERTS"}})
	out := e.RenderKeys(e.Baked())
	for _, want := range []string{
		"export HOME=" + ShellQuote(e.values["HOME"]),
		"export PATH=",
		"export XDG_CONFIG_HOME=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("launcher render missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "export WARREN_") {
		t.Error("launcher must not re-export nesting markers")
	}
	if !strings.Contains(out, "NODE_EXTRA_CA_CERTS") {
		t.Error("a driver keep-var is a decision, not an accident, so it is baked")
	}
}
