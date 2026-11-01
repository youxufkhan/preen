// Command preen cleans, uninstalls, analyzes and monitors a Linux system.
package main

import (
	"fmt"
	"os"
)

var version = "0.1.2"

const usageText = `Preen: clean, uninstall, analyze and monitor Linux.

usage: preen <command> [flags]

  clean       remove caches, logs, old packages and other regenerable files
  uninstall   remove an app and its leftovers (apt, snap, Flatpak, AppImage)
  optimize    run maintenance tasks and check system health
  analyze     show what uses disk space (preen analyze [path])
  status      live system dashboard (--watch)
  purge       remove project build artifacts (node_modules, target, ...)
  installer   remove downloaded installer files
  history     show what Preen removed
  trash       move paths to the Trash
  version     print version

Every command that removes something accepts --dry-run, --yes and --json.
Run without sudo; Preen asks for your password only for system items.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usageText)
		return
	}
	cmds := map[string]func([]string) error{
		"clean": cmdClean, "uninstall": cmdUninstall, "optimize": cmdOptimize, "analyze": cmdAnalyze, "analyse": cmdAnalyze,
		"status": cmdStatus, "purge": cmdPurge, "installer": cmdInstaller, "history": cmdHistory, "trash": cmdTrash,
	}
	switch name := os.Args[1]; name {
	case "version", "--version":
		fmt.Println("preen", version)
	case "help", "--help", "-h":
		fmt.Print(usageText)
	case "helper":
		os.Exit(helperMain(os.Args[2:]))
	default:
		fn, ok := cmds[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "preen: unknown command %q\n\n%s", name, usageText)
			os.Exit(2)
		}
		if err := fn(os.Args[2:]); err != nil {
			clearProgress()
			emit(Event{Type: "error", Message: err.Error()})
			fmt.Fprintln(os.Stderr, "preen:", err)
			os.Exit(1)
		}
	}
}
