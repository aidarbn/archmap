package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSpec(t *testing.T, root, name, text string) {
	t.Helper()
	dir := filepath.Join(root, "arch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lintOut(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{"lint", "--root", root}, args...), &out, &errb)
	return code, out.String(), errb.String()
}

func TestLintSpecClean(t *testing.T) {
	root := lintRepo(t, goodSrc)
	writeSpec(t, root, "pay.arch", `states order
  new -> paid
  final paid

flow pay
  step Pay
  step request   ext Kaspi
  step check
  alt paid -> done
journey j: pay > done
`)
	code, out, errb := lintOut(t, root)
	if code != 0 || !strings.Contains(out, "ok (2 flows, 4 annotated functions, 1 spec flows)") {
		t.Fatalf("exit %d\n%s%s", code, out, errb)
	}
}

func TestLintSpecDrift(t *testing.T) {
	root := lintRepo(t, goodSrc)
	writeSpec(t, root, "pay.arch", `states order
  new -> paid
  paid -> refunded   on refund
  new -> expired

flow pay
  step Pay
  step nothing.Here
  step plan "Check limit"
  step check
  alt x -> ghost

flow refund
  step plan "Return money"

journey j: pay > phantom
`)
	// warnings alone pass; errors fail
	code, out, errb := lintOut(t, root)
	if code != 1 {
		t.Fatalf("exit %d\n%s%s", code, out, errb)
	}
	for _, want := range []string{
		`arch/pay.arch:8: flow pay: step "nothing.Here": no such function in the code`,
		`arch/pay.arch:11: flow pay: alt x -> "ghost": no such flow`,
		`arch/pay.arch:16: journey j: no such flow "phantom"`,
		`warning: arch/pay.arch:3: states order: paid -> refunded is in the spec, but the code has no such transition`,
		`warning: arch/pay.arch:4: states order: new -> expired is in the spec`,
		`info: 2 planned steps in the spec`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	// only warnings: ok without --strict, failure with it
	root = lintRepo(t, goodSrc)
	writeSpec(t, root, "pay.arch", "states order\n  new -> paid\n  paid -> refunded\n")
	if code, out, errb := lintOut(t, root); code != 0 || !strings.Contains(out, "warning: arch/pay.arch:3:") {
		t.Fatalf("exit %d\n%s%s", code, out, errb)
	}
	code, out, errb = lintOut(t, root, "--strict")
	if code != 1 || strings.Contains(out, "warning:") || !strings.Contains(out, "arch/pay.arch:3:") || !strings.Contains(errb, "--strict") {
		t.Fatalf("strict: exit %d\n%s%s", code, out, errb)
	}
}

func TestLintSpecUndeclaredAndSyntax(t *testing.T) {
	root := lintRepo(t, goodSrc)
	writeSpec(t, root, "pay.arch", "states order\n  new -> cancelled\n")
	code, out, _ := lintOut(t, root)
	// the code moves new -> paid, which the spec lacks: an error at the function
	if code != 1 || !strings.Contains(out, "main.go:") || !strings.Contains(out, "the code moves new -> paid, which the spec (arch/pay.arch:1) does not declare") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	writeSpec(t, root, "pay.arch", "flow pay\n  step\n")
	code, out, _ = lintOut(t, root)
	if code != 1 || !strings.Contains(out, "arch/pay.arch:2:") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestLintSpecJSON(t *testing.T) {
	root := lintRepo(t, goodSrc)
	writeSpec(t, root, "pay.arch", "flow refund\n  step plan \"x\"\n")
	code, out, _ := lintOut(t, root, "--format", "json")
	var res struct {
		Spec     bool                    `json:"spec"`
		Planned  int                     `json:"planned"`
		Info     []struct{ File string } `json:"info"`
		Problems []struct{ Line int }    `json:"problems"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err, out)
	}
	if code != 0 || !res.Spec || res.Planned != 1 || len(res.Info) != 1 || res.Info[0].File != "arch/pay.arch" {
		t.Fatalf("exit %d %+v", code, res)
	}
}

func TestMapShowsSpec(t *testing.T) {
	root := lintRepo(t, goodSrc)
	writeSpec(t, root, "pay.arch", "flow refund \"Возврат\"\n  step plan \"Вернуть деньги\"\n")
	out := filepath.Join(root, "out.html")
	var so, se bytes.Buffer
	if code := run([]string{"map", "--root", root, "--out", out}, &so, &se); code != 0 {
		t.Fatalf("%d %s", code, se.String())
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "Вернуть деньги") {
		t.Fatal("planned step is not in the page")
	}
}
