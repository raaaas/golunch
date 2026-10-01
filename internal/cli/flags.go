package cli

import (
	"flag"
	"fmt"
	"strings"
)

// splitArgs is the reason this package does not use the flag package directly
// on os.Args: flag.Parse stops at the first non-flag argument, which would make
// `golunch run kilo-work --prompt "hi"` impossible (the alias is positional) and
// would silently swallow the flags of a passthrough command
// (`golunch run k1 -- ls -la`).
//
// Instead: scan the whole argv, pull out every token that names a registered
// flag, and leave everything else in `rest` in order. A literal `--` ends flag
// scanning, and all remaining tokens go to `rest` verbatim — that is how a child
// flag golunch does not define survives.
//
// Both dash styles work for every long name, because the flag package the values
// are set through accepts both: `-out f.json` and `--out f.json` are the same
// flag. Treating `-out` as a cluster of single-letter flags instead silently
// turned it into a stray positional and the flag did nothing.
func splitArgs(fs *flag.FlagSet, args []string) (rest []string, err error) {
	known := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) { known[f.Name] = true })

	takesValue := func(name string) bool {
		f := fs.Lookup(name)
		if f == nil {
			return false
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
			return !bf.IsBoolFlag()
		}
		return true
	}

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			return append(rest, args[i+1:]...), nil
		case a == "-":
			// A bare dash means stdin to the child; not a golunch flag.
			rest = append(rest, a)
			i++
		case strings.HasPrefix(a, "-"):
			name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
			if name == "" || !known[name] {
				// The plan's escape hatch: an unknown flag belongs to the
				// agent, not to golunch, so it reaches Extra rather than
				// erroring out. Whether it consumes the next token is the
				// agent's business, so it is left to the agent too.
				rest = append(rest, a)
				i++
				continue
			}
			if !hasVal && takesValue(name) {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag %s needs a value", a)
				}
				val = args[i+1]
				i++
			} else if !hasVal {
				// A boolean flag given as --verbose has no value; the flag
				// package rejects "" as a bool, so "true" is what is meant.
				val = "true"
			}
			if err := fs.Set(name, val); err != nil {
				return nil, fmt.Errorf("flag %s: %w", a, err)
			}
			i++
		default:
			rest = append(rest, a)
			i++
		}
	}
	return rest, nil
}

// stringList is a repeatable string flag; used for --env, --noproxy, --file and
// --extra, where repeating is clearer than a comma-separated value a path or URL
// might contain.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// envMap turns repeated K=V flags into a map. A value of "" or a bare K= means
// "unset this variable in the child", which is the only way to remove something
// the isolation block set.
func envMap(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, usageErr("--env %q is not K=V (use K= to unset)", p)
		}
		if k == "" {
			return nil, usageErr("--env %q has an empty variable name", p)
		}
		out[k] = v
	}
	return out, nil
}

// commaSplit expands a comma-separated flag value, so both --noproxy a,b and
// repeated flags work.
func commaSplit(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
