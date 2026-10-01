package proxy

import (
	"strings"
	"testing"
)

func TestFromSpec(t *testing.T) {
	cases := []struct {
		in            string
		wantHTTP      string
		wantSOCKS     string
		wantDisabled  bool
		wantErrSubstr string
	}{
		{in: "http://127.0.0.1:7890", wantHTTP: "http://127.0.0.1:7890"},
		{in: "socks5://127.0.0.1:1080", wantSOCKS: "socks5://127.0.0.1:1080"},
		{in: "SOCKS5H://host:1080", wantSOCKS: "SOCKS5H://host:1080"},
		{in: "none", wantDisabled: true},
		{in: "NONE", wantDisabled: true},
		{in: "127.0.0.1:7890", wantErrSubstr: "must include a scheme"},
		{in: "http://", wantErrSubstr: "no host"},
		{in: "", wantHTTP: "", wantSOCKS: ""},
	}
	for _, c := range cases {
		p, err := FromSpec(c.in)
		if c.wantErrSubstr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErrSubstr) {
				t.Errorf("FromSpec(%q) err = %v, want containing %q", c.in, err, c.wantErrSubstr)
			}
			continue
		}
		if err != nil {
			t.Errorf("FromSpec(%q) unexpected error %v", c.in, err)
			continue
		}
		if p.HTTP != c.wantHTTP {
			t.Errorf("FromSpec(%q).HTTP = %q, want %q", c.in, p.HTTP, c.wantHTTP)
		}
		if p.SOCKS != c.wantSOCKS {
			t.Errorf("FromSpec(%q).SOCKS = %q, want %q", c.in, p.SOCKS, c.wantSOCKS)
		}
		if p.Disabled() != c.wantDisabled {
			t.Errorf("FromSpec(%q).Disabled() = %v, want %v", c.in, p.Disabled(), c.wantDisabled)
		}
	}
}

// A bare http URL must populate both HTTP and HTTPS: an agent hitting an
// https model endpoint with only HTTP_PROXY set silently bypasses the proxy.
func TestBareURLCoversHTTPS(t *testing.T) {
	p, err := FromSpec("http://gw.internal:3128")
	if err != nil {
		t.Fatal(err)
	}
	if p.HTTPS != p.HTTP {
		t.Errorf("HTTPS = %q, want %q", p.HTTPS, p.HTTP)
	}
	vals := p.Values(nil)
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		if vals[name] != "http://gw.internal:3128" {
			t.Errorf("%s = %q, want the URL in both cases", name, vals[name])
		}
	}
}

func TestResolvePrecedence(t *testing.T) {
	profiles := map[string]Profile{
		"clash": {Name: "clash", HTTP: "http://127.0.0.1:7890", HTTPS: "http://127.0.0.1:7890"},
	}
	t.Run("flag beats instance beats global", func(t *testing.T) {
		r, err := Resolve("http://flag:1", "http://inst:2", "http://glob:3", profiles, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.Profile.HTTP != "http://flag:1" || r.Source != SourceFlag {
			t.Errorf("got %q / %v, want flag winner", r.Profile.HTTP, r.Source)
		}
	})
	t.Run("empty flag falls through to instance", func(t *testing.T) {
		r, _ := Resolve("", "http://inst:2", "http://glob:3", profiles, nil)
		if r.Profile.HTTP != "http://inst:2" || r.Source != SourceInstance {
			t.Errorf("got %q / %v, want instance winner", r.Profile.HTTP, r.Source)
		}
	})
	t.Run("nothing configured means inherit", func(t *testing.T) {
		r, _ := Resolve("", "", "", profiles, nil)
		if r.Source != SourceInherit || r.Profile.Any() {
			t.Errorf("got %+v, want bare inherit", r)
		}
	})
	t.Run("named profile resolves from global table", func(t *testing.T) {
		r, err := Resolve("", "clash", "", profiles, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.Profile.Name != "clash" || r.Source != SourceProfile {
			t.Errorf("got %+v / %v, want clash profile", r.Profile, r.Source)
		}
	})
	t.Run("unknown profile names an error listing what exists", func(t *testing.T) {
		_, err := Resolve("", "typo", "", profiles, nil)
		if err == nil || !strings.Contains(err.Error(), "clash") {
			t.Errorf("err = %v, want mention of configured profiles", err)
		}
	})
	t.Run("none sentinel disables and beats a lower url", func(t *testing.T) {
		r, err := Resolve("", "none", "http://glob:3", profiles, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Profile.Disabled() || r.Source != SourceNone {
			t.Errorf("got %+v / %v, want disabled", r.Profile, r.Source)
		}
	})
}

// The loopback floor is the single most important invariant in this package:
// losing it breaks `kilo run --attach http://127.0.0.1:PORT` and any local
// model runner.
func TestNoProxyFloorAlwaysPresent(t *testing.T) {
	p := Profile{Name: "x", HTTP: "http://h:1", HTTPS: "http://h:1"}
	for _, extra := range [][]string{nil, {".corp.example"}, {"--garbage--"}} {
		vals := p.Values(extra)
		np := vals["NO_PROXY"]
		for _, must := range []string{"localhost", "127.0.0.0/8", "::1"} {
			if !strings.Contains(np, must) {
				t.Errorf("NO_PROXY %q missing floor entry %q (extra=%v)", np, must, extra)
			}
		}
		if _, ok := vals["no_proxy"]; !ok {
			t.Error("lowercase no_proxy missing; Node agents read only that form")
		}
	}
}

func TestMergeEntriesOrderAndDedup(t *testing.T) {
	got := MergeEntries([]string{"b,a"}, []string{"A", "c"}, Floor())
	joined := strings.Join(got, ",")
	if !strings.HasPrefix(joined, "b,a,c,") {
		t.Errorf("MergeEntries = %q, want user order preserved and case-insensitive dedup", joined)
	}
	seen := map[string]bool{}
	for _, e := range got {
		if seen[e] {
			t.Errorf("duplicate entry %q in %q", e, joined)
		}
		seen[e] = true
	}
	// Empty and whitespace-only entries are dropped.
	for _, e := range got {
		if strings.TrimSpace(e) != e || e == "" {
			t.Errorf("untrimmed or empty entry %q", e)
		}
	}
}

func TestDisabledProfileEmitsNothing(t *testing.T) {
	p, _ := FromSpec("none")
	if len(p.Values([]string{"x"})) != 0 {
		t.Error("disabled profile must not set any variable")
	}
}

// A socks-only upstream should not claim to proxy plain HTTP.
func TestSocksOnly(t *testing.T) {
	p, _ := FromSpec("socks5://127.0.0.1:1080")
	vals := p.Values(nil)
	if vals["ALL_PROXY"] == "" {
		t.Error("ALL_PROXY unset for socks upstream")
	}
	if _, ok := vals["HTTP_PROXY"]; ok {
		t.Error("HTTP_PROXY set for a socks-only profile")
	}
}
