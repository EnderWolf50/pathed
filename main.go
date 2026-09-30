// pathed views and edits the persistent Windows PATH (User and Machine) from a TUI or the
// command line. It writes the registry directly, keeping REG_EXPAND_SZ values and their
// %VARS% intact (.NET's SetEnvironmentVariable rewrites them as REG_SZ), saves the old value
// before each write and tells running programs that the environment changed.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const usage = `pathed - edit the persistent PATH

  pathed                   interactive editor (User and Machine)
  pathed list [-m]         numbered entries; missing folders and duplicates are marked
  pathed add <dir> [-m] [--front]
  pathed rm <dir|N> [-m]   N is the number shown by 'pathed list'
  pathed clean [-m]        drop duplicates and folders that do not exist
  pathed --version         print the version

-m works on the Machine PATH, which needs admin: 'gsudo pathed ...'.
Every write first saves the old value to %LOCALAPPDATA%\pathed\.`

type scope struct {
	name string
	root registry.Key
	key  string
}

var scopes = []scope{
	{"User", registry.CURRENT_USER, `Environment`},
	{"Machine", registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
}

// pathValue is one scope's PATH as stored: the entries unexpanded, plus the registry type.
type pathValue struct {
	entries []string
	typ     uint32
}

func read(s scope) (pathValue, error) {
	k, err := registry.OpenKey(s.root, s.key, registry.QUERY_VALUE)
	if err != nil {
		return pathValue{}, err
	}
	defer k.Close()
	v, typ, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return pathValue{typ: registry.EXPAND_SZ}, nil
	}
	if err != nil {
		return pathValue{}, err
	}
	return pathValue{split(v), typ}, nil
}

func write(s scope, old, new pathValue) error {
	k, err := registry.OpenKey(s.root, s.key, registry.SET_VALUE)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("the %s PATH needs admin: run it through gsudo", s.name)
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := backup(s, old); err != nil {
		return fmt.Errorf("backup failed, PATH not changed: %w", err)
	}
	v := strings.Join(new.entries, ";")
	if new.typ == registry.EXPAND_SZ || strings.Contains(v, "%") {
		err = k.SetExpandStringValue("Path", v)
	} else {
		err = k.SetStringValue("Path", v)
	}
	if err != nil {
		return err
	}
	broadcast()
	return nil
}

// ponytail: backups are never pruned; they are a few KB each.
func backup(s scope, v pathValue) error {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "pathed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%s.txt", s.name, time.Now().Format("20060102-150405.000"))
	return os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(v.entries, ";")), 0o644)
}

// broadcast sends WM_SETTINGCHANGE("Environment") so Explorer, and whatever it starts next,
// sees the new PATH without signing out.
func broadcast() {
	env, _ := windows.UTF16PtrFromString("Environment")
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, 0)
}

func split(v string) []string {
	var out []string
	for _, e := range strings.Split(v, ";") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func expand(e string) string {
	if x, err := registry.ExpandString(e); err == nil {
		return x
	}
	return e
}

// key is what two entries are compared by: expanded, case-insensitive, no trailing slash.
func key(e string) string {
	return strings.ToLower(strings.TrimRight(expand(e), `\/`))
}

func exists(e string) bool {
	fi, err := os.Stat(expand(e))
	return err == nil && fi.IsDir()
}

// status reports, per entry, whether it repeats an earlier one and whether its folder exists.
func status(entries []string, exists func(string) bool) (dup, missing []bool) {
	seen := map[string]bool{}
	dup, missing = make([]bool, len(entries)), make([]bool, len(entries))
	for i, e := range entries {
		k := key(e)
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
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pathed:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	s, front := scopes[0], false
	var rest []string
	for _, a := range args {
		switch a {
		case "-m", "--machine":
			s = scopes[1]
		case "--front":
			front = true
		case "-h", "--help", "help":
			fmt.Println(usage)
			return nil
		case "-v", "--version":
			fmt.Println("pathed", versionString())
			return nil
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		_, err := tea.NewProgram(newModel()).Run()
		return err
	}

	v, err := read(s)
	if err != nil {
		return err
	}
	switch cmd := rest[0]; {
	case cmd == "list" || cmd == "ls":
		dup, missing := status(v.entries, exists)
		for i, e := range v.entries {
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
		for _, e := range v.entries {
			if key(e) == key(dir) {
				return fmt.Errorf("%s is already in the %s PATH", dir, s.name)
			}
		}
		if !exists(dir) {
			return fmt.Errorf("%s is not a folder", dir)
		}
		n := pathValue{append(v.entries[:len(v.entries):len(v.entries)], dir), v.typ}
		if front {
			n.entries = append([]string{dir}, v.entries...)
		}
		return report(s, v, n, "added "+dir)
	case (cmd == "rm" || cmd == "remove") && len(rest) == 2:
		n := pathValue{typ: v.typ}
		if i, err := strconv.Atoi(rest[1]); err == nil {
			if i < 1 || i > len(v.entries) {
				return fmt.Errorf("no entry %d in the %s PATH (1-%d)", i, s.name, len(v.entries))
			}
			n.entries = append(append(n.entries, v.entries[:i-1]...), v.entries[i:]...)
		} else {
			abs, _ := filepath.Abs(rest[1])
			for _, e := range v.entries {
				if key(e) != key(rest[1]) && key(e) != key(abs) {
					n.entries = append(n.entries, e)
				}
			}
		}
		if len(n.entries) == len(v.entries) {
			return fmt.Errorf("%s is not in the %s PATH", rest[1], s.name)
		}
		return report(s, v, n, fmt.Sprintf("removed %d entry", len(v.entries)-len(n.entries)))
	case cmd == "clean":
		n := pathValue{clean(v.entries, exists), v.typ}
		if len(n.entries) == len(v.entries) {
			fmt.Println("nothing to clean")
			return nil
		}
		return report(s, v, n, fmt.Sprintf("removed %d entries", len(v.entries)-len(n.entries)))
	}
	return errors.New("unknown command\n\n" + usage)
}

func report(s scope, old, new pathValue, what string) error {
	if err := write(s, old, new); err != nil {
		return err
	}
	fmt.Printf("%s PATH: %s.\n", s.name, what)
	return nil
}
