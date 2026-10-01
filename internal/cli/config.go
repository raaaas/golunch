package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/raaaas/golunch/proxy"
)

func (a *App) CmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	if _, err := splitArgs(fs, args); err != nil {
		return err
	}
	a.printf("golunch %s\n", versionText)
	return nil
}

// CmdConfig shows the effective global configuration, or writes a starting one.
// The file is TOML and hand-edited, so what this prints is what the user has,
// with defaults resolved and nothing invented.
func (a *App) CmdConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	initFile := fs.Bool("init", false, "write a starting config.toml if none exists")
	showPath := fs.Bool("path", false, "print only the file path")
	if _, err := splitArgs(fs, args); err != nil {
		return err
	}

	path := filepath.Join(a.Cfg.Root, "config.toml")
	if *showPath {
		a.printf("%s\n", path)
		return nil
	}
	if *initFile {
		if _, err := os.Stat(path); err == nil {
			return usageErr("%s already exists; not overwriting it", path)
		}
		if err := os.MkdirAll(a.Cfg.Root, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(sampleConfig), 0o644); err != nil {
			return err
		}
		a.printf("wrote %s\n", path)
		return nil
	}

	a.printf("# %s\n", path)
	if _, err := os.Stat(path); err != nil {
		a.printf("# (no file; these are defaults. Create one with: golunch config --init)\n\n")
	}
	a.printf("[paths]\n")
	a.printf("instances_dir = %s\n", quote(a.InstancesDir()))
	a.printf("bin_dir       = %s\n\n", quote(a.BinDir()))
	a.printf("[defaults]\n")
	a.printf("shell           = %s\n", quote(firstNonEmpty(a.Cfg.Defaults.Shell, "/bin/bash")))
	a.printf("timeout_seconds = %d\n", a.Cfg.Defaults.Timeout)
	a.printf("proxy           = %s\n", quote(a.Cfg.Defaults.Proxy))
	if len(a.Cfg.Defaults.KeepVars) > 0 {
		a.printf("keep_vars       = %s\n", quoteList(a.Cfg.Defaults.KeepVars))
	}
	a.printf("\n[proxy]\n")
	a.printf("default = %s\n", quote(a.Cfg.Proxy.Default))
	if len(a.Cfg.Proxy.NoProxyExtra) > 0 {
		a.printf("no_proxy_extra = %s\n", quoteList(a.Cfg.Proxy.NoProxyExtra))
	}
	if len(a.Cfg.Proxy.Profiles) > 0 {
		a.printf("\n# named profiles: reference one with proxy = \"name\"\n")
		for _, name := range sortedProfiles(a.Cfg.Proxy.Profiles) {
			p := a.Cfg.Proxy.Profiles[name]
			a.printf("[proxy.profiles.%s]\n", name)
			if p.HTTP != "" {
				a.printf("  http  = %s\n", quote(p.HTTP))
			}
			if p.HTTPS != "" {
				a.printf("  https = %s\n", quote(p.HTTPS))
			}
			if p.SOCKS != "" {
				a.printf("  socks = %s\n", quote(p.SOCKS))
			}
			if len(p.NoProxy) > 0 {
				a.printf("  no_proxy = %s\n", quoteList(p.NoProxy))
			}
		}
	}
	if a.Cfg.NestingWarning != "" {
		a.printf("\n# WARNING: %s\n", a.Cfg.NestingWarning)
	}
	return nil
}

func sortedProfiles(m map[string]proxy.Profile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func quote(s string) string { return fmt.Sprintf("%q", s) }

func quoteList(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = quote(s)
	}
	return "[" + strings.Join(out, ", ") + "]"
}

// sampleConfig is what `--init` writes: commented, minimal, and honest about
// what the proxy layer does and does not do.
const sampleConfig = `# golunch global configuration.
#
# Everything here is a default. An instance's metadata.toml overrides it, and a
# command-line flag overrides that.

[paths]
# instances_dir = "` + "/home/you/.golunch/instances" + `"
# bin_dir       = "` + "/home/you/.local/bin" + `"

[defaults]
shell = "/bin/bash"
# timeout_seconds = 0                # 0 means no timeout
# proxy = "corp"                     # a profile name below, or a bare URL

[proxy]
# default = "none"                   # "none" force-unsets every *_PROXY var

# Named groups you can point an instance at. A URL without a scheme is a
# profile reference, so keep profile names free of "://".
# [proxy.profiles.corp]
# http  = "http://127.0.0.1:7890"
# socks = "socks5://127.0.0.1:7891"
# no_proxy = [".corp.example"]

[ui]
color = true
max_ring_events = 200
`
