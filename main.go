// pathed views and edits the persistent Windows PATH (User and Machine) from a TUI or the
// command line. It is enved's list editor on one variable: the registry, UAC and the screen
// come from github.com/EnderWolf50/enved, which writes REG_EXPAND_SZ values with their
// %VARS% intact, saves the old value before each write and tells running programs that the
// environment changed.
package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/elevate"
	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

const usage = `pathed - edit the persistent PATH

  pathed                   interactive editor (User and Machine)
  pathed list [-m]         numbered entries; missing folders and duplicates are marked
  pathed add <dir> [-m] [--front]
  pathed rm <dir|N> [-m]   N is the number shown by 'pathed list'
  pathed clean [-m]        drop duplicates and folders that do not exist
  pathed init pwsh         print a pathed function that also updates this shell's PATH;
                           in $PROFILE: pathed.exe init pwsh | Out-String | Invoke-Expression
  pathed --version         print the version
  pathed --config          show where the settings file is
  pathed --default-config  print the default settings, a starting point for your own

-m works on the Machine PATH; saving it asks for admin (UAC) unless pathed already runs
elevated.
Every write first saves the old value to %LOCALAPPDATA%\enved\.
Settings (theme, sidebar width) are read from ~/.config/pathed/config.toml, or the file
named by $PATHED_CONFIG.
For the other environment variables, see enved: https://github.com/EnderWolf50/enved`

// pwshInit is the PowerShell wrapper 'pathed init pwsh' prints: pathed.exe cannot change
// the PATH of the shell that runs it, so a function in that shell has to.
//
//go:embed init.ps1
var pwshInit string

// shellInit returns the wrapper for a shell.
func shellInit(shell string) (string, error) {
	switch strings.ToLower(shell) {
	case "pwsh", "powershell":
		return pwshInit, nil
	}
	return "", fmt.Errorf("no init for %q; the shells are: pwsh (or powershell)", shell)
}

// exists is whether a PATH entry is a folder.
var exists = listedit.Exists(listedit.Folders)

// pathKey is what two entries are compared by: expanded, case-insensitive, no trailing slash.
func pathKey(e string) string {
	return strings.ToLower(strings.TrimRight(winenv.Expand(e), `\/`))
}

// status reports, per entry, whether it repeats an earlier one and whether its folder exists.
func status(entries []string, exists func(string) bool) (dup, missing []bool) {
	seen := map[string]bool{}
	dup, missing = make([]bool, len(entries)), make([]bool, len(entries))
	for i, e := range entries {
		k := pathKey(e)
		dup[i], missing[i] = seen[k], !exists(e)
		seen[k] = true
	}
	return dup, missing
}

func clean(entries []string, exists func(string) bool) []string {
	dup, missing := status(entries, exists)
	var out []string
	for i, e := range entries {
		if !dup[i] && !missing[i] {
			out = append(out, e)
		}
	}
	return out
}

func main() {
	if handled, code := elevate.HandleArgs(os.Args[1:]); handled {
		os.Exit(code)
	}
	if err := run(os.Args[1:], elevate.Registry); err != nil {
		fmt.Fprintln(os.Stderr, "pathed:", err)
		os.Exit(1)
	}
}

func run(args []string, st winenv.Store) error {
	s, front := winenv.User, false
	var rest []string
	for _, a := range args {
		switch a {
		case "-m", "--machine":
			s = winenv.Machine
		case "--front":
			front = true
		case "-h", "--help", "help":
			fmt.Println(usage)
			return nil
		case "-v", "--version":
			fmt.Println("pathed", versionString())
			return nil
		case "--default-config":
			fmt.Print(defaultConfig)
			return nil
		case "--config":
			path := theme.Path("pathed")
			if _, err := os.Stat(path); err != nil {
				path += "  (not there yet: pathed --default-config > it, then edit)"
			}
			fmt.Println(path)
			return nil
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		c, err := theme.Load(theme.Path("pathed"), cfg, checkConfig)
		if err != nil {
			return err
		}
		cfg = c
		theme.Apply(cfg.Theme)
		_, err = tea.NewProgram(newModel(st, exists)).Run()
		return err
	}
	if rest[0] == "init" {
		if len(rest) != 2 {
			return errors.New("init needs a shell: pathed init pwsh")
		}
		script, err := shellInit(rest[1])
		if err != nil {
			return err
		}
		fmt.Print(script)
		return nil
	}

	v, err := readPath(st, s)
	if err != nil {
		return err
	}
	var entries []string
	if v != nil {
		entries = winenv.Split(v.Data)
	}
	switch cmd := rest[0]; {
	case cmd == "list" || cmd == "ls":
		dup, missing := status(entries, exists)
		for i, e := range entries {
			note := ""
			if missing[i] {
				note += "  [missing]"
			}
			if dup[i] {
				note += "  [duplicate]"
			}
			fmt.Printf("%3d  %s%s\n", i+1, e, note)
		}
		return nil
	case cmd == "add" && len(rest) == 2:
		dir, err := filepath.Abs(rest[1])
		if err != nil {
			return err
		}
		for _, e := range entries {
			if pathKey(e) == pathKey(dir) {
				return fmt.Errorf("%s is already in the %s PATH", dir, s)
			}
		}
		if !exists(dir) {
			return fmt.Errorf("%s is not a folder", dir)
		}
		n := append(entries[:len(entries):len(entries)], dir)
		if front {
			n = append([]string{dir}, entries...)
		}
		return report(st, s, v, n, "added "+dir)
	case (cmd == "rm" || cmd == "remove") && len(rest) == 2:
		var n []string
		if i, err := strconv.Atoi(rest[1]); err == nil {
			if i < 1 || i > len(entries) {
				return fmt.Errorf("no entry %d in the %s PATH (1-%d)", i, s, len(entries))
			}
			n = append(append(n, entries[:i-1]...), entries[i:]...)
		} else {
			abs, _ := filepath.Abs(rest[1])
			for _, e := range entries {
				if pathKey(e) != pathKey(rest[1]) && pathKey(e) != pathKey(abs) {
					n = append(n, e)
				}
			}
		}
		if len(n) == len(entries) {
			return fmt.Errorf("%s is not in the %s PATH", rest[1], s)
		}
		return report(st, s, v, n, fmt.Sprintf("removed %d entry", len(entries)-len(n)))
	case cmd == "clean":
		n := clean(entries, exists)
		if len(n) == len(entries) {
			fmt.Println("nothing to clean")
			return nil
		}
		return report(st, s, v, n, fmt.Sprintf("removed %d entries", len(entries)-len(n)))
	}
	return errors.New("unknown command\n\n" + usage)
}

func report(st winenv.Store, s winenv.Scope, old *winenv.Var, entries []string, what string) error {
	if err := st.Apply([]winenv.Change{pathChange(s, old, entries)}); err != nil {
		return err
	}
	fmt.Printf("%s PATH: %s.\n", s, what)
	return nil
}

// newModel is the editor: one tab per scope's PATH.
func newModel(st winenv.Store, exists func(string) bool) frame.Model {
	var tabs []frame.Tab
	for _, s := range winenv.Scopes {
		tabs = append(tabs, newPathTab(s, st, exists))
	}
	opts := frame.Options{
		SidebarWidth: cfg.SidebarWidth,
		Apply:        st.Apply,
		AfterSave:    "New terminals see the change; with the pwsh wrapper, this one does too once pathed exits.",
	}
	return frame.New(opts, tabs, 0, false)
}
