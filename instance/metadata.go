package instance

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/raaaas/golunch/atomicfile"

	"github.com/BurntSushi/toml"
)

// Metadata is the per-instance record at <root>/metadata.toml. It is
// hand-editable by design — the proxy override in particular is meant to be
// tweaked with an editor — so every field is a plain scalar or table, and
// writing goes through an atomic rename to survive a crash mid-save.
type Metadata struct {
	Instance InstanceInfo `toml:"instance"`
	Launch   LaunchInfo   `toml:"launch"`
	Proxy    ProxyInfo    `toml:"proxy"`
	Paths    PathsInfo    `toml:"paths"`
	Install  *InstallInfo `toml:"install,omitempty"`
	LastRun  *RunRecord   `toml:"lastrun,omitempty"`
}

type InstanceInfo struct {
	Alias string `toml:"alias"`
	// Agent names the driver in the registry ("kilo", "cline"). Empty means
	// this instance is a dumb wrapper around an arbitrary binary.
	Agent string `toml:"agent,omitempty"`
	// Version is captured at creation; refresh with `golunch ls -l`.
	Version string `toml:"version,omitempty"`
	// Shell is exported as $SHELL so a `golunch shell` inside the instance
	// still reports something sensible.
	Shell     string    `toml:"shell"`
	CreatedAt time.Time `toml:"created_at"`
	UpdatedAt time.Time `toml:"updated_at"`
}

type LaunchInfo struct {
	// Command is the host argv to exec inside the sandbox. For an agent
	// instance this is the absolute path of the located binary.
	Command []string `toml:"command"`
	// Binary is the instance-local bin name that the launcher prefers if it
	// exists, letting an instance override the host program.
	Binary string `toml:"binary,omitempty"`
	// NativeFlags records whether the agent's own isolation flags were passed
	// on the headless path (cline --config/--data-dir).
	NativeFlags bool `toml:"native_flags,omitempty"`
	// DriverBinary overrides the agent executable for the headless run.
	// Set by tests to /bin/cat against a fixture; never by a user.
	DriverBinary string `toml:"driver_binary_override,omitempty"`
	// KeepVars names extra host variables to copy into this instance, on top
	// of whatever its driver already requires. A corporate CA bundle or a
	// machine-scoped credential path belongs here.
	KeepVars []string `toml:"keep_vars,omitempty"`
}

type ProxyInfo struct {
	// Spec is a profile name, a URL, or "none" to force-unset. Empty means
	// inherit the global default or the host environment.
	Spec string `toml:"spec,omitempty"`
	// NoProxy adds exceptions on top of whatever wins the precedence contest.
	NoProxy []string `toml:"no_proxy,omitempty"`
}

type PathsInfo struct {
	Root     string `toml:"root"`
	Bin      string `toml:"bin"`
	Launcher string `toml:"launcher"`
	// LinkPath is the ~/.local/bin entry exposing the alias as a command.
	LinkPath string `toml:"link_path,omitempty"`
}

// InstallInfo records that this instance's agent binary was downloaded rather
// than pointed at the host's, and exactly what the installer did, so `golunch
// doctor` can prove the copy is intact and `golunch clone` can reprefix the
// paths into the new tree. Like the other tables it is scalars only, because
// metadata.toml is hand-editable: a user correcting a stale Version or pointing
// ScriptPath at a saved audit copy must be able to do it in an editor.
//
// It is a pointer, so an instance that wraps a host binary (the common case)
// simply has no [install] table rather than an empty one.
type InstallInfo struct {
	// Agent names the registry entry the install was for.
	Agent string `toml:"agent,omitempty"`
	// URL is the installer that was fetched, kept so doctor can re-hash it and
	// a human can see where this binary came from.
	URL string `toml:"url,omitempty"`
	// SHA256 pins the fetched script, or records the hash it actually had when
	// no pin was supplied. A changed script is drift, and doctor says so.
	SHA256 string `toml:"sha256,omitempty"`
	// ScriptPath is the audit copy retained under the instance, not the binary.
	ScriptPath string `toml:"script_path,omitempty"`
	// Version is the agent version the installer reported.
	Version string `toml:"version,omitempty"`
	// Dir is the directory inside the instance the binary was installed to.
	Dir string `toml:"dir,omitempty"`
	// Binary is the located executable, an absolute path under the root so
	// doctor can check it still exists and clone can reprefix it.
	Binary string `toml:"binary,omitempty"`
	// FetchedAt stamps when the download ran.
	FetchedAt time.Time `toml:"fetched_at,omitempty"`
}

type RunRecord struct {
	StartedAt time.Time `toml:"started_at"`
	ExitCode  int       `toml:"exit_code"`
	SessionID string    `toml:"session_id,omitempty"`
	// ProxySource explains which precedence layer supplied the effective
	// proxy, so `golunch ls -l` can answer "why isn't this going through my
	// proxy" without rerunning the launch.
	ProxySource string `toml:"proxy_source,omitempty"`
	Log         string `toml:"log,omitempty"`
	Model       string `toml:"model,omitempty"`
}

func (i *Instance) MetadataPath() string { return filepath.Join(i.Root, "metadata.toml") }

func Load(path string) (Metadata, error) {
	var m Metadata
	data, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("read metadata %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse metadata %s: %w", path, err)
	}
	if m.Instance.Alias == "" {
		return m, fmt.Errorf("metadata %s is missing [instance] alias", path)
	}
	return m, nil
}

func (m *Metadata) Save(path string) error {
	var out bytes.Buffer
	enc := toml.NewEncoder(&out)
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	return atomicfile.Write(path, out.Bytes(), 0o644)
}

// Touch stamps UpdatedAt, and CreatedAt on the first save, so callers cannot
// forget one of the two.
func (m *Metadata) Touch() *Metadata {
	now := time.Now().UTC().Truncate(time.Second)
	if m.Instance.CreatedAt.IsZero() {
		m.Instance.CreatedAt = now
	}
	m.Instance.UpdatedAt = now
	return m
}

func (i *Instance) LoadMetadata() (Metadata, error) { return Load(i.MetadataPath()) }

func (i *Instance) SaveMetadata(m *Metadata) error { return m.Save(i.MetadataPath()) }
