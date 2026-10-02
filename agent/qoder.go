package agent

import (
	"encoding/json"
	"path/filepath"
)

// qoderEntry drives Qoder CLI (qodercli). Its headless contract was measured
// against qodercli 1.1.64 on this machine, not copied from documentation:
// `-p --output-format json` emits one result record whose shape is the
// Claude-Code style {type, subtype, result, usage, session_id, is_error}.
// The binary is found through the dispatcher in ~/.qoder/entry/qoder, which
// resolves the real qodercli under $HOME/.qoder/bin — HOME-relative, which is
// what makes a golunch instance of it genuinely isolated once the instance's
// .qoder/bin/qodercli link and .auth/user are in place.
var qoderEntry = Entry{
	Name:      "qoder",
	Aliases:   []string{"qodercli"},
	Binary:    "qoder",
	KnownDirs: []string{"~/.qoder/entry/qoder", "~/.qoder/bin/qodercli/qodercli"},
	// Subcmd is nil: -p is a flag, not a subcommand.
	VersionArgs:  []string{"--version"},
	PromptAsArgv: true,
	BuildArgs: func(s RunSpec) []string {
		// The prompt is a positional query; keep it last so a flag that
		// takes a value never swallows it.
		var a []string
		a = append(a, "-p", "--output-format", "json")
		if s.Model != "" {
			a = append(a, "-m", s.Model)
		}
		if s.Thinking != "" {
			a = append(a, "--thinking", s.Thinking)
		}
		switch {
		case s.SessionID != "":
			a = append(a, "--session-id", s.SessionID)
		case s.Continue:
			a = append(a, "--continue")
		}
		if s.Fork {
			a = append(a, "--fork-session")
		}
		if s.Title != "" {
			a = append(a, "-n", s.Title)
		}
		if s.WorkDir != "" {
			a = append(a, "-w", s.WorkDir)
		}
		for _, f := range s.Files {
			a = append(a, "--attachment", f)
		}
		if s.AutoApprove {
			a = append(a, "--dangerously-skip-permissions")
		}
		a = append(a, s.Extra...)
		if s.Prompt != "" {
			a = append(a, s.Prompt)
		}
		return a
	},
	Env: EnvProfile{
		// Login is browser-sourced and lives under $HOME/.qoder, not XDG.
		// Existence of .auth/user means "this instance is logged in";
		// machine_id is the pair it is validated against.
		Secrets: func(p Paths) []string {
			d := filepath.Join(p.Home, ".qoder", ".auth")
			return []string{
				filepath.Join(d, "user"),
				filepath.Join(d, "machine_id"),
			}
		},
	},
	OwnPaths: func(root string) []string {
		return []string{
			root + "/home/.qoder",
			root + "/home/.qoder/.auth",
		}
	},
	// Seed is deliberately nil: the files worth seeding (skills, plugins) live
	// beside the credentials in the same $HOME/.qoder tree, and the host copy
	// is not reviewed per-file yet. A nil row makes `golunch seed` say so
	// instead of guessing.
	ParseLine: parseQoderLine,
}

// parseQoderLine maps one stdout record. Anything that is not the result
// envelope becomes Log with Raw preserved, so a future qodercli that streams
// intermediate records degrades recoverably instead of losing the answer.
func parseQoderLine(line []byte) ([]Event, bool) {
	var r struct {
		Type      string  `json:"type"`
		Subtype   string  `json:"subtype"`
		IsError   bool    `json:"is_error"`
		Result    string  `json:"result"`
		SessionID string  `json:"session_id"`
		TotalCost float64 `json:"total_cost_usd"`
		Usage     struct {
			Input  float64 `json:"input_tokens"`
			Output float64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &r); err != nil || r.Type == "" {
		return nil, false
	}
	raw := append(json.RawMessage(nil), line...)
	if r.Type != "result" {
		return []Event{{Type: Log, Agent: "qoder", Raw: raw}}, true
	}
	if r.IsError || (r.Subtype != "" && r.Subtype != "success") {
		return []Event{{Type: Error, Agent: "qoder", Text: r.Result,
			SessionID: r.SessionID, Final: true, Raw: raw}}, true
	}
	evs := []Event{{Type: Text, Agent: "qoder", Text: r.Result,
		SessionID: r.SessionID, Final: true, Raw: raw}}
	tk := Tokens{Input: r.Usage.Input, Output: r.Usage.Output, Cost: r.TotalCost}
	if tk.Any() {
		evs = append(evs, Event{Type: Usage, Agent: "qoder", Usage: &tk})
	}
	return evs, true
}

func init() { register(qoderEntry) }
