package install

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raaaas/golunch/proxy"
)

// The ambient environment is poisoned on every case below. http.ProxyFromEnvironment
// would hand the transport this dead address for a public host, so a selector that
// ignores it is proof the client is built from the instance's resolution and from
// nothing else — which is the whole point of not using that helper.
func poisonEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "socks5://127.0.0.1:1")
	t.Setenv("NO_PROXY", "example.com,should-not-matter")
}

func TestProxyFuncUsesInstanceDecision(t *testing.T) {
	poisonEnv(t)
	fn, err := proxyFunc(proxy.Resolution{Profile: proxy.Profile{HTTP: "http://proxy.test:1"}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		url  string
		want string
	}{
		{"https://example.com/install.sh", "http://proxy.test:1"},
		{"http://example.com/install.sh", "http://proxy.test:1"},
		// Loopback must bypass whatever the profile says, or an installer
		// talking to a local model runner gets routed out to the upstream proxy.
		{"http://127.0.0.1:8080/install.sh", ""},
		{"http://localhost:8080/install.sh", ""},
		{"http://[::1]:8080/install.sh", ""},
		// An address inside the range but not equal to the literal is matched
		// only by the CIDR, which is the half of Floor() that curl ignores and
		// Go honours.
		{"http://127.0.0.99/install.sh", ""},
		// A public host is not exempt just because the developer's own shell
		// exported NO_PROXY=example.com: that is golunch's environment, not the
		// instance's decision.
		{"https://sub.example.com/install.sh", "http://proxy.test:1"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.url, nil)
		got, err := fn(req)
		if err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		want := ""
		if got != nil {
			want = got.String()
		}
		if want != c.want {
			t.Errorf("%s: proxy = %q, want %q", c.url, want, c.want)
		}
	}
}

func TestProxyFuncExceptionsAndSchemes(t *testing.T) {
	poisonEnv(t)
	// An instance-level exception must cover the zone, not one hostname, and a
	// port-bearing entry must still bypass: failing to match would send internal
	// traffic back out through the proxy.
	fn, err := proxyFunc(proxy.Resolution{
		Profile:      proxy.Profile{HTTP: "http://proxy.test:1", HTTPS: "https://secure.test:2"},
		NoProxyExtra: []string{"intranet.example", "proxy.test:8443", "10.0.0.0/8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		url  string
		want string
	}{
		{"https://sub.intranet.example/x", ""},
		{"https://proxy.test:8443/x", ""},
		{"http://10.1.2.3/x", ""},
		{"http://10.99.0.1/x", ""},
		// https prefers HTTPS_PROXY, and the scheme that survives is the one the
		// request needs.
		{"https://example.com/x", "https://secure.test:2"},
		{"http://example.com/x", "http://proxy.test:1"},
	}
	for _, c := range cases {
		got, err := fn(httptest.NewRequest(http.MethodGet, c.url, nil))
		if err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		want := ""
		if got != nil {
			want = got.String()
		}
		if want != c.want {
			t.Errorf("%s: proxy = %q, want %q", c.url, want, c.want)
		}
	}

	// A socks-only upstream carries both schemes, as ALL_PROXY does in the
	// child's environment.
	socks, err := proxyFunc(proxy.Resolution{Profile: proxy.Profile{SOCKS: "socks5://proxy.test:1080"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"https://example.com/x", "http://example.com/x"} {
		got, err := socks(httptest.NewRequest(http.MethodGet, url, nil))
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.String() != "socks5://proxy.test:1080" {
			t.Errorf("%s: proxy = %v, want the socks upstream", url, got)
		}
	}
}

func TestProxyFuncDisabledAndUnsetBothGoDirect(t *testing.T) {
	poisonEnv(t)
	for name, res := range map[string]proxy.Resolution{
		// The "none" sentinel: an actively disabled instance proxy.
		"disabled": {Profile: proxy.Profile{Name: proxy.Sentinel}},
		// Nothing configured at any layer is not the same decision, but it has
		// the same effect here: golunch's own proxy is not the instance's.
		"unset": {},
	} {
		fn, err := proxyFunc(res)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, url := range []string{"https://example.com/x", "http://example.com/x"} {
			got, err := fn(httptest.NewRequest(http.MethodGet, url, nil))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got != nil {
				t.Errorf("%s: %s proxied through %v, want direct", name, url, got)
			}
		}
	}
}

func TestProxyFuncRejectsMalformedProfile(t *testing.T) {
	if _, err := proxyFunc(proxy.Resolution{Profile: proxy.Profile{HTTP: "http://ex ample/"}}); err == nil {
		t.Error("a malformed proxy URL must be an error, not a silent direct connection")
	}
	if _, err := proxyFunc(proxy.Resolution{Profile: proxy.Profile{HTTPS: "socks5://"}}); err == nil {
		t.Error("a proxy URL with no host must be an error")
	}
}

func TestExceptHostDetails(t *testing.T) {
	entries := []string{"example.com", ".zone.test", "127.0.0.0/8", "::1", "HOST4.LOCAL"}
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"EXAMPLE.com", true},
		{"sub.example.com", true},
		{"notexample.com", false},
		{".zone.test", true},
		{"deep.zone.test", true},
		{"127.0.0.1", true},
		{"127.5.5.5", true},
		{"128.0.0.1", false},
		{"::1", true},
		{"host4.local", true},
		{"", false},
	}
	for _, c := range cases {
		if got := exceptHost(entries, c.host); got != c.want {
			t.Errorf("exceptHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1":   true,
		"localhost":   true,
		"LOCALHOST":   true,
		"::1":         true,
		"127.0.0.99":  true,
		"10.0.0.1":    false,
		"example.com": false,
	} {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestParseScriptURLGate(t *testing.T) {
	// https is always fine, loopback http is the offline test suite and local
	// file servers, and everything else is refused because these bytes are
	// executed.
	for _, ok := range []string{
		"https://get.example.com/install.sh",
		"http://127.0.0.1:8080/install.sh",
		"http://localhost:8080/install.sh",
	} {
		if _, err := parseScriptURL(ok); err != nil {
			t.Errorf("parseScriptURL(%q) = %v, want it accepted", ok, err)
		}
	}
	for bad, why := range map[string]string{
		"":                            "no script URL",
		"http://get.example.com/x.sh": "plaintext",
		"ftp://example.com/x.sh":      "unsupported",
		"file:///tmp/x.sh":            "ScriptPath",
		"/tmp/x.sh":                   "has no host",
	} {
		_, err := parseScriptURL(bad)
		if err == nil {
			t.Errorf("parseScriptURL(%q) accepted a URL it should refuse", bad)
			continue
		}
		if want := why; !strings.Contains(err.Error(), want) {
			t.Errorf("parseScriptURL(%q) error = %v, want it to mention %q", bad, err, want)
		}
	}
}
