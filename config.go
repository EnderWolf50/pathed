package main

import (
	"errors"

	"github.com/EnderWolf50/enved/theme"
)

// defaultConfig is both the defaults and their documentation: pathed reads it before the
// user's file, and `pathed --default-config` prints it as a starting point.
const defaultConfig = `# pathed's settings. Every key is optional: one left out keeps the value shown here.
# Colors are "#rrggbb" or an ANSI color number, "0" to "255".

# Width of the sidebar, in cells.
sidebar_width = 24

` + theme.Default

type config struct {
	SidebarWidth int         `toml:"sidebar_width"`
	Theme        theme.Theme `toml:"theme"`
}

func checkConfig(c config) error {
	if c.SidebarWidth < 16 {
		return errors.New("sidebar_width must be at least 16")
	}
	return c.Theme.Check()
}

// cfg is the settings in force: the defaults until main loads the user's file.
var cfg config

func init() {
	c, err := theme.Parse(config{}, defaultConfig, checkConfig)
	if err != nil {
		panic("the default config is broken: " + err.Error())
	}
	cfg = c
}
