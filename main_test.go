package main

import (
	"slices"
	"strings"
	"testing"
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
