package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raaaas/golunch/agent"
)

// seedFixture builds a stand-in host home with the layout measured on a real
// machine: config in XDG_CONFIG_HOME, cline state under ~/.cline, credentials
// sitting one directory away from the MCP settings that must be copyable.
func seedFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	files := map[string]string{
		".config/kilo/kilo.jsonc": `{
  "mcp": {
    "codebase-memory-mcp": {"type": "local", "command": ["` + home + `/.local/bin/codebase-memory-mcp"]},
    "ghidra-mcp": {"type": "local", "command": ["uv", "run", "bridge-mcp-ghidra", "--transport", "stdio"]}
  },
  "provider": {"openai": {"options": {"apiKey": "sk-hostsecret", "baseURL": "https://api.openai.com/v1"}}}
}`,
		".config/kilo/package.json":                    `{"name": "kilo-config", "dependencies": {}}`,
		".config/kilo/agents/reviewer.md":              "you review code",
		".config/kilo/node_modules/left-pad/index.js":  "module.exports = 1",
		".local/share/kilo/auth.json":                  `{"access_token": "nope"}`,
		".cline/data/settings/cline_mcp_settings.json": `{"mcpServers": {}}`,
		".cline/data/settings/global-settings.json":    `{"mode": "act"}`,
		".cline/data/settings/providers.json":          `{"apiKey": "sk-should-never-be-copied"}`,
		".cline/skills/pdf/SKILL.md":                   "read pdfs",
	}
	for rel, body := range files {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(home, ".config", "kilo", "node_modules", ".bin", "left-pad")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../left-pad/index.js", link); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestSeedCopiesMcpAndSkillsNotCredentials(t *testing.T) {
	app, out, errb := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")
	inst, _, err := app.Open("k1")
	if err != nil {
		t.Fatal(err)
	}

	if err := app.CmdSeed([]string{"k1", "mcp", "skills", "--host-home", host}); err != nil {
		t.Fatalf("seed: %v\n%s\n%s", err, out.String(), errb.String())
	}

	want := filepath.Join(inst.ConfigDir(), "kilo", "kilo.jsonc")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("seeded config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inst.ConfigDir(), "kilo", "agents", "reviewer.md")); err != nil {
		t.Errorf("skills tree not seeded: %v", err)
	}
	// node_modules is 58 MB on a real host and is code, not configuration.
	if _, err := os.Stat(filepath.Join(inst.ConfigDir(), "kilo", "node_modules")); err == nil {
		t.Error("deps group was copied without --with-deps")
	}
	// The login lives in the host data dir; no group names it, and the guard
	// would refuse it even if one did.
	if _, err := os.Stat(filepath.Join(inst.DataDir(), "kilo", "auth.json")); err == nil {
		t.Error("auth.json was copied into the instance")
	}

	// kilo.json does not exist in the fixture, only kilo.jsonc.
	if !strings.Contains(out.String(), "absent") {
		t.Errorf("expected a note about the missing sibling config, got %q", out.String())
	}
	// The absolute host path in the mcp block is the isolation hole seed must
	// name instead of silently shipping.
	if !strings.Contains(errb.String(), "references "+host) {
		t.Errorf("no warning about the absolute host path: %q", errb.String())
	}
	// And a copied apiKey must not land world-readable.
	st, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("seeded config mode = %v, want 0600 (it holds apiKey)", st.Mode().Perm())
	}
	if !strings.Contains(errb.String(), "0600") {
		t.Errorf("no warning about the credential key: %q", errb.String())
	}
}

func TestSeedKeepsExistingFilesUnlessForce(t *testing.T) {
	app, out, _ := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")
	inst, _, err := app.Open("k1")
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(inst.ConfigDir(), "kilo", "kilo.jsonc")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := app.CmdSeed([]string{"k1", "mcp", "--host-home", host}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "mine" {
		t.Errorf("seed overwrote an instance file without --force: %q", b)
	}
	if !strings.Contains(out.String(), "keep") {
		t.Errorf("expected a keep note, got %q", out.String())
	}

	if err := app.CmdSeed([]string{"k1", "mcp", "--host-home", host, "--force"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(dst)
	if !strings.Contains(string(b), "mcp") {
		t.Errorf("--force did not overwrite: %q", b)
	}
}

func TestSeedWithDepsPreservesSymlinks(t *testing.T) {
	app, _, _ := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")
	inst, _, err := app.Open("k1")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.CmdSeed([]string{"k1", "deps", "--host-home", host}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(inst.ConfigDir(), "kilo", "node_modules", ".bin", "left-pad")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink not seeded: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("a package symlink became a real file; node_modules would double in size")
	}
	if target, _ := os.Readlink(link); target != "../left-pad/index.js" {
		t.Errorf("symlink target = %q", target)
	}
	if _, err := os.Stat(filepath.Join(inst.ConfigDir(), "kilo", "node_modules", "left-pad", "index.js")); err != nil {
		t.Error("regular file in the deps tree was not copied")
	}
}

func TestSeedClineMcpLandsInBothLocations(t *testing.T) {
	app, _, _ := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "c1", "--agent", "cline", "--binary", "/bin/cat", "--link=false")
	inst, _, err := app.Open("c1")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.CmdSeed([]string{"c1", "--all", "--host-home", host}); err != nil {
		t.Fatal(err)
	}

	mcp := "cline_mcp_settings.json"
	homePath := filepath.Join(inst.HomeDir(), ".cline", "data", "settings", mcp)
	nativePath := filepath.Join(inst.DataDir(), "settings", mcp)
	for _, p := range []string{homePath, nativePath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("mcp settings missing at %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(inst.HomeDir(), ".cline", "skills", "pdf", "SKILL.md")); err != nil {
		t.Errorf("cline skills not seeded: %v", err)
	}
	for _, cred := range []string{"providers.json", "secrets.json", "globalState.json"} {
		for _, found := range findNamed(t, inst.Root, cred) {
			t.Errorf("credential file %s was seeded into the instance at %s", cred, found)
		}
	}
}

// findNamed walks a whole tree, because "did any credential reach the instance"
// has to be answered for every depth: filepath.Glob does not cross separators.
func findNamed(t *testing.T, root, name string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestSeedRefusesDeclaredSecrets pins the guard itself, with an entry that asks
// for a credential file on purpose. The shipped drivers never do, which is why
// the guard needs its own fixture rather than relying on real ones.
func TestSeedRefusesDeclaredSecrets(t *testing.T) {
	host := seedFixture(t)
	app, out, _ := testApp(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")
	inst, _, err := app.Open("k1")
	if err != nil {
		t.Fatal(err)
	}

	entry := agent.Entry{
		Name: "fake",
		Env: agent.EnvProfile{
			Secrets: func(p agent.Paths) []string {
				return []string{filepath.Join(p.Home, ".config", "kilo", "kilo.jsonc")}
			},
		},
		Seed: func(h agent.HostPaths) []agent.SeedItem {
			return []agent.SeedItem{
				{Group: "mcp", Src: filepath.Join(h.Config, "kilo", "kilo.jsonc"), Dst: "config/kilo/kilo.jsonc"},
				{Group: "mcp", Src: filepath.Join(h.Config, "kilo", "package.json"), Dst: "../../escape.json"},
			}
		},
	}
	plan, note := buildSeedPlan(entry, mustHost(t, host), inst, []string{"mcp"})
	if len(plan) != 0 {
		t.Errorf("planned %d item(s), want 0: %+v", len(plan), plan)
	}
	joined := strings.Join(note, "\n")
	if !strings.Contains(joined, "credential file") {
		t.Errorf("no credential refusal in %q", joined)
	}
	if !strings.Contains(joined, "outside the instance") {
		t.Errorf("no traversal refusal in %q; got %q", out.String(), joined)
	}
}

func TestSeedDryRunWritesNothing(t *testing.T) {
	app, out, _ := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")
	inst, _, err := app.Open("k1")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.CmdSeed([]string{"k1", "mcp", "skills", "--host-home", host, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(inst.ConfigDir(), "kilo", "kilo.jsonc")); err == nil {
		t.Error("--dry-run wrote the config file")
	}
	if !strings.Contains(out.String(), "would") {
		t.Errorf("expected a would-copy plan, got %q", out.String())
	}
}

func TestSeedNeedsAGroup(t *testing.T) {
	app, _, _ := testApp(t)
	host := seedFixture(t)
	newTestInstance(t, app, "k1", "--agent", "kilo", "--binary", "/bin/cat", "--link=false")
	err := app.CmdSeed([]string{"k1", "--host-home", host})
	if err == nil || !strings.Contains(err.Error(), "what to seed") {
		t.Errorf("want a usage error naming the groups, got %v", err)
	}
	if err := app.CmdSeed([]string{"nope", "mcp"}); err == nil {
		t.Error("seed of a missing instance succeeded")
	}
}

// TestHostPathsIgnoresNestedXDG is the nesting guard: with $HOME and
// $XDG_CONFIG_HOME pointing into an outer instance, "the host config" must stay
// the real home's, or seed would cheerfully copy an empty tree.
func TestHostPathsIgnoresNestedXDG(t *testing.T) {
	t.Setenv("HOME", "/home/tester/.warren/instances/outer/home")
	t.Setenv("WARREN_INSTANCE", "outer")
	t.Setenv("XDG_CONFIG_HOME", "/home/tester/.warren/instances/outer/home/.config")
	if !nested() {
		t.Fatal("expected this to look nested")
	}
	h, err := hostPathsUnder("/home/tester")
	if err != nil {
		t.Fatal(err)
	}
	if h.Config != "/home/tester/.config" {
		t.Errorf("Config = %q, want the real home's default", h.Config)
	}

	t.Setenv("WARREN_INSTANCE", "")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("XDG_CONFIG_HOME", "/home/tester/custom-config")
	h, err = hostPathsUnder("/home/tester")
	if err != nil {
		t.Fatal(err)
	}
	if h.Config != "/home/tester/custom-config" {
		t.Errorf("Config = %q, want the honored XDG value", h.Config)
	}

	// An XDG value outside the real home is someone else's tree.
	t.Setenv("XDG_CONFIG_HOME", "/srv/shared-config")
	h, _ = hostPathsUnder("/home/tester")
	if h.Config != "/home/tester/.config" {
		t.Errorf("Config = %q, want the fallback", h.Config)
	}
}

func mustHost(t *testing.T, home string) agent.HostPaths {
	t.Helper()
	h, err := hostPathsUnder(home)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
