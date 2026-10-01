package instance

import (
	"strings"
	"testing"

	"github.com/raaaas/golunch/proxy"
)

// The one invariant the two execution paths must never break: `golunch run`
// hands the child an explicit environ, the launcher exports a subset and lets
// the shell supply the rest. Every value golunch decides must appear in the
// generated script with the same bytes, and nothing golunch decided to unset
// may survive there.
func TestLauncherMatchesBuildEnv(t *testing.T) {
	inst := setup(t)
	meta := &Metadata{}
	meta.Proxy.Spec = "http://proxy.test:8888"
	meta.Proxy.NoProxy = []string{".corp.test"}

	res, err := proxy.Resolve("", meta.Proxy.Spec, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := BuildEnv(Request{Host: testHost("HTTPS_PROXY=http://host:1/"), Inst: inst, Meta: meta, Proxy: res})
	if err != nil {
		t.Fatal(err)
	}
	body := RenderLauncher(inst.Alias, e, inst, []string{"/usr/bin/kilo"}, res)

	child := e.Map()
	for _, k := range e.Baked() {
		want := "export " + k + "=" + ShellQuote(child[k])
		if !strings.Contains(body, want) {
			t.Errorf("launcher lacks %q\n---\n%s", want, body)
		}
	}
	// The other direction: a value exported here but not produced by BuildEnv
	// means the launcher grew a second implementation of something.
	baked := map[string]bool{}
	for _, k := range e.Baked() {
		baked[k] = true
	}
	for _, line := range strings.Split(body, "\n") {
		name := strings.TrimPrefix(line, "export ")
		if line == name || !strings.Contains(name, "=") {
			continue
		}
		if k := name[:strings.Index(name, "=")]; !baked[k] {
			t.Errorf("launcher exports %s, which BuildEnv does not set: the two paths have diverged", k)
		}
	}
	if strings.Contains(body, "unset") {
		t.Errorf("a decided proxy must not also be unset:\n%s", body)
	}
	// The host's own HTTPS_PROXY must not survive our override in either path.
	if v := child["HTTPS_PROXY"]; v == "http://host:1/" {
		t.Fatal("BuildEnv failed to override the host proxy")
	}
	if !strings.HasSuffix(body, "exec /usr/bin/kilo \"$@\"\n") {
		t.Errorf("launcher must exec the recorded command with caller args, got %q", tailLines(body, 2))
	}
}

// A "none" spec has to reach the launcher too, otherwise the flag that exists
// specifically to beat an inherited host proxy only works on one of the two
// paths.
func TestLauncherUnsetsDisabledProxy(t *testing.T) {
	inst := setup(t)
	res, err := proxy.Resolve("none", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := BuildEnv(Request{Host: testHost("HTTP_PROXY=http://host:1/"), Inst: inst, Proxy: res})
	if err != nil {
		t.Fatal(err)
	}
	body := RenderLauncher(inst.Alias, e, inst, []string{"/usr/bin/kilo"}, res)
	if !strings.Contains(body, "unset HTTP_PROXY http_proxy") {
		t.Errorf("disabled proxy must be unset in the launcher, got\n%s", body)
	}
	if strings.Contains(body, "export HTTP_PROXY") {
		t.Error("disabled proxy must not be exported")
	}
}

func TestLauncherKeepsHostVarsOut(t *testing.T) {
	inst := setup(t)
	e, err := BuildEnv(Request{
		Host:  testHost("SSH_AUTH_SOCK=/run/user/1000/s", "TERM=xterm-256color"),
		Inst:  inst,
		Proxy: proxy.Resolution{},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := RenderLauncher(inst.Alias, e, inst, nil, proxy.Resolution{})
	for _, k := range []string{"SSH_AUTH_SOCK", "TERM"} {
		if strings.Contains(body, "export "+k+"=") {
			t.Errorf("%s is inherited by the shell already; baking it freezes a session into %s", k, body)
		}
	}
}

func tailLines(body string, n int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
