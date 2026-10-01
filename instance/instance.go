package instance

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// storageDirs are created eagerly for every instance. Isolation is achieved
// entirely by pointing HOME/XDG_* at these, so each one must exist before an
// agent is exec'd — many agents do not mkdir -p their own state dirs.
var storageDirs = []string{
	"bin", "home", "config", "cache", "data", "state", "runtime", "tmp", "logs",
}

var aliasRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]*[a-z0-9])?$`)

// ErrInvalidAlias covers every rejection reason. The pattern is deliberately
// the same as warren's: an alias becomes a directory name, a ~/.local/bin
// entry and a shell command, so nothing path-shaped or shell-shaped is legal.
var ErrInvalidAlias = errors.New("invalid alias")

func ValidateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("%w: alias must not be empty", ErrInvalidAlias)
	}
	if len(alias) > 64 {
		return fmt.Errorf("%w: %q is %d characters, maximum is 64", ErrInvalidAlias, alias, len(alias))
	}
	if !aliasRe.MatchString(alias) {
		return fmt.Errorf("%w: %q must be lowercase letters, digits and single hyphens, starting and ending with a letter or digit", ErrInvalidAlias, alias)
	}
	if strings.Contains(alias, "--") {
		return fmt.Errorf("%w: %q must not contain consecutive hyphens", ErrInvalidAlias, alias)
	}
	// Defense in depth: the regex already excludes separators and dots, but
	// the alias is joined into paths in several places and a future loosening
	// here would become a traversal bug.
	if alias == "." || alias == ".." || filepath.Base(alias) != alias {
		return fmt.Errorf("%w: %q is not a safe directory name", ErrInvalidAlias, alias)
	}
	return nil
}

// Instance is one isolated world: a directory tree plus the metadata that
// describes how to launch into it.
type Instance struct {
	Alias string
	Root  string
}

func New(instancesDir, alias string) (*Instance, error) {
	if err := ValidateAlias(alias); err != nil {
		return nil, err
	}
	return &Instance{Alias: alias, Root: filepath.Join(instancesDir, alias)}, nil
}

func (i *Instance) Dir(sub string) string { return filepath.Join(i.Root, sub) }

func (i *Instance) BinDir() string     { return i.Dir("bin") }
func (i *Instance) HomeDir() string    { return i.Dir("home") }
func (i *Instance) ConfigDir() string  { return i.Dir("config") }
func (i *Instance) CacheDir() string   { return i.Dir("cache") }
func (i *Instance) DataDir() string    { return i.Dir("data") }
func (i *Instance) StateDir() string   { return i.Dir("state") }
func (i *Instance) RuntimeDir() string { return i.Dir("runtime") }
func (i *Instance) TmpDir() string     { return i.Dir("tmp") }
func (i *Instance) LogsDir() string    { return i.Dir("logs") }
func (i *Instance) LockPath() string   { return filepath.Join(filepath.Dir(i.Root), "."+i.Alias+".lock") }

func (i *Instance) Exists() bool {
	st, err := os.Stat(i.Root)
	return err == nil && st.IsDir()
}

func (i *Instance) Create() error {
	for _, d := range storageDirs {
		if err := os.MkdirAll(i.Dir(d), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", i.Dir(d), err)
		}
	}
	return nil
}

// EnsureStorage recreates the storage dirs. Run before every exec so an
// instance whose agent deleted its own tmp or runtime dir still launches.
func (i *Instance) EnsureStorage() {
	for _, d := range storageDirs {
		_ = os.MkdirAll(i.Dir(d), 0o755)
	}
}

func (i *Instance) Destroy() error {
	if err := os.RemoveAll(i.Root); err != nil {
		return fmt.Errorf("remove %s: %w", i.Root, err)
	}
	_ = os.Remove(i.LockPath())
	return nil
}

// DiskUsage walks the instance tree. Callers should make this opt-in: an
// instance holding a 1.2 GB agent database (kilo.db on this machine) makes a
// full du far too slow to run on every `ls`.
func (i *Instance) DiskUsage() (map[string]uint64, uint64, error) {
	out := make(map[string]uint64, len(storageDirs))
	var total uint64
	for _, d := range storageDirs {
		size, err := dirSize(i.Dir(d))
		if err != nil {
			return nil, 0, err
		}
		out[d] = size
		total += size
	}
	return out, total, nil
}

func dirSize(path string) (uint64, error) {
	var size uint64
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// A dangling symlink or a file an agent removed mid-walk is not
			// worth failing a size report over.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Do not follow symlinks into the host filesystem: an instance's home
		// can legitimately contain links pointing at large shared trees.
		info, err := d.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		size += uint64(info.Size())
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	return size, err
}

// List returns the aliases present under instancesDir.
func List(instancesDir string) ([]string, error) {
	ents, err := os.ReadDir(instancesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if ValidateAlias(e.Name()) != nil {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

func FormatBytes(b uint64) string {
	const kb, mb, gb = uint64(1024), uint64(1024 * 1024), uint64(1024 * 1024 * 1024)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%d KB", b/kb)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
