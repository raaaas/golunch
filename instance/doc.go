// Package instance is what an instance actually is: a directory plus a set of
// environment variables.
//
// BuildEnv gives a child a private HOME, every XDG base directory, TMPDIR, and a
// bin/ that comes first on PATH. Nothing is namespaced and nothing runs in the
// background — the isolation is real because the agent is told, correctly, where
// its files live, which is also why a new instance starts logged out.
//
// RenderLauncher emits the bash script that makes an alias an ordinary command.
// It renders from the same BuildEnv that a headless run uses, and launcher_test
// asserts that in both directions: `work` and `golunch run work --` cannot drift
// apart.
//
// Locking is advisory flock on <root>/.<alias>.lock. Agent runs take it shared,
// admin operations exclusive, and a shared-filesystem peer is reported as busy
// rather than corrupting the instance. A crash or SIGKILL cannot leave a lock
// that blocks every future operation.
package instance
