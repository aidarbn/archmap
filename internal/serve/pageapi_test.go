package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodePage(t *testing.T, body []byte) PageState {
	t.Helper()
	var st PageState
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return st
}

func TestSpecPageCreateAddDelete(t *testing.T) {
	root := newRepo(t)
	h := newServer(t, root).Handler()

	w := send(t, h, "POST", "/spec/page", `{"op":"create","name":"Отмена заказа"}`, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	st := decodePage(t, w.Body.Bytes())
	if st.Page != "otmena-zakaza" || len(st.Model.Pages) != 1 || st.Model.Pages[0].Name != "Отмена заказа" || st.Edits[0].Before != nil {
		t.Fatalf("create: %+v", st)
	}
	const p = "arch/pages/otmena-zakaza.arch"
	if got := readFile(t, root, p); got != "page otmena-zakaza \"Отмена заказа\"\n" {
		t.Fatalf("file %q", got)
	}
	// the same name again: a clear error, nothing written
	if w := send(t, h, "POST", "/spec/page", `{"op":"create","name":"отмена заказа"}`, nil); w.Code != 400 || !strings.Contains(w.Body.String(), "already exists") {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}

	// a module the code has; then one it does not: 422 and the file stays
	w = send(t, h, "POST", "/spec/page", `{"op":"add","page":"otmena-zakaza","kind":"module","refs":["app"],"expect":{"name":"Отмена заказа","frames":[]}}`, nil)
	if w.Code != 200 {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	if got := readFile(t, root, p); got != "page otmena-zakaza \"Отмена заказа\"\n  module app\n" {
		t.Fatalf("after add %q", got)
	}
	if fr := decodePage(t, w.Body.Bytes()).Model.Pages[0].Frames; len(fr) != 1 || fr[0].Modules[0] != "app" {
		t.Fatalf("frames %+v", fr)
	}
	w = send(t, h, "POST", "/spec/page", `{"op":"add","page":"otmena-zakaza","kind":"tables","refs":["ghosts"]}`, nil)
	if w.Code != 422 || !strings.Contains(w.Body.String(), `tables \"ghosts\"`) {
		t.Fatalf("broken ref: %d %s", w.Code, w.Body.String())
	}
	if got := readFile(t, root, p); strings.Contains(got, "ghosts") {
		t.Fatal("a broken ref was written")
	}
	// the same module again: a notice, no write
	w = send(t, h, "POST", "/spec/page", `{"op":"add","page":"otmena-zakaza","kind":"module","refs":["app"]}`, nil)
	if st := decodePage(t, w.Body.Bytes()); w.Code != 200 || st.Notice != "already" || len(st.Edits) != 0 {
		t.Fatalf("already: %d %s", w.Code, w.Body.String())
	}

	// a stale view (the file was edited by hand): 409 with the fresh model
	write(t, root, p, "page otmena-zakaza \"Отмена заказа\"\n  module app\n  note \"by hand\"\n")
	w = send(t, h, "POST", "/spec/page", `{"op":"remove_frame","page":"otmena-zakaza","index":0,"expect":{"name":"Отмена заказа","frames":[{"kind":"module","refs":["app"]}]}}`, nil)
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"conflict":true`) || !strings.Contains(w.Body.String(), "by hand") {
		t.Fatalf("conflict: %d %s", w.Code, w.Body.String())
	}

	// undo of a removal: restore with the text it expects
	w = send(t, h, "POST", "/spec/page", `{"op":"remove_frame","page":"otmena-zakaza","index":1}`, nil)
	ed := decodePage(t, w.Body.Bytes()).Edits[0]
	body, _ := json.Marshal(map[string]any{"op": "restore", "files": []any{map[string]any{"path": ed.Path, "text": ed.Before, "expect": ed.After}}})
	if w := send(t, h, "POST", "/spec/page", string(body), nil); w.Code != 200 || !strings.Contains(readFile(t, root, p), "by hand") {
		t.Fatalf("undo: %d %s", w.Code, w.Body.String())
	}

	// delete: the file goes
	if w := send(t, h, "POST", "/spec/page", `{"op":"delete","page":"otmena-zakaza"}`, nil); w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, p)); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
}

func TestSpecPageKeepsSharedFile(t *testing.T) {
	root := newRepo(t)
	text := "# Orders\nflow pay \"Pay\"   # keep me\n  step alpha\n\npage p\n    module app   # mine\n"
	write(t, root, "arch/main.arch", text)
	h := newServer(t, root).Handler()
	w := send(t, h, "POST", "/spec/page", `{"op":"add_note","page":"p","text":"заметка"}`, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	want := "# Orders\nflow pay \"Pay\"   # keep me\n  step alpha\n\npage p\n    module app   # mine\n    note   \"заметка\"\n"
	if got := readFile(t, root, "arch/main.arch"); got != want {
		t.Fatalf("file:\n%s\nwant:\n%s", got, want)
	}
}

func TestSpecPagePaths(t *testing.T) {
	root := newRepo(t)
	h := newServer(t, root).Handler()
	for _, p := range []string{"../evil.arch", "arch/pages/../../evil.arch", "arch/x/evil.arch", "/tmp/evil.arch", "arch/pages/evil.txt"} {
		body, _ := json.Marshal(map[string]any{"op": "restore", "files": []any{map[string]any{"path": p, "text": "page evil\n", "expect": nil}}})
		if w := send(t, h, "POST", "/spec/page", string(body), nil); w.Code < 400 {
			t.Errorf("restore to %q: %d", p, w.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil.arch")); err == nil {
		t.Fatal("wrote outside the root")
	}
	// a symlinked arch/pages is not followed
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "arch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(root, "arch", "pages")); err != nil {
		t.Skip(err)
	}
	if w := send(t, h, "POST", "/spec/page", `{"op":"create","name":"x"}`, nil); w.Code != 500 || !strings.Contains(w.Body.String(), "not a plain directory") {
		t.Errorf("symlink: %d %s", w.Code, w.Body.String())
	}
	if ents, _ := os.ReadDir(other); len(ents) != 0 {
		t.Error("wrote through the symlink")
	}
	// writers need JSON from this origin
	if w := send(t, h, "POST", "/spec/page", `{"op":"create","name":"y"}`, map[string]string{"Origin": "http://evil.example"}); w.Code != 403 {
		t.Errorf("foreign origin: %d", w.Code)
	}
	if w := send(t, h, "GET", "/spec/page", "", nil); w.Code != 405 {
		t.Errorf("GET /spec/page: %d", w.Code)
	}
}
