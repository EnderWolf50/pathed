package main

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// store is where the PATHs live: the registry, or a fake in tests.
type store struct {
	read     func(scope) (pathValue, error)
	write    func(s scope, old, new pathValue) error
	canWrite func(scope) bool
	exists   func(string) bool
}

var registryStore = store{read: read, write: write, canWrite: canWrite, exists: exists}

// entry is one PATH entry and how it changed since the last save.
type entry struct {
	value   string // what will be saved
	orig    string // what was saved; "" for an entry added since
	removed bool   // marked to go, still shown until the save
}

func (e *entry) added() bool  { return e.orig == "" }
func (e *entry) edited() bool { return e.orig != "" && e.value != e.orig }

// tab is one PATH, User or Machine.
type tab struct {
	scope    scope
	saved    pathValue
	entries  []*entry
	err      error // it could not be read
	readOnly bool  // it cannot be written without admin
}

func (t *tab) load(st store) {
	t.saved, t.err = st.read(t.scope)
	t.readOnly = !st.canWrite(t.scope)
	t.entries = nil
	for _, v := range t.saved.entries {
		t.entries = append(t.entries, &entry{value: v, orig: v})
	}
}

// result is the PATH as saving would write it.
func (t *tab) result() []string {
	var out []string
	for _, e := range t.entries {
		if !e.removed {
			out = append(out, e.value)
		}
	}
	return out
}

func (t *tab) dirty() bool { return !slices.Equal(t.result(), t.saved.entries) }

// reordered says whether the entries kept from the last save are in another order.
func (t *tab) reordered() bool {
	var kept []string
	for _, e := range t.entries {
		if !e.added() && !e.removed {
			kept = append(kept, e.orig)
		}
	}
	var was []string
	for _, v := range t.saved.entries {
		if slices.Contains(kept, v) {
			was = append(was, v)
		}
	}
	return !slices.Equal(kept, was)
}

// health is what is wrong with each entry: its folder is missing, or it repeats an earlier
// entry (dupOf is that entry's position, from 1). Removed entries are left out of both.
func (t *tab) health(exists func(string) bool) (missing []bool, dupOf []int) {
	missing, dupOf = make([]bool, len(t.entries)), make([]int, len(t.entries))
	first := map[string]int{}
	for i, e := range t.entries {
		if e.removed {
			continue
		}
		missing[i] = !exists(e.value)
		k := pathKey(e.value)
		if j, seen := first[k]; seen {
			dupOf[i] = j + 1
		} else {
			first[k] = i
		}
	}
	return missing, dupOf
}

var (
	keyUp     = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown   = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyMoveUp = key.NewBinding(key.WithKeys("K", "shift+up"), key.WithHelp("K/J", "move"))
	keyMoveDn = key.NewBinding(key.WithKeys("J", "shift+down"))
	keyAdd    = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	keyEdit   = key.NewBinding(key.WithKeys("enter", "e"), key.WithHelp("enter/e", "edit"))
	keyRemove = key.NewBinding(key.WithKeys("d", "x", "delete"), key.WithHelp("d", "remove"))
	keyClean  = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clean"))
	keyUndo   = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo all"))
	keyOpen   = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open folder"))
	keyFilter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyReload = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload"))
	keySave   = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save"))
	keyBack   = key.NewBinding(key.WithKeys("left", "h", "esc", "q"), key.WithHelp("←/h/esc/q", "back"))
)

type model struct {
	st     store
	tabs   []*tab
	on     int   // the sidebar's selection
	inList bool  // focus: the table (true) or the sidebar
	shown  []int // positions in the tab's entries that the table shows, narrowed by the filter
	table  table.Model
	filter textinput.Model
	help   help.Model
	w, h   int    // terminal size
	status string // a one-off note next to the heading, cleared by the next key

	// At most one of these is open over or instead of the list.
	input       *inputBox // adding or editing an entry
	confirmQuit bool      // unsaved changes: quit anyway?
	reviewing   bool      // the changes, before saving them
	outcome     []string  // what the save did, shown on the review screen afterwards
}

func newModel(st store) model {
	m := model{st: st, filter: textinput.New(), help: help.New()}
	for _, s := range scopes {
		t := &tab{scope: s}
		t.load(st)
		m.tabs = append(m.tabs, t)
	}
	m.filter.Prompt = "/ "
	m.filter.Placeholder = "filter the entries"

	km := table.DefaultKeyMap() // moving; everything else is ours
	km.PageUp = key.NewBinding(key.WithKeys("pgup"))
	km.PageDown = key.NewBinding(key.WithKeys("pgdown"))
	km.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"))
	km.HalfPageDown = key.NewBinding(key.WithKeys("ctrl+d"))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Padding(0).Foreground(colorDim).BorderForeground(colorFaint)
	styles.Cell = lipgloss.NewStyle()     // redraw pads and paints every cell itself
	styles.Selected = lipgloss.NewStyle() // the cursor row is painted by redraw too
	m.table = table.New(table.WithColumns(columns(80)), table.WithKeyMap(km), table.WithStyles(styles), table.WithFocused(true))
	m.refresh()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) tab() *tab { return m.tabs[m.on] }

func sideWidth() int { return cfg.SidebarWidth }

// Lines of the right panel around the table: heading and filter above; a blank, the
// divider, two detail lines and the help below.
const tableChrome = 7

// columns fits the table to the panel: the path takes what the others leave. The widths
// include a cell of padding on each side, which redraw paints itself so a row's background
// runs unbroken.
func columns(width int) []table.Column {
	cols := []table.Column{{Title: "", Width: 2}, {Title: "#", Width: 3}, {Title: "PATH"}, {Title: "STATUS", Width: 12}}
	used := 0
	for _, c := range cols {
		used += c.Width + 2
	}
	cols[2].Width = max(width-used, 10)
	for i := range cols {
		cols[i].Width += 2
		cols[i].Title = " " + cols[i].Title
	}
	return cols
}

// refresh recomputes which entries the table shows.
func (m *model) refresh() {
	query := strings.ToLower(m.filter.Value())
	m.shown = nil
	for i, e := range m.tab().entries {
		if strings.Contains(strings.ToLower(e.value), query) {
			m.shown = append(m.shown, i)
		}
	}
	m.redraw()
}

// redraw turns the shown entries into table cells; it runs after every change or cursor
// move, because the cursor mark, the status and the row colors live in the cells.
func (m *model) redraw() {
	t := m.tab()
	missing, dupOf := t.health(m.st.exists)
	cursor := min(m.table.Cursor(), max(len(m.shown)-1, 0))
	cols := m.table.Columns()
	cells := make([]table.Row, len(m.shown))
	for row, i := range m.shown {
		e := t.entries[i]
		onCursor := row == cursor && m.inList
		// A background only holds up to the next reset, so every piece of the row is painted
		// with it: each colored run and each cell's padding.
		var bg lipgloss.Style
		sign := " "
		switch {
		case e.removed:
			bg, sign = bg.Background(pick(onCursor, bgRemovedCursor, bgRemoved)), "-"
		case e.added():
			bg, sign = bg.Background(pick(onCursor, bgAddedCursor, bgAdded)), "+"
		case e.edited():
			bg, sign = bg.Background(pick(onCursor, bgEditedCursor, bgEdited)), "~"
		case onCursor:
			bg = bg.Background(bgCursor)
		}
		paint := func(s lipgloss.Style, text string) string {
			if c := bg.GetBackground(); c != nil {
				s = s.Background(c)
			}
			return s.Render(text)
		}
		plain := lipgloss.NewStyle()

		mark := paint(plain, " ")
		if onCursor {
			mark = paint(styleAccent, "›")
		}
		path, state := paint(plain, e.value), paint(styleOK, "ok")
		switch {
		case e.removed:
			path, state = paint(styleDim.Strikethrough(true), e.value), paint(styleErr, "removed")
		case dupOf[i] > 0:
			state = paint(styleWarn, fmt.Sprintf("same as #%d", dupOf[i]))
		case missing[i]:
			state = paint(styleErr, "missing")
		}
		cellsOf := table.Row{mark + paint(styleAccent, sign), paint(styleDim, fmt.Sprint(i+1)), path, state}
		for c := range cellsOf {
			w := cols[c].Width
			cellsOf[c] = bg.Width(w).Padding(0, 1).Render(ansi.Truncate(cellsOf[c], max(w-2, 0), paint(plain, "…")))
		}
		cells[row] = cellsOf
	}
	m.table.SetRows(cells)
	m.table.SetCursor(cursor)
}

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// current is the entry under the cursor and its position in the tab.
func (m model) current() (*entry, int, bool) {
	c := m.table.Cursor()
	if c < 0 || c >= len(m.shown) {
		return nil, -1, false
	}
	i := m.shown[c]
	return m.tab().entries[i], i, true
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		frameW, frameH := stylePanel.GetFrameSize()
		width := m.w - sideWidth() - frameW
		m.table.SetColumns(columns(width))
		m.table.SetWidth(width)
		m.table.SetHeight(m.h - frameH - tableChrome)
		m.filter.SetWidth(width - 2)
		m.help.SetWidth(width)
		m.redraw()
	case tea.KeyPressMsg:
		m.status = ""
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch {
		case m.confirmQuit:
			return m.updateQuit(msg)
		case m.input != nil:
			return m.updateInput(msg)
		case m.reviewing:
			return m.updateReview(msg)
		case m.filter.Focused():
			return m.updateFilter(msg)
		case m.inList:
			return m.updateList(msg)
		default:
			return m.updateSide(msg)
		}
	}
	var cmd tea.Cmd
	if m.input != nil {
		m.input.field, cmd = m.input.field.Update(msg) // the cursor blink
	} else {
		m.filter, cmd = m.filter.Update(msg)
	}
	return m, cmd
}

func (m model) anyDirty() bool {
	return slices.ContainsFunc(m.tabs, func(t *tab) bool { return t.dirty() })
}

// The sidebar picks a PATH; enter hands the keys to its table.
func (m model) updateSide(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		if m.anyDirty() {
			m.confirmQuit = true
			return m, nil
		}
		return m, tea.Quit
	case "s":
		return m.startReview()
	case "r":
		return m.reload()
	case "up", "k":
		if m.on > 0 {
			m.on--
			m.table.SetCursor(0)
			m.refresh()
		}
	case "down", "j":
		if m.on < len(m.tabs)-1 {
			m.on++
			m.table.SetCursor(0)
			m.refresh()
		}
	case "enter", "right", "l":
		m.inList = true
		m.redraw()
	}
	return m, nil
}

// reload reads the selected PATH again, unless it has changes that would be lost.
func (m model) reload() (tea.Model, tea.Cmd) {
	if m.tab().dirty() {
		m.status = "unsaved changes: u undoes them first"
		return m, nil
	}
	m.tab().load(m.st)
	m.refresh()
	m.status = "reloaded"
	return m, nil
}

// Typing a filter: the table narrows as you type; enter keeps it, esc drops it.
func (m model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter.Blur()
		return m, nil
	case "esc":
		m.filter.Blur()
		m.filter.SetValue("")
		m.refresh()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.table.SetCursor(0)
	m.refresh()
	return m, cmd
}

// editable refuses changes to a PATH that cannot be written, saying why.
func (m *model) editable() bool {
	t := m.tab()
	switch {
	case t.err != nil:
		m.status = "this PATH could not be read"
	case t.readOnly:
		m.status = "read-only: the " + t.scope.name + " PATH needs admin (gsudo pathed)"
	default:
		return true
	}
	return false
}

// The table: change entries, filter them, save; ←/h/esc/q go back to the sidebar.
func (m model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := m.tab()
	e, i, ok := m.current()
	switch {
	case key.Matches(msg, keyBack): // one level up: first out of a filter, then to the sidebar
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refresh()
			return m, nil
		}
		m.inList = false
		m.redraw()
		return m, nil
	case key.Matches(msg, keySave):
		return m.startReview()
	case key.Matches(msg, keyReload):
		return m.reload()
	case key.Matches(msg, keyFilter):
		return m, m.filter.Focus()
	case key.Matches(msg, keyOpen):
		if ok {
			exec.Command("explorer", expand(e.value)).Start()
		}
		return m, nil
	case key.Matches(msg, keyAdd):
		if m.editable() {
			return m.openInput(i, true)
		}
		return m, nil
	case key.Matches(msg, keyEdit):
		if ok && m.editable() {
			return m.openInput(i, false)
		}
		return m, nil
	case key.Matches(msg, keyRemove):
		if ok && m.editable() {
			if e.added() { // never saved: nothing to keep around
				t.entries = slices.Delete(t.entries, i, i+1)
				m.refresh()
			} else {
				e.removed = !e.removed
				m.redraw()
			}
		}
		return m, nil
	case key.Matches(msg, keyMoveUp, keyMoveDn):
		switch {
		case !ok || !m.editable():
		case m.filter.Value() != "":
			m.status = "clear the filter (esc) to reorder"
		default:
			to := i - 1
			if key.Matches(msg, keyMoveDn) {
				to = i + 1
			}
			if to >= 0 && to < len(t.entries) {
				t.entries[i], t.entries[to] = t.entries[to], t.entries[i]
				m.table.SetCursor(to)
				m.refresh()
			}
		}
		return m, nil
	case key.Matches(msg, keyClean):
		if m.editable() {
			missing, dupOf := t.health(m.st.exists)
			n := 0
			for j, e := range t.entries {
				if !e.removed && (missing[j] || dupOf[j] > 0) {
					e.removed = true
					n++
				}
			}
			m.status = fmt.Sprintf("clean: %d entries marked for removal", n)
			m.redraw()
		}
		return m, nil
	case key.Matches(msg, keyUndo):
		t.load(m.st)
		m.refresh()
		m.status = "changes undone"
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	m.redraw()
	return m, cmd
}

func (m model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.w == 0 { // the first frame comes before the terminal's size is known
		return v
	}
	v.Content = m.viewList()
	if m.reviewing {
		v.Content = m.viewReview()
	}
	switch {
	case m.confirmQuit:
		v.Content = m.overlay(v.Content, m.quitDialog())
	case m.input != nil:
		v.Content = m.overlay(v.Content, m.input.view(m.tab(), m.st.exists))
	}
	return v
}

// overlay draws a box over the middle of the screen.
func (m model) overlay(under, box string) string {
	x := max((m.w-lipgloss.Width(box))/2, 0)
	y := max((m.h-lipgloss.Height(box))/2, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(under), lipgloss.NewLayer(box).X(x).Y(y).Z(1)).Render()
}
