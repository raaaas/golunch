package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/raaaas/golunch/instance"
)

// CmdRemove deletes an instance tree and the command link that points into it.
func (a *App) CmdRemove(args []string) error {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	keepLink := fs.Bool("keep-link", false, "leave the ~/.local/bin entry in place")
	rest, err := splitArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: golunch rm <alias> [--yes]")
	}
	inst, meta, err := a.Open(rest[0])
	if err != nil {
		return err
	}

	// A running agent holds a shared flock, so attempting the exclusive lock is
	// the check. Deleting a tree an agent is writing into would leave that agent
	// with a vanished HOME and no record of why it broke.
	lock, err := inst.Acquire(instance.Exclusive)
	if err != nil {
		return busyErr("%s is busy: an agent is running in it, or another admin command holds it", inst.Alias)
	}
	defer lock.Release()

	if _, total, err := inst.DiskUsage(); err == nil {
		a.warnf("%s holds %s in %s\n", inst.Alias, instance.FormatBytes(total), inst.Root)
	}
	if meta.Instance.Agent != "" {
		a.warnf("this deletes the login, sessions and configuration of agent %q.\n", meta.Instance.Agent)
	}
	// Worth saying outright, because the common case is a link to a host install
	// that survives the instance: here golunch downloaded the agent, so the
	// executable itself goes with the tree. The path is printed the way the
	// layout is documented everywhere else — relative to the root — unless the
	// record points outside it, which must not be described as an in-tree file.
	if in := meta.Install; in != nil {
		where := in.Binary
		switch {
		case where == "":
			where = "a path this record does not name"
		case pathUnder(inst.Root, where):
			where = relOf(inst.Root, where)
		}
		a.warnf("this deletes the copy of %s that golunch downloaded at %s, not a link to a host "+
			"install; your host binary, if you have one, is untouched.\n",
			firstNonEmpty(in.Agent, meta.Instance.Agent), where)
	}
	if !*yes && !a.confirm(fmt.Sprintf("delete instance %q", inst.Alias)) {
		a.warnf("left untouched.\n")
		return nil
	}

	if !*keepLink && meta.Paths.LinkPath != "" {
		if instance.LinkIsOurs(meta.Paths.LinkPath, inst.LauncherPath()) {
			if err := instance.RemoveLink(meta.Paths.LinkPath); err != nil {
				a.warnf("could not remove %s: %v\n", meta.Paths.LinkPath, err)
			}
		} else if _, err := os.Lstat(meta.Paths.LinkPath); err == nil {
			// Somebody else owns that name. Deleting their command because an
			// alias collided would be the worst possible failure here.
			a.warnf("leaving %s in place: it is not the launcher golunch wrote\n", meta.Paths.LinkPath)
		}
	}

	// The lock file sits beside the tree rather than inside it, so it needs its
	// own sweep; otherwise a stale dot-file outlives the instance.
	if err := os.Remove(inst.LockPath()); err != nil && !os.IsNotExist(err) {
		a.warnf("could not remove lock %s: %v\n", inst.LockPath(), err)
	}
	if err := inst.Destroy(); err != nil {
		return err
	}
	a.printf("removed %s\n", inst.Alias)
	return nil
}

// confirm asks on the controlling terminal. It refuses when /dev/tty is
// unavailable so a script cannot delete an instance by answering itself.
func (a *App) confirm(what string) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		a.warnf("%s? [y/N]: refused, no terminal available (pass --yes)\n", what)
		return false
	}
	defer tty.Close()
	fmt.Fprintf(tty, "%s? [y/N]: ", what)
	var answer string
	_, _ = fmt.Fscanln(tty, &answer)
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
