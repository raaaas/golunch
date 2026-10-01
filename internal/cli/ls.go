package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/raaaas/golunch/agent"
	"github.com/raaaas/golunch/instance"
)

// CmdList prints one row per instance. Disk usage is opt-in with -l because one
// agent's data dir measured over a gigabyte on this machine (kilo keeps a
// 1.2 GB sqlite file), and walking every instance tree on a plain `ls` is not
// acceptable.
func (a *App) CmdList(args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	long := fs.Bool("l", false, "show size, version, proxy source and last run")
	jsonOut := fs.Bool("json", false, "one JSON object per instance")
	if _, err := splitArgs(fs, args); err != nil {
		return err
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return usageErr("golunch ls takes no arguments")
	}

	names, err := instance.List(a.InstancesDir())
	if err != nil {
		return err
	}
	if len(names) == 0 {
		a.printf("no instances yet. Create one with: golunch new <alias> --agent %s\n",
			strings.Join(agent.Names(), "|"))
		return nil
	}

	head := []string{"ALIAS", "AGENT", "BINARY", "LAST RUN"}
	if *long {
		head = append(head, "SIZE", "VERSION", "PROXY SOURCE", "COMMAND LINK")
	}
	var rows [][]string
	var jsonRows []map[string]any

	for _, name := range names {
		inst, err := instance.New(a.InstancesDir(), name)
		if err != nil {
			a.warnf("skipping %s: %v\n", name, err)
			continue
		}
		meta, err := inst.LoadMetadata()
		if err != nil {
			rows = append(rows, []string{name, "! unreadable metadata", "-", "-"})
			continue
		}
		agentCol := meta.Instance.Agent
		if agentCol == "" {
			agentCol = "(wrapper)"
		}
		binCol := "-"
		if len(meta.Launch.Command) > 0 {
			binCol = meta.Launch.Command[0]
		}
		lastCol := "-"
		if meta.LastRun != nil && !meta.LastRun.StartedAt.IsZero() {
			lastCol = meta.LastRun.StartedAt.Local().Format("2006-01-02 15:04")
		}
		cols := []string{name, agentCol, binCol, lastCol}

		extras := []string{}
		if *long {
			size := "-"
			if _, total, err := inst.DiskUsage(); err == nil {
				size = instance.FormatBytes(total)
			}
			link := "-"
			if meta.Paths.LinkPath != "" {
				if _, err := os.Lstat(meta.Paths.LinkPath); err != nil {
					link = "missing"
				} else {
					link = meta.Paths.LinkPath
				}
			}
			extras = []string{size, dash(meta.Instance.Version),
				dash(string(a.ResolveSource(meta))), link}
			cols = append(cols, extras...)
		}
		rows = append(rows, cols)

		if *jsonOut {
			jsonRows = append(jsonRows, map[string]any{
				"alias": name, "agent": meta.Instance.Agent, "binary": binCol,
				"root": inst.Root, "last_run": lastCol, "version": meta.Instance.Version,
				"proxy_source": string(a.ResolveSource(meta)), "link": meta.Paths.LinkPath,
			})
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(a.Out)
		for _, r := range jsonRows {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	}
	printTable(a.Out, head, rows)
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// printTable computes every column width before printing anything, so the
// header lines up with the widest value in each column.
func printTable(out io.Writer, head []string, rows [][]string) {
	widths := make([]int, len(head))
	for i, h := range head {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	line := func(cols []string) {
		parts := make([]string, 0, len(cols))
		for i, c := range cols {
			parts = append(parts, fmt.Sprintf("%-*s", widths[i], c))
		}
		fmt.Fprintln(out, strings.TrimRight(strings.Join(parts, "  "), " "))
	}
	line(head)
	for _, r := range rows {
		line(r)
	}
}
