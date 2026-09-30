package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	styleTab     = lipgloss.NewStyle().Padding(0, 1)
	styleTabOn   = styleTab.Reverse(true).Bold(true)
	styleCursor  = lipgloss.NewStyle().Reverse(true)
	styleMissing = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8080"))
	styleDup     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffc799"))
	styleHelp    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b8b8b"))
)

type tab struct {
	saved, cur pathValue
	err        error
}

func (t tab) dirty() bool {
	return strings.Join(t.saved.entries, ";") != strings.Join(t.cur.entries, ";")
}

type model struct {
	tabs        []tab
	on, cursor  int
	top, height int
	input       textinput.Model
	editing     int // -1 when not editing; len(entries) means adding
	msg         string
	confirmQuit bool
}

func newModel() model {
	m := model{editing: -1, height: 20, input: textinput.New()}
	for _, s := range scopes {
		v, err := read(s)
		m.tabs = append(m.tabs, tab{saved: v, cur: pathValue{append([]string(nil), v.entries...), v.typ}, err: err})
	}
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) entries() []string { return m.tabs[m.on].cur.entries }

func (m *model) setEntries(e []string) { m.tabs[m.on].cur.entries = e }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = max(msg.Height-7, 3)
	case tea.KeyPressMsg:
		if m.editing >= 0 {
			m, cmd = m.updateInput(msg)
		} else {
			m, cmd = m.updateList(msg)
		}
	}
	// Scroll so the cursor stays inside the visible window.
	m.top = min(m.top, m.cursor)
	if m.cursor >= m.top+m.height {
		m.top = m.cursor - m.height + 1
	}
	return m, cmd
}

func (m model) updateInput(msg tea.KeyPressMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = -1
		return m, nil
	case "enter":
		v := strings.TrimSpace(m.input.Value())
		e := append([]string(nil), m.entries()...)
		if v != "" && !strings.Contains(v, "%") {
			if abs, err := filepath.Abs(v); err == nil {
				v = abs
			}
		}
		switch {
		case v == "":
		case m.editing == len(e):
			e = append(e[:m.cursor+min(1, len(e))], append([]string{v}, e[m.cursor+min(1, len(e)):]...)...)
			m.cursor = min(m.cursor+1, len(e)-1)
		default:
			e[m.editing] = v
		}
		m.setEntries(e)
		m.editing = -1
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyPressMsg) (model, tea.Cmd) {
	k := msg.String()
	if k != "q" && k != "esc" && k != "ctrl+c" {
		m.confirmQuit = false
	}
	m.msg = ""
	e := m.entries()
	switch k {
	case "q", "esc", "ctrl+c":
		if k != "ctrl+c" && !m.confirmQuit && (m.tabs[0].dirty() || m.tabs[1].dirty()) {
			m.confirmQuit = true
			m.msg = "unsaved changes: s saves, q again discards them"
			return m, nil
		}
		return m, tea.Quit
	case "tab", "left", "right", "h", "l":
		m.on, m.cursor, m.top = 1-m.on, 0, 0
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, max(len(e)-1, 0))
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(len(e)-1, 0)
	case "shift+up", "K":
		if m.cursor > 0 {
			e = append([]string(nil), e...)
			e[m.cursor-1], e[m.cursor] = e[m.cursor], e[m.cursor-1]
			m.setEntries(e)
			m.cursor--
		}
	case "shift+down", "J":
		if m.cursor < len(e)-1 {
			e = append([]string(nil), e...)
			e[m.cursor+1], e[m.cursor] = e[m.cursor], e[m.cursor+1]
			m.setEntries(e)
			m.cursor++
		}
	case "d", "x", "delete":
		if len(e) > 0 {
			m.setEntries(append(append([]string(nil), e[:m.cursor]...), e[m.cursor+1:]...))
			m.cursor = min(m.cursor, max(len(e)-2, 0))
		}
	case "a", "o":
		m.editing = len(e)
		m.input.SetValue("")
		return m, m.input.Focus()
	case "e", "enter":
		if len(e) > 0 {
			m.editing = m.cursor
			m.input.SetValue(e[m.cursor])
			m.input.CursorEnd()
			return m, m.input.Focus()
		}
	case "c":
		n := clean(e, exists)
		m.msg = fmt.Sprintf("clean: %d entries marked for removal (s saves)", len(e)-len(n))
		m.setEntries(n)
		m.cursor = min(m.cursor, max(len(n)-1, 0))
	case "u":
		t := &m.tabs[m.on]
		t.cur.entries = append([]string(nil), t.saved.entries...)
		m.cursor = min(m.cursor, max(len(t.cur.entries)-1, 0))
		m.msg = "reverted to the saved value"
	case "s":
		t := &m.tabs[m.on]
		if !t.dirty() {
			m.msg = "nothing to save"
		} else if err := write(scopes[m.on], t.saved, t.cur); err != nil {
			m.msg = "not saved: " + err.Error()
		} else {
			t.saved.entries = append([]string(nil), t.cur.entries...)
			m.msg = scopes[m.on].name + " PATH saved"
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	var b strings.Builder
	for i, s := range scopes {
		label := s.name
		if m.tabs[i].dirty() {
			label += " *"
		}
		if i == m.on {
			b.WriteString(styleTabOn.Render(label))
		} else {
			b.WriteString(styleTab.Render(label))
		}
	}
	b.WriteString("\n\n")

	t := m.tabs[m.on]
	e := t.cur.entries
	if t.err != nil {
		b.WriteString(styleMissing.Render("cannot read: "+t.err.Error()) + "\n")
	}
	dup, missing := status(e, exists)
	for i := m.top; i < len(e) && i < m.top+m.height; i++ {
		line := fmt.Sprintf("%3d  %s", i+1, e[i])
		if i == m.editing {
			line = fmt.Sprintf("%3d  %s", i+1, m.input.View())
		}
		switch {
		case missing[i]:
			line = styleMissing.Render(line + "  missing")
		case dup[i]:
			line = styleDup.Render(line + "  duplicate")
		}
		if i == m.cursor && m.editing < 0 {
			line = styleCursor.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if m.editing == len(e) {
		b.WriteString("new  " + m.input.View() + "\n")
	}

	b.WriteString("\n")
	if m.msg != "" {
		b.WriteString(m.msg + "\n")
	}
	if m.editing >= 0 {
		b.WriteString(styleHelp.Render("enter keep · esc cancel"))
	} else {
		b.WriteString(styleHelp.Render("↑↓ move · K/J reorder · a add · e edit · d delete · c clean · u revert · s save · tab User/Machine · q quit"))
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}
