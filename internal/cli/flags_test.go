package cli

import (
	"flag"
	"reflect"
	"testing"
)

func newTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.String("out", "", "")
	fs.String("m", "", "")
	fs.String("model", "", "")
	fs.Bool("dry-run", false, "")
	fs.Int("parallel", 0, "")
	fs.SetOutput(devNull{})
	return fs
}

type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

// The bug this pins: `-out summary.json` was read as a cluster of one-letter
// flags, matched nothing, and became a stray positional — the flag silently did
// nothing and its value was mistaken for an argument. Both dash styles must
// reach the same flag, because the flag package accepts both.
func TestSplitArgsAcceptsBothDashStyles(t *testing.T) {
	for _, args := range [][]string{
		{"taskfile.json", "-out", "s.json"},
		{"taskfile.json", "--out", "s.json"},
		{"taskfile.json", "-out=s.json"},
	} {
		fs := newTestFlagSet()
		rest, err := splitArgs(fs, args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got := fs.Lookup("out").Value.String(); got != "s.json" {
			t.Errorf("%v: out=%q want s.json", args, got)
		}
		if !reflect.DeepEqual(rest, []string{"taskfile.json"}) {
			t.Errorf("%v: rest=%v want [taskfile.json]", args, rest)
		}
	}
}

func TestSplitArgsShapes(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantRest  []string
		wantModel string
		wantM     string
		wantPar   string
		wantDry   bool
	}{
		{
			name:     "short flag with separate value",
			args:     []string{"k1", "-m", "ollama/llama3", "prompt words"},
			wantRest: []string{"k1", "prompt words"},
			wantM:    "ollama/llama3",
		},
		{
			name:      "positional before flags",
			args:      []string{"k1", "--model", "x/y", "-parallel", "3"},
			wantRest:  []string{"k1"},
			wantModel: "x/y",
			wantPar:   "3",
		},
		{
			name:     "boolean without value",
			args:     []string{"k1", "-dry-run"},
			wantRest: []string{"k1"},
			wantDry:  true,
		},
		{
			// The agent's own flags must survive untouched, including the token
			// after them, which golunch cannot know is a value.
			name:     "unknown flag passes through",
			args:     []string{"k1", "--unknown", "--agent", "plan", "hi"},
			wantRest: []string{"k1", "--unknown", "--agent", "plan", "hi"},
		},
		{
			name:      "double dash ends scanning",
			args:      []string{"k1", "--", "sh", "-c", "echo hi"},
			wantRest:  []string{"k1", "sh", "-c", "echo hi"},
			wantModel: "",
		},
		{
			name:     "bare dash is stdin, not a flag",
			args:     []string{"k1", "--unknown", "-"},
			wantRest: []string{"k1", "--unknown", "-"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := newTestFlagSet()
			rest, err := splitArgs(fs, c.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rest, c.wantRest) {
				t.Errorf("rest=%v want %v", rest, c.wantRest)
			}
			if got := fs.Lookup("model").Value.String(); got != c.wantModel {
				t.Errorf("model=%q want %q", got, c.wantModel)
			}
			if c.wantM != "" {
				if got := fs.Lookup("m").Value.String(); got != c.wantM {
					t.Errorf("m=%q want %q", got, c.wantM)
				}
			}
			if c.wantPar != "" {
				if got := fs.Lookup("parallel").Value.String(); got != c.wantPar {
					t.Errorf("parallel=%q want %q", got, c.wantPar)
				}
			}
			if got := fs.Lookup("dry-run").Value.String(); got != boolString(c.wantDry) {
				t.Errorf("dry-run=%q want %v", got, c.wantDry)
			}
		})
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
