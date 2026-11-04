package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// File is one parsed .arch file. Path is slash-separated and relative to the
// repository root ("arch/orders.arch").
type File struct {
	Path string
	Text string // as read; empty for files created by ops
	Doc  *Doc
}

// Set is all spec files of a repository.
type Set struct {
	Files  []*File
	Errors []Error
}

var fileRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*\.arch$`)

// ValidPath reports whether p is "arch/<name>.arch" with a plain name.
func ValidPath(p string) bool {
	dir, name := path.Split(p)
	return dir == Dir+"/" && fileRe.MatchString(name) && path.Clean(p) == p
}

// Load reads <root>/arch/*.arch (not recursive). A missing directory is an empty Set.
func Load(root string) (*Set, error) {
	dir := filepath.Join(root, Dir)
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return &Set{}, nil
	}
	if err != nil {
		return nil, err
	}
	texts := map[string]string{}
	for _, e := range ents {
		p := Dir + "/" + e.Name()
		if e.IsDir() || !ValidPath(p) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		texts[p] = string(b)
	}
	return FromTexts(texts), nil
}

// FromTexts parses files given as path -> text.
func FromTexts(texts map[string]string) *Set {
	s := &Set{}
	paths := make([]string, 0, len(texts))
	for p := range texts {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		doc, errs := Parse(p, texts[p])
		s.Files = append(s.Files, &File{Path: p, Text: texts[p], Doc: doc})
		s.Errors = append(s.Errors, errs...)
	}
	return s
}

// Empty reports a Set with no declarations at all.
func (s *Set) Empty() bool {
	if s == nil {
		return true
	}
	for _, f := range s.Files {
		if len(f.Doc.States)+len(f.Doc.Flows)+len(f.Doc.Journeys) > 0 {
			return false
		}
	}
	return true
}

// File returns the file with the given path, or nil.
func (s *Set) File(p string) *File {
	for _, f := range s.Files {
		if f.Path == p {
			return f
		}
	}
	return nil
}

// Clone copies the Set (documents are deep-copied).
func (s *Set) Clone() *Set {
	c := &Set{Errors: append([]Error(nil), s.Errors...)}
	for _, f := range s.Files {
		var d Doc
		b, _ := json.Marshal(f.Doc)
		_ = json.Unmarshal(b, &d)
		c.Files = append(c.Files, &File{Path: f.Path, Text: f.Text, Doc: &d})
	}
	return c
}

// Replace parses text and puts it in place of the file p (adds the file when
// new). On a syntax error the Set is not changed and the errors are returned.
func (s *Set) Replace(p, text string) []Error {
	if !ValidPath(p) {
		return []Error{{File: p, Line: 0, Msg: "path must be arch/<name>.arch"}}
	}
	doc, errs := Parse(p, text)
	if len(errs) > 0 {
		return errs
	}
	if f := s.File(p); f != nil {
		f.Doc, f.Text = doc, text
	} else {
		s.Files = append(s.Files, &File{Path: p, Text: text, Doc: doc})
		sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	}
	var kept []Error
	for _, e := range s.Errors {
		if e.File != p {
			kept = append(kept, e)
		}
	}
	s.Errors = kept
	return nil
}

// Texts renders every file in the canonical form (path -> text).
func (s *Set) Texts() map[string]string {
	out := map[string]string{}
	for _, f := range s.Files {
		out[f.Path] = Format(f.Doc)
	}
	return out
}

// Model flattens the files into one Model. A declaration seen twice (same
// entity or id) keeps the first one; Check reports the duplicate.
func (s *Set) Model() Model {
	m := Model{States: []States{}, Flows: []Flow{}, Journeys: []Journey{}}
	if s == nil {
		return m
	}
	seenS, seenF, seenJ := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range s.Files {
		for _, st := range f.Doc.States {
			if seenS[st.Entity] {
				continue
			}
			seenS[st.Entity] = true
			c := copyJSON(*st)
			c.File = f.Path
			norm(&c.States)
			norm(&c.Final)
			if c.Transitions == nil {
				c.Transitions = []Transition{}
			}
			for i := range c.Transitions {
				c.Transitions[i].Source = SourceSpec
			}
			m.States = append(m.States, c)
		}
		for _, fl := range f.Doc.Flows {
			if seenF[fl.ID] {
				continue
			}
			seenF[fl.ID] = true
			c := copyJSON(*fl)
			c.File = f.Path
			if c.Steps == nil {
				c.Steps = []Step{}
			}
			if c.States == nil {
				c.States = []StateRef{}
			}
			if c.Alts == nil {
				c.Alts = []Alt{}
			}
			m.Flows = append(m.Flows, c)
		}
		for _, j := range f.Doc.Journeys {
			if seenJ[j.ID] {
				continue
			}
			seenJ[j.ID] = true
			c := copyJSON(*j)
			c.File = f.Path
			norm(&c.Flows)
			m.Journeys = append(m.Journeys, c)
		}
	}
	return m
}

func norm(s *[]string) {
	if *s == nil {
		*s = []string{}
	}
}

func copyJSON[T any](v T) T {
	var c T
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &c)
	return c
}

// FlowIDs returns the ids of flows declared in the spec.
func (s *Set) FlowIDs() map[string]bool {
	out := map[string]bool{}
	if s == nil {
		return out
	}
	for _, f := range s.Files {
		for _, fl := range f.Doc.Flows {
			out[fl.ID] = true
		}
	}
	return out
}

// fileFor chooses the file for a new entity: arch/<prefix>.arch where prefix is
// the part of the id before the first "-", "." or "_", else arch/main.arch.
func fileFor(id string) string {
	if i := strings.IndexAny(id, "-._"); i > 0 {
		name := strings.ToLower(id[:i])
		if p := Dir + "/" + name + Ext; ValidPath(p) {
			return p
		}
	}
	return Dir + "/main" + Ext
}

func (s *Set) ensure(p string) (*File, error) {
	if !ValidPath(p) {
		return nil, fmt.Errorf("file %q: path must be arch/<name>.arch", p)
	}
	if f := s.File(p); f != nil {
		return f, nil
	}
	f := &File{Path: p, Doc: &Doc{}}
	s.Files = append(s.Files, f)
	sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	return f, nil
}
