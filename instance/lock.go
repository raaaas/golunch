package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockMode selects shared or exclusive.
type LockMode int

const (
	// Shared is taken for agent runs: several concurrent invocations against
	// one instance are legitimate, each gets its own session.
	Shared LockMode = iota
	// Exclusive is taken for admin operations (rm/clone/import) and for the
	// one run shape that genuinely conflicts: --continue without an explicit
	// session, where two runs would race to append to the same latest session.
	Exclusive
)

// Lock is an advisory flock on <instancesDir>/.<alias>.lock.
//
// warren implements the same idea with O_CREAT|O_EXCL and a PID written into
// the file, but never checks whether that PID is alive — so any crash or
// SIGKILL leaves a lock file that blocks every future operation until someone
// deletes it by hand. flock is released by the kernel when the descriptor
// closes, including on process death, which makes a stale lock impossible.
type Lock struct {
	f    *os.File
	path string
	mode LockMode
}

var ErrLocked = errors.New("instance is busy")

// instanceLockPath is the lockfile for alias i. Lockfiles live in the parent
// directory so that removing the instance tree also cannot strand a lock, and
// so List() skips them via the leading dot.
func (i *Instance) globalLockPath() string {
	return filepath.Join(filepath.Dir(i.Root), ".golunch.lock")
}

func Acquire(path string, mode LockMode) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	sys := syscall.LOCK_NB
	if mode == Shared {
		sys |= syscall.LOCK_SH
	} else {
		sys |= syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), sys); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: another golunch operation holds %s", ErrLocked, filepath.Base(path))
		}
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	// Record the holder purely for diagnostics; nothing reads it to decide
	// whether the lock is live, which is what makes this crash-safe.
	_ = f.Truncate(0)
	_, _ = f.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	return &Lock{f: f, path: path, mode: mode}, nil
}

func (i *Instance) Acquire(mode LockMode) (*Lock, error) {
	return Acquire(i.LockPath(), mode)
}

// AcquireGlobal serializes operations that swap an instance directory into
// place, where two concurrent runs would race on the same rename target.
func AcquireGlobal(instancesDir string) (*Lock, error) {
	if err := os.MkdirAll(instancesDir, 0o755); err != nil {
		return nil, err
	}
	return Acquire(filepath.Join(instancesDir, ".golunch.lock"), Exclusive)
}

func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}

// Held reports whether the lock is currently contended, for `golunch doctor`.
func Held(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}
