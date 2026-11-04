package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func send(t *testing.T, h http.Handler, method, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = "127.0.0.1:7777"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) SpecState {
	t.Helper()
	var st SpecState
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return st
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSpecGetEmpty(t *testing.T) {
	h := newServer(t, newRepo(t)).Handler()
	w := send(t, h, "GET", "/spec", "", nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"files":[]`) || !strings.Contains(w.Body.String(), `"errors":[]`) || !strings.Contains(w.Body.String(), `"flows":[]`) {
		t.Fatalf("empty spec must give empty arrays: %s", w.Body.String())
	}
}

func TestSpecOpsWritesFormattedFile(t *testing.T) {
	root := newRepo(t)
	write(t, root, "arch/orders.arch", "states Order\n  new->paid   on pay\n")
	s := newServer(t, root)
	h := s.Handler()

	w := send(t, h, "POST", "/spec/ops", `{"op":"add_transition","entity":"Order","from":"paid","to":"refunded","on":"refund","label":"возврат"}`, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	want := "states Order\n  new -> paid        on pay\n  paid -> refunded   on refund   \"возврат\"\n"
	if got := readFile(t, root, "arch/orders.arch"); got != want {
		t.Fatalf("file:\n%s\nwant:\n%s", got, want)
	}
	st := decode(t, w)
	if len(st.Changed) != 1 || st.Changed[0] != "arch/orders.arch" || st.Files[0].Text != want || len(st.Model.States[0].Transitions) != 2 {
		t.Fatalf("response: %+v", st)
	}
	if st.Model.States[0].Transitions[1].Drift != "not_implemented" {
		t.Fatalf("drift: %+v", st.Model.States[0].Transitions)
	}

	// a batch; new flow goes to arch/<prefix>.arch, planned step round trips
	w = send(t, h, "POST", "/spec/ops", `{"ops":[{"op":"add_flow","id":"kaspi-pay","name":"Оплата"},{"op":"add_step","flow":"kaspi-pay","ref":"alpha","text":"старт"},{"op":"add_step","flow":"kaspi-pay","planned":true,"text":"лимит"}]}`, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got := readFile(t, root, "arch/kaspi.arch"); got != "flow kaspi-pay \"Оплата\"\n  step alpha   \"старт\"\n  step plan    \"лимит\"\n" {
		t.Fatalf("kaspi.arch:\n%s", got)
	}
	st = decode(t, w)
	var fl = st.Model.Flows[0]
	if fl.Steps[0].Fid != "app/a.py::alpha" || fl.Planned != 1 {
		t.Fatalf("flow: %+v", fl)
	}

	// rejected ops: 400, files untouched
	before := readFile(t, root, "arch/kaspi.arch")
	for _, body := range []string{
		`{"op":"remove_flow","flow":"nope"}`,
		`{"op":"add_flow","id":"x","file":"../../etc/x.arch"}`,
		`{"op":"add_flow","id":"x","file":"arch/sub/x.arch"}`,
		`{"op":"bogus"}`,
		`{"op":"add_step","flow":"kaspi-pay","ref":"a b"}`,
		`{"nonsense":1}`,
		`not json`,
		`[]`,
	} {
		if w := send(t, h, "POST", "/spec/ops", body, nil); w.Code != 400 || !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	if readFile(t, root, "arch/kaspi.arch") != before {
		t.Fatal("a rejected op changed the file")
	}
	if _, err := os.Stat(filepath.Join(root, "etc")); err == nil {
		t.Fatal("escaped the arch directory")
	}
	// no temp files are left behind
	ents, _ := os.ReadDir(filepath.Join(root, "arch"))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp") {
			t.Fatalf("temp file left: %s", e.Name())
		}
	}
}

func TestSpecOpsRefusedOnSyntaxError(t *testing.T) {
	root := newRepo(t)
	write(t, root, "arch/bad.arch", "flow\n")
	h := newServer(t, root).Handler()
	w := send(t, h, "POST", "/spec/ops", `{"op":"add_flow","id":"x"}`, nil)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "arch/bad.arch:1") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	// the map still serves, the error shows in /spec
	st := decode(t, send(t, h, "GET", "/spec", "", nil))
	if len(st.Errors) != 1 || st.Errors[0].Line != 1 || st.Errors[0].File != "arch/bad.arch" {
		t.Fatalf("%+v", st.Errors)
	}
	if c := get(t, h, "GET", "/data.json", "").Code; c != 200 {
		t.Fatalf("data.json: %d", c)
	}
}

func TestSpecPutFile(t *testing.T) {
	root := newRepo(t)
	write(t, root, "arch/a.arch", "flow a\n")
	h := newServer(t, root).Handler()

	body := func(path, text string) string {
		b, _ := json.Marshal(map[string]string{"path": path, "text": text})
		return string(b)
	}
	// syntax error: 422 with a position, the file is not touched
	w := send(t, h, "PUT", "/spec/file", body("arch/a.arch", "flow a\n  step\n"), nil)
	if w.Code != 422 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var e struct {
		Error string `json:"error"`
		At    struct {
			File      string
			Line, Col int
		}
		Errors []any
	}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e.At.Line != 2 || e.At.File != "arch/a.arch" || !strings.Contains(e.Error, "arch/a.arch:2") {
		t.Fatalf("%v %s", err, w.Body.String())
	}
	if readFile(t, root, "arch/a.arch") != "flow a\n" {
		t.Fatal("file changed despite a syntax error")
	}

	// good text is stored as typed (not reformatted)
	text := "flow a   \"A\"\n  step   alpha\n"
	w = send(t, h, "PUT", "/spec/file", body("arch/a.arch", text), nil)
	if w.Code != 200 || readFile(t, root, "arch/a.arch") != text {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if st := decode(t, w); st.Model.Flows[0].Steps[0].Fid != "app/a.py::alpha" || st.Files[0].Text != text {
		t.Fatalf("%+v", st)
	}
	// a new file
	if w := send(t, h, "PUT", "/spec/file", body("arch/new.arch", "flow n\n"), nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	// paths outside arch/<name>.arch
	for _, p := range []string{"../x.arch", "arch/../x.arch", "/etc/passwd", "arch/x.txt", "arch/sub/x.arch", "x.arch", "arch/.hidden", ""} {
		if w := send(t, h, "PUT", "/spec/file", body(p, "flow z\n"), nil); w.Code != 400 {
			t.Errorf("path %q: %d", p, w.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "x.arch")); err == nil {
		t.Fatal("wrote outside the root")
	}
}

func TestSpecGuards(t *testing.T) {
	root := newRepo(t)
	h := newServer(t, root).Handler()
	op := `{"op":"add_flow","id":"x"}`
	cases := []struct {
		name, method, target string
		host                 string
		hdr                  map[string]string
		want                 int
	}{
		{"ok", "POST", "/spec/ops", "", nil, 200},
		{"foreign host", "POST", "/spec/ops", "evil.example:80", nil, 403},
		{"foreign origin", "POST", "/spec/ops", "", map[string]string{"Origin": "https://evil.example"}, 403},
		{"same origin", "POST", "/spec/ops", "", map[string]string{"Origin": "http://127.0.0.1:7777"}, 200},
		{"form post", "POST", "/spec/ops", "", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, 415},
		{"text/plain", "POST", "/spec/ops", "", map[string]string{"Content-Type": "text/plain"}, 415},
		{"wrong method", "PUT", "/spec/ops", "", nil, 405},
		{"wrong method 2", "POST", "/spec/file", "", nil, 405},
		{"post elsewhere", "POST", "/data.json", "", nil, 405},
		{"delete", "DELETE", "/spec", "", nil, 405},
		{"get spec", "GET", "/spec", "", nil, 200},
	}
	for _, c := range cases {
		body := op
		if c.method == "GET" {
			body = ""
		}
		r := httptest.NewRequest(c.method, c.target, strings.NewReader(body))
		r.Host = "127.0.0.1:7777"
		if c.host != "" {
			r.Host = c.host
		}
		r.Header.Set("Content-Type", "application/json")
		for k, v := range c.hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: %d, want %d: %s", c.name, w.Code, c.want, w.Body.String())
		}
		// only the first accepted write creates the flow; reset between cases
		_ = os.RemoveAll(filepath.Join(root, "arch"))
	}
	// body limit
	big := `{"op":"add_flow","id":"` + strings.Repeat("a", maxSpecBody) + `"}`
	if w := send(t, h, "POST", "/spec/ops", big, nil); w.Code != 413 {
		t.Errorf("big body: %d", w.Code)
	}
}

func TestSpecWriteTriggersRebuild(t *testing.T) {
	root := newRepo(t)
	s := newServer(t, root)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go s.Watch(ctx)
	ch, cancel := events(t, ts.URL)
	defer cancel()
	next(t, ch, Ready)

	resp, err := http.Post(ts.URL+"/spec/ops", "application/json", bytes.NewReader([]byte(`{"ops":[{"op":"add_flow","id":"refund"},{"op":"add_step","flow":"refund","planned":true,"text":"Вернуть деньги"}]}`)))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", err, resp)
	}
	resp.Body.Close()
	next(t, ch, Building)
	u := next(t, ch, Ready)
	if u.Seq != 2 {
		t.Fatalf("seq %d", u.Seq)
	}
	r2, err := http.Get(ts.URL + "/data.json")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r2.Body)
	if !strings.Contains(buf.String(), "Вернуть деньги") || !strings.Contains(buf.String(), `"spec":true`) {
		t.Fatalf("the spec is not in the live model: %.300s", buf.String())
	}
	_ = time.Second
}
