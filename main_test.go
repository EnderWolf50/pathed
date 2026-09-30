package main

import (
	"slices"
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
