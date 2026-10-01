// Package osutil holds the two environment lookups that both the public runner
// and the CLI need, and that neither should reimplement.
package osutil

import (
	"os"
	"strings"
)

// Cwd returns the working directory, or "/" if it cannot be determined. A
// process that cannot read its own cwd is broken in ways this fallback will not
// hide, but it must not crash a run.
func Cwd() string {
	if d, err := os.Getwd(); err == nil {
		return d
	}
	return "/"
}

// Exists reports whether path is a regular file.
func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// FirstLine collapses a value that may hold paragraphs into one display line,
// truncated, so a tool result cannot push a status line off the terminal.
func FirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
