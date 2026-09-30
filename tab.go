package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/winenv"
)

// pathTab is one scope's PATH in the list editor.
type pathTab struct {
	scope      winenv.Scope
	st         winenv.Store
	saved      *winenv.Var // nil: the scope has no PATH yet
	err        error       // it could not be read
	needsAdmin bool        // saving it asks for admin (UAC)
	list       *listedit.Model
}

func newPathTab(s winenv.Scope, st winenv.Store, exists func(string) bool) *pathTab {
	t := &pathTab{scope: s, st: st}
	t.list = listedit.New(t.Label(), listedit.Folders, nil, nil, exists)
	t.Load()
	return t
}

func (t *pathTab) Name() string     { return string(t.scope) }
func (t *pathTab) Label() string    { return string(t.scope) + " PATH" }
func (t *pathTab) Count() string    { return fmt.Sprint(len(t.list.Result())) }
func (t *pathTab) Err() error       { return t.err }
func (t *pathTab) ReadOnly() bool   { return false }
func (t *pathTab) NeedsAdmin() bool { return t.needsAdmin }

// Load reads the PATH again, dropping every change.
func (t *pathTab) Load() {
	t.saved, t.err = readPath(t.st, t.scope)
	t.needsAdmin = !t.st.CanWrite(t.scope)
	var entries []string
	if t.saved != nil {
		entries = winenv.Split(t.saved.Data)
	}
	t.list.ReadOnly = t.err != nil
	t.list.Reset(entries, entries)
}

// readPath is a scope's PATH; nil when it has none.
func readPath(st winenv.Store, s winenv.Scope) (*winenv.Var, error) {
	vars, err := st.ReadAll(s)
	if err != nil {
		return nil, err
	}
	for _, v := range vars {
		if strings.EqualFold(v.Name, "Path") {
			return &v, nil
		}
	}
	return nil, nil
}

// pathChange is the write that turns old (nil: none yet) into entries. A PATH is
// REG_EXPAND_SZ unless it was saved as REG_SZ and still has no %VARS%.
func pathChange(s winenv.Scope, old *winenv.Var, entries []string) winenv.Change {
	c := winenv.Change{Scope: s, Name: "Path", New: &winenv.Value{Data: winenv.Join(entries), Type: winenv.ExpandSZ}}
	if old != nil {
		c.Name, c.Old = old.Name, &old.Value
		if old.Type == winenv.SZ && !strings.Contains(c.New.Data, "%") {
			c.New.Type = winenv.SZ
		}
	}
	return c
}

func (t *pathTab) Dirty() bool        { return t.list.Dirty() }
func (t *pathTab) Pending() int       { return t.list.Pending() }
func (t *pathTab) Review() []string   { return t.list.Review() }
func (t *pathTab) Warnings() []string { return nil }

func (t *pathTab) Changes() []winenv.Change {
	if !t.Dirty() {
		return nil
	}
	return []winenv.Change{pathChange(t.scope, t.saved, t.list.Result())}
}

func (t *pathTab) Update(msg tea.Msg) (tea.Cmd, frame.Event) { return t.list.Update(msg) }

func (t *pathTab) Resize(w, h int) { t.list.Resize(w, h) }
func (t *pathTab) Focus(on bool)   { t.list.Focus(on) }
func (t *pathTab) Heading() string { return t.list.Heading() }
func (t *pathTab) Body() string    { return t.list.Body() }
func (t *pathTab) Overlay() string { return t.list.Overlay() }
func (t *pathTab) Status() string  { return t.list.Status() }
