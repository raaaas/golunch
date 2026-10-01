package golunch

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/execd"
	"github.com/raaaas/golunch/internal/osutil"
)

// eventPrinter renders the normalized stream. In human mode stdout carries only
// the answer and stderr carries progress, so `golunch run k --prompt ... > f`
// gives you the text and nothing else.
type eventPrinter struct {
	accum   agent.Accumulator
	out     io.Writer
	meta    io.Writer
	f       func(agent.Event) bool
	plain   bool
	jsonOut bool
	agent   string
	// streamed records whether any answer text reached stdout, so a driver that
	// reports only the final whole answer still produces output.
	streamed bool
}

type promptPrinter struct {
	p *eventPrinter
}

func (r *Runner) eventPrinter(pr PromptOptions, agentName string) *promptPrinter {
	p := &eventPrinter{out: r.Out, meta: r.Err, plain: !pr.JSONL && !pr.Quiet,
		jsonOut: pr.JSONL, agent: agentName}
	pp := &promptPrinter{p: p}
	p.f = func(ev agent.Event) bool {
		p.accum.Add(ev)
		return true
	}
	if pr.JSONL {
		enc := json.NewEncoder(r.Out)
		p.f = func(ev agent.Event) bool {
			p.accum.Add(ev)
			_ = enc.Encode(ev)
			return true
		}
		return pp
	}
	if !pr.Stream {
		return pp
	}
	p.f = func(ev agent.Event) bool {
		p.accum.Add(ev)
		switch ev.Type {
		case agent.Text:
			// Only streamed text is printed here: a driver's final record
			// repeats the whole answer, and printing both shows it twice.
			if !ev.Final && ev.Text != "" {
				fmt.Fprint(p.out, ev.Text)
				p.streamed = true
			}
		case agent.Thinking:
			if p.plain && ev.Text != "" {
				fmt.Fprintf(p.meta, "\n[thinking] %s\n", ev.Text)
			}
		case agent.ToolCall:
			if ev.Tool != nil {
				label := ev.Tool.Name
				if ev.Tool.State != "" {
					label += " " + ev.Tool.State
				}
				fmt.Fprintf(p.meta, "\n[%s] %s %s\n", p.agent, label, summarize(ev.Tool.Input))
			}
		case agent.ToolResult:
			if ev.Text != "" && p.plain {
				fmt.Fprintf(p.meta, "[result] %s\n", osutil.FirstLine(ev.Text))
			}
		case agent.Error:
			fmt.Fprintf(p.meta, "\nerror: %s\n", ev.Text)
		case agent.Status:
			if p.plain && ev.Text != "" {
				fmt.Fprintf(p.meta, "[%s] %s\n", ev.Status, osutil.FirstLine(ev.Text))
			}
		case agent.Usage:
		}
		return true
	}
	return pp
}

// finish closes the human rendering.
func (pp *promptPrinter) finish(r execd.Result) {
	p := pp.p
	if p.jsonOut {
		return
	}
	if p.accum.Text != "" {
		if !p.streamed {
			fmt.Fprint(p.out, p.accum.Text)
		}
		// Streamed text arrives without a trailing newline; end the line either
		// way so a pipe and a terminal both finish cleanly.
		fmt.Fprintln(p.out)
	}
	if r.Stats.Skipped > 0 {
		fmt.Fprintf(p.meta, "(%d unparsed lines ignored)\n", r.Stats.Skipped)
	}
	if p.accum.Usage.Any() {
		u := p.accum.Usage
		fmt.Fprintf(p.meta, "usage: in=%.0f out=%.0f reasoning=%.0f cost=%s\n",
			u.Input, u.Output, u.Reasoning, money(u))
	}
	if s := p.accum.SessionID(); s != "" {
		fmt.Fprintf(p.meta, "session: %s\n", s)
	}
	if r.Stderr != "" {
		fmt.Fprintf(p.meta, "--- agent stderr, last %d bytes ---\n%s\n", len(r.Stderr), r.Stderr)
	}
}

func summarize(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return osutil.FirstLine(string(raw))
	}
	for _, k := range []string{"command", "path", "filePath", "pattern", "url", "description"} {
		if s, ok := v[k].(string); ok && s != "" {
			return osutil.FirstLine(s)
		}
	}
	return osutil.FirstLine(string(raw))
}

func money(u agent.Tokens) string {
	if u.Currency == "" {
		return fmt.Sprintf("$%.4f", u.Cost)
	}
	return fmt.Sprintf("%.4f %s", u.Cost, u.Currency)
}
