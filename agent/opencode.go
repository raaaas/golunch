package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// openCodeFamily builds a driver for the opencode codebase and its forks.
//
// kilo is a fork of opencode: the subcommand surface, the JSON event format
// and the XDG layout are identical, so one parameterized factory covers both
// and demonstrates the registry contract — a new agent is data plus two
// closures, not new plumbing. If a future fork's flags diverge, that shows up
// as a different BuildArgs here rather than a change anywhere else.
func openCodeFamily(name, binary string, knownDirs []string, envPrefix string, install *InstallInfo) Entry {
	lower := strings.ToLower(name)
	return Entry{
		Name:         lower,
		Aliases:      []string{lower},
		Binary:       binary,
		KnownDirs:    knownDirs,
		Subcmd:       []string{"run"},
		VersionArgs:  []string{"--version"},
		PromptAsArgv: true,
		BuildArgs: func(s RunSpec) []string {
			// "run" is the subcommand; message is an array positional, so it
			// goes last to avoid swallowing a following flag value.
			var a []string
			a = append(a, "--format", "json")
			if s.Model != "" {
				a = append(a, "--model", s.Model)
			}
			if s.Agent != "" {
				a = append(a, "--agent", s.Agent)
			}
			if s.Thinking != "" {
				a = append(a, "--thinking")
			}
			switch {
			case s.SessionID != "":
				a = append(a, "--session", s.SessionID)
			case s.Continue:
				a = append(a, "--continue")
			}
			if s.Fork {
				a = append(a, "--fork")
			}
			if s.Attach != "" {
				a = append(a, "--attach", s.Attach)
			}
			if s.Title != "" {
				a = append(a, "--title", s.Title)
			}
			if s.WorkDir != "" {
				a = append(a, "--dir", s.WorkDir)
			}
			for _, f := range s.Files {
				a = append(a, "--file", f)
			}
			if s.APIKey != "" {
				// Both names are set because the fork kept the upstream
				// variable name in some code paths.
				a = append(a, "--var", envPrefix+"_API_KEY="+s.APIKey)
			}
			a = append(a, s.Extra...)
			if s.Prompt != "" {
				a = append(a, s.Prompt)
			}
			return a
		},
		Env: EnvProfile{
			KeepVars: []string{"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "XDG_SESSION_TYPE", "WAYLAND_DISPLAY", "DISPLAY"},
			BaseURLVars: []string{
				envPrefix + "_BASE_URL", "ANTHROPIC_BASE_URL", "OPENAI_BASE_URL", "OPENROUTER_BASE_URL",
			},
			APIKeyVars: []string{envPrefix + "_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"},
			Secrets: func(p Paths) []string {
				// Verified on this machine: both agents keep auth.json and the
				// account record in XDG_DATA_HOME, which is why overriding HOME
				// plus XDG_DATA_HOME genuinely produces a logged-out instance.
				d := filepath.Join(p.Data, lower)
				return []string{
					filepath.Join(d, "auth.json"),
					filepath.Join(d, "account.json"),
				}
			},
		},
		OwnPaths: func(root string) []string {
			return []string{
				root + "/config/" + lower,
				root + "/data/" + lower,
				root + "/data/" + lower + "/storage",
			}
		},
		// Seed: MCP servers are declared inside the config file for this
		// family, so "mcp" means that one file and both spellings are listed
		// (kilo writes kilo.jsonc, opencode writes opencode.json, and a missing
		// Src is skipped rather than an error).
		//
		// node_modules is its own group because it is 58 MB on this machine and
		// is code, not configuration; copying it silently would make every
		// instance a quarter-gigabyte for no reason.
		Seed: func(h HostPaths) []SeedItem {
			src := filepath.Join(h.Config, lower)
			dst := "config/" + lower
			items := []SeedItem{
				{Group: "mcp", Src: filepath.Join(src, lower+".jsonc"), Dst: dst + "/" + lower + ".jsonc"},
				{Group: "mcp", Src: filepath.Join(src, lower+".json"), Dst: dst + "/" + lower + ".json"},
				{Group: "config", Src: filepath.Join(src, "package.json"), Dst: dst + "/package.json"},
				{Group: "deps", Tree: true, Src: filepath.Join(src, "node_modules"), Dst: dst + "/node_modules"},
			}
			for _, d := range []string{"agents", "agent", "skills", "skill", "plugins", "plugin", "commands", "command", "scripts"} {
				items = append(items, SeedItem{
					Group: "skills", Tree: true,
					Src: filepath.Join(src, d), Dst: dst + "/" + d,
				})
			}
			return items
		},
		ParseLine: parseOpenCodeLine(lower),
		// Install is passed in because kilo and opencode are separate rows over
		// one factory: their upstream installers differ, so each names its own
		// prereqs and note even though everything else is shared.
		Install: install,
	}
}

// opencodeWire is a flat record: the SDK spreads the part object onto the
// envelope rather than nesting it, so {type, sessionID, ...part}. An earlier
// draft of this project assumed a {type:"agent_event", event:{text}} envelope
// and parsed zero events out of kilo's stream while exiting 0 — the quiet
// failure this shape exists to avoid. Both forms are accepted here.
type opencodeWire struct {
	Type      string          `json:"type"`
	Timestamp json.RawMessage `json:"timestamp"`
	SessionID string          `json:"sessionID"`
	Part      json.RawMessage `json:"part"`
	// Spread onto the envelope by the current SDK.
	ID     string `json:"id"`
	Text   string `json:"text"`
	Tool   string `json:"tool"`
	CallID string `json:"callID"`
	// step_finish accounting.
	Cost   float64         `json:"cost"`
	Tokens json.RawMessage `json:"tokens"`
	State  *ocState        `json:"state"`
	Error  json.RawMessage `json:"error"`
	// error shapes vary; keep it generic.
	Message string `json:"message"`
}

type ocState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"`
	Error  string          `json:"error"`
	Title  string          `json:"title"`
}

type ocTokens struct {
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	Reasoning float64 `json:"reasoning"`
	Total     float64 `json:"total"`
	Cache     struct {
		Read  float64 `json:"read"`
		Write float64 `json:"write"`
	} `json:"cache"`
}

// mergeFrom fills the fields this record leaves empty from a nested "part"
// record, so a spread envelope and a {"type":…,"part":{…}} envelope parse to
// the same event. Values already on the outer record are kept.
func (w *opencodeWire) mergeFrom(n opencodeWire) {
	if w.Type == "" {
		w.Type = n.Type
	}
	if w.SessionID == "" {
		w.SessionID = n.SessionID
	}
	if w.Text == "" {
		w.Text = n.Text
	}
	if w.Tool == "" {
		w.Tool = n.Tool
	}
	if w.CallID == "" {
		w.CallID = n.CallID
	}
	if w.ID == "" {
		w.ID = n.ID
	}
	if w.Message == "" {
		w.Message = n.Message
	}
	if w.Cost == 0 {
		w.Cost = n.Cost
	}
	if len(w.Tokens) == 0 {
		w.Tokens = n.Tokens
	}
	if w.State == nil {
		w.State = n.State
	}
	if len(w.Error) == 0 {
		w.Error = n.Error
	}
}

func parseOpenCodeLine(agentName string) ParseLine {
	return func(line []byte) ([]Event, bool) {
		raw := compactRaw(line)
		var w opencodeWire
		if err := json.Unmarshal(line, &w); err != nil {
			return nil, false
		}
		// The SDK has shipped both envelopes: the part spread onto the record,
		// and {"type":…,"part":{…}} with the content nested inside "part". A
		// real opencode 1.18 text line carries type on the outer record and the
		// answer only in part.text, so choosing one envelope loses the text.
		// Merge instead: the outer record wins where it has a value.
		if len(w.Part) > 0 {
			var nested opencodeWire
			if json.Unmarshal(w.Part, &nested) == nil {
				w.mergeFrom(nested)
			}
		}
		base := Event{Agent: agentName, SessionID: w.SessionID, Raw: raw}

		switch strings.ToLower(w.Type) {
		case "text":
			e := base
			e.Type, e.Text = Text, w.Text
			return []Event{e}, true
		case "reasoning":
			e := base
			e.Type, e.Text = Thinking, w.Text
			return []Event{e}, true
		case "tool_use", "tool":
			e := base
			e.Type = ToolCall
			state := ""
			if w.State != nil {
				state = w.State.Status
			}
			e.Tool = &Tool{ID: w.CallID, Name: w.Tool, Input: toolInput(w.State), State: state}
			out := []Event{e}
			if w.State != nil && w.State.Output != "" {
				r := base
				r.Type, r.Text = ToolResult, w.State.Output
				r.Tool = e.Tool
				out = append(out, r)
			}
			return out, true
		case "step_start":
			e := base
			e.Type, e.Status = Status, "step_start"
			return []Event{e}, true
		case "step_finish":
			e := base
			e.Type = Usage
			var tk ocTokens
			if len(w.Tokens) > 0 {
				_ = json.Unmarshal(w.Tokens, &tk)
			}
			e.Usage = &Tokens{
				Input: tk.Input, Output: tk.Output, Reasoning: tk.Reasoning,
				TotalTokens: tk.Total, Cost: w.Cost, Currency: "USD",
			}
			return []Event{e}, true
		case "error":
			e := base
			e.Type, e.Text = Error, errorText(w)
			return []Event{e}, true
		case "":
			// A record with no type is not an event we can name.
			e := base
			e.Type, e.Text = Log, w.Text
			return []Event{e}, true
		default:
			// Unknown lifecycle records (session.idle, permission.updated, ...)
			// become Status so a consumer can still see that something happened.
			e := base
			e.Type, e.Status = Status, w.Type
			e.Text = w.Text
			return []Event{e}, true
		}
	}
}

func toolInput(s *ocState) json.RawMessage {
	if s == nil || len(s.Input) == 0 {
		return nil
	}
	return s.Input
}

// errorText flattens kilo/opencode's error record, whose real shape (captured
// from a 401 on this machine) nests the human message two levels deep:
//
//	{"type":"error","sessionID":"ses_...","error":{"name":"APIError",
//	 "data":{"message":"You need to sign in to use this model.","statusCode":401}}}
//
// Reading only error.message would report an empty error string and hide the
// reason the run failed.
func errorText(w opencodeWire) string {
	if w.Message != "" {
		return w.Message
	}
	if len(w.Error) == 0 {
		return ""
	}
	var obj struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		Data    *struct {
			Message    string `json:"message"`
			StatusCode int    `json:"statusCode"`
		} `json:"data"`
	}
	if json.Unmarshal(w.Error, &obj) != nil {
		// Unrecognized error encoding: keep the bytes rather than lose them.
		return string(w.Error)
	}
	if obj.Data != nil && obj.Data.Message != "" {
		if obj.Data.StatusCode != 0 {
			return fmt.Sprintf("%s (HTTP %d)", obj.Data.Message, obj.Data.StatusCode)
		}
		return obj.Data.Message
	}
	for _, cand := range []string{obj.Message, obj.Name} {
		if cand != "" {
			return cand
		}
	}
	return string(w.Error)
}

// kilo and opencode are two registry entries over one factory. Keeping them
// as named vars lets tests and docs refer to either without going through
// Lookup.
var (
	kiloEntry = openCodeFamily("kilo", "kilo",
		[]string{".kilo/bin/kilo", ".local/bin/kilo", ".bun/bin/kilo"}, "KILO",
		&InstallInfo{
			// ScriptURL empty until the kilo installer URL is verified. kilo
			// distributes a self-contained binary via a curl script, so the
			// child needs sh and curl, plus git/bun for the bun-based path.
			Prereqs: []string{"sh", "curl", "git", "node", "npm"},
			Note: "Runs the upstream installer as a child of this instance under its " +
				"redirected HOME, so the binary lands in the tree rather than the " +
				"host's ~/.local/bin. An installer that sudo's or writes to " +
				"/usr/local/bin will fail here by design.",
		})
	opencodeEntry = openCodeFamily("opencode", "opencode",
		[]string{".opencode/bin/opencode", ".local/bin/opencode", ".bun/bin/opencode"}, "OPENCODE",
		&InstallInfo{
			Prereqs: []string{"sh", "curl", "git", "node", "npm"},
			Note: "Runs the upstream installer as a child of this instance under its " +
				"redirected HOME, so the binary lands in the tree rather than the " +
				"host's ~/.local/bin. An installer that sudo's or writes to " +
				"/usr/local/bin will fail here by design.",
		})
)

func init() {
	register(kiloEntry)
	register(opencodeEntry)
}
