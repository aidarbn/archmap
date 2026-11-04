package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidarbn/archmap/internal/lang"
)

// fakeParser understands lines "def [Cls.]name -> call, ..." (language "python").
type fakeParser struct{}

func (fakeParser) Name() string         { return "python" }
func (fakeParser) Extensions() []string { return []string{".py"} }
func (fakeParser) Parse(_ string, src []byte) (lang.File, error) {
	var f lang.File
	for i, l := range strings.Split(string(src), "\n") {
		if !strings.HasPrefix(l, "def ") {
			continue
		}
		head, calls, _ := strings.Cut(strings.TrimPrefix(l, "def "), "->")
		fn := lang.Func{Start: i + 1, End: i + 1, Sig: strings.TrimSpace(head), Name: strings.TrimSpace(head)}
		var cl lang.CallList
		for _, c := range strings.Split(calls, ",") {
			if c = strings.TrimSpace(c); c != "" {
				cl.Add(c, "")
			}
		}
		fn.Calls = cl.Calls()
		f.Funcs = append(f.Funcs, fn)
	}
	return f, nil
}

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "app/a.py", "def alpha -> beta\ndef beta\n")
	write(t, root, "lib/b.py", "def gamma\n")
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	return root
}

func newServer(t *testing.T, root string) *Server {
	t.Helper()
	s, err := New(Options{Root: root, Parsers: []lang.Parser{fakeParser{}}, Poll: 20 * time.Millisecond, Debounce: 40 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(t *testing.T, h http.Handler, method, target, host string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	r.Host = "127.0.0.1"
	if host != "" {
		r.Host = host
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHandlers(t *testing.T) {
	s := newServer(t, newRepo(t))
	h := s.Handler()

	w := get(t, h, "GET", "/", "127.0.0.1:8000")
	body := w.Body.String()
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("index: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if strings.Contains(body, "/*ARCH_DATA*/null") || !strings.Contains(body, `"serve":{"seq":1,"events":"/events","data":"/data.json","src":"/src"}`) {
		t.Error("index must carry data with the serve endpoints")
	}
	if !strings.Contains(body, "app/a.py::alpha") {
		t.Error("index lacks facts")
	}

	w = get(t, h, "GET", "/data.json", "localhost:8000")
	var d struct {
		Seq   int `json:"seq"`
		Facts struct {
			Functions map[string]json.RawMessage `json:"functions"`
		} `json:"facts"`
		Model json.RawMessage `json:"model"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("data.json: %v: %s", err, w.Body.String())
	}
	if d.Seq != 1 || len(d.Facts.Functions) != 3 || !strings.Contains(string(d.Model), `"modules":{"app":`) {
		t.Errorf("data = %+v", d)
	}

	if c := get(t, h, "GET", "/nope", "").Code; c != 404 {
		t.Errorf("unknown path: %d", c)
	}
	if c := get(t, h, "POST", "/", "").Code; c != 405 {
		t.Errorf("POST: %d", c)
	}
	if c := get(t, h, "GET", "/", "evil.example.com").Code; c != 403 {
		t.Errorf("foreign host: %d", c)
	}
	if c := get(t, h, "GET", "/data.json", "[::1]:9").Code; c != 200 {
		t.Errorf("ipv6 loopback host: %d", c)
	}
}

func TestListenLoopbackOnly(t *testing.T) {
	if _, err := Listen("0.0.0.0:0"); err == nil {
		t.Error("0.0.0.0 must be refused")
	}
	if _, err := Listen("example.com:80"); err == nil {
		t.Error("foreign host must be refused")
	}
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}

// events reads SSE data lines from the stream into a channel.
func events(t *testing.T, url string) (<-chan Update, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	ch := make(chan Update, 32)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if b, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var u Update
				if json.Unmarshal([]byte(b), &u) == nil {
					ch <- u
				}
			}
		}
	}()
	return ch, cancel
}

func next(t *testing.T, ch <-chan Update, state string) Update {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case u := <-ch:
			if u.State == state {
				return u
			}
		case <-timeout:
			t.Fatalf("no %q event", state)
		}
	}
}

func TestSSEUpdateAfterEdit(t *testing.T) {
	root := newRepo(t)
	s := newServer(t, root)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go s.Watch(ctx)

	ch, cancel := events(t, ts.URL)
	defer cancel()
	if u := next(t, ch, Ready); u.Seq != 1 || len(u.Flash) != 0 {
		t.Fatalf("initial event = %+v", u)
	}

	// add a function and edit another one's body (line count unchanged)
	write(t, root, "app/a.py", "def alpha -> beta, delta\ndef beta -> gamma\ndef delta\n")
	next(t, ch, Building)
	u := next(t, ch, Ready)
	if u.Seq != 2 {
		t.Fatalf("seq = %d", u.Seq)
	}
	want := map[string]bool{"app/a.py::alpha": true, "app/a.py::beta": true, "app/a.py::delta": true}
	got := map[string]bool{}
	for _, f := range u.Flash {
		got[f] = true
	}
	for f := range want {
		if !got[f] {
			t.Errorf("flash lacks %s: %v", f, u.Flash)
		}
	}
	if got["lib/b.py::gamma"] {
		t.Errorf("untouched function flashed: %v", u.Flash)
	}

	// the new data is served
	resp, err := http.Get(ts.URL + "/data.json")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "app/a.py::delta") || !strings.Contains(string(b), `"seq":2`) {
		t.Errorf("data.json not refreshed: %.200s", b)
	}

	// unrelated files do not trigger a rebuild; a reconnect does not replay flashes
	write(t, root, "README.txt", "x")
	time.Sleep(200 * time.Millisecond)
	if s.Seq() != 2 {
		t.Errorf("seq moved to %d on an unrelated file", s.Seq())
	}
	ch2, cancel2 := events(t, ts.URL)
	defer cancel2()
	if u := next(t, ch2, Ready); u.Seq != 2 || len(u.Flash) != 0 {
		t.Errorf("reconnect event = %+v", u)
	}
}

func TestSrcEndpoint(t *testing.T) {
	root := newRepo(t)
	s := newServer(t, root)
	h := s.Handler()
	fid := "app/a.py::alpha"
	w := get(t, h, "GET", "/src?fid="+fid, "127.0.0.1:8000")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "def alpha -> beta" {
		t.Errorf("src: %d %q", w.Code, w.Body.String())
	}
	if w := get(t, h, "GET", "/src?fid=../../etc/passwd::x", "127.0.0.1:8000"); w.Code != 404 {
		t.Errorf("a fid that is not in the facts must be 404, got %d", w.Code)
	}
	fj, _, _ := s.current()
	if strings.Contains(string(fj), `"src"`) {
		t.Error("serve must not embed bodies in the data")
	}
}

func TestNoSrcServer(t *testing.T) {
	s, err := New(Options{Root: newRepo(t), Parsers: []lang.Parser{fakeParser{}}, NoSrc: true})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	if w := get(t, h, "GET", "/src?fid=app/a.py::alpha", "127.0.0.1:8000"); w.Code != 404 {
		t.Errorf("--no-src: /src must be 404, got %d", w.Code)
	}
	if w := get(t, h, "GET", "/", "127.0.0.1:8000"); strings.Contains(w.Body.String(), `"src":"/src"`) {
		t.Error("--no-src: the page must not advertise /src")
	}
}
