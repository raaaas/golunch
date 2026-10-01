package instance

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusiveBlocksSecondAcquire(t *testing.T) {
	dir := t.TempDir()
	inst, _ := New(dir, "lock1")
	inst.Create()

	a, err := inst.Acquire(Exclusive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Acquire(Exclusive); err == nil {
		t.Error("second exclusive acquire succeeded while first held")
	} else if !os.IsNotExist(err) && err.Error() == "" {
		t.Errorf("unexpected error %v", err)
	}
	if _, err := inst.Acquire(Shared); err == nil {
		t.Error("shared acquire succeeded while exclusive held")
	}
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	if b, err := inst.Acquire(Exclusive); err != nil {
		t.Errorf("acquire after release failed: %v", err)
	} else {
		b.Release()
	}
}

// The whole reason for using flock instead of warren's create_new lockfile:
// a holder that is SIGKILLed must not leave the instance permanently locked.
func TestLockReleasedWhenHolderKilled(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".killed.lock")

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperHoldLock", "--", lockPath)
	cmd.Env = append(os.Environ(), "GOLUNCH_TEST_HELPER=1")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if Held(lockPath) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !Held(lockPath) {
		cmd.Process.Kill()
		t.Fatal("helper never acquired the lock")
	}

	// Kill -9: no cleanup code runs, only the kernel closing descriptors.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	// Give the kernel a moment to reap.
	for i := 0; i < 100; i++ {
		if !Held(lockPath) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("lock survived SIGKILL of its holder — stale lock bug reproduced")
}

func TestHelperHoldLock(t *testing.T) {
	if os.Getenv("GOLUNCH_TEST_HELPER") != "1" {
		t.Skip("not a real test run")
	}
	args := os.Args
	path := args[len(args)-1]
	l, err := Acquire(path, Exclusive)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	// Signal readiness, then block until killed.
	time.Sleep(300 * time.Millisecond)
	select {}
}

func TestSharedAllowsConcurrency(t *testing.T) {
	dir := t.TempDir()
	inst, _ := New(dir, "conc")
	inst.Create()

	a, err := inst.Acquire(Shared)
	if err != nil {
		t.Fatal(err)
	}
	b, err := inst.Acquire(Shared)
	if err != nil {
		t.Fatalf("two shared locks should coexist: %v", err)
	}
	a.Release()
	b.Release()
}

func TestGlobalLock(t *testing.T) {
	dir := t.TempDir()
	a, err := AcquireGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireGlobal(dir); err == nil {
		t.Error("global lock is not exclusive")
	}
	a.Release()
}
