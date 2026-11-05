package serve

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/spec"
)

// maxSpecBody caps the body of /spec/ops and /spec/file.
const maxSpecBody = 1 << 20

// SpecFile is a spec file as stored on disk.
type SpecFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// SpecState is the answer of GET /spec and of the writing endpoints.
type SpecState struct {
	Seq      int            `json:"seq"`
	Files    []SpecFile     `json:"files"`
	Model    spec.Model     `json:"model"`
	Errors   []spec.Error   `json:"errors"`
	Findings []spec.Finding `json:"findings"`
	// Changed lists the files a write touched (writing endpoints only).
	Changed []string `json:"changed,omitempty"`
}

func (s *Server) loadSpec() *spec.Set {
	set, err := spec.Load(s.opts.Root)
	if err != nil {
		return &spec.Set{Errors: []spec.Error{{File: spec.Dir, Msg: err.Error()}}}
	}
	return set
}

func (s *Server) currentFacts() *facts.Facts {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.facts
}

func (s *Server) specState(set *spec.Set, changed []string) SpecState {
	f := s.currentFacts()
	cache := map[string]map[string]bool{}
	var reach func(string) map[string]bool
	if f != nil {
		reach = func(entry string) map[string]bool {
			if cache[entry] == nil {
				cache[entry] = autosem.Reachable(f, entry)
			}
			return cache[entry]
		}
	}
	m, findings := spec.Check(set, f, reach)
	st := SpecState{Seq: s.Seq(), Files: []SpecFile{}, Model: m, Errors: set.Errors, Findings: findings, Changed: changed}
	if st.Errors == nil {
		st.Errors = []spec.Error{}
	}
	if st.Findings == nil {
		st.Findings = []spec.Finding{}
	}
	for _, f := range set.Files {
		st.Files = append(st.Files, SpecFile{Path: f.Path, Text: f.Text})
	}
	return st
}

func writeJSONResp(w http.ResponseWriter, code int, v any) {
	b, err := marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

func jsonErr(w http.ResponseWriter, code int, msg string, errs []spec.Error) {
	body := map[string]any{"error": msg}
	if len(errs) > 0 {
		body["errors"] = errs
		body["at"] = errs[0]
	}
	writeJSONResp(w, code, body)
}

// handleSpec: GET /spec.
func (s *Server) handleSpec(w http.ResponseWriter, r *http.Request) {
	s.specMu.Lock()
	defer s.specMu.Unlock()
	writeJSONResp(w, http.StatusOK, s.specState(s.loadSpec(), nil))
}

// writeSpecFile writes arch/<name>.arch atomically (temp file + rename).
func (s *Server) writeSpecFile(rel, text string) error {
	if !spec.ValidPath(rel) {
		return fmt.Errorf("path %q must be arch/<name>.arch", rel)
	}
	// arch/ and, for a page file, arch/pages/: plain directories, made when missing
	dirs := []string{spec.Dir}
	if path.Dir(rel) == spec.PagesDir {
		dirs = append(dirs, spec.PagesDir)
	}
	var dir string
	for _, d := range dirs {
		dir = filepath.Join(s.opts.Root, filepath.FromSlash(d))
		if fi, err := os.Lstat(dir); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
				return fmt.Errorf("%s is not a plain directory", d)
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(dir, 0o755); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	dst := filepath.Join(s.opts.Root, filepath.FromSlash(rel))
	if fi, err := os.Lstat(dst); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", rel)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, dst); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSpecBody))
	if err != nil {
		jsonErr(w, http.StatusRequestEntityTooLarge, "request body too large or unreadable", nil)
		return nil, false
	}
	return b, true
}

// parseOps accepts {"op": ...}, [{"op": ...}, ...] and {"ops": [...]}.
func parseOps(b []byte) ([]spec.Op, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, errors.New("empty body")
	}
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		return d.Decode(v)
	}
	if b[0] == '[' {
		var ops []spec.Op
		if err := dec(&ops); err != nil {
			return nil, err
		}
		return ops, nil
	}
	var multi struct {
		Ops []spec.Op `json:"ops"`
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, err
	}
	if _, ok := probe["ops"]; ok {
		if err := dec(&multi); err != nil {
			return nil, err
		}
		return multi.Ops, nil
	}
	var one spec.Op
	if err := dec(&one); err != nil {
		return nil, err
	}
	return []spec.Op{one}, nil
}

// handleSpecOps: POST /spec/ops applies edit operations to the model, formats
// the touched files canonically and writes them.
func (s *Server) handleSpecOps(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	ops, err := parseOps(body)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request: "+err.Error(), nil)
		return
	}
	if len(ops) == 0 {
		jsonErr(w, http.StatusBadRequest, "no operations", nil)
		return
	}
	s.specMu.Lock()
	defer s.specMu.Unlock()
	set := s.loadSpec()
	if len(set.Errors) > 0 {
		jsonErr(w, http.StatusConflict, "the spec has syntax errors, fix the text first: "+set.Errors[0].Error(), set.Errors)
		return
	}
	touched, err := set.Apply(ops)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	for _, p := range touched {
		text := spec.Format(set.File(p).Doc)
		if err := s.writeSpecFile(p, text); err != nil {
			jsonErr(w, http.StatusInternalServerError, "write "+p+": "+err.Error(), nil)
			return
		}
		set.File(p).Text = text
	}
	writeJSONResp(w, http.StatusOK, s.specState(set, touched))
}

// handleSpecFile: PUT /spec/file {path, text} replaces a file with the text as
// typed. A syntax error answers 422 with the position and leaves the file alone.
func (s *Server) handleSpecFile(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var req SpecFile
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request: "+err.Error(), nil)
		return
	}
	if !spec.ValidPath(req.Path) {
		jsonErr(w, http.StatusBadRequest, fmt.Sprintf("path %q must be arch/<name>.arch", req.Path), nil)
		return
	}
	if _, errs := spec.Parse(req.Path, req.Text); len(errs) > 0 {
		jsonErr(w, http.StatusUnprocessableEntity, errs[0].Error(), errs)
		return
	}
	s.specMu.Lock()
	defer s.specMu.Unlock()
	if err := s.writeSpecFile(req.Path, req.Text); err != nil {
		jsonErr(w, http.StatusInternalServerError, "write "+req.Path+": "+err.Error(), nil)
		return
	}
	writeJSONResp(w, http.StatusOK, s.specState(s.loadSpec(), []string{req.Path}))
}

// sameOrigin allows a request without an Origin header (curl, same-origin
// GET/HEAD) or whose Origin is this server; a page of another site that talks
// to localhost carries a foreign Origin.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mt, "application/json")
}
