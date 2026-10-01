package atomicfile

import (
	"os"
	"path/filepath"
)

// Write creates the parent directory, writes to a sibling temp file and
// renames it into place. A crash mid-write then leaves the previous contents
// intact instead of a truncated config or metadata file.
//
// The temp file must be in the same directory as the destination: rename(2)
// across filesystems fails, and $TMPDIR is frequently a different mount.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
