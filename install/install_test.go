package install_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/install"
	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/proxy"
)

// The suite is offline and non-destructive by construction: downloads go against
// httptest's loopback server, installs run against an instance tree under
// t.TempDir(), and the only host paths touched are stats of files named
// "fakesvc", which cannot exist on a real machine.

const fakeInstaller = `#!/bin/sh
set -e
mkdir -p "$HOME/.fakesvc/bin"
printf '#!/bin/sh\necho fakesvc 9.9.9\n' > "$HOME/.fakesvc/bin/fakesvc"
chmod 755 "$HOME/.fakesvc/bin/fakesvc"
echo "fakesvc installed under $HOME" >&2
`

// fakeInstallerWithLocalBin is the shape that actually matters: an installer that
// also uses the modern personal bin directory. HOME points inside the instance, so
// both copies must land there.
const fakeInstallerWithLocalBin = `#!/bin/sh
set -e
mkdir -p "$HOME/.fakesvc/bin" "$HOME/.local/bin"
printf '#!/bin/sh\necho fakesvc 9.9.9\n' > "$HOME/.fakesvc/bin/fakesvc"
chmod 755 "$HOME/.fakesvc/bin/fakesvc"
printf '#!/bin/sh\necho fakesvc 9.9.9\n' > "$HOME/.local/bin/fakesvc"
chmod 755 "$HOME/.local/bin/fakesvc"
`

const nothingInstaller = `#!/bin/sh
# Exits cleanly and installs nothing, the way an installer that decides it needs
# root or that writes to /usr/local/bin looks from in here.
exit 0
`

const failingInstaller = `#!/bin/sh
echo "nope" >&2
exit 7
`

func fakeEntry() agent.Entry {
	return agent.Entry{
		Name:        "fakesvc",
		Binary:      "fakesvc",
		KnownDirs:   []string{".fakesvc/bin/fakesvc"},
		VersionArgs: []string{"--version"},
	}
}

// testHost is the same fabrication instance/env_test.go uses. HostFromOs is not
// an option here: the environment this suite happens to run in contains a
// developer's proxy, PATH and HOME, and none of those belong in an assertion
// about isolation.
func testHost() instance.HostEnv {
	raw := []string{
		"HOME=/host/home",
		"PATH=/usr/bin:/bin",
		"USER=hostuser",
		"TERM=xterm-256color",
		"TMPDIR=/host/tmp",
	}
	h := instance.HostEnv{Raw: raw, Home: "/host/home", Path: "/usr/bin:/bin",
		User: "hostuser", TmpDir: "/host/tmp", Runtime: "/run/user/1000", Extra: map[string]string{}}
	for _, kv := range raw {
		k, v, _ := strings.Cut(kv, "=")
		h.Extra[k] = v
	}
	return h
}

func newInst(t *testing.T) *instance.Instance {
	t.Helper()
	inst, err := instance.New(filepath.Join(t.TempDir(), "instances"), "fake1")
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Create(); err != nil {
		t.Fatal(err)
	}
	return inst
}

// envFor builds the real child environment, because the isolation this package
// claims comes from BuildEnv's redirections and nothing a test hand-wrote.
func envFor(t *testing.T, inst *instance.Instance) []string {
	t.Helper()
	e, err := instance.BuildEnv(instance.Request{Host: testHost(), Inst: inst})
	if err != nil {
		t.Fatal(err)
	}
	return e.Slice()
}

// interpreter picks an absolute shell. The contract requires one, because execd
// resolves argv[0] against golunch's own PATH rather than the instance's.
func interpreter(t *testing.T) string {
	t.Helper()
	for _, s := range []string{"/bin/bash", "/bin/sh"} {
		if st, err := os.Stat(s); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return s
		}
	}
	t.Skip("no /bin/bash or /bin/sh on this host")
	return ""
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

// runInstall is the shortest honest path: a fresh instance, a real BuildEnv, and
// a script handed over the way --script would hand it over.
func runInstall(t *testing.T, body string, mutate func(*install.Spec)) (install.Result, *instance.Instance, error) {
	t.Helper()
	inst := newInst(t)
	spec := install.Spec{
		Entry:      fakeEntry(),
		Inst:       inst,
		Env:        envFor(t, inst),
		ScriptPath: writeScript(t, body),
		Shell:      interpreter(t),
	}
	if mutate != nil {
		mutate(&spec)
	}
	res, err := install.Install(context.Background(), spec)
	return res, inst, err
}

func assertRealExecutable(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s is a symlink; <root>/bin must hold a real file, because clone copies bin/ but not home/", path)
	}
	if !st.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file (mode %v)", path, st.Mode())
	}
	if st.Mode()&0o111 == 0 {
		t.Fatalf("%s mode %v is not executable", path, st.Mode().Perm())
	}
	return st.Mode().Perm()
}

func TestFetchLandsBytesAndHashesThem(t *testing.T) {
	body := "#!/bin/sh\necho vendor\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	// A dead address in the environment: if the client were built with
	// http.ProxyFromEnvironment this fetch would either fail or be quietly
	// proxied. It must do neither, because the instance decided "no proxy".
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	dst := filepath.Join(t.TempDir(), "scratch", "install.sh")
	res := proxy.Resolution{Profile: proxy.Profile{Name: proxy.Sentinel}}
	got, err := install.Fetch(context.Background(), srv.URL+"/install.sh", dst, 0, res)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("sha256 = %q, want the independently computed %q", got, want)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("wrote %q, want %q", data, body)
	}
}

func TestFetchRejectsOversizeBody(t *testing.T) {
	body := strings.Repeat("# golunch\n", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "install.sh")
	_, err := install.Fetch(context.Background(), srv.URL, dst, 64, proxy.Resolution{})
	if err == nil {
		t.Fatal("an oversized installer was accepted; it would have been executed")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error = %v, want it to name the size limit", err)
	}
	if exists(t, dst) {
		t.Errorf("%s was written although the body was over the limit", dst)
	}
}

func TestFetchRefusesPlaintextRemoteURL(t *testing.T) {
	// No server is needed: the gate runs before any dial, which is the point.
	cases := map[string]string{
		"http://get.example.com/install.sh": "plaintext",
		"ftp://example.com/install.sh":      "unsupported",
		"file:///tmp/install.sh":            "ScriptPath",
	}
	for url, want := range cases {
		_, err := install.Fetch(context.Background(), url, filepath.Join(t.TempDir(), "x.sh"), 0, proxy.Resolution{})
		if err == nil {
			t.Errorf("Fetch(%q) succeeded, want refusal", url)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Fetch(%q) error = %v, want it to mention %q", url, err, want)
		}
	}
}

// TestFetchDefaultLimitIsOneMebibyte pins the sane default. A vendor installer is
// tens of kilobytes, so a body past a megabyte is either a compromised endpoint or
// the wrong endpoint, and either way it must not reach an interpreter.
func TestFetchDefaultLimitIsOneMebibyte(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := strings.Repeat("# golunch install script\n", 50000) // ~1.25 MiB
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "install.sh")
	_, err := install.Fetch(context.Background(), srv.URL, dst, 0, proxy.Resolution{})
	if err == nil {
		t.Fatal("a body over the default 1 MiB limit was accepted")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error = %v", err)
	}

	// Raised explicitly, the same body is fine: the limit is a guard, not a
	// judgment about who writes long scripts.
	if _, err := install.Fetch(context.Background(), srv.URL, dst, 4<<20, proxy.Resolution{}); err != nil {
		t.Errorf("Fetch with a 4 MiB limit = %v, want success", err)
	}
}

func TestInstallWithScriptPathEndToEnd(t *testing.T) {
	res, inst, err := runInstall(t, fakeInstaller, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	// The installer wrote under the redirected HOME, i.e. inside the tree.
	if !exists(t, filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc")) {
		t.Errorf("fakesvc did not land in %s", inst.HomeDir())
	}
	want := filepath.Join(inst.BinDir(), "fakesvc")
	if res.BinaryPath != want {
		t.Errorf("BinaryPath = %q, want %q", res.BinaryPath, want)
	}
	assertRealExecutable(t, res.BinaryPath)
	if res.Version != "9.9.9" {
		t.Errorf("Version = %q, want 9.9.9 probed from the instance copy", res.Version)
	}
	if len(res.Escaped) != 0 {
		t.Errorf("Escaped = %v, want empty: nothing about this run may touch the host", res.Escaped)
	}
	if res.ExitCode != 0 || res.TimedOut {
		t.Errorf("ExitCode = %d, TimedOut = %v, want 0 and false", res.ExitCode, res.TimedOut)
	}
	if res.FetchedAt.IsZero() {
		t.Error("FetchedAt is zero; metadata's fetched_at would be unrecorded")
	}
	// The audit copy is the bytes that ran, kept inside the instance.
	if !pathUnder(inst.LogsDir(), res.ScriptPath) {
		t.Errorf("ScriptPath %q is not under the logs dir %s", res.ScriptPath, inst.LogsDir())
	}
	if !strings.HasSuffix(res.ScriptPath, ".sh") {
		t.Errorf("ScriptPath %q is not a .sh audit copy", res.ScriptPath)
	}
	if got := hashFile(t, res.ScriptPath); got != res.SHA256 {
		t.Errorf("audit copy hashes to %q but Result reports %q; doctor would call that drift", got, res.SHA256)
	}
}

// TestInstallLocalBinStaysInsideInstance is the load-bearing one. The modern
// installer convention is $HOME/.local/bin, which on an unwrapped host is the one
// directory every instance shares; the redirected environment has to keep it
// private.
func TestInstallLocalBinStaysInsideInstance(t *testing.T) {
	hostLocal := filepath.Join(config.RealHome(), ".local", "bin", "fakesvc")
	if exists(t, hostLocal) {
		t.Skipf("%s already exists on this machine; refusing to test against it", hostLocal)
	}
	res, inst, err := runInstall(t, fakeInstallerWithLocalBin, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	inside := filepath.Join(inst.HomeDir(), ".local", "bin", "fakesvc")
	if !exists(t, inside) {
		t.Errorf("%s was not created: the redirected HOME did not reach the installer", inside)
	}
	if exists(t, hostLocal) {
		t.Errorf("the installer wrote %s on the host: HOME was not redirected", hostLocal)
		_ = os.Remove(hostLocal)
	}
	if len(res.Escaped) != 0 {
		t.Errorf("Escaped = %v, want empty", res.Escaped)
	}
	assertRealExecutable(t, res.BinaryPath)
}

func TestInstallRefusesWhenNothingLands(t *testing.T) {
	res, inst, err := runInstall(t, nothingInstaller, nil)
	if err == nil {
		t.Fatalf("Install reported success with BinaryPath %q; an installer that installed nothing must not pass", res.BinaryPath)
	}
	if res.BinaryPath != "" {
		t.Errorf("BinaryPath = %q although nothing was located", res.BinaryPath)
	}
	// The error has to name the escape routes, or the user is left guessing.
	for _, want := range []string{"inside the instance", "/usr/local/bin", "--binary"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	// The script still failed *cleanly*, so the audit copy is the useful artifact.
	if !pathUnder(inst.LogsDir(), res.ScriptPath) {
		t.Errorf("ScriptPath %q: the audit copy should still be retained on failure", res.ScriptPath)
	}
}

func TestInstallReportsScriptFailure(t *testing.T) {
	_, _, err := runInstall(t, failingInstaller, nil)
	if err == nil {
		t.Fatal("a failing installer reported success")
	}
	if !strings.Contains(err.Error(), "code 7") || !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %v, want the exit code and the installer's own stderr", err)
	}
}

// TestInstallTimeoutBoundsTheRun is behavior 9's proof: a network install is not
// allowed to inherit the unbounded default, and the kill has to reach the whole
// process group or the installer's own background children survive as daemons.
func TestInstallTimeoutBoundsTheRun(t *testing.T) {
	hung := "#!/bin/sh\nsleep 60 &\nsleep 60\n"
	start := time.Now()
	res, _, err := runInstall(t, hung, func(s *install.Spec) {
		s.Timeout = 500 * time.Millisecond
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("a hung installer reported success: %+v", res)
	}
	if !res.TimedOut {
		t.Errorf("TimedOut = false, ExitCode = %d", res.ExitCode)
	}
	if elapsed > 20*time.Second {
		t.Errorf("the run took %s; Spec.Timeout did not bound it", elapsed)
	}
	if !strings.Contains(err.Error(), "process group") {
		t.Errorf("error = %v, want it to say the group was killed", err)
	}
}

func TestInstallPinMismatchRefusesToRun(t *testing.T) {
	// The two-call path the CLI stream uses: fetch, print the hash, confirm, then
	// install the saved file with the pin attached. A mismatch has to stop the run.
	res, inst, err := runInstall(t, fakeInstaller, func(s *install.Spec) {
		s.ScriptSHA256 = strings.Repeat("ab", 32)
	})
	if err == nil {
		t.Fatalf("Install ran a script whose hash did not match the pin: %+v", res)
	}
	for _, want := range []string{"sha256", "refusing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	if exists(t, filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc")) {
		t.Error("the mismatched script was executed anyway")
	}
	// Nothing is retained either: the pin is checked before the audit copy is
	// written, so a refused script never becomes instance data.
	if ents, err := os.ReadDir(inst.LogsDir()); err == nil && len(ents) != 0 {
		t.Errorf("logs holds %v after a refused install", ents)
	}
}

func TestInstallPinMismatchAfterFetch(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, fakeInstaller)
	}))
	defer srv.Close()
	inst := newInst(t)
	_, err := install.Install(context.Background(), install.Spec{
		Entry:        fakeEntry(),
		Inst:         inst,
		Env:          envFor(t, inst),
		ScriptURL:    srv.URL + "/install.sh",
		ScriptSHA256: strings.Repeat("00", 32),
		Shell:        interpreter(t),
	})
	if err == nil {
		t.Fatal("a fetched script was run despite the pin")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("the installer was fetched %d times, want exactly once before the hash check", got)
	}
	if exists(t, filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc")) {
		t.Error("the fetched script executed anyway")
	}
}

func TestInstallFromURLPromotesScratchToAudit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fakeInstaller)
	}))
	defer srv.Close()

	inst := newInst(t)
	res, err := install.Install(context.Background(), install.Spec{
		Entry:     fakeEntry(),
		Inst:      inst,
		Env:       envFor(t, inst),
		ScriptURL: srv.URL + "/install.sh",
		Shell:     interpreter(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRealExecutable(t, res.BinaryPath)
	if res.Version != "9.9.9" {
		t.Errorf("Version = %q", res.Version)
	}
	if !pathUnder(inst.LogsDir(), res.ScriptPath) {
		t.Errorf("ScriptPath %q should be the audit copy under %s", res.ScriptPath, inst.LogsDir())
	}
	if hashFile(t, res.ScriptPath) != res.SHA256 {
		t.Errorf("audit copy hash differs from Result.SHA256 = %q", res.SHA256)
	}
	// The scratch download has to be promoted, not left in tmp where an agent
	// could find it and `golunch ls` would count it as instance data forever.
	ents, err := os.ReadDir(inst.TmpDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Errorf("%s still holds %v after the install", inst.TmpDir(), ents)
	}
}

// TestFetchThenConfirmThenInstall is the exact sequence the `golunch install`
// command needs, and the reason Fetch is exported: hash first, show the human,
// and only then hand the saved file to Install with the pin attached. Nothing is
// downloaded twice and the bytes that ran are the bytes that were approved.
func TestFetchThenConfirmThenInstall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fakeInstaller)
	}))
	defer srv.Close()

	inst := newInst(t)
	pending := filepath.Join(inst.TmpDir(), "pending.sh")
	sha, err := install.Fetch(context.Background(), srv.URL+"/install.sh", pending, 0,
		proxy.Resolution{Profile: proxy.Profile{Name: proxy.Sentinel}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := install.Install(context.Background(), install.Spec{
		Entry:        fakeEntry(),
		Inst:         inst,
		Env:          envFor(t, inst),
		ScriptPath:   pending,
		ScriptSHA256: sha,
		Shell:        interpreter(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SHA256 != sha {
		t.Errorf("SHA256 = %q, want the hash the human confirmed (%q)", res.SHA256, sha)
	}
	assertRealExecutable(t, res.BinaryPath)
	if res.Version != "9.9.9" {
		t.Errorf("Version = %q", res.Version)
	}
	// The confirmed file was promoted out of tmp into the audit copy, so tmp does
	// not accumulate installer leftovers.
	if exists(t, pending) {
		t.Errorf("%s survived the install; it should have become the audit copy", pending)
	}
	if !pathUnder(inst.LogsDir(), res.ScriptPath) {
		t.Errorf("ScriptPath %q is not the audit copy under %s", res.ScriptPath, inst.LogsDir())
	}
}

// TestPinVersionReachesTheScript covers both channels a vendor installer can use
// for a requested version, because the registry rows differ: some scripts read an
// environment variable, some take the version as a positional.
func TestPinVersionReachesTheScript(t *testing.T) {
	cases := map[string]struct {
		body string
		info *agent.InstallInfo
	}{
		// The script refuses an extra positional, so the env channel is proven by
		// the install succeeding at all.
		"env var": {
			body: `#!/bin/sh
set -e
[ "$1" = "--quiet" ] || { echo "unexpected argument $1" >&2; exit 1; }
[ -n "$FAKE_VERSION" ] || { echo "FAKE_VERSION was not set" >&2; exit 1; }
mkdir -p "$HOME/.fakesvc/bin"
printf '#!/bin/sh\necho "fakesvc %s"\n' "$FAKE_VERSION" > "$HOME/.fakesvc/bin/fakesvc"
chmod 755 "$HOME/.fakesvc/bin/fakesvc"
`,
			info: &agent.InstallInfo{Args: []string{"--quiet"}, PinEnv: "FAKE_VERSION"},
		},
		// No PinEnv, so the version has to arrive after the registry's fixed args.
		"positional": {
			body: `#!/bin/sh
set -e
[ "$1" = "--quiet" ] || { echo "fixed args were not first" >&2; exit 1; }
mkdir -p "$HOME/.fakesvc/bin"
printf '#!/bin/sh\necho "fakesvc %s"\n' "$2" > "$HOME/.fakesvc/bin/fakesvc"
chmod 755 "$HOME/.fakesvc/bin/fakesvc"
`,
			info: &agent.InstallInfo{Args: []string{"--quiet"}},
		},
	}
	for name, c := range cases {
		e := fakeEntry()
		e.Install = c.info
		inst := newInst(t)
		res, err := install.Install(context.Background(), install.Spec{
			Entry:      e,
			Inst:       inst,
			Env:        envFor(t, inst),
			ScriptPath: writeScript(t, c.body),
			PinVersion: "1.2.3",
			Shell:      interpreter(t),
		})
		if err != nil {
			t.Errorf("%s: Install = %v", name, err)
			continue
		}
		// The probe runs the instance's own copy, so this also proves the pin was
		// honored rather than merely passed.
		if res.Version != "1.2.3" {
			t.Errorf("%s: Version = %q, want the pinned 1.2.3", name, res.Version)
		}
	}
}

func TestInstallMissingPrereqNeverDownloads(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, fakeInstaller)
	}))
	defer srv.Close()

	e := fakeEntry()
	// The registry's claim about the sandbox, disproved: curl is what the vendor
	// script needs and the instance PATH does not have it.
	e.Install = &agent.InstallInfo{Prereqs: []string{"golunch-not-a-real-program"}}
	inst := newInst(t)
	_, err := install.Install(context.Background(), install.Spec{
		Entry:     e,
		Inst:      inst,
		Env:       envFor(t, inst),
		ScriptURL: srv.URL + "/install.sh",
		Shell:     interpreter(t),
	})
	if err == nil {
		t.Fatal("a missing prerequisite was accepted")
	}
	for _, want := range []string{"golunch-not-a-real-program", "instance's PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %q", err, want)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("the server was contacted %d times; prerequisites are checked before any download", got)
	}
	if ents, err := os.ReadDir(inst.LogsDir()); err == nil && len(ents) != 0 {
		t.Errorf("%s holds %v: nothing should have been staged", inst.LogsDir(), ents)
	}
}

// TestInstallPrereqsSeenOnChildPATH documents the parent/child split: golunch's
// own PATH is irrelevant, the instance's bin/ is what the script will use.
func TestInstallPrereqsSeenOnChildPATH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fakeInstaller)
	}))
	defer srv.Close()

	e := fakeEntry()
	e.Install = &agent.InstallInfo{Prereqs: []string{"fakesvchelper"}}
	inst := newInst(t)
	// Place the helper in the instance's bin/, which BuildEnv prepends to PATH.
	helper := filepath.Join(inst.BinDir(), "fakesvchelper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := install.Install(context.Background(), install.Spec{
		Entry:     e,
		Inst:      inst,
		Env:       envFor(t, inst),
		ScriptURL: srv.URL + "/install.sh",
		Shell:     interpreter(t),
	}); err != nil {
		t.Fatalf("a helper found on the child PATH must satisfy the prereq check: %v", err)
	}
	// The same spec without the helper fails, proving the check consulted the
	// child PATH and not merely whatever the host has.
	inst2 := newInst(t)
	if _, err := install.Install(context.Background(), install.Spec{
		Entry:     e,
		Inst:      inst2,
		Env:       envFor(t, inst2),
		ScriptURL: srv.URL + "/install.sh",
		Shell:     interpreter(t),
	}); err == nil {
		t.Fatal("the prereq check passed with no helper on the instance PATH")
	}
}

func TestInstallRequiresAbsoluteUsableShell(t *testing.T) {
	inst := newInst(t)
	env := envFor(t, inst)
	script := writeScript(t, fakeInstaller)
	notExecutable := filepath.Join(t.TempDir(), "not-exec")
	if err := os.WriteFile(notExecutable, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// execd resolves argv[0] against golunch's own PATH, so anything that is not an
	// absolute runnable interpreter is either guessed by the host or not a shell at
	// all. All of it must be refused before the script runs.
	for name, shell := range map[string]string{
		"unset":          "",
		"relative":       "bash",
		"missing":        "/nonexistent/golunch-bash",
		"not-executable": notExecutable,
		"a directory":    inst.BinDir(),
	} {
		spec := install.Spec{Entry: fakeEntry(), Inst: inst, Env: env, ScriptPath: script, Shell: shell}
		if _, err := install.Install(context.Background(), spec); err == nil {
			t.Errorf("%s: interpreter %q was accepted", name, shell)
		}
	}
	// The honest value works, so the refusals above are a gate and not a broken
	// fixture.
	spec := install.Spec{Entry: fakeEntry(), Inst: inst, Env: env, ScriptPath: script, Shell: interpreter(t)}
	if _, err := install.Install(context.Background(), spec); err != nil {
		t.Errorf("Install with %q = %v, want success", spec.Shell, err)
	}
}

func TestInstallNeedsAScript(t *testing.T) {
	inst := newInst(t)
	_, err := install.Install(context.Background(), install.Spec{
		Entry: fakeEntry(),
		Inst:  inst,
		Env:   envFor(t, inst),
		Shell: interpreter(t),
	})
	if err == nil || !strings.Contains(err.Error(), "ScriptPath or ScriptURL") {
		t.Errorf("error = %v, want it to say which field is missing", err)
	}
}

func TestLinkHardlinksWithinTheTree(t *testing.T) {
	inst := newInst(t)
	src := filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(inst.BinDir(), "fakesvc")
	if err := install.Link(src, dst, "fakesvc"); err != nil {
		t.Fatal(err)
	}
	assertRealExecutable(t, dst)
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o755 {
		t.Errorf("mode = %v, want the source's 0755", perm)
	}
	sst, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sst, st) {
		t.Error("the link is not the same file as the source and not a hardlink of it")
	}
}

// TestLinkMaterialisesSymlinkSource is the clone hazard: an installer that leaves
// a symlink in ~/.local/bin must not have that symlink copied into bin/, because
// clone takes bin/ always and home/ only with --copy-data.
func TestLinkMaterialisesSymlinkSource(t *testing.T) {
	inst := newInst(t)
	real := filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("#!/bin/sh\necho fakesvc 9.9.9\n"), 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(inst.HomeDir(), ".local", "bin", "fakesvc")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(inst.BinDir(), "fakesvc")
	if err := install.Link(link, dst, "fakesvc"); err != nil {
		t.Fatal(err)
	}
	perm := assertRealExecutable(t, dst)
	if perm != 0o750 {
		t.Errorf("mode = %v, want the target's 0750 preserved", perm)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "9.9.9") {
		t.Errorf("copy holds %q, want the target's bytes", data)
	}
}

func TestLinkRefusesSourcesOutsideTheInstance(t *testing.T) {
	inst := newInst(t)
	outside := filepath.Join(t.TempDir(), "fakesvc")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(inst.BinDir(), "fakesvc")
	err := install.Link(outside, dst, "fakesvc")
	if err == nil {
		t.Fatal("a host binary was linked into the instance without complaint")
	}
	if !strings.Contains(err.Error(), "outside the instance tree") {
		t.Errorf("error = %v", err)
	}
	if exists(t, dst) {
		t.Errorf("%s was created despite the refusal", dst)
	}
	// A destination that does not name the binary is a metadata typo waiting to
	// become a launcher that execs the wrong program.
	src := filepath.Join(inst.HomeDir(), "fakesvc")
	if err := os.WriteFile(src, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := install.Link(src, filepath.Join(inst.BinDir(), "other"), "fakesvc"); err == nil {
		t.Error("Link accepted a destination that is not the binary's bin entry")
	}
	// Already in place is not an error.
	if err := install.Link(src, src, "fakesvc"); err != nil {
		t.Errorf("Link of a file onto itself = %v, want nil", err)
	}
}

func TestVerifyReportsEscapedHostPaths(t *testing.T) {
	inst := newInst(t)
	host := t.TempDir()
	found := filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc")
	newFile := filepath.Join(host, ".local", "bin", "fakesvc")
	alreadyThere := filepath.Join(host, ".fakesvc", "bin", "fakesvc")
	pattern := filepath.Join(host, "nvm", "versions", "node", "*", "bin", "fakesvc")
	patternMatch := filepath.Join(host, "nvm", "versions", "node", "v22", "bin", "fakesvc")
	if err := os.MkdirAll(filepath.Dir(alreadyThere), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alreadyThere, []byte("host copy"), 0o755); err != nil {
		t.Fatal(err)
	}

	before := map[string]bool{newFile: false, alreadyThere: true, pattern: false}
	// The run escapes: it drops a file in the host's personal bin and creates a
	// whole new node version dir on the host.
	if err := os.MkdirAll(filepath.Dir(newFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(patternMatch), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patternMatch, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	escaped, err := install.Verify(fakeEntry(), inst, found, before)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{newFile, patternMatch}
	sort.Strings(want)
	if len(escaped) != len(want) {
		t.Fatalf("Escaped = %v, want %v", escaped, want)
	}
	for i := range want {
		if escaped[i] != want[i] {
			t.Fatalf("Escaped = %v, want %v (sorted, so the printed list is stable)", escaped, want)
		}
	}
}

func TestVerifyRejectsBinariesOutsideTheInstance(t *testing.T) {
	inst := newInst(t)
	host := t.TempDir()
	outside := filepath.Join(host, ".local", "bin", "fakesvc")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := install.Verify(fakeEntry(), inst, outside, map[string]bool{}); err == nil {
		t.Error("Verify accepted a binary outside the instance root")
	} else if !strings.Contains(err.Error(), "outside the instance root") {
		t.Errorf("error = %v", err)
	}
	// A located file whose name is not the registry's binary means the lookup and
	// the row disagree, which would otherwise be recorded as the right install.
	inside := filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "notfakesvc")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := install.Verify(fakeEntry(), inst, inside, nil); err == nil {
		t.Error("Verify accepted a binary that is not the entry's")
	}
}

func pathUnder(root, p string) bool {
	r := filepath.Clean(root)
	c := filepath.Clean(p)
	return c == r || strings.HasPrefix(c, r+string(filepath.Separator))
}
