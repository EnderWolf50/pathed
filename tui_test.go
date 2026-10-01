package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/elevate"
	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// fakeStore keeps PATHs in memory, and only the folders in dirs exist. Machine needs admin,
// and its UAC prompt is always declined.
func fakeStore(user, machine []string, dirs ...string) (winenv.Store, func(string) bool, map[winenv.Scope][]string) {
	saved := map[winenv.Scope][]string{winenv.User: user, winenv.Machine: machine}
	return winenv.Store{
			ReadAll: func(s winenv.Scope) ([]winenv.Var, error) {
				return []winenv.Var{{Name: "Path", Value: winenv.Value{Data: winenv.Join(saved[s]), Type: winenv.ExpandSZ}}}, nil
			},
			Apply: func(changes []winenv.Change) error {
				for _, c := range changes {
					if c.Scope == winenv.Machine {
						return elevate.ErrDeclined
					}
					saved[c.Scope] = winenv.Split(c.New.Data)
				}
				return nil
			},
			CanWrite: func(s winenv.Scope) bool { return s == winenv.User },
		}, func(p string) bool { return slices.Contains(dirs, p) },
		saved
}

func press(m frame.Model, keys ...string) frame.Model {
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
		m = next.(frame.Model)
	}
	return m
}

func typeText(m frame.Model, text string) frame.Model {
	for _, r := range text {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(frame.Model)
	}
	return m
}

// save presses enter on the review and runs the save it starts, as the program would.
func save(t *testing.T, m frame.Model) frame.Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(frame.Model)
	if !m.Saving() || cmd == nil {
		t.Fatal("enter on the review did not start saving")
	}
	next, _ = m.Update(cmd())
	return next.(frame.Model)
}

func sized(m frame.Model) frame.Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return next.(frame.Model)
}

func tab(m frame.Model) *pathTab { return m.Tab().(*pathTab) }

func screen(m frame.Model) string { return ansi.Strip(m.View().Content) }

func TestEditAndSave(t *testing.T) {
	st, exists, saved := fakeStore([]string{`C:\a`, `C:\gone`, `C:\b`, `c:\A\`}, []string{`C:\Windows`}, `C:\a`, `C:\b`, `C:\new`, `c:\A\`)
	m := sized(newModel(st, exists))
	m = press(m, "enter") // into the User table

	// a adds after the cursor (#1), through the input box; other keys go to the box.
	m = press(m, "a")
	if tab(m).Overlay() == "" {
		t.Fatal("a did not open the input box")
	}
	m = typeText(m, `C:\new`)
	m = press(m, "enter")
	if got := tab(m).list.Result(); !slices.Equal(got, []string{`C:\a`, `C:\new`, `C:\gone`, `C:\b`, `c:\A\`}) {
		t.Fatalf("after adding: %q", got)
	}
	if !strings.Contains(screen(m), `#2  C:\new`) || !strings.Contains(screen(m), "added") {
		t.Fatalf("the cursor is not on the new entry:\n%s", screen(m))
	}

	// J moves it down; c marks the missing folder and the duplicate of #1 for removal.
	m = press(m, "J", "c")
	if got := tab(m).list.Result(); !slices.Equal(got, []string{`C:\a`, `C:\new`, `C:\b`}) {
		t.Fatalf("after clean: %q", got)
	}
	// d on a removed entry keeps it after all.
	m = press(m, "k", "d") // C:\gone, now above the moved entry
	if got := tab(m).list.Result(); !slices.Equal(got, []string{`C:\a`, `C:\gone`, `C:\new`, `C:\b`}) {
		t.Fatalf("d did not keep the entry: %q", got)
	}

	// s shows the review first; enter there saves.
	m = press(m, "s")
	if !m.Reviewing() || strings.Contains(strings.Join(saved[winenv.User], ";"), "new") {
		t.Fatal("s did not stop at the review")
	}
	m = save(t, m)
	want := []string{`C:\a`, `C:\gone`, `C:\new`, `C:\b`}
	if !slices.Equal(saved[winenv.User], want) {
		t.Fatalf("saved %q, want %q", saved[winenv.User], want)
	}
	if m.Outcome() == nil || tab(m).Dirty() {
		t.Fatal("no outcome, or the tab still has changes after saving")
	}
	m = press(m, "esc")
	if m.Reviewing() {
		t.Fatal("esc did not leave the outcome")
	}
}

func TestMachineNeedsAdmin(t *testing.T) {
	st, exists, saved := fakeStore([]string{`C:\a`, `C:\b`}, []string{`C:\Windows`, `C:\old`}, `C:\a`, `C:\b`, `C:\Windows`)
	m := sized(newModel(st, exists))

	// Machine can be changed; only saving it needs admin.
	m = press(m, "j", "enter", "j", "d")
	machine := tab(m)
	if got := machine.list.Result(); !slices.Equal(got, []string{`C:\Windows`}) {
		t.Fatalf("the Machine PATH could not be changed (status %q)", m.Status())
	}
	// Change User too, then save both: User is written, Machine's declined UAC marks it.
	m = press(m, "q", "k", "enter", "d", "s")
	m = save(t, m)
	if !slices.Equal(saved[winenv.User], []string{`C:\b`}) {
		t.Fatalf("User saved %q", saved[winenv.User])
	}
	if m.SaveErr(1) == nil || !machine.Dirty() {
		t.Fatalf("Machine after a declined UAC: err %v", m.SaveErr(1))
	}
	if out := strings.Join(m.Outcome(), "\n"); !strings.Contains(out, "Machine PATH not saved") || !strings.Contains(out, "User PATH saved") {
		t.Fatalf("outcome:\n%s", out)
	}
	m = press(m, "esc", "q")
	if m.InBody() {
		t.Fatal("q did not go back to the sidebar")
	}

	// The unsaved Machine change makes quitting ask; only the question's keys count.
	m = press(m, "q")
	if !m.ConfirmingQuit() {
		t.Fatal("quitting with a change did not ask")
	}
	m = press(m, "j", "s", "enter")
	if !m.ConfirmingQuit() || m.Reviewing() {
		t.Fatal("other keys acted behind the quit dialog")
	}
	m = press(m, "n")
	if m.ConfirmingQuit() {
		t.Fatal("n did not close the dialog")
	}
}

func TestFilterBlocksReorder(t *testing.T) {
	st, exists, _ := fakeStore([]string{`C:\a`, `C:\b`, `C:\ab`}, nil, `C:\a`, `C:\b`, `C:\ab`)
	m := sized(newModel(st, exists))
	m = press(m, "enter", "/")
	m = typeText(m, "a")
	m = press(m, "enter")
	if s := screen(m); !strings.Contains(s, `C:\ab`) || strings.Contains(s, `C:\b `) {
		t.Fatalf("the filter does not narrow:\n%s", s)
	}
	m = press(m, "J")
	if tab(m).list.Result()[0] != `C:\a` || !strings.Contains(m.Status(), "filter") {
		t.Fatal("moved an entry while filtered")
	}
}

// The first frame is drawn before any WindowSizeMsg, and a terminal can be tiny.
func TestViewAtAnySize(t *testing.T) {
	st, exists, _ := fakeStore([]string{`C:\a`, `C:\b`}, []string{`C:\Windows`}, `C:\a`)
	for _, size := range [][2]int{{0, 0}, {10, 3}, {30, 8}, {200, 60}} {
		next, _ := newModel(st, exists).Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m := next.(frame.Model)
		m.View()
		m = press(m, "enter", "a")
		m.View()
		m = press(m, "esc", "d", "s")
		m.View()
		m = press(m, "esc", "esc", "q")
		m.View()
	}
}

func TestConfigMistakesAreErrors(t *testing.T) {
	for text, want := range map[string]string{
		`[theme]` + "\n" + `acent = "#112233"`: "theme.acent",
		`[theme]` + "\n" + `accent = "red"`:    "theme.accent",
		`sidebar_width = 3`:                    "sidebar_width",
	} {
		if _, err := theme.Parse(cfg, text, checkConfig); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want one naming %s", text, err, want)
		}
	}
	c, err := theme.Parse(cfg, "[theme]\nok = \"42\"", checkConfig)
	if err != nil || c.Theme.OK != "42" || c.Theme.Accent != "#ffc799" {
		t.Errorf("override: %+v, %v", c.Theme, err)
	}
}
