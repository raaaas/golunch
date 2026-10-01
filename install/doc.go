// Package install runs an agent vendor's own installer script as a child of a
// golunch instance, so the binary it produces belongs to that instance alone.
//
// golunch otherwise only wraps a binary that already exists on the host, and
// whatever the user installs afterwards lands in the shared ~/.local/bin at one
// version for every instance. A vendor installer run under the instance's
// environment — the HOME, XDG_* and TMPDIR redirection and the prepended bin/
// that instance.BuildEnv already assembles (instance/env.go) — is simply a child
// process told the truth about where its files live. A $HOME-relative script
// therefore writes into the instance tree, and `golunch rm` deletes the download
// with the tree.
//
// This package is mechanism. The user interface is `golunch install` in package
// internal/cli, which owns the flags, the consent prompt and the metadata,
// because what happens here is the execution of bytes fetched from the network
// and must never be reachable implicitly. The rules that follow from that:
//
//   - Nothing is piped into a shell. The script is fetched to a file, hashed,
//     and then the saved file is executed, so what a human confirmed or a
//     ScriptSHA256 pin approved is byte-for-byte what ran. Fetch is exported
//     separately precisely so the caller can hash, confirm and only then install.
//   - No sudo, no escalation, and the child is never detached. Every run goes
//     through execd, which puts the installer in its own process group and kills
//     the group on timeout or cancel, preserving the zero-daemon guarantee
//     described in execd/doc.go. An installer that insists on sudo, or on writing
//     to /usr/local/bin, fails loudly here rather than escaping onto the host.
//   - The Go-side download uses the instance's resolved proxy, the same decision
//     BuildEnv wrote into the environment for the script's own curl, and never
//     golunch's ambient proxy settings.
//   - Nothing is reported as installed unless the binary actually landed inside
//     the instance, and Verify still reports any host path the run created.
package install
