package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/EnderWolf50/enved/winenv"
)

func TestClean(t *testing.T) {
	in := []string{`C:\a`, `C:\gone`, `c:\A\`, `C:\b`, `C:\a`}
	exists := func(e string) bool { return e != `C:\gone` }
	got := clean(in, exists)
	if want := []string{`C:\a`, `C:\b`}; !slices.Equal(got, want) {
		t.Fatalf("clean = %q, want %q", got, want)
	}
}

func TestShellInit(t *testing.T) {
	for _, shell := range []string{"pwsh", "PowerShell"} {
		if got, err := shellInit(shell); err != nil || got != pwshInit {
			t.Errorf("shellInit(%q) = %q, %v; want the pwsh wrapper", shell, got, err)
		}
	}
	if _, err := shellInit("bash"); err == nil {
		t.Error("shellInit(bash) did not fail")
	}
	if !strings.Contains(pwshInit, "function pathed {") {
		t.Error("the pwsh wrapper does not define pathed")
	}
}

func TestCLI(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)
	st, _, saved := fakeStore([]string{a}, nil)
	if err := run([]string{"add", b, "--front"}, st); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(saved[winenv.User], []string{b, a}) {
		t.Fatalf("add --front: %q", saved[winenv.User])
	}
	if err := run([]string{"add", b}, st); err == nil {
		t.Fatal("adding a folder twice did not fail")
	}
	if err := run([]string{"rm", "1"}, st); err != nil || !slices.Equal(saved[winenv.User], []string{a}) {
		t.Fatalf("rm 1: %v, %q", err, saved[winenv.User])
	}
	if err := run([]string{"add", filepath.Join(dir, "nope")}, st); err == nil {
		t.Fatal("adding a missing folder did not fail")
	}
	if err := run([]string{"add", b, "-m"}, st); err == nil {
		t.Fatal("a declined UAC prompt did not fail add -m")
	}
}

func TestPathChangeKeepsType(t *testing.T) {
	sz := &winenv.Var{Name: "PATH", Value: winenv.Value{Data: `C:\a`, Type: winenv.SZ}}
	if c := pathChange(winenv.User, sz, []string{`C:\b`}); c.New.Type != winenv.SZ || c.Name != "PATH" {
		t.Errorf("a REG_SZ PATH without %%VARS%% became %+v", c)
	}
	if c := pathChange(winenv.User, sz, []string{`%X%\b`}); c.New.Type != winenv.ExpandSZ {
		t.Error("a PATH that gains a %VAR% did not become REG_EXPAND_SZ")
	}
	if c := pathChange(winenv.User, nil, []string{`C:\b`}); c.New.Type != winenv.ExpandSZ || c.Old != nil {
		t.Errorf("a new PATH: %+v", c)
	}
}
