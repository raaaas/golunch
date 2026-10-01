package agent

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
)

// clineEntry models the Cline CLI's headless contract.
//
// Two corrections against the sample code this project started from:
//   - there is no "-y". Auto-approval in the installed version is
//     --auto-approve <boolean>, and passing -y makes cline exit nonzero.
//   - the prompt is a positional argument, so it must survive argv ordering.
var clineEntry = Entry{
	Name:        "cline",
	Aliases:     []string{"cline-cli"},
	Binary:      "cline",
	KnownDirs:   []string{"~/.cline/bin/cline", "~/nvm/versions/node/*/bin/cline"},
	VersionArgs: []string{"--version"},
	// PromptAsArgv: cline takes the prompt as a trailing positional.
	PromptAsArgv:  true,
	SupportNative: true,
	NativeArgs: func(s RunSpec) []string {
		// cline is the only v1 agent offering first-class isolation flags.
		// They complement the HOME/XDG override rather than replace it: HOME
		// alone is sufficient because cline's defaults (~/.cline, ~/.cline/data)
		// are HOME-relative, but passing the flags means an instance's state
		// location is explicit in the argv we log rather than implied by env.
		var out []string
		if s.Native.ConfigDir != "" {
			out = append(out, "--config", s.Native.ConfigDir)
		}
		if s.Native.DataDir != "" {
			out = append(out, "--data-dir", s.Native.DataDir)
		}
		return out
	},
	BuildArgs: func(s RunSpec) []string {
		var a []string
		a = append(a, "--json")
		// Default to act mode with approvals on: a headless run with nothing
		// listening on stdin deadlocks on the first tool confirmation.
		if s.AutoApprove {
			a = append(a, "--auto-approve", "true")
		} else {
			a = append(a, "--auto-approve", "false")
		}
		if s.Plan {
			a = append(a, "--plan")
		}
		if s.Model != "" {
			a = append(a, "--model", s.Model)
		}
		if s.Provider != "" {
			a = append(a, "--provider", s.Provider)
		}
		if s.Thinking != "" {
			a = append(a, "--thinking", s.Thinking)
		}
		if s.SessionID != "" {
			a = append(a, "--id", s.SessionID)
		}
		if s.WorkDir != "" {
			a = append(a, "--cwd", s.WorkDir)
		}
		if secs := int(s.Timeout.Seconds()); secs > 0 {
			a = append(a, "--timeout", strconv.Itoa(secs))
		}
		a = append(a, s.Extra...)
		if s.Prompt != "" {
			a = append(a, s.Prompt)
		}
		return a
	},
	Env: EnvProfile{
		KeepVars:    []string{"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "GIT_EXEC_PATH"},
		BaseURLVars: []string{"ANTHROPIC_BASE_URL", "OPENAI_BASE_URL"},
		APIKeyVars:  []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY"},
		Secrets: func(p Paths) []string {
			// cline keeps state under $HOME/.cline rather than the XDG dirs,
			// so these are relative to the instance HOME.
			return []string{
				filepath.Join(p.Home, ".cline", "data", "secrets.json"),
				filepath.Join(p.Home, ".cline", "data", "globalState.json"),
				// Measured on this machine: data/settings/providers.json holds
				// "apiKey" and "accessToken" in plaintext. It sits beside the MCP
				// settings file, so anything copying that directory must treat it
				// as a credential, not configuration.
				filepath.Join(p.Home, ".cline", "data", "settings", "providers.json"),
			}
		},
	},
	// Install describes how a downloader fetches cline's upstream installer
	// and runs it under the instance's redirected HOME. ScriptURL is left
	// empty: the vendor URL was not verified when this row was written, so
	// `golunch install cline` requires an explicit --url/--script until a later
	// commit checks the docs. Prereqs name what the child PATH must carry for
	// the script itself to run: cline ships through npm, so node and npm are
	// load-bearing, and the script's own fetches need sh and curl.
	Install: &InstallInfo{
		Prereqs: []string{"sh", "curl", "git", "node", "npm"},
		Note: "Runs an upstream shell installer as a child of this instance. It inherits " +
			"the redirected HOME and PATH, so npm global installs land inside the tree; " +
			"an installer that calls sudo or hardcodes /usr/local/bin will fail here by " +
			"design rather than escape onto the host.",
	},
	OwnPaths: func(root string) []string {
		return []string{
			root + "/home/.cline",
			root + "/home/.cline/data",
			root + "/home/.cline/data/sessions",
		}
	},
	// Seed lists single files, not the directory that holds them, precisely
	// because that directory also contains providers.json.
	//
	// The MCP settings file is seeded twice: cline reads it from ~/.cline when
	// HOME points at the instance, and from --data-dir on the headless path
	// where golunch passes that flag. Measured here, `--config`/`--data-dir`
	// still make cline create and use ~/.cline, so HOME-relative is the truth
	// for some code paths and the flag-relative one is the truth for others.
	// Copying to both costs one small file; guessing wrong costs a confusing
	// "my MCP servers vanished" report.
	Seed: func(h HostPaths) []SeedItem {
		st := filepath.Join(h.Home, ".cline", "data", "settings")
		homeDst := filepath.Join("home", ".cline", "data", "settings")
		nativeDst := filepath.Join("data", "settings")
		mcp := "cline_mcp_settings.json"
		return []SeedItem{
			{Group: "mcp", Src: filepath.Join(st, mcp), Dst: filepath.Join(homeDst, mcp)},
			{Group: "mcp", Src: filepath.Join(st, mcp), Dst: filepath.Join(nativeDst, mcp)},
			{Group: "config", Src: filepath.Join(st, "global-settings.json"),
				Dst: filepath.Join(homeDst, "global-settings.json")},
			{Group: "skills", Tree: true,
				Src: filepath.Join(h.Home, ".cline", "skills"),
				Dst: filepath.Join("home", ".cline", "skills")},
		}
	},
	ParseLine: parseClineLine,
}

// clineWire is the outer envelope, recorded from `cline --json` on this
// machine. Every record carries ts plus one of three shapes:
//
//	{type:"hook_event", hookEventName, agentId, taskId, parentAgentId}
//	{type:"agent_event", event:{type:<tag>, ...tag-specific fields}}
//	{type:"run_result", finishReason, text, usage, aggregateUsage, model, ...}
//
// The sample this project started from assumed event.text always exists and
// read nothing else. In reality event.type is a tagged union, and the final
// answer plus the authoritative cost total lives in run_result, which the
// sample never looked at.
type clineWire struct {
	Type          string          `json:"type"`
	TS            string          `json:"ts"`
	Event         json.RawMessage `json:"event"`
	Payload       json.RawMessage `json:"payload"`
	HookEventName string          `json:"hookEventName"`
	AgentID       string          `json:"agentId"`
	TaskID        string          `json:"taskId"`
	// run_result fields.
	FinishReason   string      `json:"finishReason"`
	Text           string      `json:"text"`
	Iterations     int         `json:"iterations"`
	DurationMs     int64       `json:"durationMs"`
	Usage          *clineUsage `json:"usage"`
	AggregateUsage *clineUsage `json:"aggregateUsage"`
	Model          *clineModel `json:"model"`
	// Some builds flatten these onto the envelope.
	SessionID string `json:"sessionId"`
}

type clineModel struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

type clineEvent struct {
	Type        string          `json:"type"`
	Text        string          `json:"text"`
	ContentType string          `json:"contentType"`
	ToolName    string          `json:"toolName"`
	ToolCallID  string          `json:"toolCallId"`
	Input       json.RawMessage `json:"input"`
	SessionID   string          `json:"sessionId"`
	Iteration   int             `json:"iteration"`
	Usage       *clineUsage     `json:"usage"`
	// event.type == "error" nests an Error object; event.message does not exist.
	Error *struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		Stack   string `json:"stack"`
	} `json:"error"`
	Recoverable *bool  `json:"recoverable"`
	Message     string `json:"message"`
}

type clineUsage struct {
	InputTokens      float64 `json:"inputTokens"`
	OutputTokens     float64 `json:"outputTokens"`
	CacheReadTokens  float64 `json:"cacheReadTokens"`
	CacheWriteTokens float64 `json:"cacheWriteTokens"`
	TotalCost        float64 `json:"totalCost"`
	// Older/keyed spellings seen in the hub path.
	TokensIn  float64 `json:"tokensIn"`
	TokensOut float64 `json:"tokensOut"`
	Cost      float64 `json:"cost"`
}

func (u *clineUsage) tokens() *Tokens {
	if u == nil {
		return nil
	}
	in := firstNonZeroF(u.InputTokens, u.TokensIn)
	out := firstNonZeroF(u.OutputTokens, u.TokensOut)
	return &Tokens{
		Input:       in,
		Output:      out,
		Cost:        u.TotalCost,
		TotalTokens: in + out + u.CacheReadTokens + u.CacheWriteTokens,
		Currency:    "USD",
	}
}

func parseClineLine(line []byte) ([]Event, bool) {
	raw := compactRaw(line)
	var w clineWire
	if err := json.Unmarshal(line, &w); err != nil {
		return nil, false
	}
	// taskId (conv_...) is the identifier cline itself reports a run under.
	session := firstNonEmptyStr(w.TaskID, w.SessionID, w.AgentID)

	switch strings.ToLower(w.Type) {
	case "hook_event":
		return []Event{{Type: Status, Agent: "cline", SessionID: session,
			Status: w.HookEventName, Raw: raw}}, true

	case "run_result":
		// The terminal summary. Emitted last, always. Its text and usage are
		// whole-run values that duplicate earlier events, so they are marked
		// Final and consumed by replacement.
		base := Event{Agent: "cline", SessionID: session, Raw: raw, Final: true}
		if strings.EqualFold(w.FinishReason, "error") || strings.Contains(strings.ToLower(w.FinishReason), "abort") {
			e := base
			e.Type, e.Text, e.Status = Error, w.Text, w.FinishReason
			e.Usage = w.AggregateUsage.tokens()
			return []Event{e}, true
		}
		out := []Event{}
		if w.Text != "" {
			e := base
			e.Type, e.Text = Text, w.Text
			out = append(out, e)
		}
		if u := w.AggregateUsage.tokens(); u != nil {
			e := base
			e.Type, e.Usage, e.Status = Usage, u, w.FinishReason
			out = append(out, e)
		}
		e := base
		e.Type, e.Status = Status, "done:"+w.FinishReason
		out = append(out, e)
		return out, true

	case "agent_event":
		var ev clineEvent
		if err := json.Unmarshal(w.Event, &ev); err != nil {
			return []Event{{Type: Log, Agent: "cline", SessionID: session, Raw: raw}}, true
		}
		return clineInner(ev, firstNonEmptyStr(ev.SessionID, session), raw)

	case "chunk":
		// Hub/team relay path.
		body := w.Payload
		var ev clineEvent
		if len(body) > 0 && json.Unmarshal(body, &ev) == nil && ev.Type != "" {
			return clineInner(ev, firstNonEmptyStr(ev.SessionID, session), raw)
		}
		return []Event{{Type: Log, Agent: "cline", SessionID: session, Text: w.Text, Raw: raw}}, true

	default:
		if w.Type == "" {
			return []Event{{Type: Log, Agent: "cline", Raw: raw}}, true
		}
		return []Event{{Type: Status, Agent: "cline", SessionID: session,
			Status: w.Type, Text: w.Text, Raw: raw}}, true
	}
}

func clineInner(ev clineEvent, session string, raw json.RawMessage) ([]Event, bool) {
	base := Event{Agent: "cline", SessionID: session, Raw: raw}
	switch strings.ToLower(ev.Type) {
	case "content_start", "content":
		switch strings.ToLower(ev.ContentType) {
		case "tool":
			e := base
			e.Type = ToolCall
			e.Tool = &Tool{ID: ev.ToolCallID, Name: ev.ToolName, Input: ev.Input, State: "called"}
			return []Event{e}, true
		case "text", "":
			e := base
			e.Type, e.Text = Text, ev.Text
			return []Event{e}, true
		default:
			e := base
			e.Type, e.Text = Log, ev.Text
			return []Event{e}, true
		}
	case "tool_result", "tool_output":
		e := base
		e.Type, e.Text = ToolResult, ev.Text
		e.Tool = &Tool{ID: ev.ToolCallID, Name: ev.ToolName, State: "completed"}
		return []Event{e}, true
	case "usage":
		e := base
		e.Type, e.Usage = Usage, ev.Usage.tokens()
		return []Event{e}, true
	case "error":
		e := base
		e.Type = Error
		switch {
		case ev.Error != nil && ev.Error.Message != "":
			e.Text = ev.Error.Message
		case ev.Message != "":
			e.Text = ev.Message
		default:
			e.Text = ev.Text
		}
		if ev.Recoverable != nil && !*ev.Recoverable {
			e.Status = "fatal"
		}
		return []Event{e}, true
	case "iteration_start", "iteration_end", "task_start", "task_end", "done", "compact":
		e := base
		e.Type, e.Status = Status, ev.Type
		e.Text = ev.Text
		return []Event{e}, true
	case "text", "assistant", "message", "delta":
		e := base
		e.Type, e.Text = Text, ev.Text
		return []Event{e}, true
	default:
		if ev.Text != "" {
			e := base
			e.Type, e.Text = Text, ev.Text
			return []Event{e}, true
		}
		e := base
		e.Type, e.Status = Status, ev.Type
		return []Event{e}, true
	}
}

func firstNonZeroF(vals ...float64) float64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func init() { register(clineEntry) }
