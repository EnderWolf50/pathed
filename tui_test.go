package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeStore keeps PATHs in memory, and only the folders in dirs exist. Machine needs admin,
// and its UAC prompt is always declined.
func fakeStore(user, machine []string, dirs ...string) (store, map[string][]string) {
	saved := map[string][]string{"User": user, "Machine": machine}
	return store{
		read: func(s scope) (pathValue, error) {
			return pathValue{entries: slices.Clone(saved[s.name])}, nil
		},
		write: func(s scope, _, new pathValue) error {
			if s.name == "Machine" {
				return errUACDeclined
			}
			saved[s.name] = slices.Clone(new.entries)
			return nil
		},
		canWrite: func(s scope) bool { return s.name == "User" },
		exists:   func(p string) bool { return slices.Contains(dirs, p) },
	}, saved
}

func press(m model, keys ...string) model {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

func typeText(m model, text string) model {
	for _, r := range text {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(model)
	}
	return m
}

// save presses enter on the review and runs the save it starts, as the program would.
func save(t *testing.T, m model) model {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if !m.saving || cmd == nil {
		t.Fatal("enter on the review did not start saving")
	}
	next, _ = m.Update(cmd())
	return next.(model)
}

func sized(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return next.(model)
}

func TestEditAndSave(t *testing.T) {
	st, saved := fakeStore([]string{`C:\a`, `C:\gone`, `C:\b`, `c:\A\`}, []string{`C:\Windows`}, `C:\a`, `C:\b`, `C:\new`, `c:\A\`)
	m := sized(newModel(st))
	m = press(m, "enter") // into the User table

	// a adds after the cursor (#1), through the input box; other keys go to the box.
	m = press(m, "a")
	if m.input == nil {
		t.Fatal("a did not open the input box")
	}
	m = typeText(m, `C:\new`)
	m = press(m, "enter")
	if got := m.tab().result(); !slices.Equal(got, []string{`C:\a`, `C:\new`, `C:\gone`, `C:\b`, `c:\A\`}) {
		t.Fatalf("after adding: %q", got)
	}
	if e, _, _ := m.current(); e.value != `C:\new` || !e.added() {
		t.Fatalf("the cursor is not on the new entry: %+v", e)
	}

	// J moves it down; c marks the missing folder and the duplicate of #1 for removal.
	m = press(m, "J")
	m = press(m, "c")
	var removed []string
	for _, e := range m.tab().entries {
		if e.removed {
			removed = append(removed, e.value)
		}
	}
	if !slices.Equal(removed, []string{`C:\gone`, `c:\A\`}) {
		t.Fatalf("clean removed %q", removed)
	}
	// d on a removed entry keeps it after all.
	m.table.SetCursor(1) // C:\gone, now above the moved entry
	m = press(m, "d")
	if m.tab().entries[1].removed {
		t.Fatal("d did not keep the entry")
	}

	// s shows the review first; enter there saves.
	m = press(m, "s")
	if !m.reviewing || strings.Contains(strings.Join(saved["User"], ";"), "new") {
		t.Fatal("s did not stop at the review")
	}
	m = save(t, m)
	want := []string{`C:\a`, `C:\gone`, `C:\new`, `C:\b`}
	if !slices.Equal(saved["User"], want) {
		t.Fatalf("saved %q, want %q", saved["User"], want)
	}
	if m.outcome == nil || m.tab().dirty() {
		t.Fatal("no outcome, or the tab still has changes after saving")
	}
	m = press(m, "esc")
	if m.reviewing {
		t.Fatal("esc did not leave the outcome")
	}
}

func TestMachineNeedsAdmin(t *testing.T) {
	st, saved := fakeStore([]string{`C:\a`, `C:\b`}, []string{`C:\Windows`, `C:\old`}, `C:\a`, `C:\b`, `C:\Windows`)
	m := sized(newModel(st))

	// Machine can be changed; only saving it needs admin.
	m = press(m, "j", "enter", "j", "d")
	if !m.tab().entries[1].removed {
		t.Fatalf("the Machine PATH could not be changed (status %q)", m.status)
	}
	// Change User too, then save both: User is written, Machine's declined UAC marks it.
	m = press(m, "q", "k", "enter", "d", "s")
	m = save(t, m)
	if !slices.Equal(saved["User"], []string{`C:\b`}) {
		t.Fatalf("User saved %q", saved["User"])
	}
	machine := m.tabs[1]
	if machine.saveErr == nil || !machine.dirty() || !machine.entries[1].removed {
		t.Fatalf("Machine after a declined UAC: err %v, dirty %v", machine.saveErr, machine.dirty())
	}
	if out := strings.Join(m.outcome, "\n"); !strings.Contains(out, "Machine PATH not saved") || !strings.Contains(out, "User PATH saved") {
		t.Fatalf("outcome:\n%s", out)
	}
	m = press(m, "esc", "q")
	if m.inList {
		t.Fatal("q did not go back to the sidebar")
	}

	// The unsaved Machine change makes quitting ask; only the question's keys count.
	if m.inList {
		t.Fatal("q did not go back to the sidebar")
	}
	m = press(m, "q")
	if !m.confirmQuit {
		t.Fatal("quitting with a change did not ask")
	}
	m = press(m, "j", "s", "enter")
	if !m.confirmQuit || m.reviewing {
		t.Fatal("other keys acted behind the quit dialog")
	}
	m = press(m, "n")
	if m.confirmQuit {
		t.Fatal("n did not close the dialog")
	}
}

func TestFilterBlocksReorder(t *testing.T) {
	st, _ := fakeStore([]string{`C:\a`, `C:\b`, `C:\ab`}, nil, `C:\a`, `C:\b`, `C:\ab`)
	m := sized(newModel(st))
	m = press(m, "enter", "/")
	m = typeText(m, "a")
	m = press(m, "enter")
	if len(m.shown) != 2 {
		t.Fatalf("filter shows %d entries", len(m.shown))
	}
	m = press(m, "J")
	if m.tab().entries[0].value != `C:\a` || !strings.Contains(m.status, "filter") {
		t.Fatal("moved an entry while filtered")
	}
}

// The first frame is drawn before any WindowSizeMsg, and a terminal can be tiny.
func TestViewAtAnySize(t *testing.T) {
	st, _ := fakeStore([]string{`C:\a`, `C:\b`}, []string{`C:\Windows`}, `C:\a`)
	for _, size := range [][2]int{{0, 0}, {10, 3}, {30, 8}, {200, 60}} {
		next, _ := newModel(st).Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m := next.(model)
		m.View()
		m = press(m, "enter", "a")
		m.View()
		m = press(m, "esc", "d", "s")
		m.View()
		m = press(m, "esc", "esc", "q")
		m.View()
	}
}

// A background holds only until the next reset, so the cursor row must paint every run of
// text itself, padding and the panel's full width included.
func TestCursorRowPaintedEdgeToEdge(t *testing.T) {
	token := regexp.MustCompile(`\x1b\[([0-9;]*)m|[^\x1b]+`)
	st, _ := fakeStore([]string{`C:\a`, `C:\missing`}, nil, `C:\a`)
	for _, w := range []int{90, 120, 200} {
		next, _ := newModel(st).Update(tea.WindowSizeMsg{Width: w, Height: 20})
		m := press(next.(model), "enter", "j", "d") // cursor on a removed, missing entry
		for _, line := range strings.Split(m.table.View(), "\n") {
			if !strings.Contains(ansi.Strip(line), `C:\missing`) {
				continue
			}
			active, bare := false, 0
			for _, tok := range token.FindAllStringSubmatch(line, -1) {
				if strings.HasPrefix(tok[0], "\x1b") {
					active = tok[1] != "" && tok[1] != "0" && (active || strings.Contains(tok[1], "48;"))
				} else if !active {
					bare += len(tok[0])
				}
			}
			if bare > 0 {
				t.Errorf("width %d: %d cells of the cursor row have no background", w, bare)
			}
		}
	}
}

func TestConfigMistakesAreErrors(t *testing.T) {
	for text, want := range map[string]string{
		`[theme]` + "\n" + `acent = "#112233"`: "theme.acent",
		`[theme]` + "\n" + `accent = "red"`:    "theme.accent",
		`sidebar_width = 3`:                    "sidebar_width",
	} {
		if _, err := parseConfig(cfg, text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want one naming %s", text, err, want)
		}
	}
	c, err := parseConfig(cfg, "[theme]\nok = \"42\"")
	if err != nil || c.Theme.OK != "42" || c.Theme.Accent != "#ffc799" {
		t.Errorf("override: %+v, %v", c.Theme, err)
	}
}

func TestWriteRequestRoundTrip(t *testing.T) {
	r := writeRequest{Scope: "Machine", Old: []string{`D:\a`}, New: []string{`D:\a`, `%ProgramFiles%\Tool "x"`}, Type: 2}
	arg, err := r.encode()
	if err != nil || strings.ContainsAny(arg, " \"") {
		t.Fatalf("argument %q (%v) would need quoting on a command line", arg, err)
	}
	back, err := decodeRequest(arg)
	if err != nil || back.Scope != r.Scope || !slices.Equal(back.New, r.New) || back.Type != r.Type {
		t.Fatalf("round trip: %+v, %v", back, err)
	}
}
