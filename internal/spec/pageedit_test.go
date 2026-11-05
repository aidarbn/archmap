package spec

import (
	"errors"
	"strings"
	"testing"
)

// edit applies op to a set made of texts and returns the new text of the only
// edited file ("<deleted>" when it goes).
func edit(t *testing.T, texts map[string]string, op PageOp) (PageResult, string) {
	t.Helper()
	res, err := setOf(t, texts).EditPage(op)
	if err != nil {
		t.Fatalf("%s: %v", op.Op, err)
	}
	if len(res.Edits) != 1 {
		return res, ""
	}
	if res.Edits[0].After == nil {
		return res, "<deleted>"
	}
	return res, *res.Edits[0].After
}

func TestPageIDFor(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Отмена заказа", "otmena-zakaza"},
		{"Оплата Kaspi", "oplata-kaspi"},
		{"Тапсырыс қабылдау", "tapsyrys-qabyldau"},
		{"Menu sync / POS", "menu-sync-pos"},
		{"支付", ""},
		{"!!!", ""},
	} {
		if got := PageIDFor(c.in); got != c.want {
			t.Errorf("PageIDFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCreatePage(t *testing.T) {
	texts := map[string]string{"arch/pages/a.arch": "page otmena-zakaza \"Отмена\"\n"}
	res, text := edit(t, texts, PageOp{Op: "create", Name: "  Отмена заказа "})
	if res.Page != "otmena-zakaza-2" || res.Edits[0].Path != "arch/pages/otmena-zakaza-2.arch" || res.Edits[0].Before != nil {
		t.Errorf("result %+v", res)
	}
	if text != "page otmena-zakaza-2 \"Отмена заказа\"\n" {
		t.Errorf("text %q", text)
	}
	doc, errs := Parse("x", text)
	if len(errs) > 0 || doc.Pages[0].Name != "Отмена заказа" {
		t.Errorf("parse %v %+v", errs, doc.Pages)
	}
	// latin name: the short form
	if _, text := edit(t, nil, PageOp{Op: "create", Name: "Cancel"}); text != "page \"Cancel\"\n" {
		t.Errorf("latin %q", text)
	}
	// non-latin with nothing to spell: "page"
	if res, _ := edit(t, nil, PageOp{Op: "create", Name: "支付"}); res.Page != "page" {
		t.Errorf("id %q", res.Page)
	}
	for _, c := range []struct{ name, msg string }{
		{"", "needs a name"},
		{"   ", "needs a name"},
		{"отмена ", "already exists"},
		{"a\nb", "one line"},
	} {
		_, err := setOf(t, texts).EditPage(PageOp{Op: "create", Name: c.name})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("create %q: %v, want %q", c.name, err, c.msg)
		}
	}
}

func TestAddFrames(t *testing.T) {
	base := "# Отмена\npage p \"Отмена\"   # the page\n  journey guest-cancel   # main path\n  tables  orders\n\n# next\nflow f\n  step A\n"
	texts := map[string]string{"arch/pages/p.arch": base}

	// tables join the existing frame; comments and other blocks stay
	_, got := edit(t, texts, PageOp{Op: "add", Page: "p", Kind: FrameTables, Refs: []string{"payments", "orders", "items"}})
	want := "# Отмена\npage p \"Отмена\"   # the page\n  journey guest-cancel   # main path\n  tables  orders, payments, items\n\n# next\nflow f\n  step A\n"
	if got != want {
		t.Errorf("tables:\n%s\nwant:\n%s", got, want)
	}
	// a ref already there: nothing to do
	res, _ := edit(t, texts, PageOp{Op: "add", Page: "p", Kind: FrameTables, Refs: []string{"orders"}})
	if res.Notice != NoticeAlready || len(res.Edits) != 0 {
		t.Errorf("already: %+v", res)
	}
	// api: a new line after the last frame, values re-aligned
	_, got = edit(t, texts, PageOp{Op: "add", Page: "p", Kind: FrameAPI, Refs: []string{"CancelOrder", "POST /orders"}})
	want = "# Отмена\npage p \"Отмена\"   # the page\n  journey guest-cancel   # main path\n  tables  orders\n  api     CancelOrder, \"POST /orders\"\n\n# next\nflow f\n  step A\n"
	if got != want {
		t.Errorf("api:\n%s\nwant:\n%s", got, want)
	}
	// states: a line per entity
	_, got = edit(t, texts, PageOp{Op: "add", Page: "p", Kind: FrameStates, Refs: []string{"Order", "Payment"}})
	if !strings.Contains(got, "  tables  orders\n  states  Order\n  states  Payment\n\n# next") {
		t.Errorf("states:\n%s", got)
	}
	// an empty page: the line goes after the header, the width is the new one
	_, got = edit(t, map[string]string{"arch/pages/e.arch": "page e\n"}, PageOp{Op: "add", Page: "e", Kind: FrameAPI, Refs: []string{"X"}})
	if got != "page e\n  api X\n" {
		t.Errorf("empty: %q", got)
	}
	for _, c := range []struct {
		op  PageOp
		msg string
	}{
		{PageOp{Op: "add", Page: "p", Kind: "note", Refs: []string{"x"}}, "cannot put"},
		{PageOp{Op: "add", Page: "p", Kind: "tables", Refs: []string{"a b"}}, "not a valid"},
		{PageOp{Op: "add", Page: "p", Kind: "tables"}, "nothing to add"},
		{PageOp{Op: "add", Page: "nope", Kind: "tables", Refs: []string{"a"}}, "no such page"},
	} {
		_, err := setOf(t, texts).EditPage(c.op)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%+v: %v, want %q", c.op, err, c.msg)
		}
	}
}

func TestRemoveMoveRenameNote(t *testing.T) {
	base := "page p \"Отмена\"\n  journey j\n  # about the tables\n  tables  orders, payments  # two\n  note    \"старый\"\n  api     X\n"
	texts := map[string]string{"arch/pages/p.arch": base}

	_, got := edit(t, texts, PageOp{Op: "remove_frame", Page: "p", Index: ip(0)})
	if got != "page p \"Отмена\"\n  # about the tables\n  tables orders, payments  # two\n  note   \"старый\"\n  api    X\n" {
		t.Errorf("remove (re-aligned):\n%s", got)
	}
	_, got = edit(t, texts, PageOp{Op: "remove_ref", Page: "p", Index: ip(1), Refs: []string{"orders"}})
	if !strings.Contains(got, "  tables  payments  # two\n") {
		t.Errorf("remove_ref:\n%s", got)
	}
	_, got = edit(t, texts, PageOp{Op: "remove_ref", Page: "p", Index: ip(3), Refs: []string{"X"}})
	if strings.Contains(got, "api") {
		t.Errorf("remove_ref of the last ref removes the frame:\n%s", got)
	}
	// move api to the front, and note to the end
	_, got = edit(t, texts, PageOp{Op: "move_frame", Page: "p", Index: ip(3), To: ip(0)})
	if got != "page p \"Отмена\"\n  api     X\n  journey j\n  # about the tables\n  tables  orders, payments  # two\n  note    \"старый\"\n" {
		t.Errorf("move to 0:\n%s", got)
	}
	_, got = edit(t, texts, PageOp{Op: "move_frame", Page: "p", Index: ip(2), To: ip(3)})
	if !strings.HasSuffix(got, "  api     X\n  note    \"старый\"\n") {
		t.Errorf("move to end:\n%s", got)
	}
	_, got = edit(t, texts, PageOp{Op: "rename", Page: "p", Name: "Отмена заказа"})
	if !strings.HasPrefix(got, "page p \"Отмена заказа\"\n  journey j\n") {
		t.Errorf("rename:\n%s", got)
	}
	_, got = edit(t, texts, PageOp{Op: "set_note", Page: "p", Index: ip(2), Text: "новый \"текст\"\nвторая строка"})
	if !strings.Contains(got, "  note    \"новый \\\"текст\\\"\\nвторая строка\"\n") {
		t.Errorf("set_note:\n%s", got)
	}
	_, got = edit(t, texts, PageOp{Op: "add_note", Page: "p", Text: "ещё"})
	if !strings.HasSuffix(got, "  api     X\n  note    \"ещё\"\n") {
		t.Errorf("add_note:\n%s", got)
	}
	for _, c := range []struct {
		op  PageOp
		msg string
	}{
		{PageOp{Op: "set_note", Page: "p", Index: ip(0), Text: "x"}, "not a note"},
		{PageOp{Op: "set_note", Page: "p", Index: ip(2), Text: " "}, "empty"},
		{PageOp{Op: "remove_frame", Page: "p", Index: ip(9)}, "no frame"},
		{PageOp{Op: "move_frame", Page: "p", Index: ip(0), To: ip(4)}, "bad target"},
		{PageOp{Op: "rename", Page: "p", Name: ""}, "needs a name"},
		{PageOp{Op: "frobnicate", Page: "p"}, "unknown page operation"},
	} {
		_, err := setOf(t, texts).EditPage(c.op)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%+v: %v, want %q", c.op, err, c.msg)
		}
	}
}

func TestDeletePage(t *testing.T) {
	// alone in its file: the file goes
	if _, got := edit(t, map[string]string{"arch/pages/p.arch": "# c\npage p\n  journey j\n"}, PageOp{Op: "delete", Page: "p"}); got != "<deleted>" {
		t.Errorf("alone: %q", got)
	}
	// in a shared arch/*.arch: only the block (with its comment) and one blank line
	texts := map[string]string{"arch/main.arch": "flow a\n  step A\n\n# the page\npage p\n  journey j\n  note \"x\"\n\njourney j: a\n"}
	if _, got := edit(t, texts, PageOp{Op: "delete", Page: "p"}); got != "flow a\n  step A\n\njourney j: a\n" {
		t.Errorf("shared:\n%s", got)
	}
}

func TestPageConflictAndLastFrame(t *testing.T) {
	texts := map[string]string{"arch/pages/p.arch": "page p\n  tables orders\n"}
	seen := PageView{Name: "p", Frames: []FrameView{{Kind: "tables", Refs: []string{"orders"}}}}
	_, got := edit(t, texts, PageOp{Op: "remove_frame", Page: "p", Index: ip(0), Expect: &seen})
	if got != "page p\n" {
		t.Errorf("last frame: %q", got)
	}
	stale := PageView{Name: "p", Frames: []FrameView{{Kind: "tables", Refs: []string{"payments"}}}}
	_, err := setOf(t, texts).EditPage(PageOp{Op: "remove_frame", Page: "p", Index: ip(0), Expect: &stale})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("stale view: %v", err)
	}
	// restore: only over the text it expects
	before, after := "page p\n  tables orders\n", "page p\n"
	res, err := setOf(t, map[string]string{"arch/pages/p.arch": after}).EditPage(PageOp{Op: "restore", Files: []RestoreFile{{Path: "arch/pages/p.arch", Text: &before, Expect: &after}}})
	if err != nil || *res.Edits[0].After != before {
		t.Errorf("restore: %v %+v", err, res)
	}
	_, err = setOf(t, map[string]string{"arch/pages/p.arch": "page p\n  note \"by hand\"\n"}).EditPage(PageOp{Op: "restore", Files: []RestoreFile{{Path: "arch/pages/p.arch", Text: &before, Expect: &after}}})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("restore over a hand edit: %v", err)
	}
	// restore of a created page: the file goes when it is still what was created
	res, err = setOf(t, map[string]string{"arch/pages/p.arch": after}).EditPage(PageOp{Op: "restore", Files: []RestoreFile{{Path: "arch/pages/p.arch", Text: nil, Expect: &after}}})
	if err != nil || res.Edits[0].After != nil {
		t.Errorf("restore delete: %v %+v", err, res)
	}
	for _, p := range []string{"../x.arch", "arch/pages/../../x.arch", "arch/pages/sub/x.arch", "/etc/x.arch"} {
		if _, err := setOf(t, nil).EditPage(PageOp{Op: "restore", Files: []RestoreFile{{Path: p, Text: &after}}}); err == nil {
			t.Errorf("restore to %q allowed", p)
		}
	}
	junk := "flow x\n  step A\n"
	if _, err := setOf(t, nil).EditPage(PageOp{Op: "restore", Files: []RestoreFile{{Path: "arch/new.arch", Text: &junk}}}); err == nil {
		t.Error("restore creates a non-page arch/*.arch")
	}
}
