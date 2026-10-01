package agent

import (
	"encoding/json"
)

// Type is the normalized event vocabulary. Every driver maps its own wire
// format onto these; anything unrecognized becomes Log with Raw preserved so
// an agent update that renames a field degrades to "unparsed but recoverable"
// instead of silently dropping the assistant's answer.
type Type string

const (
	Text       Type = "text"        // assistant prose
	Thinking   Type = "thinking"    // reasoning / chain-of-thought block
	ToolCall   Type = "tool_call"   // agent asks to run a tool
	ToolResult Type = "tool_result" // tool output comes back
	Usage      Type = "usage"       // token/cost accounting
	Status     Type = "status"      // lifecycle: start, step boundary, done
	Error      Type = "error"
	Log        Type = "log" // anything we did not understand
)

type Tool struct {
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	State string          `json:"state,omitempty"`
}

type Tokens struct {
	Input       float64 `json:"input_tokens,omitempty"`
	Output      float64 `json:"output_tokens,omitempty"`
	Reasoning   float64 `json:"reasoning_tokens,omitempty"`
	Cost        float64 `json:"cost,omitempty"`
	Currency    string  `json:"currency,omitempty"`
	TotalTokens float64 `json:"total_tokens,omitempty"`
}

// Total reports the token count, preferring the agent's own sum when it
// supplied one and falling back to the parts.
func (t Tokens) Total() float64 {
	if t.TotalTokens > 0 {
		return t.TotalTokens
	}
	return t.Input + t.Output + t.Reasoning
}

// Any reports whether the agent reported usage at all, so a caller can tell
// "zero tokens" from "never told me".
func (t Tokens) Any() bool { return t.Total() > 0 || t.Cost > 0 }

// Event is one normalized unit of agent output.
type Event struct {
	Type      Type    `json:"type"`
	Agent     string  `json:"agent,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
	Text      string  `json:"text,omitempty"`
	Tool      *Tool   `json:"tool,omitempty"`
	Usage     *Tokens `json:"usage,omitempty"`
	Status    string  `json:"status,omitempty"`
	// Final marks an event that carries a whole-run value rather than one
	// step's contribution, and is how a driver says "replace, do not append".
	// cline needs this twice over: run_result repeats the assistant text that
	// streamed in earlier content_start events, and its aggregateUsage sums
	// usage that already arrived per iteration. Accumulating either additively
	// double-counts the answer and every token.
	Final bool            `json:"final,omitempty"`
	Raw   json.RawMessage `json:"raw,omitempty"`
}

// Accumulator flattens a stream into the fields a caller usually wants: the
// final answer text, every tool invocation, and summed usage.
type Accumulator struct {
	Text      string
	Tools     []*Tool
	Errors    []string
	Usage     Tokens
	Skipped   int
	sessionID string
}

func (a *Accumulator) Add(e Event) {
	switch e.Type {
	case Text:
		if e.Final {
			// Authoritative whole-run answer: it repeats what already
			// streamed, so appending would print the reply twice.
			a.Text = e.Text
		} else {
			a.Text += e.Text
		}
	case ToolCall:
		if e.Tool != nil {
			a.Tools = append(a.Tools, e.Tool)
		}
	case Error:
		// cline reports a failure as an agent_event error and then repeats it
		// in run_result; keeping only distinct messages avoids three copies of
		// one 401.
		if e.Text != "" && (len(a.Errors) == 0 || a.Errors[len(a.Errors)-1] != e.Text) {
			a.Errors = append(a.Errors, e.Text)
		}
	case Usage:
		if e.Usage != nil {
			if e.Final {
				// A run total replaces the sum of per-step events, which
				// would otherwise count those tokens again.
				a.Usage = *e.Usage
			} else {
				a.Usage.Input += e.Usage.Input
				a.Usage.Output += e.Usage.Output
				a.Usage.Reasoning += e.Usage.Reasoning
				a.Usage.Cost += e.Usage.Cost
				a.Usage.TotalTokens += e.Usage.TotalTokens
			}
			if e.Usage.Currency != "" {
				a.Usage.Currency = e.Usage.Currency
			}
		}
	}
	if e.SessionID != "" {
		a.sessionID = e.SessionID
	}
}

func (a *Accumulator) SessionID() string { return a.sessionID }
