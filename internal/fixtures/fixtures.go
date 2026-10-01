// Package fixtures locates the recorded agent transcripts that the offline test
// suite replays. It exists so no test package hardcodes how deep it sits in the
// tree: the lookup walks up from the working directory until it finds a
// testdata/fixtures, which keeps the fixtures in one place at the repo root and
// survives a package being moved.
package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
)

// Path returns the absolute path to name under the repo's testdata/fixtures.
func Path(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	start := dir
	for {
		p := filepath.Join(dir, "testdata", "fixtures", name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("fixture %q not found under any testdata/fixtures above %s", name, start)
}
