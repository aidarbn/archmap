package diagdiff

import (
	"strings"

	"github.com/aidarbn/archmap/internal/spec"
)

// KindPage is a page (`page` in arch/pages/*.arch): a set of diagrams.
const KindPage = "page"

// Page changes.
const (
	FrameAdded   = "frame-added"   // a frame (one line of the page) appeared
	FrameRemoved = "frame-removed" // a frame is gone
	PageRenamed  = "page-renamed"  // the page has a new name
)

// BrokenPageRef: a page frame names something the code does not have.
const BrokenPageRef = "page-ref"

// Page is a page as one revision declares it. Frames are the lines of the
// page as "kind ref, ref" ("tables orders, payments"), notes as `note "…"`.
type Page struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Frames []string `json:"frames"`
}

// frameText is the line of a frame without the alignment of the file.
func frameText(fr spec.Frame) string {
	if fr.Kind == spec.FrameNote {
		return fr.Kind + " \"" + fr.Text + "\""
	}
	return fr.Kind + " " + strings.Join(fr.Refs, ", ")
}

// buildPages adds the pages of the checked spec to the snapshot, and a broken
// entry for every reference that points nowhere.
func (s *Snapshot) buildPages(m *spec.Model) {
	if m == nil {
		return
	}
	for _, pg := range m.Pages {
		if _, dup := s.Pages[pg.ID]; dup {
			continue
		}
		p := &Page{ID: pg.ID, Name: pg.Name, File: pg.File, Line: pg.Line, Frames: []string{}}
		for _, fr := range pg.Frames {
			p.Frames = append(p.Frames, frameText(fr))
			for _, r := range fr.Missing {
				s.Broken = append(s.Broken, Broken{Kind: BrokenPageRef, Diagram: pg.ID, Ref: fr.Kind + " " + r, File: pg.File, Line: fr.Line})
			}
		}
		s.Pages[pg.ID] = p
		s.pageOrder = append(s.pageOrder, pg.ID)
	}
}

func comparePages(base, head *Snapshot) []Diff {
	var out []Diff
	for _, id := range order(head.pageOrder, base.pageOrder, head.Pages, base.Pages) {
		b, h := base.Pages[id], head.Pages[id]
		switch {
		case b != nil && h != nil:
			var ch []Change
			if b.Name != h.Name {
				ch = append(ch, Change{Code: PageRenamed, Old: b.Name, Text: h.Name})
			}
			hs, bs := set(h.Frames), set(b.Frames)
			for _, f := range b.Frames {
				if !hs[f] {
					ch = append(ch, Change{Code: FrameRemoved, Text: f})
				}
			}
			for _, f := range h.Frames {
				if !bs[f] {
					ch = append(ch, Change{Code: FrameAdded, Text: f})
				}
			}
			if len(ch) > 0 {
				out = append(out, Diff{Kind: KindPage, ID: id, Name: h.Name, Status: Changed, Changes: ch})
			}
		case b != nil:
			out = append(out, Diff{Kind: KindPage, ID: id, Name: b.Name, Status: Removed, Reason: ReasonSpecRemoved})
		case h != nil:
			d := Diff{Kind: KindPage, ID: id, Name: h.Name, Status: Added}
			for _, f := range h.Frames {
				d.Changes = append(d.Changes, Change{Code: FrameAdded, Text: f})
			}
			out = append(out, d)
		}
	}
	return out
}
