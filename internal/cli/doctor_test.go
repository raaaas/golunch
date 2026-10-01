package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raaaas/golunch/instance"
)

// TestEmptyDirIsNotAlwaysFalse pins the bug that made doctor's config warning
// dead code: Readdirnames reports io.EOF together with an empty slice, so a
// check written as "no error and no names" is false for every genuinely empty
// directory.
func TestEmptyDirIsNotAlwaysFalse(t *testing.T) {
	dir := t.TempDir()
	if !emptyDir(dir) {
		t.Error("emptyDir returned false for an empty directory")
	}
	if err := os.WriteFile(dir+"/x", []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if emptyDir(dir) {
		t.Error("emptyDir returned false positive for a populated directory")
	}
	if emptyDir(dir + "/nope") {
		t.Error("emptyDir returned true for a missing directory")
	}
}

// TestDoctorConfigWarningClearsOnSeed is the loop the two commands are for: a
// fresh instance is warned that it loads no host plugins, and seeding makes the
// warning go away without ever copying the login.
func TestDoctorConfigWarningClearsOnSeed(t *testing.T) {
	app, out, _ := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")

	if err := app.CmdDoctor([]string{"k1"}); err != nil {
		t.Fatalf("doctor: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "plugins/agents/skills") {
		t.Fatalf("expected the empty-config warning, got %q", out.String())
	}

	out.Reset()
	if err := app.CmdSeed([]string{"k1", "mcp", "skills", "--host-home", host}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := app.CmdDoctor([]string{"k1"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "plugins/agents/skills") {
		t.Errorf("warning survived seeding: %q", out.String())
	}
	if !strings.Contains(out.String(), "logged out") {
		t.Errorf("a seeded instance should still be logged out: %q", out.String())
	}
}

// installedFixture fabricates the state a download leaves behind: a real
// executable inside the instance, the vendor install script retained under
// logs/, and the [install] table recording both. It is assembled by hand because
// metadata.toml is hand-editable in production too, and a doctor that only
// trusts a machine-written record is not checking anything.
func installedFixture(t *testing.T, app *App, alias, sub string) (*instance.Instance, instance.Metadata) {
	t.Helper()
	newTestInstance(t, app, alias, "--agent", "cline", "--binary", "/bin/cat", "--link=false")
	inst, meta, err := app.Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	dir := inst.Dir(sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "cline")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(inst.LogsDir(), "install-cline.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n# the installer we fetched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(script)
	if err != nil {
		t.Fatal(err)
	}
	meta.Launch.Command = []string{bin}
	meta.Launch.Binary = "cline"
	meta.Install = &instance.InstallInfo{
		Agent:      "cline",
		URL:        "https://cline.bot/install.sh",
		SHA256:     sum,
		ScriptPath: script,
		Version:    "1.2.3",
		Dir:        dir,
		Binary:     bin,
		FetchedAt:  time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
	}
	if err := inst.SaveMetadata(&meta); err != nil {
		t.Fatal(err)
	}
	// Pointing the command at the instance binary makes the baked launcher
	// stale, and doctor reports that as launcher drift. Regenerate it so these
	// tests see only the install findings they are about.
	if err := app.CmdRefresh([]string{alias}); err != nil {
		t.Fatal(err)
	}
	return inst, meta
}

func TestDoctorReportsInstallProvenance(t *testing.T) {
	app, out, errb := testApp(t)
	inst, meta := installedFixture(t, app, "d1", "bin")

	if err := app.CmdDoctor([]string{"d1"}); err != nil {
		t.Fatalf("a complete download must not fail doctor: %v\n%s\n%s", err, out.String(), errb.String())
	}
	got := out.String()
	for _, want := range []string{
		"INFO [d1] install:",
		meta.Install.URL,
		"into " + relOf(inst.Root, inst.BinDir()),
		meta.Install.SHA256[:12],
		"2026-03-04T05:06:07Z",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the install finding lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "install integrity") || strings.Contains(got, "install drift") {
		t.Errorf("a sound download raised neither finding, got:\n%s", got)
	}
}

// TestDoctorFlagsAHostBinaryInTheInstallTable is the whole point of the check:
// the promise is that a downloaded instance runs a file inside its own tree, and
// an edited metadata.toml claiming /usr/local/bin/cline breaks that promise
// quietly unless something actually looks.
func TestDoctorFlagsAHostBinaryInTheInstallTable(t *testing.T) {
	app, out, _ := testApp(t)
	inst, meta := installedFixture(t, app, "d1", "bin")
	meta.Install.Binary = "/usr/local/bin/cline"
	if err := inst.SaveMetadata(&meta); err != nil {
		t.Fatal(err)
	}

	err := app.CmdDoctor([]string{"d1"})
	if err == nil {
		t.Fatalf("doctor accepted an install record pointing outside the instance:\n%s", out.String())
	}
	if ce, ok := err.(CommandError); !ok || ce.Code != ExitError {
		t.Errorf("error = %v (%T), want CommandError with code %d", err, err, ExitError)
	}
	got := out.String()
	if !strings.Contains(got, "ERROR [d1] install integrity:") || !strings.Contains(got, "/usr/local/bin/cline") {
		t.Errorf("no integrity finding for the escaped path:\n%s", got)
	}
	if !strings.Contains(got, "golunch install d1") || !strings.Contains(got, "golunch rm d1") {
		t.Errorf("the finding must name both recoveries:\n%s", got)
	}
}

func TestDoctorFlagsAMissingInstallBinary(t *testing.T) {
	app, out, errb := testApp(t)
	inst, meta := installedFixture(t, app, "d1", "bin")
	// The instance itself still launches, so the ordinary command check stays
	// quiet; only the install record knows a downloaded file went missing.
	meta.Install.Binary = filepath.Join(inst.BinDir(), "gone")
	if err := inst.SaveMetadata(&meta); err != nil {
		t.Fatal(err)
	}

	if err := app.CmdDoctor([]string{"d1"}); err == nil {
		t.Fatalf("doctor accepted an install record whose binary is absent:\n%s\n%s", out.String(), errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "ERROR [d1] install integrity:") || !strings.Contains(got, "gone is no longer present") {
		t.Errorf("no integrity finding for a vanished binary:\n%s", got)
	}
}

func TestDoctorWarnsOnInstallerDrift(t *testing.T) {
	app, out, errb := testApp(t)
	inst, _ := installedFixture(t, app, "d1", "bin")
	script := filepath.Join(inst.LogsDir(), "install-cline.sh")

	if err := os.WriteFile(script, []byte("#!/bin/sh\n# something else entirely\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.CmdDoctor([]string{"d1"}); err != nil {
		t.Fatalf("drift alone is a warning, not an error: %v\n%s\n%s", err, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "WARN [d1] install drift:") {
		t.Errorf("a rewritten audit script must be named:\n%s", out.String())
	}

	// logs/ is the one directory a user is expected to prune, so a missing audit
	// copy is benign and must not train them to ignore the real finding.
	out.Reset()
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}
	if err := app.CmdDoctor([]string{"d1"}); err != nil {
		t.Fatalf("a pruned audit copy must not fail doctor: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "install drift") {
		t.Errorf("an absent audit copy was reported as drift:\n%s", out.String())
	}
}

func TestCloneCarriesTheDownloadedCopy(t *testing.T) {
	app, out, errb := testApp(t)
	src, meta := installedFixture(t, app, "src", "bin")
	if err := app.CmdClone([]string{"src", "dst", "--link=false"}); err != nil {
		t.Fatalf("clone: %v\n%s\n%s", err, out.String(), errb.String())
	}

	dst, err := instance.New(app.InstancesDir(), "dst")
	if err != nil {
		t.Fatal(err)
	}
	dmeta, err := dst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if dmeta.Install == nil {
		t.Fatal("clone dropped the [install] table, so the new instance cannot say where its binary came from")
	}
	want := filepath.Join(dst.BinDir(), "cline")
	if dmeta.Install.Binary != want {
		t.Errorf("Install.Binary = %q, want %q", dmeta.Install.Binary, want)
	}
	if dmeta.Install.Dir != dst.BinDir() {
		t.Errorf("Install.Dir = %q, want %q", dmeta.Install.Dir, dst.BinDir())
	}
	if sp := filepath.Join(dst.LogsDir(), "install-cline.sh"); dmeta.Install.ScriptPath != sp {
		t.Errorf("Install.ScriptPath = %q, want %q", dmeta.Install.ScriptPath, sp)
	} else if sum, err := fileSHA256(sp); err != nil {
		t.Errorf("the clone did not carry the audit copy of the installer: %v", err)
	} else if sum != dmeta.Install.SHA256 {
		t.Errorf("the clone's audit copy hashes to %s, but the record says %s", sum, dmeta.Install.SHA256)
	}
	if len(dmeta.Launch.Command) == 0 || dmeta.Launch.Command[0] != want {
		t.Errorf("the clone still execs %v, want %q", dmeta.Launch.Command, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the downloaded binary was not carried into the clone: %v", err)
	}
	// Provenance is data rather than a path, so it moves unchanged.
	for _, p := range []struct{ field, got, want string }{
		{"url", dmeta.Install.URL, meta.Install.URL},
		{"sha256", dmeta.Install.SHA256, meta.Install.SHA256},
		{"version", dmeta.Install.Version, meta.Install.Version},
	} {
		if p.got != p.want {
			t.Errorf("clone lost Install.%s: %q, want %q", p.field, p.got, p.want)
		}
	}
	smeta, err := src.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if smeta.Install.Binary != filepath.Join(src.BinDir(), "cline") {
		t.Errorf("cloning rewrote the source's record: %q", smeta.Install.Binary)
	}

	// The rewriting is what makes the clone stand on its own: doctor must believe
	// it without ever touching the source again.
	out.Reset()
	if err := app.CmdDoctor([]string{"dst"}); err != nil {
		t.Fatalf("the clone is not self-sufficient: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "install integrity") {
		t.Errorf("doctor distrusts the clone's install record:\n%s", out.String())
	}
}

// TestCloneOfAHomeInstalledBinaryWarns covers the one shape the rewriting cannot
// fix: an installer that honored $HOME put the binary under home/, and the
// default clone does not copy home/. Pretending would leave a launcher that
// cannot exec anything and no record of why.
func TestCloneOfAHomeInstalledBinaryWarns(t *testing.T) {
	app, _, errb := testApp(t)
	_, meta := installedFixture(t, app, "src", "home")
	name := filepath.Base(meta.Install.Binary)

	if err := app.CmdClone([]string{"src", "shallow", "--link=false"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb.String(), "--copy-data") {
		t.Errorf("the warning must name the flag that would have carried it:\n%s", errb.String())
	}
	shallow, err := instance.New(app.InstancesDir(), "shallow")
	if err != nil {
		t.Fatal(err)
	}
	smeta, err := shallow.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if smeta.Install == nil || smeta.Install.Binary != filepath.Join(shallow.HomeDir(), name) {
		t.Fatalf("the clone's record should describe its own tree, got %+v", smeta.Install)
	}
	if _, err := os.Stat(filepath.Join(shallow.HomeDir(), name)); err == nil {
		t.Error("a clone without --copy-data must not have copied home/")
	}
	// The clone's own record is honest about what it lacks, and doctor says so.
	if err := app.CmdDoctor([]string{"shallow"}); err == nil {
		t.Error("doctor should refuse a clone whose downloaded binary never arrived")
	}

	if err := app.CmdClone([]string{"src", "full", "--copy-data", "--yes", "--link=false"}); err != nil {
		t.Fatal(err)
	}
	full, err := instance.New(app.InstancesDir(), "full")
	if err != nil {
		t.Fatal(err)
	}
	fmeta, err := full.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(full.HomeDir(), name)
	if fmeta.Install == nil || fmeta.Install.Binary != want {
		t.Fatalf("--copy-data clone recorded %v, want %q", fmeta.Install, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("--copy-data did not carry the binary: %v", err)
	}
}

func TestRemoveNamesTheDownloadedCopy(t *testing.T) {
	app, out, errb := testApp(t)
	installedFixture(t, app, "d1", "bin")

	if err := app.CmdRemove([]string{"d1", "--yes"}); err != nil {
		t.Fatalf("rm: %v\n%s\n%s", err, out.String(), errb.String())
	}
	got := errb.String()
	if !strings.Contains(got, "downloaded") || !strings.Contains(got, "host binary") {
		t.Errorf("rm of a downloaded instance must say the copy itself goes and the host is untouched:\n%s", got)
	}
}
