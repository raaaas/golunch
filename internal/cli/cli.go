package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/raaaas/golunch"
	"github.com/raaaas/golunch/config"
	"github.com/raaaas/golunch/instance"
)

// The exit codes and the error constructors are package golunch's: the CLI and
// the library must not have two opinions about what "busy" costs a caller. These
// aliases keep every command file reading the way it always did.
const (
	ExitOK       = golunch.ExitOK
	ExitError    = golunch.ExitError
	ExitUsage    = golunch.ExitUsage
	ExitBusy     = golunch.ExitBusy
	ExitNotFound = golunch.ExitNotFound
	ExitTimeout  = golunch.ExitTimeout
)

type CommandError = golunch.CommandError

var (
	usageErr    = golunch.UsageErr
	notFoundErr = golunch.NotFoundErr
	busyErr     = golunch.BusyErr
)

// App is one command invocation: the resolved configuration, the streams, and
// everything in package golunch that actually runs an instance. What is left
// here is flags, output shape, and the dispatch table.
type App struct {
	*golunch.Runner
}

// New builds an App on the given streams. Only cmd/golunch passes the real
// os.Std*.
func New(in io.Reader, out, errw io.Writer) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &App{Runner: golunch.NewRunner(cfg, in, out, errw, instance.HostFromOs())}, nil
}

func (a *App) print(s string) { fmt.Fprint(a.Out, s) }

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}
func (a *App) warnf(format string, args ...any) {
	fmt.Fprintf(a.Err, format, args...)
}

// mustJSON renders one value on a single line, for --json output that is safe
// to consume even when a field holds a message with newlines in it.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{\"error\":\"unencodable\",\"detail\":\"" + err.Error() + "\"}"
	}
	return string(b)
}
