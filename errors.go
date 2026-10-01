package golunch

import (
	"errors"
	"fmt"

	"github.com/raaaas/golunch/instance"
)

// Exit codes are part of the interface: a task fan-out and a human shell both
// need to tell "you asked for it wrong" from "the instance is busy" from "the
// agent failed". They are the values `golunch` returns to the operating system,
// and ErrCode gives a library caller the same number without exec'ing anything.
const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitBusy     = 3
	ExitNotFound = 4
	// ExitTimeout follows the FreeBSD timeout(1) convention, so a shell that
	// already knows 124 means "took too long" keeps knowing it.
	ExitTimeout = 124
)

// Kind says what sort of failure this was, independent of the number a shell
// would see.
type Kind int

const (
	// KindUnknown is the zero value, so a caller that asks for the kind of an
	// error golunch did not produce gets this rather than a plausible answer.
	KindUnknown Kind = iota
	KindUsage
	KindBusy
	KindNotFound
	KindTimeout
	// KindAgent is a failure inside the agent that was launched, including a
	// nonzero exit from the child itself.
	KindAgent
)

func (k Kind) String() string {
	switch k {
	case KindUsage:
		return "usage"
	case KindBusy:
		return "busy"
	case KindNotFound:
		return "not found"
	case KindTimeout:
		return "timeout"
	case KindAgent:
		return "agent"
	default:
		return "unknown"
	}
}

// CommandError carries an exit code alongside its message so a run can say
// "this is a usage error" without printing a stack or a help dump.
type CommandError struct {
	Code int
	Err  error
	// Cause keeps what actually failed classifiable after the message was
	// rewritten for a human. A busy run replaces flock's error with "work is
	// busy (another run holds it)", and without Cause a caller could not still
	// tell a lock collision from any other failure — the printed message is for
	// the terminal, Cause is for errors.Is.
	Cause error
}

func (e CommandError) Error() string { return e.Err.Error() }

// Unwrap exposes the message error and, when there is one, the underlying
// cause, so errors.Is and errors.As see both.
func (e CommandError) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Err}
	}
	return []error{e.Err, e.Cause}
}

// CodeError returns the exit code carried by err, or -1 when err is not a
// golunch error. Callers that want the process-level number usually want
// ErrCode instead, which treats an unremarkable error as failure.
func CodeError(err error) int {
	var ce CommandError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return -1
}

// ErrCode maps an error onto the exit code golunch would return for it. A child
// exit code is passed through unchanged, so `golunch run k -- make` reports
// make's status, and a nil error is 0.
func ErrCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ce CommandError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ExitError
}

// ErrKind classifies an error. The child's own exit code is KindAgent whatever
// the number is: golunch can only say the agent failed, not what failing meant
// to it.
func ErrKind(err error) Kind {
	switch {
	case err == nil:
		return KindUnknown
	case errors.Is(err, instance.ErrLocked):
		return KindBusy
	}
	code := CodeError(err)
	switch code {
	case ExitUsage:
		return KindUsage
	case ExitBusy:
		return KindBusy
	case ExitNotFound:
		return KindNotFound
	case ExitTimeout:
		return KindTimeout
	case -1:
		return KindUnknown
	default:
		return KindAgent
	}
}

func UsageErr(format string, a ...any) error {
	return CommandError{Code: ExitUsage, Err: fmt.Errorf(format, a...)}
}
func NotFoundErr(format string, a ...any) error {
	return CommandError{Code: ExitNotFound, Err: fmt.Errorf(format, a...)}
}
func BusyErr(format string, a ...any) error {
	return CommandError{Code: ExitBusy, Err: fmt.Errorf(format, a...)}
}

// IsBusy reports whether an error is a lock collision, which is the difference
// between retrying in a moment and retrying forever.
func IsBusy(err error) bool { return errors.Is(err, instance.ErrLocked) }
