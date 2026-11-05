package spec

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Page edits from the viewer (`archmap serve`). Unlike Apply, which rewrites a
// whole file in the canonical form, a page edit works on the text: it touches
// only the lines of one page block (its header, its frame lines) and keeps the
// rest of the file — other blocks, comments, blank lines, end-of-line comments —
// byte for byte. The frame lines of the edited page are re-aligned (the values
// in one column), nothing else is.

// PageOp is one edit of a page, sent by the viewer as JSON.
//
//	create        name                       new arch/pages/<id>.arch, id from the name (latin)
//	rename        page, name                 the id (and the file) stay
//	delete        page                       the block; the file too when nothing else is left in it
//	add           page, kind, refs           journey/flow/states/module: a line per ref;
//	                                         tables/api: into the existing frame of the kind when there is one
//	add_note      page, text
//	set_note      page, index, text
//	remove_frame  page, index
//	remove_ref    page, index, refs          drops refs from a frame (the frame goes with the last one)
//	move_frame    page, index, to            to: the index in the list without the moved frame
//	restore       files                      undo / redo: puts texts back when the disk still has `expect`
//
// Expect, when set, is the page as the viewer shows it: an edit of a page that
// was changed on disk meanwhile is refused (ErrConflict) rather than applied to
// lines the user has not seen.
type PageOp struct {
	Op     string        `json:"op"`
	Page   string        `json:"page,omitempty"`
	Name   string        `json:"name,omitempty"`
	Kind   string        `json:"kind,omitempty"`
	Refs   []string      `json:"refs,omitempty"`
	Text   string        `json:"text,omitempty"`
	Index  *int          `json:"index,omitempty"`
	To     *int          `json:"to,omitempty"`
	Expect *PageView     `json:"expect,omitempty"`
	Files  []RestoreFile `json:"files,omitempty"`
}

// PageView is what the viewer shows of a page: its name and frames.
type PageView struct {
	Name   string      `json:"name"`
	Frames []FrameView `json:"frames"`
}

// FrameView is a frame as the viewer shows it.
type FrameView struct {
	Kind string   `json:"kind"`
	Refs []string `json:"refs,omitempty"`
	Text string   `json:"text,omitempty"`
}

// RestoreFile puts Text back into Path (nil: the file is removed) when the file
// on disk is still Expect (nil: there is no file).
type RestoreFile struct {
	Path   string  `json:"path"`
	Text   *string `json:"text"`
	Expect *string `json:"expect"`
}

// FileEdit is one file a page edit changes: Before is the text it had (nil:
// there was no file), After the new one (nil: the file goes).
type FileEdit struct {
	Path   string  `json:"path"`
	Before *string `json:"before"`
	After  *string `json:"after"`
}

// PageResult is what a page edit does: the files to write, the page it is
// about and, for an edit with nothing to do (a ref already on the page), a
// notice instead of edits.
type PageResult struct {
	Edits  []FileEdit `json:"edits"`
	Page   string     `json:"page,omitempty"`
	Notice string     `json:"notice,omitempty"`
}

// ErrConflict: the page (or a file of a restore) is not what the viewer saw.
var ErrConflict = errors.New("changed on disk")

// ConflictError says what changed.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }
func (e *ConflictError) Unwrap() error { return ErrConflict }

// NoticeAlready: everything dropped is on the page already.
const NoticeAlready = "already"

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "i", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "",
	'э': "e", 'ю': "iu", 'я': "ia",
	// Kazakh
	'ә': "a", 'ғ': "g", 'қ': "q", 'ң': "n", 'ө': "o", 'ұ': "u", 'ү': "u", 'һ': "h", 'і': "i",
	// Ukrainian / Belarusian
	'є': "ie", 'ї': "i", 'ґ': "g", 'ў': "u",
}

// PageIDFor makes the id of a new page from its name: the Slug of the name
// with Cyrillic letters spelled in Latin and any other non-Latin letter
// dropped, so that the id is also a valid file name (arch/pages/<id>.arch).
// The name stays as typed: `page otmena-zakaza "Отмена заказа"`.
func PageIDFor(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if s, ok := translit[r]; ok {
			sb.WriteString(s)
			continue
		}
		if r < 128 {
			sb.WriteRune(r)
			continue
		}
		sb.WriteByte(' ')
	}
	id := Slug(sb.String())
	id = strings.Trim(id, "_")
	if len(id) > 60 {
		id = strings.TrimRight(id[:60], "-_")
	}
	return id
}

func pageNameErr(name string) error {
	if strings.TrimSpace(name) == "" {
		return bad("the page needs a name")
	}
	if strings.ContainsAny(name, "\r\n") {
		return bad("a page name is one line")
	}
	if width(name) > 120 {
		return bad("the page name is too long (120 characters at most)")
	}
	return nil
}

// findPage returns the file and the parsed page with the id.
func (s *Set) findPage(id string) (*File, *Page) {
	for _, f := range s.Files {
		for _, pg := range f.Doc.Pages {
			if pg.ID == id {
				return f, pg
			}
		}
	}
	return nil, nil
}

func (s *Set) pageNamed(name, except string) *Page {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, f := range s.Files {
		for _, pg := range f.Doc.Pages {
			if pg.ID != except && strings.ToLower(strings.TrimSpace(pg.Name)) == n {
				return pg
			}
		}
	}
	return nil
}

func viewOf(pg *Page) PageView {
	v := PageView{Name: pg.Name, Frames: []FrameView{}}
	for _, fr := range pg.Frames {
		v.Frames = append(v.Frames, FrameView{Kind: fr.Kind, Refs: fr.Refs, Text: fr.Text})
	}
	return v
}

func sameView(a, b PageView) bool {
	if a.Name != b.Name || len(a.Frames) != len(b.Frames) {
		return false
	}
	for i := range a.Frames {
		x, y := a.Frames[i], b.Frames[i]
		if x.Kind != y.Kind || x.Text != y.Text || !slices.Equal(x.Refs, y.Refs) {
			return false
		}
	}
	return true
}

func strp(s string) *string { return &s }

// EditPage computes the files a page edit changes. It does not write and does
// not change s; the caller writes the edits and checks the references.
func (s *Set) EditPage(op PageOp) (PageResult, error) {
	switch op.Op {
	case "create":
		return s.createPage(op)
	case "restore":
		return s.restore(op)
	}
	f, pg := s.findPage(op.Page)
	if pg == nil {
		return PageResult{}, &ConflictError{fmt.Sprintf("page %q: no such page (deleted or renamed on disk?)", op.Page)}
	}
	for _, e := range s.Errors {
		if e.File == f.Path {
			return PageResult{}, bad("%s has a syntax error (%s); fix the text first", f.Path, e)
		}
	}
	if op.Expect != nil && !sameView(*op.Expect, viewOf(pg)) {
		return PageResult{}, &ConflictError{fmt.Sprintf("page %q was changed in %s meanwhile", pg.ID, f.Path)}
	}
	ed := newBlockEdit(f, pg)
	res := PageResult{Page: pg.ID}
	index := func() (int, error) {
		if op.Index == nil || *op.Index < 0 || *op.Index >= len(pg.Frames) {
			return 0, bad("page %q: no frame with this index", pg.ID)
		}
		return *op.Index, nil
	}
	switch op.Op {
	case "rename":
		name := strings.TrimSpace(op.Name)
		if err := pageNameErr(name); err != nil {
			return res, err
		}
		if o := s.pageNamed(name, pg.ID); o != nil {
			return res, bad("a page named %q already exists (%s)", o.Name, o.ID)
		}
		if name == pg.Name {
			res.Notice = "same"
			return res, nil
		}
		ed.setHeader(pageHeader(pg.ID, name))
	case "delete":
		after := ed.deleteBlock()
		var a *string
		if strings.TrimSpace(after) != "" {
			a = strp(after)
		}
		res.Edits = []FileEdit{{Path: f.Path, Before: strp(f.Text), After: a}}
		return res, nil
	case "add":
		if err := s.addFrames(ed, pg, op.Kind, op.Refs, &res); err != nil || res.Notice != "" {
			return res, err
		}
	case "add_note":
		if err := noteErr(op.Text); err != nil {
			return res, err
		}
		ed.insert(len(pg.Frames), Frame{Kind: FrameNote, Text: op.Text})
	case "set_note":
		i, err := index()
		if err != nil {
			return res, err
		}
		if pg.Frames[i].Kind != FrameNote {
			return res, bad("frame %d of page %q is not a note", i, pg.ID)
		}
		if err := noteErr(op.Text); err != nil {
			return res, err
		}
		if op.Text == pg.Frames[i].Text {
			res.Notice = "same"
			return res, nil
		}
		ed.replace(i, Frame{Kind: FrameNote, Text: op.Text})
	case "remove_frame":
		i, err := index()
		if err != nil {
			return res, err
		}
		ed.remove(i)
	case "remove_ref":
		i, err := index()
		if err != nil {
			return res, err
		}
		fr := pg.Frames[i]
		keep := slices.DeleteFunc(slices.Clone(fr.Refs), func(r string) bool { return slices.Contains(op.Refs, r) })
		switch {
		case len(keep) == len(fr.Refs):
			return res, bad("frame %d of page %q has none of %v", i, pg.ID, op.Refs)
		case len(keep) == 0:
			ed.remove(i)
		default:
			ed.replace(i, Frame{Kind: fr.Kind, Refs: keep})
		}
	case "move_frame":
		i, err := index()
		if err != nil {
			return res, err
		}
		if op.To == nil || *op.To < 0 || *op.To > len(pg.Frames)-1 {
			return res, bad("page %q: bad target position", pg.ID)
		}
		if *op.To == i {
			res.Notice = "same"
			return res, nil
		}
		ed.move(i, *op.To)
	default:
		return res, bad("unknown page operation %q", op.Op)
	}
	text, err := ed.finish()
	if err != nil {
		return res, err
	}
	res.Edits = []FileEdit{{Path: f.Path, Before: strp(f.Text), After: strp(text)}}
	return res, nil
}

func noteErr(t string) error {
	if strings.TrimSpace(t) == "" {
		return bad("the note is empty")
	}
	if strings.Contains(t, "\r") {
		return bad("note: carriage return is not allowed")
	}
	return nil
}

func validRef(kind, r string) bool {
	switch kind {
	case FrameJourney, FrameFlow, FrameStates, FrameTables:
		return isIdent(r, false)
	case FrameAPI:
		r = strings.TrimSpace(r)
		return r != "" && !strings.ContainsAny(r, "\"\r\n") && (isEndpointRef(r) || strings.Contains(r, " "))
	case FrameModule:
		return isModule(r)
	}
	return false
}

// addFrames: journey / flow / states / module get a line per ref, tables and
// api one frame per kind (refs join the existing one). Refs already on the page
// (in any frame of the kind) are skipped; nothing new gives NoticeAlready.
func (s *Set) addFrames(ed *blockEdit, pg *Page, kind string, refs []string, res *PageResult) error {
	if !isFrameKind(kind) || kind == FrameNote {
		return bad("cannot put %q on a page (journey, flow, states, tables, api or module)", kind)
	}
	if len(refs) == 0 {
		return bad("nothing to add")
	}
	have := map[string]bool{}
	for _, fr := range pg.Frames {
		if fr.Kind == kind {
			for _, r := range fr.Refs {
				have[r] = true
			}
		}
	}
	var add []string
	for _, r := range refs {
		r = strings.TrimSpace(r)
		if !validRef(kind, r) {
			return bad("%q is not a valid %s reference", r, kind)
		}
		if !have[r] {
			have[r] = true
			add = append(add, r)
		}
	}
	if len(add) == 0 {
		res.Notice = NoticeAlready
		return nil
	}
	if kind == FrameTables || kind == FrameAPI {
		for i, fr := range pg.Frames {
			if fr.Kind == kind {
				ed.replace(i, Frame{Kind: kind, Refs: append(slices.Clone(fr.Refs), add...)})
				return nil
			}
		}
		ed.insert(len(pg.Frames), Frame{Kind: kind, Refs: add})
		return nil
	}
	for _, r := range add {
		ed.insert(len(ed.frames), Frame{Kind: kind, Refs: []string{r}})
	}
	return nil
}

func (s *Set) createPage(op PageOp) (PageResult, error) {
	name := strings.TrimSpace(op.Name)
	if err := pageNameErr(name); err != nil {
		return PageResult{}, err
	}
	if o := s.pageNamed(name, ""); o != nil {
		return PageResult{}, bad("a page named %q already exists (%s)", o.Name, o.ID)
	}
	base := PageIDFor(name)
	if base == "" {
		base = "page"
	}
	id := base
	for n := 2; ; n++ {
		_, pg := s.findPage(id)
		if pg == nil && s.File(PagesDir+"/"+id+Ext) == nil {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	p := PagesDir + "/" + id + Ext
	if !ValidPath(p) || !isIdent(id, false) {
		return PageResult{}, bad("cannot make a file name from %q", name)
	}
	text := pageHeader(id, name) + "\n"
	return PageResult{Page: id, Edits: []FileEdit{{Path: p, After: strp(text)}}}, nil
}

func (s *Set) restore(op PageOp) (PageResult, error) {
	if len(op.Files) == 0 {
		return PageResult{}, bad("nothing to restore")
	}
	var res PageResult
	for _, rf := range op.Files {
		if !ValidPath(rf.Path) {
			return res, bad("path %q must be arch/<name>.arch or arch/pages/<name>.arch", rf.Path)
		}
		f := s.File(rf.Path)
		var cur *string
		if f != nil {
			cur = strp(f.Text)
		}
		if (cur == nil) != (rf.Expect == nil) || (cur != nil && *cur != *rf.Expect) {
			return res, &ConflictError{rf.Path + " was changed on disk meanwhile; not undone"}
		}
		if rf.Text != nil {
			doc, errs := Parse(rf.Path, *rf.Text)
			if len(errs) > 0 {
				return res, bad("%s", errs[0])
			}
			// outside arch/pages only a file that is there, or one with a page, is written
			if path.Dir(rf.Path) != PagesDir && f == nil && len(doc.Pages) == 0 {
				return res, bad("%s: only page files are restored", rf.Path)
			}
		}
		res.Edits = append(res.Edits, FileEdit{Path: rf.Path, Before: cur, After: rf.Text})
	}
	return res, nil
}

/* ---------- editing the lines of one block ---------- */

// blockEdit edits the lines of one page block. Frame lines are tracked by
// their position in lines; lines between them (blank, indented comments) stay
// where they are.
type blockEdit struct {
	file   string
	lines  []string
	header int   // index of the `page` line
	pos    []int // index in lines of every frame line, in frame order
	frames []Frame
	dirty  []bool
	indent string
	err    error
}

func newBlockEdit(f *File, pg *Page) *blockEdit {
	e := &blockEdit{file: f.Path, lines: strings.Split(f.Text, "\n"), header: pg.Line - 1, indent: "  "}
	for _, fr := range pg.Frames {
		e.pos = append(e.pos, fr.Line-1)
		e.frames = append(e.frames, fr)
		e.dirty = append(e.dirty, false)
	}
	if len(e.pos) > 0 {
		l := e.lines[e.pos[0]]
		e.indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
	}
	return e
}

// splitComment cuts an end-of-line comment off a line (a `#` outside quotes).
func splitComment(line string) (code, comment string) {
	in := false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case in && c == '\\':
			i++
		case c == '"':
			in = !in
		case !in && c == '#':
			code = strings.TrimRight(line[:i], " \t")
			return code, line[len(code):] // the comment with the spaces before it
		}
	}
	return strings.TrimRight(line, " \t\r"), ""
}

func withComment(code, comment string) string {
	if comment == "" {
		return code
	}
	if comment[0] == '#' {
		return code + " " + comment
	}
	return code + comment
}

func (e *blockEdit) setHeader(h string) {
	_, c := splitComment(e.lines[e.header])
	e.lines[e.header] = withComment(h, c)
}

// shift moves every tracked position at or after at by d.
func (e *blockEdit) shift(at, d int) {
	for k := range e.pos {
		if e.pos[k] >= at {
			e.pos[k] += d
		}
	}
}

// insert puts a new frame at frame index i (only at the end is used: i ==
// len(frames)): its line goes after the last frame line, or after the header.
func (e *blockEdit) insert(i int, fr Frame) {
	at := e.header + 1
	if len(e.pos) > 0 {
		at = e.pos[len(e.pos)-1] + 1
	}
	e.lines = slices.Insert(e.lines, at, "")
	e.shift(at, 1)
	e.pos = slices.Insert(e.pos, i, at)
	e.frames = slices.Insert(e.frames, i, fr)
	e.dirty = slices.Insert(e.dirty, i, true)
}

func (e *blockEdit) replace(i int, fr Frame) {
	e.frames[i] = fr
	e.dirty[i] = true
}

func (e *blockEdit) remove(i int) {
	at := e.pos[i]
	e.lines = slices.Delete(e.lines, at, at+1)
	e.pos = slices.Delete(e.pos, i, i+1)
	e.frames = slices.Delete(e.frames, i, i+1)
	e.dirty = slices.Delete(e.dirty, i, i+1)
	e.shift(at+1, -1)
}

// move takes frame i out and puts it at index to of the remaining list; the
// line goes right before the line of the frame now at to (after the last
// frame line when to is the end).
func (e *blockEdit) move(i, to int) {
	line, fr, d := e.lines[e.pos[i]], e.frames[i], e.dirty[i]
	e.remove(i)
	at := e.header + 1
	switch {
	case to < len(e.pos):
		at = e.pos[to]
	case len(e.pos) > 0:
		at = e.pos[len(e.pos)-1] + 1
	}
	e.lines = slices.Insert(e.lines, at, line)
	e.shift(at, 1)
	e.pos = slices.Insert(e.pos, to, at)
	e.frames = slices.Insert(e.frames, to, fr)
	e.dirty = slices.Insert(e.dirty, to, d)
}

var frameLineRe = regexp.MustCompile(`^([ \t]+)(\S+)[ \t]+(.*)$`)

// finish writes the changed frame lines, re-aligns the values of all frame
// lines of the block and checks that the file still parses.
func (e *blockEdit) finish() (string, error) {
	w := framesWidth(e.frames)
	for k, fr := range e.frames {
		pad := strings.Repeat(" ", w-width(fr.Kind)+1)
		at := e.pos[k]
		if e.dirty[k] {
			_, c := splitComment(e.lines[at])
			e.lines[at] = withComment(e.indent+fr.Kind+pad+frameValue(fr), c)
			continue
		}
		if m := frameLineRe.FindStringSubmatch(e.lines[at]); m != nil && m[2] == fr.Kind {
			e.lines[at] = m[1] + m[2] + pad + m[3]
		}
	}
	text := strings.Join(e.lines, "\n")
	if _, errs := Parse(e.file, text); len(errs) > 0 {
		return "", bad("the edit would break %s: %s", e.file, errs[0])
	}
	return text, nil
}

// deleteBlock removes the page: its comment lines right above, the header,
// the frame lines and the lines between them; one blank line around it goes too.
func (e *blockEdit) deleteBlock() string {
	start, end := e.header, e.header
	if len(e.pos) > 0 {
		end = e.pos[len(e.pos)-1]
	}
	for start > 0 && strings.HasPrefix(e.lines[start-1], "#") {
		start--
	}
	end++
	blank := func(i int) bool { return i >= 0 && i < len(e.lines) && strings.TrimSpace(e.lines[i]) == "" }
	if blank(end) && (start == 0 || blank(start-1)) {
		end++
	}
	lines := slices.Delete(slices.Clone(e.lines), start, end)
	return strings.Join(lines, "\n")
}
