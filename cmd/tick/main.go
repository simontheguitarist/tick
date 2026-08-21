// Command tick is a tiny CLI for jotting down the next open steps in your
// projects and crossing them off. Run it with no args for the interactive TUI,
// or use the subcommands for quick scriptable edits.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/simontheguitarist/tick/internal/cli"
	tsync "github.com/simontheguitarist/tick/internal/sync"
	"github.com/simontheguitarist/tick/internal/tui"
)

var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		// Bare `tick`: tracked dir -> its steps; untracked -> global overview.
		if err := tui.RunAuto(); err != nil {
			return fail(err)
		}
		return 0
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "ui", "menu":
		if err := tui.RunOverview(); err != nil {
			return fail(err)
		}

	case "add":
		rest, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.Add(path, name, strings.Join(rest, " ")); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "ls", "list":
		tsync.Auto()
		all, rest := popFlag(rest, "--all")
		asJSON, rest := popFlag(rest, "--json")
		_, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.List(path, name, all, asJSON); err != nil {
			return fail(err)
		}

	case "done":
		rest, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		nums, err := cli.ParseNums(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.Done(path, name, nums); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "undone", "reopen":
		rest, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		nums, err := cli.ParseNums(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.Undone(path, name, nums); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "rm", "remove":
		rest, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		nums, err := cli.ParseNums(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.Remove(path, name, nums); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "edit":
		rest, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		if len(rest) < 2 {
			return fail(fmt.Errorf("usage: tick edit <n> \"new text\""))
		}
		nums, err := cli.ParseNums(rest[:1])
		if err != nil {
			return fail(err)
		}
		if err := cli.Edit(path, name, nums[0], strings.Join(rest[1:], " ")); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "projects":
		tsync.Auto()
		asJSON, _ := popFlag(rest, "--json")
		path, _, err := cli.CurrentProject()
		if err != nil {
			return fail(err)
		}
		if err := cli.Projects(path, asJSON); err != nil {
			return fail(err)
		}

	case "untrack":
		yes, rest := popFlag(rest, "-y")
		_, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.Untrack(path, name, yes); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "clear":
		allProjects, rest := popFlag(rest, "--all-projects")
		_, path, name, err := cli.ResolveProject(rest)
		if err != nil {
			return fail(err)
		}
		if err := cli.Clear(path, name, allProjects); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "link":
		arg := ""
		if len(rest) > 0 {
			arg = rest[0]
		}
		if err := cli.Link(arg); err != nil {
			return fail(err)
		}
		tsync.Auto()

	case "sync":
		if err := runSync(rest); err != nil {
			return fail(err)
		}

	case "version", "--version", "-v":
		fmt.Printf("tick %s\n", version)

	case "help", "-h", "--help":
		printHelp()

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q — try `tick help`\n", cmd)
		return 2
	}
	return 0
}

// runSync dispatches the `tick sync` subcommands.
func runSync(args []string) error {
	full, args := popFlag(args, "--full")
	if len(args) == 0 {
		return cli.SyncRun(full)
	}
	switch args[0] {
	case "setup":
		if len(args) < 2 {
			return fmt.Errorf("usage: tick sync setup <url>")
		}
		return cli.SyncSetup(args[1])
	case "qr":
		return cli.SyncQR()
	case "status":
		return cli.SyncStatus()
	case "off":
		return cli.SyncOff()
	default:
		return fmt.Errorf("unknown sync command %q — try sync, sync setup <url>, sync qr, sync status, sync off", args[0])
	}
}

// popFlag removes a boolean flag from args, reporting whether it was present.
func popFlag(args []string, flag string) (bool, []string) {
	out := args[:0:0]
	found := false
	for _, a := range args {
		if a == flag {
			found = true
			continue
		}
		out = append(out, a)
	}
	return found, out
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "tick:", err)
	return 1
}

func printHelp() {
	fmt.Print(`tick — jot down next steps and cross them off

Usage:
  tick                       interactive TUI (current project, or overview if untracked)
  tick ui | menu             open the global overview directly

  tick add "text"            add a step to the current project
  tick ls [--all] [--json]   list steps (open only unless --all)
  tick done <n>...           cross off step(s)
  tick undone <n>...         reopen step(s)
  tick rm <n>...             delete step(s)
  tick edit <n> "text"       change a step's text
  tick clear [--all-projects] drop done steps
  tick projects [--json]     list all tracked projects
  tick untrack [-y]          stop tracking the current project
  tick link [name]           attach a project synced from another device to this dir
  tick version               print version

  tick sync                  sync now with the server (--full re-syncs everything)
  tick sync setup <url>      connect this machine (prompts for the token)
  tick sync qr               show the QR code that pairs the iPhone app
  tick sync status           connection, last sync, queued changes
  tick sync off              disconnect (local steps stay)

Most commands accept -p <path> to target another project. When sync is set up,
every command syncs automatically (1s budget, silent offline; TICK_NO_SYNC=1
skips it).
`)
}
