package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/raaaas/golunch/atomicfile"
	"github.com/raaaas/golunch/proxy"

	"github.com/BurntSushi/toml"
)

// Config is golunch's global configuration, read from <dataRoot>/config.toml.
type Config struct {
	Paths    Paths    `toml:"paths"`
	Defaults Defaults `toml:"defaults"`
	Proxy    Proxy    `toml:"proxy"`
	UI       UI       `toml:"ui"`

	// Root is the resolved golunch data directory. Not part of the file.
	Root string `toml:"-"`
	// NestingWarning is set when golunch appears to be running inside another
	// isolation tool's instance (e.g. warren). Surfaced by `golunch doctor`.
	NestingWarning string `toml:"-"`
}

type Paths struct {
	InstancesDir string `toml:"instances_dir"`
	BinDir       string `toml:"bin_dir"`
}

type Defaults struct {
	Shell   string `toml:"shell"`
	Timeout int    `toml:"timeout_seconds"`
	// Proxy names a profile in Proxy.Profiles, or is a bare proxy URL.
	Proxy string `toml:"proxy"`
	// KeepVars are host variables copied into every instance regardless of
	// driver: a site-wide CA bundle, a credential helper path.
	KeepVars []string `toml:"keep_vars"`
}

type Proxy struct {
	// Default is a profile name or a bare URL applied to every instance
	// unless the instance or the run flag overrides it.
	Default string `toml:"default"`
	// NoProxyExtra is merged into NO_PROXY for every instance, on top of the
	// always-present loopback floor.
	NoProxyExtra []string                 `toml:"no_proxy_extra"`
	Profiles     map[string]proxy.Profile `toml:"profiles"`
}

type UI struct {
	Color bool `toml:"color"`
	// MaxRingEvents bounds how many normalized events a task keeps in memory.
	// Full NDJSON always goes to the instance log; this only limits RAM.
	MaxRingEvents int `toml:"max_ring_events"`
}

func defaults(root string) Config {
	return Config{
		Paths: Paths{
			InstancesDir: filepath.Join(root, "instances"),
			BinDir:       filepath.Join(RealHome(), ".local", "bin"),
		},
		Defaults: Defaults{Shell: "/bin/bash", Timeout: 0, Proxy: ""},
		Proxy:    Proxy{Profiles: map[string]proxy.Profile{}},
		UI:       UI{Color: true, MaxRingEvents: 200},
		Root:     root,
	}
}

// RealHome returns the home directory from the passwd database, ignoring
// $HOME. golunch is designed to run inside an environment where $HOME has
// already been redirected into an isolated instance (warren does exactly
// this); using $HOME would bury golunch's own data inside a throwaway
// instance that gets deleted with it.
func RealHome() string {
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/"
}

// DataRoot resolves the golunch data directory, reporting whether the current
// environment looks nested. GOLUNCH_ROOT overrides the location, which serves
// two purposes: a machine where ~/.golunch is the wrong place, and tests that
// need a data root they can delete.
func DataRoot() (string, string) {
	root := filepath.Join(RealHome(), ".golunch")
	if override := os.Getenv("GOLUNCH_ROOT"); override != "" {
		if !filepath.IsAbs(override) {
			if abs, err := filepath.Abs(override); err == nil {
				override = abs
			}
		}
		root = override
	}
	warning := ""
	if envHome := os.Getenv("HOME"); envHome != "" && envHome != RealHome() {
		parent := "an isolated instance"
		if alias := os.Getenv("WARREN_INSTANCE"); alias != "" {
			parent = fmt.Sprintf("warren instance %q", alias)
		}
		warning = fmt.Sprintf(
			"$HOME points to %q (inside %s) which differs from the real home %q. "+
				"Using %s for instance storage; instances created here will not "+
				"disappear with the outer instance.",
			envHome, parent, RealHome(), root)
	}
	return root, warning
}

func path() string {
	root, _ := DataRoot()
	return filepath.Join(root, "config.toml")
}

// Load reads the config, falling back to defaults for a missing file. An
// existing but unparsable file is an error rather than a silent reset.
func Load() (Config, error) {
	root, warning := DataRoot()
	cfg := defaults(root)
	cfg.NestingWarning = warning

	data, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path(), err)
	}
	if cfg.Root == "" {
		cfg.Root = root
	}
	if cfg.UI.MaxRingEvents == 0 {
		cfg.UI.MaxRingEvents = 200
	}
	if cfg.Proxy.Profiles == nil {
		cfg.Proxy.Profiles = map[string]proxy.Profile{}
	}
	return cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(c.Root, 0o755); err != nil {
		return err
	}
	buf, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return atomicfile.Write(path(), buf, 0o644)
}
