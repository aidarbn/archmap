package generic

import "testing"

func docOf(t *testing.T, name, src, fn string) string {
	t.Helper()
	for _, f := range parse(t, name, src).Funcs {
		if f.Name == fn {
			return f.Doc
		}
	}
	t.Fatalf("no function %s", fn)
	return ""
}

func TestRustDocOnlyFromCommentsDirectlyAbove(t *testing.T) {
	src := `//! The crate: this describes the module, not the first function.
use std::io;

/// Direct doc.
fn direct() {}

/// Detached by a blank line.

fn detached() {}

#[cfg(unix)] // trailing remark
#[test]
fn trailing() {}

/// Docs above an attribute.
#[inline]
fn attributed() {}

//! stray inner doc
fn inner() {}
`
	for fn, want := range map[string]string{
		"direct":     "Direct doc.",
		"detached":   "",
		"trailing":   "",
		"attributed": "Docs above an attribute.",
		"inner":      "",
	} {
		if got := docOf(t, "rust", src, fn); got != want {
			t.Errorf("doc of %s = %q, want %q", fn, got, want)
		}
	}
}
