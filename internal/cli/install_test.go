package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/raaaas/golunch"
	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/instance"
)

// The suite is offline by construction: installs go in through --script or an
// httptest loopback URL, the data root is testApp's temp dir, and the only
// host names involved are stats of files called "fakesvc"/"cline", whose
// absence before and after the run is exactly what the isolation tests
// assert.

// fakeInstaller writes the binary to the classic curl-bash location AND to
// $HOME/.local/bin — the second is the load-bearing part, because on an
// unwrapped host that directory is shared by every instance. The installed
// binary records its execution in marker, so "this ran, here" is a fact a
// test reads rather than infers.
func fakeInstaller(marker string) string {
	if marker == "" {
		marker = "/dev/null"
	}
	return fmt.Sprintf(`#!/bin/sh
set -e
mkdir -p "$HOME/.fakesvc/bin" "$HOME/.local/bin"
cat > "$HOME/.fakesvc/bin/fakesvc" <<'EOF'
#!/bin/sh
echo "fakesvc 9.9.9" > %s
echo "fakesvc 9.9.9"
EOF
chmod 755 "$HOME/.fakesvc/bin/fakesvc"
cp "$HOME/.fakesvc/bin/fakesvc" "$HOME/.local/bin/fakesvc"
`, instance.ShellQuote(marker))
}

// clineInstaller is the same transaction for a real registry row: cline's
// KnownDirs plus LocateIn's .local/bin fallback are how the install package
// proves the binary landed inside the instance.
const clineInstaller = `#!/bin/sh
set -e
mkdir -p "$HOME/.cline/bin" "$HOME/.local/bin"
cat > "$HOME/.cline/bin/cline" <<'EOF'
#!/bin/sh
echo "cline 8.8.8"
EOF
chmod 755 "$HOME/.cline/bin/cline"
cp "$HOME/.cline/bin/cline" "$HOME/.local/bin/cline"
`

func fakeSvcEntry() agent.Entry {
	return agent.Entry{
		Name:        "fakesvc",
		Binary:      "fakesvc",
		KnownDirs:   []string{".fakesvc/bin/fakesvc"},
		VersionArgs: []string{"--version"},
	}
}

func writeInstallerFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "installer.sh")
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileSHA(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

var installerHex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// requireNoTerminal makes a.confirm's /dev/tty open fail, so the consent path
// refuses exactly the way it does when scripted. Setsid detaches the process
// from its controlling terminal; where there was none to begin with (CI) the
// open already fails and nothing changes. What must never happen is the test
// silently inheriting a developer's terminal.
func requireNoTerminal(t *testing.T) {
	t.Helper()
	if _, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err != nil {
		return
	}
	if _, serr := syscall.Setsid(); serr != nil {
		t.Fatalf("setsid to detach from the controlling terminal: %v", serr)
	}
	if _, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		t.Fatal("/dev/tty is still openable after setsid; consent could not be tested honestly")
	}
}

// errCode names the exit code an error maps to, and -1 for "no error", so a
// test comparing against ExitOK can never pass on nil by accident.
func errCode(err error) int {
	if err == nil {
		return -1
	}
	return golunch.ErrCode(err)
}

// TestInstallIntoKeepsInstallerInsideTheInstance is the direct analogue of
// TestRunPassthroughUsesInstanceEnv: the fake installer is aimed at $HOME and
// at $HOME/.local/bin, and the real home must come out of it untouched. The
// entry is a fabricated one handed straight to installInto, because
// agent.Lookup cannot see a test's fake agent — the seam buildSeedPlan set.
func TestInstallIntoKeepsInstallerInsideTheInstance(t *testing.T) {
	app, out, errb := testApp(t)
	ctx := context.Background()

	hostLocal := filepath.Join(config.RealHome(), ".local", "bin", "fakesvc")
	if pathExists(hostLocal) {
		t.Fatalf("%s already exists on this host; the before/after isolation assertion needs a clean one", hostLocal)
	}

	newTestInstance(t, app, "i1", "--binary", "/bin/sh", "--link=false")
	inst, meta, err := app.Open("i1")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ran")

	binFakesvc := filepath.Join(inst.BinDir(), "fakesvc")
	if err := app.installInto(ctx, fakeSvcEntry(), inst, &meta, installOpts{
		script:  writeInstallerFile(t, fakeInstaller(marker)),
		yes:     true,
		timeout: time.Minute,
	}); err != nil {
		t.Fatalf("installInto: %v\nstdout:%s\nstderr:%s", err, out.String(), errb.String())
	}

	// Both landing spots are inside the instance...
	for _, p := range []string{
		filepath.Join(inst.HomeDir(), ".fakesvc", "bin", "fakesvc"),
		filepath.Join(inst.HomeDir(), ".local", "bin", "fakesvc"),
	} {
		if !pathExists(p) {
			t.Errorf("%s was not created: the redirected HOME did not reach the installer", p)
		}
	}
	// ...and the host's shared personal bin is byte-for-byte untouched.
	if pathExists(hostLocal) {
		t.Errorf("the installer created %s on the host: HOME was not redirected", hostLocal)
	}
	if !pathExists(marker) {
		t.Error("the installed binary never executed; every other assertion would be vacuous")
	}

	// <root>/bin/fakesvc must be a real executable file, not a symlink: clone
	// copies bin/ always and home/ only with --copy-data.
	st, err := os.Lstat(binFakesvc)
	if err != nil {
		t.Fatalf("lstat %s: %v", binFakesvc, err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() || st.Mode()&0o111 == 0 {
		t.Errorf("%s mode %v: want a real executable regular file", binFakesvc, st.Mode())
	}

	// The metadata contract doctor reads, from the copy on disk.
	m2, err := inst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if m2.Install == nil {
		t.Fatal("no [install] table was recorded; doctor would report nothing and clone could reprefix nothing")
	}
	if m2.Launch.Command[0] != binFakesvc {
		t.Errorf("launch command = %v, want the instance copy", m2.Launch.Command)
	}
	if m2.Launch.Binary != "fakesvc" {
		t.Errorf("launch binary = %q, want fakesvc", m2.Launch.Binary)
	}
	if m2.Install.Binary != binFakesvc || !pathUnder(inst.Root, m2.Install.Binary) {
		t.Errorf("install binary = %q, want %s inside the root", m2.Install.Binary, binFakesvc)
	}
	if !pathUnder(inst.Root, m2.Install.Dir) {
		t.Errorf("install dir = %q, want it inside %s", m2.Install.Dir, inst.Root)
	}
	if !pathUnder(inst.LogsDir(), m2.Install.ScriptPath) || !pathExists(m2.Install.ScriptPath) {
		t.Errorf("install script_path = %q, want a retained audit copy under %s", m2.Install.ScriptPath, inst.LogsDir())
	}
	if !installerHex64.MatchString(m2.Install.SHA256) {
		t.Errorf("install sha256 = %q, want 64 lowercase hex", m2.Install.SHA256)
	}
	if got := fileSHA(t, m2.Install.ScriptPath); got != m2.Install.SHA256 {
		t.Errorf("audit copy hashes to %s but the record says %s; doctor would warn install drift immediately", got, m2.Install.SHA256)
	}
	if m2.Install.FetchedAt.IsZero() {
		t.Error("install fetched_at is zero")
	}
	if m2.Install.Agent != "fakesvc" || m2.Install.Version != "9.9.9" {
		t.Errorf("install agent/version = %q/%q", m2.Install.Agent, m2.Install.Version)
	}
	if m2.Instance.Version != "9.9.9" {
		t.Errorf("instance version = %q, want the probed 9.9.9", m2.Instance.Version)
	}
	if m2.Install.URL != "" {
		t.Errorf("install url = %q for a --script install; the absence must be honest", m2.Install.URL)
	}
	// The scratch area must not keep executable leftovers.
	if ents, err := os.ReadDir(inst.TmpDir()); err == nil && len(ents) != 0 {
		t.Errorf("%s still holds %v after the install", inst.TmpDir(), ents)
	}

	// The env and the launcher both point at the instance copy.
	out.Reset()
	if err := app.CmdEnv([]string{"i1"}); err != nil {
		t.Fatal(err)
	}
	var pathVal string
	for _, l := range strings.Split(out.String(), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "PATH" {
			pathVal = f[1]
		}
	}
	if !strings.HasPrefix(pathVal, inst.BinDir()+":") {
		t.Errorf("child PATH = %q, want it to begin with %s:", pathVal, inst.BinDir())
	}
	launcher, err := os.ReadFile(inst.LauncherPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(launcher), "exec "+binFakesvc) {
		t.Errorf("regenerated launcher does not exec the instance copy:\n%s", launcher)
	}

	// Passthrough must now run the instance copy for real, and the instance
	// must be unlocked again after the install.
	if probe, lerr := inst.Acquire(instance.Shared); lerr != nil {
		t.Fatalf("the exclusive install lock was not released: %v", lerr)
	} else {
		probe.Release()
	}
	os.Remove(marker)
	if err := app.CmdRun(ctx, []string{"i1", "--", "--version"}); err != nil {
		t.Fatalf("passthrough: %v\n%s", err, errb.String())
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("passthrough did not run the instance copy (marker missing): %v", err)
	}
	if !strings.Contains(string(got), "fakesvc 9.9.9") {
		t.Errorf("marker holds %q", got)
	}

	// doctor must accept the record exactly as installInto wrote it.
	if err := app.CmdDoctor([]string{"i1"}); err != nil {
		t.Fatalf("doctor after install: %v\nstdout:%s\nstderr:%s", err, out.String(), errb.String())
	}
}

// TestCmdInstallURLPath runs the whole command surface against a real
// registry row: the two-call consent path (fetch, print URL and hash and
// note, then --yes), the prereq check on the child PATH, and the URL in the
// record.
func TestCmdInstallURLPath(t *testing.T) {
	app, out, errb := testApp(t)
	ctx := context.Background()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, clineInstaller)
	}))
	defer srv.Close()

	newTestInstance(t, app, "c9", "--agent", "cline", "--binary", "/bin/sh", "--link=false")
	inst, _, err := app.Open("c9")
	if err != nil {
		t.Fatal(err)
	}
	// cline's registry row claims sh, curl, git, node and npm on the CHILD
	// PATH. Satisfy that claim inside the instance so the test does not
	// depend on what this particular developer box happens to have.
	for _, prereq := range []string{"sh", "curl", "git", "node", "npm"} {
		p := filepath.Join(inst.BinDir(), prereq)
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := app.CmdInstall(ctx, []string{"c9", "--url", srv.URL + "/install.sh", "--yes"}); err != nil {
		t.Fatalf("install: %v\nstdout:%s\nstderr:%s", err, out.String(), errb.String())
	}
	if hits != 1 {
		t.Errorf("the installer was fetched %d times, want exactly once for the shown hash", hits)
	}
	shown := out.String()
	for _, want := range []string{srv.URL, "sha256", "upstream shell installer"} {
		if !strings.Contains(shown, want) {
			t.Errorf("install output lacks %q:\n%s", want, shown)
		}
	}

	binCline := filepath.Join(inst.BinDir(), "cline")
	m2, err := inst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if m2.Install == nil || m2.Install.URL != srv.URL+"/install.sh" {
		t.Fatalf("[install] url not recorded: %+v", m2.Install)
	}
	if m2.Launch.Command[0] != binCline || m2.Install.Binary != binCline {
		t.Errorf("launch/install binary = %v / %q, want %s", m2.Launch.Command, m2.Install.Binary, binCline)
	}
	if m2.Install.Version != "8.8.8" {
		t.Errorf("recorded version = %q", m2.Install.Version)
	}
	if !pathUnder(inst.LogsDir(), m2.Install.ScriptPath) {
		t.Errorf("audit copy %q not under %s", m2.Install.ScriptPath, inst.LogsDir())
	}
	if !installerHex64.MatchString(m2.Install.SHA256) || fileSHA(t, m2.Install.ScriptPath) != m2.Install.SHA256 {
		t.Errorf("sha256 %q does not match the audit copy", m2.Install.SHA256)
	}
	if m2.Install.FetchedAt.IsZero() {
		t.Error("fetched_at is zero")
	}
	// The promoted scratch file must not linger in tmp/.
	if ents, err := os.ReadDir(inst.TmpDir()); err == nil && len(ents) != 0 {
		t.Errorf("%s still holds %v", inst.TmpDir(), ents)
	}
	// What the user sees afterwards: the alias runs the downloaded copy.
	out.Reset()
	errb.Reset()
	if err := app.CmdRun(ctx, []string{"c9", "--", "--version"}); err != nil {
		t.Fatalf("passthrough: %v\n%s", err, errb.String())
	}
	if err := app.CmdDoctor([]string{"c9"}); err != nil {
		t.Fatalf("doctor: %v\n%s\n%s", err, out.String(), errb.String())
	}
}

func TestCmdInstallValidations(t *testing.T) {
	app, _, _ := testApp(t)
	ctx := context.Background()

	if err := app.CmdInstall(ctx, []string{}); errCode(err) != ExitUsage {
		t.Fatalf("missing alias = %v, want a usage error", err)
	}
	if err := app.CmdInstall(ctx, []string{"ghost"}); errCode(err) != ExitNotFound {
		t.Errorf("unknown alias = %v (code %d), want ExitNotFound", err, errCode(err))
	}

	newTestInstance(t, app, "b1", "--binary", "/bin/cat", "--link=false")
	err := app.CmdInstall(ctx, []string{"b1", "--url", "https://example.invalid/install.sh"})
	if code := errCode(err); code != ExitUsage || !strings.Contains(err.Error(), "agent") {
		t.Errorf("no-agent instance = %v (code %d), want a usage error naming the missing agent", err, code)
	}

	newTestInstance(t, app, "c2", "--agent", "cline", "--binary", "/bin/sh", "--link=false")
	err = app.CmdInstall(ctx, []string{"c2"})
	if code := errCode(err); code != ExitUsage {
		t.Errorf("no script source = %v (code %d), want ExitUsage", err, code)
	} else {
		for _, want := range []string{"--url", "--script", "verified"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("source error must name %q (the registry ships empty ScriptURLs on purpose): %v", want, err)
			}
		}
	}

	if err := app.CmdInstall(ctx, []string{"c2", "--url", "https://example.invalid/i.sh", "--script", "/bin/sh"}); errCode(err) != ExitUsage {
		t.Errorf("--url with --script = %v, want ExitUsage", err)
	}
	if err := app.CmdInstall(ctx, []string{"c2", "--script", "/nonexistent/golunch-installer.sh", "--yes"}); errCode(err) != ExitNotFound {
		t.Errorf("--script pointing nowhere = %v (code %d), want ExitNotFound", err, errCode(err))
	}
}

func TestCmdInstallConsentRefusedWithoutTty(t *testing.T) {
	requireNoTerminal(t)
	app, _, errb := testApp(t)
	ctx := context.Background()

	newTestInstance(t, app, "i4", "--binary", "/bin/sh", "--link=false")
	inst, meta, err := app.Open("i4")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(inst.MetadataPath())
	if err != nil {
		t.Fatal(err)
	}

	if err := app.installInto(ctx, fakeSvcEntry(), inst, &meta, installOpts{
		script:  writeInstallerFile(t, fakeInstaller("")),
		timeout: time.Minute,
	}); err != nil {
		t.Fatalf("a declined install is a refusal, not a failure: %v", err)
	}
	if !strings.Contains(errb.String(), "refused") || !strings.Contains(errb.String(), "left untouched") {
		t.Errorf("expected the refusal on stderr, got:\n%s", errb.String())
	}
	if pathExists(filepath.Join(inst.BinDir(), "fakesvc")) ||
		pathExists(filepath.Join(inst.HomeDir(), ".fakesvc")) {
		t.Error("a declined installer ran anyway")
	}
	for _, d := range []string{inst.LogsDir(), inst.TmpDir()} {
		if ents, _ := os.ReadDir(d); len(ents) != 0 {
			t.Errorf("declined install left %v in %s", ents, d)
		}
	}
	after, err := os.ReadFile(inst.MetadataPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("metadata.toml changed on a declined install:\n%s", after)
	}
}

func TestCmdInstallDryRunFetchesAndWritesNothing(t *testing.T) {
	app, out, errb := testApp(t)
	ctx := context.Background()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, clineInstaller)
	}))
	defer srv.Close()

	newTestInstance(t, app, "i5", "--agent", "cline", "--binary", "/bin/sh", "--link=false")
	inst, err := instance.New(app.InstancesDir(), "i5")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(inst.MetadataPath())
	if err != nil {
		t.Fatal(err)
	}

	if err := app.CmdInstall(ctx, []string{"i5", "--url", srv.URL + "/install.sh", "--dry-run", "--yes"}); err != nil {
		t.Fatalf("dry run: %v\n%s", err, errb.String())
	}
	if hits != 0 {
		t.Errorf("--dry-run fetched the installer %d times", hits)
	}
	got := out.String()
	for _, want := range []string{"dry run", inst.Root, filepath.Join(inst.BinDir(), "cline"), srv.URL, "proxy"} {
		if !strings.Contains(got, want) {
			t.Errorf("plan output lacks %q:\n%s", want, got)
		}
	}
	after, err := os.ReadFile(inst.MetadataPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("--dry-run rewrote metadata.toml:\n%s", after)
	}
	for _, d := range []string{inst.BinDir(), inst.LogsDir(), inst.TmpDir()} {
		if ents, _ := os.ReadDir(d); len(ents) != 0 {
			t.Errorf("%s holds %v after a dry run", d, ents)
		}
	}
}

func TestInstallShaMismatchRunsNothing(t *testing.T) {
	app, _, errb := testApp(t)
	ctx := context.Background()
	newTestInstance(t, app, "i6", "--binary", "/bin/sh", "--link=false")
	inst, meta, err := app.Open("i6")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ran")

	err = app.installInto(ctx, fakeSvcEntry(), inst, &meta, installOpts{
		script:  writeInstallerFile(t, fakeInstaller(marker)),
		sha256:  strings.Repeat("ab", 32),
		yes:     true,
		timeout: time.Minute,
	})
	if err == nil {
		t.Fatal("a script that does not match its pin was accepted")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error = %v, want it to name the digest mismatch", err)
	}
	if pathExists(marker) {
		t.Error("the mismatched installer ran anyway")
	}
	if pathExists(filepath.Join(inst.BinDir(), "fakesvc")) {
		t.Error("a mismatched install produced a binary")
	}
	if ents, _ := os.ReadDir(inst.LogsDir()); len(ents) != 0 {
		t.Errorf("a refused script must never become instance data, logs/ holds %v", ents)
	}
	_ = errb
}

func TestInstallBusyExitsBusy(t *testing.T) {
	app, _, _ := testApp(t)
	ctx := context.Background()
	newTestInstance(t, app, "i7", "--agent", "cline", "--binary", "/bin/sh", "--link=false")
	inst, err := instance.New(app.InstancesDir(), "i7")
	if err != nil {
		t.Fatal(err)
	}
	shared, err := inst.Acquire(instance.Shared)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Release()

	err = app.CmdInstall(ctx, []string{"i7", "--url", "https://127.0.0.1:9/install.sh", "--yes"})
	if code := errCode(err); code != ExitBusy {
		t.Errorf("busy instance = %v (code %d), want ExitBusy (%d)", err, code, ExitBusy)
	}
	// Nothing was fetched, staged or executed before the refusal.
	if ents, _ := os.ReadDir(inst.LogsDir()); len(ents) != 0 {
		t.Errorf("a busy refusal staged an installer anyway: %v", ents)
	}
	if ents, _ := os.ReadDir(inst.TmpDir()); len(ents) != 0 {
		t.Errorf("a busy refusal left %v in tmp/", ents)
	}
}

func TestInstallChildExitCodeIsPreserved(t *testing.T) {
	app, _, _ := testApp(t)
	ctx := context.Background()
	newTestInstance(t, app, "i8", "--binary", "/bin/sh", "--link=false")
	inst, meta, err := app.Open("i8")
	if err != nil {
		t.Fatal(err)
	}
	script := writeInstallerFile(t, "#!/bin/sh\necho nope >&2\nexit 7\n")
	err = app.installInto(ctx, fakeSvcEntry(), inst, &meta, installOpts{script: script, yes: true, timeout: time.Minute})
	if code := errCode(err); code != 7 {
		t.Fatalf("failing installer = %v (code %d), want the child's own exit code 7", err, code)
	}
	// The audit copy is the artifact an operator needs after a failure; it is
	// retained in logs/ and named in the error.
	if !strings.Contains(err.Error(), inst.LogsDir()) {
		t.Errorf("error = %v, want it to point at the retained audit copy", err)
	}
	ents, _ := os.ReadDir(inst.LogsDir())
	if len(ents) != 1 || !strings.HasPrefix(ents[0].Name(), "install-") {
		t.Errorf("logs/ holds %v, want the one audit script", ents)
	}
	m2, err := inst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if m2.Install != nil {
		t.Error("a failed install recorded an [install] table; doctor would call it an integrity error")
	}
}

// TestInstallThroughMainAndUsage covers the dispatch row: a name lookup that
// returns ExitNotFound rather than ExitUsage can only come from a wired
// command reaching a.Runner.Open.
func TestInstallThroughMainAndUsage(t *testing.T) {
	testApp(t) // registers the temp GOLUNCH_ROOT that config.Load reads
	ctx := context.Background()
	var out, errb bytes.Buffer

	if code := Main(ctx, []string{"install", "ghost"}, strings.NewReader(""), &out, &errb, ""); code != ExitNotFound {
		t.Fatalf("Main(install ghost) exit = %d, want %d (%s)", code, ExitNotFound, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Main(ctx, nil, strings.NewReader(""), &out, &errb, ""); code != ExitUsage {
		t.Fatalf("bare golunch exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "golunch install") {
		t.Errorf("usage must list the install command:\n%s", errb.String())
	}
}

func TestNewInstallWarnsAndSkipsWhenHostHasBinary(t *testing.T) {
	app, _, errb := testApp(t)
	dir := t.TempDir()
	hostCline := filepath.Join(dir, "cline")
	if err := os.WriteFile(hostCline, []byte("#!/bin/sh\necho cline 1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	newTestInstance(t, app, "n1", "--agent", "cline", "--install", "--url", "https://127.0.0.1:9/install.sh", "--link=false")
	warned := errb.String()
	if !strings.Contains(warned, "already installed on this host") || !strings.Contains(warned, "downloaded nothing") {
		t.Errorf("--install with a host copy must warn and not download:\n%s", warned)
	}
	inst, meta, err := app.Open("n1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Launch.Command[0] != hostCline {
		t.Errorf("launch command = %v, want the host copy %s", meta.Launch.Command, hostCline)
	}
	if meta.Install != nil {
		t.Error("a skipped download must not record an [install] table")
	}
	if ents, _ := os.ReadDir(inst.BinDir()); len(ents) != 0 {
		t.Errorf("bin/ holds %v although nothing was installed", ents)
	}
}

func TestNewInstallFlagValidation(t *testing.T) {
	app, _, _ := testApp(t)
	err := app.CmdNew(context.Background(), []string{"x1", "--install", "--link=false"})
	if code := errCode(err); code != ExitUsage || !strings.Contains(err.Error(), "--agent") {
		t.Errorf("--install without --agent = %v (code %d), want a usage error naming --agent", err, code)
	}
	err = app.CmdNew(context.Background(), []string{"x2", "--install", "--agent", "cline", "--binary", "/bin/sh", "--link=false"})
	if code := errCode(err); code != ExitUsage || !strings.Contains(err.Error(), "--binary") {
		t.Errorf("--install with --binary = %v (code %d), want a usage error naming --binary", err, code)
	}
}

// procLive reports whether pid is still a process. A zombie counts as dead: the
// kill already reached it and only the reaping is outstanding.
func procLive(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	i := bytes.LastIndexByte(data, ')')
	if i < 0 || i+2 >= len(data) {
		return false
	}
	return data[i+2] != 'Z'
}

// TestNewWithInstallCancellationReachesTheInstaller is why CmdNew takes a
// context: an installer is a child process that can hang, and `new --install`
// used to run it under context.Background(), so Ctrl-C stopped golunch and
// left the vendor script running against a deleted lock.
func TestNewWithInstallCancellationReachesTheInstaller(t *testing.T) {
	app, _, errb := testApp(t)

	// The script records the pid it is waiting on, so the assertion is "the
	// kernel says that process is gone", not "the call came back".
	pidfile := filepath.Join(t.TempDir(), "sleep.pid")
	script := writeInstallerFile(t, fmt.Sprintf(`#!/bin/sh
sleep 120 &
echo $! > %s
wait
`, instance.ShellQuote(pidfile)))

	inst, err := instance.New(app.InstancesDir(), "cancel1")
	if err != nil {
		t.Fatal(err)
	}
	meta := instance.Metadata{}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- app.newWithInstall(ctx, inst, &meta, fakeSvcEntry(), false, false,
			installOpts{script: script, yes: true, timeout: 5 * time.Minute})
	}()

	pid := 0
	deadline := time.Now().Add(15 * time.Second)
	for pid == 0 {
		if data, rerr := os.ReadFile(pidfile); rerr == nil {
			if n, serr := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); serr != nil || n != 1 || pid <= 0 {
				pid = 0
			}
		}
		if pid == 0 {
			if time.Now().After(deadline) {
				defer cancel()
				t.Fatalf("the installer never recorded its child:\n%s", errb.String())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	cancel()

	var gerr error
	select {
	case gerr = <-errc:
	case <-time.After(30 * time.Second):
		defer cancel()
		t.Fatalf("newWithInstall kept waiting on the installer after the context was canceled")
	}
	if gerr == nil {
		defer cancel()
		t.Fatal("a canceled install returned no error")
	}

	time.Sleep(200 * time.Millisecond)
	if procLive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("pid %d outlived the canceled command: the install child is not in the ctx's process group", pid)
	}
	if _, err := os.Stat(inst.LauncherPath()); err == nil {
		t.Error("a canceled install wrote a launcher")
	}
}
