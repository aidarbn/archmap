package serve

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/spec"
)

// PageState is the answer of POST /spec/page: the spec state after the edit
// plus what the edit did (Edits: before / after texts, the viewer keeps them
// for undo), the page it was about and a notice for an edit with nothing to do.
type PageState struct {
	SpecState
	Page   string          `json:"page,omitempty"`
	Edits  []spec.FileEdit `json:"edits"`
	Notice string          `json:"notice,omitempty"`
}

// handleSpecPage: POST /spec/page applies one page edit (spec.PageOp) to the
// text of the page's file — the rest of the file is kept as it is — checks that
// every reference the edit adds points to something the code has, and writes
// atomically. Writes go only to arch/pages/*.arch and to the existing
// arch/*.arch that holds the page. 409: the page changed on disk meanwhile (the
// answer carries the fresh state); 422: a reference to nothing.
func (s *Server) handleSpecPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		jsonErr(w, http.StatusMethodNotAllowed, "POST only", nil)
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var op spec.PageOp
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&op); err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request: "+err.Error(), nil)
		return
	}
	s.specMu.Lock()
	defer s.specMu.Unlock()
	set := s.loadSpec()
	res, err := set.EditPage(op)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, spec.ErrConflict) {
			code = http.StatusConflict
		}
		st := s.specState(set, nil)
		writeJSONResp(w, code, map[string]any{"error": err.Error(), "conflict": code == http.StatusConflict, "model": st.Model})
		return
	}
	if len(res.Edits) == 0 {
		writeJSONResp(w, http.StatusOK, PageState{SpecState: s.specState(set, nil), Page: res.Page, Edits: []spec.FileEdit{}, Notice: res.Notice})
		return
	}
	// the set after the edit: it must parse and must not name anything new that the code lacks
	texts := map[string]string{}
	for _, f := range set.Files {
		texts[f.Path] = f.Text
	}
	for _, e := range res.Edits {
		if !s.pageWritable(set, e) {
			jsonErr(w, http.StatusForbidden, "refusing to write "+e.Path, nil)
			return
		}
		if e.After == nil {
			delete(texts, e.Path)
		} else {
			texts[e.Path] = *e.After
		}
	}
	next := spec.FromTexts(texts)
	for _, e := range next.Errors {
		for _, ed := range res.Edits {
			if e.File == ed.Path {
				jsonErr(w, http.StatusUnprocessableEntity, e.Error(), []spec.Error{e})
				return
			}
		}
	}
	if op.Op != "restore" && op.Op != "delete" {
		if broken := s.newBrokenRefs(set, next, res.Page); len(broken) > 0 {
			jsonErr(w, http.StatusUnprocessableEntity, broken[0], nil)
			return
		}
	}
	var changed []string
	for _, e := range res.Edits {
		var err error
		if e.After == nil {
			err = s.removeSpecFile(e.Path)
		} else {
			err = s.writeSpecFile(e.Path, *e.After)
		}
		if err != nil {
			jsonErr(w, http.StatusInternalServerError, "write "+e.Path+": "+err.Error(), nil)
			return
		}
		changed = append(changed, e.Path)
	}
	writeJSONResp(w, http.StatusOK, PageState{SpecState: s.specState(s.loadSpec(), changed), Page: res.Page, Edits: res.Edits, Notice: res.Notice})
}

// pageWritable: a page edit writes arch/pages/<name>.arch, or an arch/<name>.arch
// that exists and holds a page (before or after), never anything else.
func (s *Server) pageWritable(set *spec.Set, e spec.FileEdit) bool {
	if !spec.ValidPath(e.Path) {
		return false
	}
	if path.Dir(e.Path) == spec.PagesDir {
		return true
	}
	hasPage := func(t *string) bool {
		if t == nil {
			return false
		}
		doc, _ := spec.Parse(e.Path, *t)
		return len(doc.Pages) > 0
	}
	return set.File(e.Path) != nil || hasPage(e.After)
}

// newBrokenRefs: the page-reference findings of page id in next that prev did not have.
func (s *Server) newBrokenRefs(prev, next *spec.Set, id string) []string {
	f := s.currentFacts()
	msgs := func(set *spec.Set) map[string]bool {
		_, fs := spec.Check(set, f, nil)
		out := map[string]bool{}
		for _, x := range fs {
			if x.Code == spec.CodePageRef && strings.HasPrefix(x.Msg, "page "+id+": ") {
				out[x.Msg] = true
			}
		}
		return out
	}
	old := msgs(prev)
	var out []string
	for m := range msgs(next) {
		if !old[m] {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// removeSpecFile deletes arch/<name>.arch or arch/pages/<name>.arch: a regular
// file inside plain directories only.
func (s *Server) removeSpecFile(rel string) error {
	if !spec.ValidPath(rel) {
		return fmt.Errorf("path %q must be arch/<name>.arch", rel)
	}
	for _, d := range []string{spec.Dir, path.Dir(rel)} {
		fi, err := os.Lstat(filepath.Join(s.opts.Root, filepath.FromSlash(d)))
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return fmt.Errorf("%s is not a plain directory", d)
		}
	}
	dst := filepath.Join(s.opts.Root, filepath.FromSlash(rel))
	fi, err := os.Lstat(dst)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", rel)
	}
	return os.Remove(dst)
}
