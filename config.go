package main

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/BurntSushi/toml"
)

// defaultConfig is both the defaults and their documentation: pathed reads it before the
// user's file, and `pathed --default-config` prints it as a starting point.
const defaultConfig = `# pathed's settings. Every key is optional: one left out keeps the value shown here.
# Colors are "#rrggbb" or an ANSI color number, "0" to "255".

# Width of the sidebar, in cells.
sidebar_width = 24

[theme]
accent = "#ffc799" # headings, the selected PATH, the focused panel's border
dim    = "#8b8b8b" # secondary text
faint  = "#505050" # borders, dividers
ok     = "#99ffe4" # folders that exist, success
bad    = "#ff8080" # missing folders, errors, the quit dialog
warn   = "#ffc799" # duplicates

# Row backgrounds: the cursor row, and rows changed since the last save.
row_cursor          = "#262626"
row_added           = "#16241f"
row_added_cursor    = "#223a30"
row_edited          = "#3a2c12"
row_edited_cursor   = "#4d3b19"
row_removed         = "#2e1616"
row_removed_cursor  = "#432020"
`

type config struct {
	SidebarWidth int   `toml:"sidebar_width"`
	Theme        theme `toml:"theme"`
}

type theme struct {
	Accent           string `toml:"accent"`
	Dim              string `toml:"dim"`
	Faint            string `toml:"faint"`
	OK               string `toml:"ok"`
	Bad              string `toml:"bad"`
	Warn             string `toml:"warn"`
	RowCursor        string `toml:"row_cursor"`
	RowAdded         string `toml:"row_added"`
	RowAddedCursor   string `toml:"row_added_cursor"`
	RowEdited        string `toml:"row_edited"`
	RowEditedCursor  string `toml:"row_edited_cursor"`
	RowRemoved       string `toml:"row_removed"`
	RowRemovedCursor string `toml:"row_removed_cursor"`
}

// cfg is the settings in force: the defaults until main loads the user's file.
var cfg config

func init() {
	c, err := parseConfig(config{}, defaultConfig)
	if err != nil {
		panic("the default config is broken: " + err.Error())
	}
	cfg = c
	applyTheme(cfg.Theme)
}

// configPath is $PATHED_CONFIG, else pathed/config.toml in $XDG_CONFIG_HOME or ~/.config.
func configPath() string {
	if p := os.Getenv("PATHED_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "pathed", "config.toml")
}

// loadConfig reads the user's file over the defaults; no file means the defaults.
func loadConfig(path string) (config, error) {
	text, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	c, err := parseConfig(cfg, string(text))
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// parseConfig decodes text over base, keeping what text leaves out, and checks the result:
// a typo in a key or a value is an error, not a setting silently ignored.
func parseConfig(base config, text string) (config, error) {
	c := base
	md, err := toml.Decode(text, &c)
	if err != nil {
		return base, err
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		var names []string
		for _, k := range keys {
			names = append(names, k.String())
		}
		return base, fmt.Errorf("unknown setting %s", strings.Join(names, ", "))
	}
	if c.SidebarWidth < 16 {
		return base, errors.New("sidebar_width must be at least 16")
	}
	for key, value := range c.Theme.colors() {
		if !validColor(value) {
			return base, fmt.Errorf("theme.%s: %q is not \"#rrggbb\" or a number from 0 to 255", key, value)
		}
	}
	return c, nil
}

func (t theme) colors() map[string]string {
	return map[string]string{
		"accent": t.Accent, "dim": t.Dim, "faint": t.Faint, "ok": t.OK, "bad": t.Bad, "warn": t.Warn,
		"row_cursor": t.RowCursor, "row_added": t.RowAdded, "row_added_cursor": t.RowAddedCursor,
		"row_edited": t.RowEdited, "row_edited_cursor": t.RowEditedCursor,
		"row_removed": t.RowRemoved, "row_removed_cursor": t.RowRemovedCursor,
	}
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validColor(s string) bool {
	if hexColor.MatchString(s) {
		return true
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 255
}

// The theme's colors and the styles made from them, set by applyTheme.
var (
	colorAccent, colorDim, colorFaint, colorOK, colorBad, colorWarn color.Color

	// Row backgrounds, by change, each plain and under the cursor.
	bgCursor                   color.Color
	bgAdded, bgAddedCursor     color.Color
	bgEdited, bgEditedCursor   color.Color
	bgRemoved, bgRemovedCursor color.Color

	styleAccent, styleOK, styleErr, styleWarn, styleDim, styleFaint lipgloss.Style
	stylePanel, styleModal, styleDialog                             lipgloss.Style
)

func applyTheme(t theme) {
	c := lipgloss.Color
	colorAccent, colorDim, colorFaint = c(t.Accent), c(t.Dim), c(t.Faint)
	colorOK, colorBad, colorWarn = c(t.OK), c(t.Bad), c(t.Warn)
	bgCursor = c(t.RowCursor)
	bgAdded, bgAddedCursor = c(t.RowAdded), c(t.RowAddedCursor)
	bgEdited, bgEditedCursor = c(t.RowEdited), c(t.RowEditedCursor)
	bgRemoved, bgRemovedCursor = c(t.RowRemoved), c(t.RowRemovedCursor)

	fg := func(col color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(col) }
	styleAccent = fg(colorAccent).Bold(true)
	styleOK, styleErr, styleWarn, styleDim, styleFaint = fg(colorOK), fg(colorBad), fg(colorWarn), fg(colorDim), fg(colorFaint)
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	stylePanel = border.BorderForeground(colorFaint).Padding(0, 1)
	styleModal = border.BorderForeground(colorBad).Padding(1, 3)     // the quit question
	styleDialog = border.BorderForeground(colorAccent).Padding(1, 2) // adding or editing an entry
}
