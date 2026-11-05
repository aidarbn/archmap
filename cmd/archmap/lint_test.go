package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintRepo(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

const goodSrc = `package main

// arch:flow pay "Pay"
// arch:actor Tablet
// arch:exit "order paid"
func Pay() { request(); check() }

// arch:step pay "ask the bank"
// arch:ext Kaspi
func request() {}

// arch:step pay "check"
// arch:on paid -> done
// arch:on expired "cancel"
// arch:state order: new -> paid
func check() {}

// arch:flow done "Done"
func Done() {}

func main() { Pay() }
`

func TestLintClean(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"lint", "--root", lintRepo(t, goodSrc)}, &out, &errb); code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "ok (2 flows, 4 annotated") {
		t.Fatal(out.String())
	}
}

func TestLintProblems(t *testing.T) {
	src := `package main

// arch:flow pay "Pay"
func Pay() { request() }

// arch:flow pay "Again"
func Again() {}

// arch:step nope "x"
// arch:on x -> ghost
// arch:state a: b -> b
func request() {}

// arch:step pay "lonely"
func lonely() {}

// arch:wat
// arch:step pay
func broken() {}

func main() { Pay() }
`
	var out, errb bytes.Buffer
	if code := run([]string{"lint", "--root", lintRepo(t, src)}, &out, &errb); code != 1 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	for _, want := range []string{
		"main.go:6: flow \"pay\" is already declared",
		"main.go:9: arch:step refers to flow \"nope\"",
		"main.go:10: arch:on -> \"ghost\": no such flow",
		"main.go:11: arch:state a: \"b\" -> \"b\" changes nothing",
		"main.go:14: step of flow \"pay\" is not reachable",
		"main.go:17: unknown directive arch:wat",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
	// json
	out.Reset()
	run([]string{"lint", "--root", lintRepo(t, src), "--format", "json"}, &out, &errb)
	var res struct {
		Problems []struct {
			File string
			Line int
			Msg  string
		}
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res.Problems) < 6 {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestLintBadFormat(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"lint", "--format", "xml"}, &out, &errb); code != 2 {
		t.Fatal(code)
	}
}

const journeySrc = `package main

// arch:flow order "Order"
// arch:journey guest "Guest"
func Order() {}

// arch:flow qr "QR"
// arch:then guest qr > ghost
// arch:then nojourney qr
func QR() {}

// arch:flow pay "Pay"
// arch:journey guest "Again"
func Pay() {}

func main() { Order(); QR(); Pay() }
`

func TestLintJourneys(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"lint", "--root", lintRepo(t, journeySrc)}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	s := out.String()
	for _, want := range []string{`no such flow "ghost"`, `no such journey "nojourney"`, `journey "guest" is already declared at main.go:4`, `info: 1 journey links are client calls`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// A gap between flows is a client link: information, the exit code stays 0
// even with --strict.
func TestLintJourneyGapIsClientLink(t *testing.T) {
	src := `package main

// arch:flow order "Order"
// arch:journey guest "Guest"
func Order() {}

// arch:flow qr "QR"
// arch:then guest qr
func QR() {}

func main() { Order(); QR() }
`
	var out, errb bytes.Buffer
	if code := run([]string{"lint", "--strict", "--root", lintRepo(t, src)}, &out, &errb); code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "info: 1 journey links are client calls") || strings.Contains(out.String(), "warning:") {
		t.Fatal(out.String())
	}
}
