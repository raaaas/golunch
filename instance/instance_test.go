package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAlias(t *testing.T) {
	good := []string{"kilo", "kilo-work", "gh2", "a", "work-2-personal", "cline2"}
	bad := []string{
		"", "-", "a-", "-a", "Kilo", "kilo_work", "kilo--work", "kilo.work",
		"../escape", "a/b", "./x", "..", "kilo work", "kilo;rm -rf /",
		strings.Repeat("a", 65),
	}
	for _, a := range good {
		if err := ValidateAlias(a); err != nil {
			t.Errorf("ValidateAlias(%q) = %v, want nil", a, err)
		}
	}
	for _, a := range bad {
		if err := ValidateAlias(a); err == nil {
			t.Errorf("ValidateAlias(%q) = nil, want error", a)
		}
	}
}

func TestCreateGoldenTree(t *testing.T) {
	dir := t.TempDir()
	inst, err := New(dir, "kilo-work")
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Create(); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"bin", "cache", "config", "data", "home", "logs", "runtime", "state", "tmp",
	}
	ents, err := os.ReadDir(inst.Root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ents {
		if !e.IsDir() {
			t.Fatalf("%s: expected a directory, got a file", e.Name())
		}
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("layout = %v, want %v", got, want)
	}
	// Nothing may escape the instances directory.
	rel, err := filepath.Rel(dir, inst.Root)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("instance root %q escapes %q", inst.Root, dir)
	}
}

func TestNewRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, a := range []string{"../outside", "a/b", ".."} {
		if _, err := New(dir, a); err == nil {
			t.Errorf("New(%q) succeeded, want rejection", a)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside")); err == nil {
		t.Fatal("a traversal alias created a directory outside instancesDir")
	}
}

func TestEnsureStorageRecreatesRemovedDirs(t *testing.T) {
	dir := t.TempDir()
	inst, _ := New(dir, "a1")
	if err := inst.Create(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(inst.TmpDir())
	os.RemoveAll(inst.RuntimeDir())
	inst.EnsureStorage()
	for _, d := range []string{inst.TmpDir(), inst.RuntimeDir()} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s not recreated: %v", d, err)
		}
	}
}

func TestListSkipsLocksAndJunk(t *testing.T) {
	dir := t.TempDir()
	for _, a := range []string{"one", "two"} {
		i, _ := New(dir, a)
		if err := i.Create(); err != nil {
			t.Fatal(err)
		}
	}
	// Lockfiles live beside instances and start with a dot.
	os.WriteFile(filepath.Join(dir, ".one.lock"), []byte("1\n"), 0o644)
	// A stray non-conforming directory must be ignored, not crash the listing.
	os.MkdirAll(filepath.Join(dir, "NotAnAlias"), 0o755)

	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "one,two" {
		t.Errorf("List = %v, want [one two]", got)
	}
}

func TestDiskUsageIgnoresSymlinks(t *testing.T) {
	dir := t.TempDir()
	inst, _ := New(dir, "u1")
	inst.Create()
	os.WriteFile(filepath.Join(inst.DataDir(), "real"), make([]byte, 1000), 0o644)
	// A link to a large host tree must not be counted (or followed).
	os.Symlink(filepath.Join(inst.DataDir(), "real"), filepath.Join(inst.HomeDir(), "link"))

	byDir, total, err := inst.DiskUsage()
	if err != nil {
		t.Fatal(err)
	}
	if total != 1000 {
		t.Errorf("total = %d, want 1000 (symlink target counted once at most)", total)
	}
	if byDir["data"] != 1000 {
		t.Errorf("data = %d, want 1000", byDir["data"])
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inst, _ := New(dir, "meta1")
	inst.Create()

	m := &Metadata{}
	m.Instance = InstanceInfo{Alias: "meta1", Agent: "kilo", Shell: "/bin/zsh", Version: "7.7.9"}
	m.Launch = LaunchInfo{Command: []string{"/home/tester/.kilo/bin/kilo"}, Binary: "kilo"}
	m.Proxy = ProxyInfo{Spec: "http://127.0.0.1:7890", NoProxy: []string{".corp"}}
	m.Paths = PathsInfo{Root: inst.Root, Bin: inst.BinDir(), Launcher: filepath.Join(inst.Root, "launcher")}
	m.Touch()
	if err := inst.SaveMetadata(m); err != nil {
		t.Fatal(err)
	}

	got, err := inst.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.Instance.Agent != "kilo" || got.Proxy.Spec != "http://127.0.0.1:7890" {
		t.Errorf("round trip lost data: %+v", got)
	}
	if len(got.Launch.Command) != 1 || got.Launch.Command[0] != "/home/tester/.kilo/bin/kilo" {
		t.Errorf("launch command = %v", got.Launch.Command)
	}
	if got.Instance.CreatedAt.IsZero() {
		t.Error("created_at should be set by Touch")
	}

	// The saved file must be human-editable TOML with the expected sections.
	raw, _ := os.ReadFile(inst.MetadataPath())
	for _, section := range []string{"[instance]", "[launch]", "[proxy]", "[paths]"} {
		if !strings.Contains(string(raw), section) {
			t.Errorf("metadata.toml missing %s:\n%s", section, raw)
		}
	}
	// No temp files left behind by the atomic write.
	ents, _ := os.ReadDir(inst.Root)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("atomic write left %s behind", e.Name())
		}
	}
}

func TestMetadataRejectsUnparsable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "metadata.toml")
	os.WriteFile(p, []byte("[instance\n  broken ="), 0o644)
	if _, err := Load(p); err == nil {
		t.Error("expected parse error, got nil")
	}
	os.WriteFile(p, []byte("[instance]\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Error("expected error for metadata without alias")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":               "''",
		"/usr/bin":       "/usr/bin",
		"a:b/c-d.e,f":    "a:b/c-d.e,f",
		"with space":     "'with space'",
		"semi;colon":     "'semi;colon'",
		"it's":           `'it'\''s'`,
		"$(rm -rf)":      "'$(rm -rf)'",
		"new\nline":      "'new\nline'",
		`back\slash`:     `'back\slash'`,
		"127.0.0.1:7890": "127.0.0.1:7890",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
