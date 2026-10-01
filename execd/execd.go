package execd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/raaaas/golunch/agent"
)

// gracePeriod is how long the child gets to leave on hearing SIGTERM before
// the whole group is SIGKILLed, and simultaneously the bound os/exec puts on
// Wait after the pipes close.
const gracePeriod = 5 * time.Second

// Spec describes one child invocation.
type Spec struct {
	Dir     string
	Argv    []string
	Env     []string
	Stdin   io.Reader
	Timeout time.Duration
	// Decided lists the variable names golunch chose itself. --dry-run shows
	// those verbatim and collapses the rest into a count: a wall of inherited
	// desktop and conda variables buries the handful of lines that decide
	// isolation. Empty means no origin information, so everything is shown.
	Decided []string
	// OnEvent receives each normalized event as it is parsed. Returning false
	// asks for the run to stop, which lets a caller take only the first
	// sentence without draining the rest of a transcript.
	OnEvent func(agent.Event) bool
	// ParseLine is the driver's decoder; nil means raw mode (no parsing).
	ParseLine agent.ParseLine
	// RawLog, when set, receives every stdout line verbatim, parsed or not,
	// so a misbehaving agent can be inspected after the fact.
	RawLog string
	// StderrTail bounds how many bytes of stderr are retained for reporting.
	StderrTail int
	// InheritStdio attaches the child to this process's terminal, for
	// interactive passthrough where parsing output would be nonsense.
	InheritStdio bool
	// Stdout and Stderr override where an InheritStdio child writes. They exist
	// so a test can assert on passthrough output instead of having it land on
	// the real terminal; nil means this process's own streams.
	Stdout io.Writer
	Stderr io.Writer
}

// Result is the outcome of one child invocation.
type Result struct {
	ExitCode int
	Duration time.Duration
	Stats    agent.Stats
	Stderr   string
	TimedOut bool
	Killed   bool
	Err      error
	Events   int
	LogPath  string
}

func (r Result) ErrString() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return ""
}

// Start is what the child looked like; `golunch run --dry-run` prints this
// without invoking Run.
type Start struct {
	Argv []string
	Dir  string
	Env  []string
}

// Run starts the child, streams its output and waits for it.
//
// The child gets its own process group and is killed there rather than by
// pid: every CLI in the registry is a Node or Bun launcher that spawns
// workers, and killing only the leader orphans them — cline's session hub
// would survive holding the instance's runtime dir open, quietly breaking the
// zero-daemon promise this tool makes.
func Run(ctx context.Context, s Spec) Result {
	start := time.Now()
	res := Result{ExitCode: -1, LogPath: s.RawLog}

	if len(s.Argv) == 0 {
		res.Err = errors.New("execd: empty argv")
		return res
	}
	abs, err := exec.LookPath(s.Argv[0])
	if err != nil {
		res.Err = fmt.Errorf("execd: %w", err)
		return res
	}

	runCtx := ctx
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, abs, s.Argv[1:]...)
	cmd.Path = abs
	cmd.Env = s.Env
	if s.Dir != "" {
		cmd.Dir = s.Dir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	done := make(chan struct{})
	defer close(done)
	// CommandContext is required, not convenient: os/exec refuses to run a
	// command with a non-nil Cancel that was not created by CommandContext.
	// Overriding Cancel is how the signal reaches the whole group.
	cmd.Cancel = func() error {
		err := killGroup(cmd, syscall.SIGTERM)
		// os/exec's own escalation kills only the group leader, which would
		// orphan the agent's workers, so the group-wide SIGKILL is scheduled
		// here. done makes the timer a no-op once the child is reaped, since
		// a stale timer could otherwise signal a recycled pid.
		time.AfterFunc(gracePeriod, func() {
			select {
			case <-done:
			default:
				_ = killGroup(cmd, syscall.SIGKILL)
			}
		})
		return err
	}
	// If SIGTERM is ignored, WaitDelay keeps Wait bounded regardless.
	cmd.WaitDelay = gracePeriod

	stderr := newTail(s.StderrTail)

	var rawFile *os.File
	if s.RawLog != "" {
		_ = os.MkdirAll(filepath.Dir(s.RawLog), 0o755)
		if f, err := os.Create(s.RawLog); err == nil {
			rawFile = f
			defer f.Close()
		} else {
			res.Err = fmt.Errorf("open run log: %w", err)
			return res
		}
	}

	if s.InheritStdio {
		// Defaults to this process's own terminal: passthrough means the user
		// sees exactly what the agent printed.
		stdoutW := s.Stdout
		if stdoutW == nil {
			stdoutW = os.Stdout
		}
		stderrW := io.Writer(stderr)
		if s.Stderr != nil {
			stderrW = io.MultiWriter(s.Stderr, stderr)
		}
		cmd.Stdin = s.Stdin
		cmd.Stdout = stdoutW
		// Also teed into the tail buffer, so an interactive failure is still
		// quotable afterwards.
		cmd.Stderr = stderrW
		if err := cmd.Start(); err != nil {
			res.Err = err
			res.Stderr = stderr.String()
			return res
		}
		res.ExitCode = wait(cmd, &res)
		res.Stderr = stderr.String()
		res.Duration = time.Since(start)
		// The deadline has to be mapped here too, as it is on the streaming
		// path below: this branch returned first, so an interactive run that
		// hit its timeout reported only Killed and exit 143 and could never
		// give `golunch shell -c` or a passthrough the 124 a caller is told a
		// timeout costs.
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			res.TimedOut = true
		}
		return res
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		res.Err = err
		return res
	}
	cmd.Stderr = stderr
	// A headless agent must never block on a human: /dev/null turns "needs
	// input" into a fast visible failure instead of a hang until timeout.
	if s.Stdin == nil {
		devnull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
		if err != nil {
			res.Err = fmt.Errorf("open %s: %w", os.DevNull, err)
			return res
		}
		defer devnull.Close()
		cmd.Stdin = devnull
	} else {
		cmd.Stdin = s.Stdin
	}

	if err := cmd.Start(); err != nil {
		res.Err = err
		res.Stderr = stderr.String()
		return res
	}

	events := make(chan agent.Event, 64)
	var (
		scanStats agent.Stats
		wg        sync.WaitGroup
		stopOnce  sync.Once
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(events)
		br := bufio.NewReaderSize(stdout, 1<<20)
		if s.ParseLine == nil {
			// Raw mode: the bytes are the product, no normalization.
			sc := bufio.NewScanner(br)
			sc.Buffer(make([]byte, 0, 64<<10), agent.MaxLineBytes)
			for sc.Scan() {
				line := sc.Bytes()
				if rawFile != nil {
					rawFile.Write(append(append([]byte{}, line...), '\n'))
				}
			}
			scanStats = agent.Stats{Lines: -1}
			return
		}
		st, _ := agent.Scan(br, func(line []byte) ([]agent.Event, bool) {
			if rawFile != nil {
				rawFile.Write(append(append([]byte{}, line...), '\n'))
			}
			return s.ParseLine(line)
		}, events)
		scanStats = st
	}()

	stopped := false
	for e := range events {
		res.Events++
		if s.OnEvent != nil && !s.OnEvent(e) {
			stopped = true
			stopOnce.Do(func() { killGroup(cmd, syscall.SIGTERM) })
			// Drain without processing so the scanner goroutine can finish.
			for range events {
			}
			break
		}
	}
	wg.Wait()

	res.ExitCode = wait(cmd, &res)
	res.Stats = scanStats
	res.Stderr = stderr.String()
	if rawFile != nil {
		_ = rawFile.Sync()
	}
	res.Duration = time.Since(start)
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
	}
	if stopped {
		res.Killed = true
	}
	return res
}

// wait collects the exit status, mapping signal deaths onto the shell
// convention (128+signum) so a caller can distinguish "the agent failed" from
// "we killed it".
func wait(cmd *exec.Cmd, res *Result) int {
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.Sys() != nil {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				// Reached on our own SIGTERM (early stop or timeout) as well
				// as a foreign signal; the shell convention lets the caller
				// tell 143 from 1 by checking TimedOut/Killed.
				res.Killed = true
				return 128 + int(ws.Signal())
			}
			return ws.ExitStatus()
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrProcessDone) {
		res.TimedOut = true
		return -1
	}
	res.Err = errors.Join(res.Err, err)
	return -1
}

func killGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// The process is already gone, or setpgid failed.
		return cmd.Process.Kill()
	}
	return syscall.Kill(-pgid, sig)
}

// tail keeps the last N bytes of stderr without buffering an unbounded log.
type tail struct {
	mu    sync.Mutex
	size  int
	buf   []byte
	total int
}

func newTail(size int) *tail {
	if size <= 0 {
		size = 8192
	}
	return &tail{size: size}
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.total += len(p)
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.size {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.size:]...)
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := string(t.buf)
	if t.total > len(t.buf) {
		return fmt.Sprintf("[... %d earlier stderr bytes truncated ...]\n%s", t.total-len(t.buf), s)
	}
	return s
}

// DryRun renders exactly what would be executed, which is the credit-free
// smoke test: argv and env are checkable without invoking an agent.
func DryRun(s Spec) string {
	var b []byte
	b = append(b, []byte("argv:\n")...)
	for i, a := range s.Argv {
		b = append(b, []byte(fmt.Sprintf("  [%d] %s\n", i, a))...)
	}
	b = append(b, []byte("env:\n")...)
	decided := make(map[string]bool, len(s.Decided))
	for _, k := range s.Decided {
		decided[k] = true
	}
	inherited := 0
	for _, e := range s.Env {
		k, _, _ := strings.Cut(e, "=")
		if len(s.Decided) == 0 || decided[k] {
			b = append(b, []byte("  "+e+"\n")...)
			continue
		}
		inherited++
	}
	if inherited > 0 {
		b = append(b, []byte(fmt.Sprintf(
			"  +%d more passed through from the calling shell unchanged "+
				"(list them with \"golunch env <alias> --all\")\n", inherited))...)
	}
	if s.Dir != "" {
		b = append(b, []byte("dir:\n  "+s.Dir+"\n")...)
	}
	if s.Timeout > 0 {
		b = append(b, []byte(fmt.Sprintf("timeout:\n  %s\n", s.Timeout))...)
	}
	return string(b)
}
