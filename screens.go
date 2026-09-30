package main

// Drawing the list, and the boxes and screens that take over from it: the entry input, the
// quit question and the review of the changes before they are saved.

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// changes counts what differs in a tab from its last save.
func (t *tab) changes() (added, edited, removed int) {
	for _, e := range t.entries {
		switch {
		case e.removed:
			removed++
		case e.added():
			added++
		case e.edited():
			edited++
		}
	}
	return
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ---- the list ----------------------------------------------------------------------------

func (m model) viewList() string {
	var side []string
	for i, t := range m.tabs {
		count := fmt.Sprint(len(t.result()))
		switch {
		case t.err != nil:
			count = styleErr.Render("!")
		case t.saveErr != nil:
			count = styleErr.Render("!") + " " + count
		case t.needsAdmin:
			count += " uac"
		}
		if t.dirty() {
			count += " *"
		}
		// The panel's border and padding take 4 cells, the "▌ " mark 2 and the name 10.
		label := fmt.Sprintf("%-9s %s", t.scope.name, lipgloss.PlaceHorizontal(sideWidth()-16, lipgloss.Right, count))
		if i == m.on {
			side = append(side, styleAccent.Render("▌ "+label))
		} else {
			side = append(side, styleDim.Render("  "+label))
		}
	}

	sideStyle, listStyle := stylePanel, stylePanel.BorderForeground(colorAccent)
	if !m.inList {
		sideStyle, listStyle = listStyle, sideStyle
	}
	// While it has focus, the sidebar keeps its keys at the bottom. (The filler string adds
	// one line more than its newlines.)
	var foot []string
	if !m.inList {
		foot = []string{"↑/k      up", "↓/j      down", "→/enter  open", "r        reload", "s        save", "esc/q    quit"}
	}
	if len(foot) > 0 {
		inner := m.h - sideStyle.GetVerticalFrameSize()
		side = append(side, strings.Repeat("\n", max(inner-len(side)-len(foot)-1, 0)))
		side = append(side, styleDim.Render(strings.Join(foot, "\n")))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		sideStyle.Width(sideWidth()).Height(m.h).Render(strings.Join(side, "\n")),
		listStyle.Width(m.w-sideWidth()).Height(m.h).Render(m.viewPanel()))
}

// viewPanel is the right panel: heading, filter, the table, the entry under the cursor, and
// the keys.
func (m model) viewPanel() string {
	t := m.tab()
	// Never below zero, even in a terminal narrower than the sidebar.
	width := max(m.w-sideWidth()-stylePanel.GetHorizontalFrameSize(), 0)
	missing, dupOf := t.health(m.st.exists)
	nMissing, nDup := 0, 0
	for i := range t.entries {
		if dupOf[i] > 0 {
			nDup++
		} else if missing[i] {
			nMissing++
		}
	}
	heading := styleAccent.Render(t.scope.name+" PATH") + styleDim.Render(" · "+plural(len(t.result()), "entry", "entries"))
	if nMissing > 0 {
		heading += styleErr.Render(" · " + fmt.Sprint(nMissing) + " missing")
	}
	if nDup > 0 {
		heading += styleWarn.Render(" · " + plural(nDup, "duplicate", "duplicates"))
	}
	if a, e, r := t.changes(); a+e+r > 0 || t.reordered() {
		heading += styleDim.Render(" · unsaved changes")
	}
	switch {
	case t.err != nil:
		heading += styleErr.Render(" · could not be read: " + t.err.Error())
	case t.saveErr != nil:
		heading += styleErr.Render(" · not saved: " + t.saveErr.Error())
	case t.needsAdmin:
		heading += styleDim.Render(" · saving asks for admin (UAC)")
	}
	if m.status != "" {
		heading += "  " + styleErr.Render(m.status)
	}

	filter := ""
	if m.filter.Focused() || m.filter.Value() != "" {
		filter = m.filter.View()
	}

	// The entry under the cursor: where it really points and whether that exists; then how
	// it changed.
	detail := make([]string, 2)
	if e, i, ok := m.current(); ok {
		where := styleAccent.Render(fmt.Sprintf("#%d", i+1)) + "  " + e.value
		if x := expand(e.value); x != e.value {
			where += styleDim.Render("  →  " + x)
		}
		switch {
		case e.removed:
		case dupOf[i] > 0:
			where += styleWarn.Render(fmt.Sprintf("  · same folder as #%d", dupOf[i]))
		case missing[i]:
			where += styleErr.Render("  · no such folder")
		default:
			where += styleOK.Render("  · folder exists") + styleDim.Render(" · o opens it")
		}
		detail[0] = where
		switch {
		case e.removed:
			detail[1] = styleErr.Render("removed on save") + styleDim.Render(" · d keeps it")
		case e.added():
			detail[1] = styleOK.Render("added")
		case e.edited():
			detail[1] = styleWarn.Render("edited") + styleDim.Render(" · was  "+e.orig)
		}
	}
	for i := range detail {
		detail[i] = ansi.Truncate(detail[i], width, "…")
	}

	body := m.table.View()
	if len(m.shown) == 0 {
		note := "this PATH is empty · a adds an entry"
		if m.filter.Value() != "" {
			note = "nothing matches the filter · esc clears it"
		}
		body = lipgloss.Place(width, lipgloss.Height(body), lipgloss.Center, lipgloss.Center, styleDim.Render(note))
	}

	keys := []key.Binding{keyUp, keyDown, keyEdit, keyAdd, keyRemove, keyMoveUp, keySave, keyBack, keyClean, keyUndo, keyFilter, keyOpen, keyReload}
	if m.filter.Focused() {
		keys = []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep filter")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		}
	}
	return strings.Join([]string{
		ansi.Truncate(heading, width, "…"),
		filter,
		body, "",
		styleFaint.Render(strings.Repeat("─", width)),
		strings.Join(detail, "\n"),
		ansi.Truncate(m.help.ShortHelpView(keys), width, "…"),
	}, "\n")
}

// ---- adding and editing -------------------------------------------------------------------

// inputBox asks for a folder, over the list, checking it as it is typed.
type inputBox struct {
	field  textinput.Model
	at     int // the entry being edited, or the one a new entry goes after (-1: first)
	adding bool
}

func (m model) openInput(at int, adding bool) (tea.Model, tea.Cmd) {
	f := textinput.New()
	f.Prompt = "› "
	f.Placeholder = `C:\Tools\bin  or  %LOCALAPPDATA%\Programs\thing`
	f.SetWidth(min(70, max(m.w-16, 20)))
	if !adding {
		f.SetValue(m.tab().entries[at].value)
		f.CursorEnd()
	}
	m.input = &inputBox{field: f, at: at, adding: adding}
	return m, m.input.field.Focus()
}

// normalize makes a typed folder absolute, unless it leans on %VARS% (kept as typed, so
// they expand wherever the PATH is read).
func normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.Contains(v, "%") {
		return v
	}
	if abs, err := filepath.Abs(v); err == nil {
		return abs
	}
	return v
}

func (b *inputBox) view(t *tab, exists func(string) bool) string {
	title := "Add to the " + t.scope.name + " PATH"
	if !b.adding {
		title = fmt.Sprintf("Edit #%d of the %s PATH", b.at+1, t.scope.name)
	}
	v := normalize(b.field.Value())
	check := styleDim.Render("type a folder")
	if v != "" {
		check = styleErr.Render("no such folder (it can still be added)")
		if exists(v) {
			check = styleOK.Render("folder exists")
		}
		for i, e := range t.entries {
			if !e.removed && (b.adding || i != b.at) && pathKey(e.value) == pathKey(v) {
				check = styleWarn.Render(fmt.Sprintf("already in the PATH as #%d", i+1))
				break
			}
		}
		if x := expand(v); x != v {
			check = styleDim.Render("→ "+x) + "\n" + check
		}
	}
	return styleDialog.Render(styleAccent.Render(title) + "\n\n" + b.field.View() + "\n\n" + check +
		"\n\n" + styleDim.Render("enter keep · esc cancel"))
}

// The input box: only its own keys count while it is open.
func (m model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b, t := m.input, m.tab()
	switch msg.String() {
	case "esc":
		m.input = nil
		return m, nil
	case "enter":
		m.input = nil
		v := normalize(b.field.Value())
		if v == "" {
			return m, nil
		}
		at := b.at
		if b.adding {
			at = b.at + 1
			t.entries = slices.Insert(t.entries, at, &entry{value: v})
		} else {
			t.entries[at].value = v
		}
		// Clear the filter if it would hide what was just typed, then put the cursor on it.
		if !strings.Contains(strings.ToLower(v), strings.ToLower(m.filter.Value())) {
			m.filter.SetValue("")
		}
		m.refresh()
		if row := slices.Index(m.shown, at); row >= 0 {
			m.table.SetCursor(row)
			m.redraw()
		}
		return m, nil
	}
	var cmd tea.Cmd
	b.field, cmd = b.field.Update(msg)
	return m, cmd
}

// ---- quitting -----------------------------------------------------------------------------

func (m model) quitDialog() string {
	n := 0
	for _, t := range m.tabs {
		a, e, r := t.changes()
		n += a + e + r
	}
	lost := plural(n, "change", "changes") + " will be lost."
	if n == 0 {
		lost = "The new order will be lost."
	}
	return styleModal.Render(lipgloss.JoinVertical(lipgloss.Center,
		styleErr.Bold(true).Render("Quit without saving?"), "", lost, "",
		styleDim.Render("y/q quit · n/esc stay")))
}

// The quit dialog only answers the question; every other key is ignored while it is open.
func (m model) updateQuit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "q":
		return m, tea.Quit
	case "n", "esc":
		m.confirmQuit = false
	}
	return m, nil
}

// ---- review and save ---------------------------------------------------------------------

func (m model) startReview() (tea.Model, tea.Cmd) {
	if !m.anyDirty() {
		m.status = "nothing to save"
		return m, nil
	}
	m.reviewing, m.outcome = true, nil
	return m, nil
}

// reviewLines lists every change, PATH by PATH.
func (m model) reviewLines(width int) []string {
	var lines []string
	for _, t := range m.tabs {
		if !t.dirty() {
			continue
		}
		head := styleAccent.Render(t.scope.name + " PATH")
		if t.needsAdmin {
			head += styleDim.Render("  saving it asks for admin: UAC will prompt")
		}
		lines = append(lines, head)
		for i, e := range t.entries {
			n := fmt.Sprintf("#%-3d ", i+1)
			switch {
			case e.removed:
				lines = append(lines, styleErr.Render("  - ")+styleDim.Render(n)+e.value)
			case e.added():
				lines = append(lines, styleOK.Render("  + ")+styleDim.Render(n)+e.value)
			case e.edited():
				lines = append(lines, styleWarn.Render("  ~ ")+styleDim.Render(n)+e.orig+styleDim.Render("  →  ")+e.value)
			}
		}
		if t.reordered() {
			lines = append(lines, styleDim.Render("  ↕ the order changes"))
		}
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return lines
}

func (m model) viewReview() string {
	width := m.w - stylePanel.GetHorizontalFrameSize()
	title := styleAccent.Render("Review") + styleDim.Render(" · what saving will write")
	lines, help := m.reviewLines(width), "enter/s save · esc/q back to the list"
	switch {
	case m.saving:
		title, help = styleAccent.Render("Saving…"), "answer the UAC prompt if one opened"
	case m.outcome != nil:
		title, lines, help = styleAccent.Render("Saved"), m.outcome, "enter/esc back to the list · q quit"
	}
	room := max(m.h-stylePanel.GetVerticalFrameSize()-4, 1)
	if len(lines) > room {
		more := len(lines) - room + 1
		lines = append(lines[:room-1], styleDim.Render(fmt.Sprintf("… and %d more lines", more)))
	}
	body := title + "\n\n" + strings.Join(lines, "\n")
	inner := m.h - stylePanel.GetVerticalFrameSize()
	body += strings.Repeat("\n", max(inner-lipgloss.Height(body), 1)) + styleDim.Render(help)
	return stylePanel.BorderForeground(colorAccent).Width(m.w).Height(m.h).Render(body)
}

func (m model) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.saving {
		return m, nil // nothing to do but wait
	}
	if m.outcome != nil {
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "enter", "esc":
			m.reviewing, m.outcome = false, nil
		}
		return m, nil
	}
	switch msg.String() {
	case "enter", "s":
		return m.save()
	case "esc", "q", "left", "h":
		m.reviewing = false
	}
	return m, nil
}

// savedMsg brings back how each write went: an error per tab, nil when it was saved.
type savedMsg struct{ errs map[int]error }

// save writes every changed PATH off the UI loop: writing the Machine PATH unelevated waits
// for UAC. The tabs are only read here; saved updates them when the answers are in.
func (m model) save() (tea.Model, tea.Cmd) {
	type job struct {
		i        int
		s        scope
		old, new pathValue
	}
	var jobs []job
	for i, t := range m.tabs {
		if t.dirty() {
			jobs = append(jobs, job{i, t.scope, t.saved, pathValue{t.result(), t.saved.typ}})
		}
	}
	m.saving = true
	write := m.st.write
	return m, func() tea.Msg {
		errs := map[int]error{}
		for _, j := range jobs { // User first: a declined UAC prompt for Machine cannot hold it up
			errs[j.i] = write(j.s, j.old, j.new)
		}
		return savedMsg{errs}
	}
}

// saved reloads what was written; a PATH that failed is marked and keeps its changes for
// another try.
func (m model) saved(msg savedMsg) (tea.Model, tea.Cmd) {
	m.saving, m.outcome = false, []string{}
	for i, t := range m.tabs {
		err, tried := msg.errs[i]
		switch {
		case !tried:
		case err != nil:
			t.saveErr = err
			m.outcome = append(m.outcome, styleErr.Render("✗ ")+t.scope.name+" PATH not saved: "+err.Error(),
				styleDim.Render("  its changes are kept: s tries again"))
		default:
			t.load(m.st)
			m.outcome = append(m.outcome, styleOK.Render("✓ ")+t.scope.name+" PATH saved"+
				styleDim.Render(`  (the old value is in %LOCALAPPDATA%\pathed)`))
		}
	}
	m.outcome = append(m.outcome, "", styleDim.Render("New terminals see the change; with the pwsh wrapper, this one does too once pathed exits."))
	m.refresh()
	return m, nil
}
