package extract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// DataUse is the aggregated access to one table by everything a function reaches.
type DataUse struct {
	Table  string
	Ops    []string // sorted distinct ops: delete, insert, read, update, upsert
	Fields []string // sorted distinct written fields
	Depth  int      // shortest call distance from the start function (0 = the function itself)
	Via    string   // function that touches the table at that distance
}

// Reads reports whether the table is read.
func (u DataUse) Reads() bool { return hasStr(u.Ops, "read") }

// Writes reports whether the table is written (insert, update, delete or upsert).
func (u DataUse) Writes() bool {
	for _, o := range u.Ops {
		if o != "read" {
			return true
		}
	}
	return false
}

func hasStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// DataClosure aggregates the data access of fid and of everything it calls
// (synchronous callees and async_callees) down to maxDepth levels (6 if <= 0).
func DataClosure(f *facts.Facts, fid string, maxDepth int) []DataUse {
	if maxDepth <= 0 {
		maxDepth = 6
	}
	type acc struct {
		ops    map[string]bool
		fields map[string]bool
		depth  int
		via    string
	}
	got := map[string]*acc{}
	dist := map[string]int{fid: 0}
	queue := []string{fid}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		fn, ok := f.Functions[id]
		if !ok {
			continue
		}
		d := dist[id]
		for _, a := range fn.Data {
			x := got[a.Table]
			if x == nil {
				x = &acc{ops: map[string]bool{}, fields: map[string]bool{}, depth: d, via: id}
				got[a.Table] = x
			}
			x.ops[a.Op] = true
			if a.Op != "read" {
				for _, fl := range a.Fields {
					x.fields[fl] = true
				}
			}
		}
		if d >= maxDepth {
			continue
		}
		for _, next := range [][]string{fn.Callees, fn.AsyncCallees} {
			for _, c := range next {
				if _, seen := dist[c]; !seen {
					dist[c] = d + 1
					queue = append(queue, c)
				}
			}
		}
	}
	out := make([]DataUse, 0, len(got))
	for t, x := range got {
		u := DataUse{Table: t, Depth: x.depth, Via: x.via}
		for o := range x.ops {
			u.Ops = append(u.Ops, o)
		}
		for fl := range x.fields {
			u.Fields = append(u.Fields, fl)
		}
		sort.Strings(u.Ops)
		sort.Strings(u.Fields)
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Table < out[j].Table })
	return out
}

func rw(u DataUse) string {
	s := ""
	if u.Reads() {
		s += "R"
	}
	if u.Writes() {
		s += "W"
	}
	return s
}

// endpointMatrix renders the top endpoints by number of tables touched,
// as `endpoint -> table:R/W/RW` lines.
func endpointMatrix(f *facts.Facts, top int) string {
	type row struct {
		label string
		uses  []DataUse
	}
	var rows []row
	for _, e := range f.Endpoints {
		if e.Fid == "" || e.Kind == "task" {
			continue
		}
		uses := DataClosure(f, e.Fid, 6)
		if len(uses) == 0 {
			continue
		}
		label := e.Method + " " + e.Path
		if e.Kind == "grpc" && e.Method == "" || e.Path == "" {
			label = e.Service + "/" + e.Name
		}
		rows = append(rows, row{strings.TrimSpace(label), uses})
	}
	if len(rows) == 0 {
		return ""
	}
	sort.SliceStable(rows, func(i, j int) bool { return len(rows[i].uses) > len(rows[j].uses) })
	if len(rows) > top {
		rows = rows[:top]
	}
	var b strings.Builder
	for _, r := range rows {
		parts := make([]string, len(r.uses))
		for i, u := range r.uses {
			parts[i] = u.Table + ":" + rw(u)
		}
		fmt.Fprintf(&b, "  %s  →  %s\n", r.label, strings.Join(parts, ", "))
	}
	return b.String()
}
