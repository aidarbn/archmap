// Package serve implements `archmap serve`: a live architecture map. It serves
// the viewer (internal/render) with the current facts on localhost, watches the repository and,
// after a debounce, re-extracts and pushes an update to browsers over SSE.
//
// Change detection is polling of file mtimes and sizes rather than fsnotify:
// no dependency, no per-directory watch limits (inotify) or per-file descriptors
// (kqueue), and a recursive tree that appears and disappears (checkouts,
// generators) needs no bookkeeping. A stat walk of ~10k files costs tens of
// milliseconds every poll interval. Rebuilds re-parse only files whose content
// changed (extract.ParseCache); resolving calls and imports is global and is
// redone in full, which is cheap next to parsing.
package serve

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/extract"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/gitstat"
	"github.com/aidarbn/archmap/internal/lang"
	"github.com/aidarbn/archmap/internal/render"
	"github.com/aidarbn/archmap/internal/walk"
)

// Options configure a Server.
type Options struct {
	Root      string
	Base      string
	WithTests bool
	Parsers   []lang.Parser
	NoSrc     bool          // do not serve function bodies (/src)
	Poll      time.Duration // mtime poll interval (default 400ms)
	Debounce  time.Duration // quiet time before a rebuild (default 300ms)
}

// State values of an Update.
const (
	Building = "building"
	Ready    = "ready"
	Failed   = "error"
)

// Update is the SSE payload. The browser re-reads the data endpoint when Seq grows.
type Update struct {
	Seq   int      `json:"seq"`
	State string   `json:"state"`
	Error string   `json:"error,omitempty"`
	Flash []string `json:"flash,omitempty"` // functions that are new or whose body/status changed
	Ms    int64    `json:"ms,omitempty"`    // rebuild time
}

type stamp struct {
	mod  int64
	size int64
}

type snapshot map[string]stamp

// Server is the live-map HTTP server.
type Server struct {
	opts  Options
	cache *extract.ParseCache

	mu     sync.Mutex
	factsJ []byte
	srcIdx map[string]srcRef // where each function lives, for /src
	modelJ []byte
	last   Update
	subs   map[chan Update]struct{}

	// owned by the watch goroutine (and New)
	snap   snapshot
	hashes map[string]string
	prev   *facts.Facts
}

// New builds the first snapshot synchronously; an error here is fatal for the command.
func New(opts Options) (*Server, error) {
	if opts.Poll <= 0 {
		opts.Poll = 400 * time.Millisecond
	}
	if opts.Debounce <= 0 {
		opts.Debounce = 300 * time.Millisecond
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	opts.Root = root
	s := &Server{opts: opts, cache: extract.NewParseCache(), subs: map[chan Update]struct{}{}}
	snap := s.takeSnapshot()
	if err := s.build(snap, nil); err != nil {
		return nil, err
	}
	return s, nil
}

// Seq returns the current update number.
func (s *Server) Seq() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last.Seq
}

// Handler returns the HTTP handler (GET / , /data.json, /events).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/data.json", s.handleData)
	mux.HandleFunc("/events", s.handleEvents)
	if !s.opts.NoSrc {
		mux.HandleFunc("/src", s.handleSrc)
	}
	return guard(mux)
}

// guard allows GET/HEAD only and rejects foreign Host headers (DNS rebinding).
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostport string) bool {
	h := hostport
	if hh, _, err := net.SplitHostPort(hostport); err == nil {
		h = hh
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) current() (factsJ, modelJ []byte, u Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.factsJ, s.modelJ, s.last
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	fj, mj, u := s.current()
	html, err := render.HTMLLive(fj, mj, s.live(u.Seq))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}

func (s *Server) handleData(w http.ResponseWriter, _ *http.Request) {
	fj, mj, u := s.current()
	if len(bytes.TrimSpace(mj)) == 0 {
		mj = []byte("{}")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"seq":%d,"facts":`, u.Seq)
	_, _ = w.Write(fj)
	_, _ = io.WriteString(w, `,"model":`)
	_, _ = w.Write(mj)
	_, _ = io.WriteString(w, "}")
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan Update, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	first := s.last
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	send := func(u Update) bool {
		b, _ := json.Marshal(u)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	first.Flash = nil // a fresh connection is not a "change"
	if !send(first) {
		return
	}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case u := <-ch:
			if !send(u) {
				return
			}
		case <-tick.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) publish(u Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = u
	for ch := range s.subs {
		select {
		case ch <- u:
		default: // slow client: drop the oldest, keep the newest
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- u:
			default:
			}
		}
	}
}

// Watch polls for changes until ctx is done and rebuilds after the debounce.
func (s *Server) Watch(ctx context.Context) {
	tick := time.NewTicker(s.opts.Poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		cur := s.takeSnapshot()
		if equal(cur, s.snap) {
			continue
		}
		_, _, u := s.current()
		s.publish(Update{Seq: u.Seq, State: Building})
		// debounce: wait until the tree is quiet
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.opts.Debounce):
			}
			next := s.takeSnapshot()
			if equal(next, cur) {
				break
			}
			cur = next
		}
		changed := diff(s.snap, cur)
		if err := s.build(cur, changed); err != nil {
			s.snap = cur
			_, _, u := s.current()
			s.publish(Update{Seq: u.Seq, State: Failed, Error: err.Error()})
		}
	}
}

func equal(a, b snapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// diff returns the source files (repo-relative) that differ between two snapshots.
func diff(old, cur snapshot) map[string]bool {
	ch := map[string]bool{}
	for k, v := range cur {
		if w, ok := old[k]; (!ok || w != v) && strings.HasPrefix(k, "f:") {
			ch[k[2:]] = true
		}
	}
	for k := range old {
		if _, ok := cur[k]; !ok && strings.HasPrefix(k, "f:") {
			ch[k[2:]] = true
		}
	}
	return ch
}

func (s *Server) exts() map[string]bool {
	m := map[string]bool{}
	for _, p := range s.opts.Parsers {
		for _, e := range p.Extensions() {
			m[e] = true
		}
	}
	return m
}

// takeSnapshot stats every candidate source file plus the git state files and the model.
func (s *Server) takeSnapshot() snapshot {
	snap := snapshot{}
	add := func(key, p string) {
		if st, err := os.Stat(p); err == nil {
			snap[key] = stamp{st.ModTime().UnixNano(), st.Size()}
		}
	}
	exts := s.exts()
	root := s.opts.Root
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (walk.SkipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !exts[path.Ext(rel)] || !walk.PathOK(rel, exts, s.opts.WithTests) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			snap["f:"+rel] = stamp{info.ModTime().UnixNano(), info.Size()}
		}
		return nil
	})
	if out, ok := gitstat.Git(root, "rev-parse", "--absolute-git-dir"); ok {
		gd := strings.TrimSpace(out)
		add("git:HEAD", filepath.Join(gd, "HEAD"))
		add("git:index", filepath.Join(gd, "index"))
		if head, err := os.ReadFile(filepath.Join(gd, "HEAD")); err == nil {
			if ref, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: "); ok {
				add("git:ref", filepath.Join(gd, filepath.FromSlash(ref)))
			}
		}
	}
	return snap
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// build re-extracts and publishes. changed == nil marks the initial build.
func (s *Server) build(snap snapshot, changed map[string]bool) error {
	t0 := time.Now()
	f, err := extract.Run(extract.Options{
		Root: s.opts.Root, Base: s.opts.Base, WithTests: s.opts.WithTests,
		Parsers: s.opts.Parsers, Cache: s.cache,
	})
	if err != nil {
		return err
	}
	fj, err := marshal(f)
	if err != nil {
		return err
	}
	mj, err := autosem.JSON(f)
	if err != nil {
		return err
	}
	idx := make(map[string]srcRef, len(f.Functions))
	for fid, fn := range f.Functions {
		idx[fid] = srcRef{fn.File, fn.Start, fn.End, fn.Lang}
	}

	// Which functions to flash: new ones, ones whose body text changed (hash of
	// their lines, so shifting line numbers alone does not count) and ones whose
	// change status against the base moved (a commit, a checkout).
	hashes := make(map[string]string, len(f.Functions))
	lines := map[string][]string{}
	var flash []string
	for fid, fn := range f.Functions {
		h, have := s.hashes[fid]
		if !have || changed[fn.File] || s.hashes == nil {
			ls, ok := lines[fn.File]
			if !ok {
				if b, err := os.ReadFile(filepath.Join(s.opts.Root, filepath.FromSlash(fn.File))); err == nil {
					ls = strings.Split(string(b), "\n")
				}
				lines[fn.File] = ls
			}
			h = bodyHash(ls, fn.Start, fn.End)
		}
		hashes[fid] = h
		if changed == nil {
			continue
		}
		old, existed := s.hashes[fid]
		switch {
		case !existed || old != h:
			flash = append(flash, fid)
		case s.prev != nil && statusKey(s.prev.Functions[fid]) != statusKey(fn):
			flash = append(flash, fid)
		}
	}
	sort.Strings(flash)

	s.snap, s.hashes, s.prev = snap, hashes, f
	s.mu.Lock()
	s.factsJ, s.modelJ = fj, mj
	s.srcIdx = idx
	seq := s.last.Seq + 1
	s.mu.Unlock()
	s.publish(Update{Seq: seq, State: Ready, Flash: flash, Ms: time.Since(t0).Milliseconds()})
	return nil
}

func statusKey(f facts.Function) string {
	u := ""
	if f.Uncommitted != nil && *f.Uncommitted {
		u = "u"
	}
	return f.Status + u
}

func bodyHash(lines []string, start, end int) string {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(lines[start-1:end], "\n")))
	return hex.EncodeToString(sum[:8])
}

// Listen validates that addr is a loopback address and opens the listener.
func Listen(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if !loopbackHost(host) {
		return nil, fmt.Errorf("--addr %q: only loopback addresses (127.0.0.1, ::1, localhost) are allowed", addr)
	}
	return net.Listen("tcp", addr)
}

type srcRef struct {
	file       string
	start, end int
	lang       string
}

// maxServedSrcLines caps a body returned by /src.
const maxServedSrcLines = 400

func (s *Server) live(seq int) render.Live {
	l := render.Live{Seq: seq, Events: "/events", Data: "/data.json"}
	if !s.opts.NoSrc {
		l.Src = "/src"
	}
	return l
}

// handleSrc returns the body of a function read from disk: GET /src?fid=<fid>.
// Only files that are in the current facts can be read.
func (s *Server) handleSrc(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ref, ok := s.srcIdx[r.URL.Query().Get("fid")]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join(s.opts.Root, filepath.FromSlash(ref.file)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, extract.SourceSlice(string(b), ref.start, ref.end, maxServedSrcLines, ref.lang))
}
