package spec

import (
	"strings"
	"unicode/utf8"
)

func width(s string) int { return utf8.RuneCountInString(s) }

func quote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// plain writes s bare when it survives a round trip through the parser.
func plain(s string) string {
	if s == "" || strings.ContainsAny(s, "\"#\\\n\r\t") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") || strings.Contains(s, "  ") {
		return quote(s)
	}
	return s
}

const gap = "   "

// rows renders table rows: cells are padded to the column width, empty
// trailing cells are dropped, so no line ends with spaces.
func rows(table [][]string) []string {
	var w []int
	for _, r := range table {
		for i, c := range r {
			if i >= len(w) {
				w = append(w, 0)
			}
			w[i] = max(w[i], width(c))
		}
	}
	out := make([]string, len(table))
	for k, r := range table {
		last := len(r) - 1
		for last >= 0 && r[last] == "" {
			last--
		}
		var sb strings.Builder
		sb.WriteString("  ")
		for i := 0; i <= last; i++ {
			if w[i] == 0 {
				continue // nobody uses this column
			}
			sb.WriteString(r[i])
			if i < last {
				sb.WriteString(strings.Repeat(" ", w[i]-width(r[i])) + gap)
			}
		}
		out[k] = sb.String()
	}
	return out
}

// Format writes a Doc in the canonical form: states, flows, journeys in their
// order, two-space indent, aligned columns. Parse(Format(d)) equals d.
func Format(d *Doc) string {
	var blocks []string
	for _, st := range d.States {
		blocks = append(blocks, comment(st.Comment)+formatStates(st))
	}
	for _, fl := range d.Flows {
		blocks = append(blocks, comment(fl.Comment)+formatFlow(fl))
	}
	for _, j := range d.Journeys {
		blocks = append(blocks, comment(j.Comment)+formatJourney(j))
	}
	if len(d.Tail) > 0 {
		blocks = append(blocks, comment(d.Tail))
	}
	return strings.Join(blocks, "\n")
}

func comment(lines []string) string {
	var sb strings.Builder
	for _, l := range lines {
		if l == "" {
			sb.WriteString("#\n")
		} else {
			sb.WriteString("# " + l + "\n")
		}
	}
	return sb.String()
}

func formatStates(st *States) string {
	var sb strings.Builder
	sb.WriteString("states " + st.Entity + "\n")
	if len(st.States) > 0 {
		sb.WriteString("  state " + strings.Join(st.States, ", ") + "\n")
	}
	table := make([][]string, len(st.Transitions))
	for i, t := range st.Transitions {
		on, label := "", ""
		if t.On != "" {
			on = "on " + t.On
		}
		if t.Label != "" {
			label = quote(t.Label)
		}
		table[i] = []string{t.From + " -> " + t.To, on, label}
	}
	for _, l := range rows(table) {
		sb.WriteString(l + "\n")
	}
	if len(st.Final) > 0 {
		sb.WriteString("  final " + strings.Join(st.Final, ", ") + "\n")
	}
	return sb.String()
}

func formatFlow(fl *Flow) string {
	var sb strings.Builder
	sb.WriteString("flow " + fl.ID)
	if fl.Name != "" {
		sb.WriteString(" " + quote(fl.Name))
	}
	sb.WriteString("\n")
	if fl.Actor != "" {
		sb.WriteString("  actor " + plain(fl.Actor) + "\n")
	}
	table := make([][]string, len(fl.Steps))
	for i, s := range fl.Steps {
		ref, ext, text, mod := "step "+s.Ref, "", "", ""
		if s.Planned {
			ref = "step plan"
			if s.Module != "" {
				mod = "in " + s.Module
			}
		}
		if s.Ext != "" {
			ext = "ext " + s.Ext
		}
		if s.Text != "" {
			text = quote(s.Text)
		}
		table[i] = []string{ref, ext, text, mod}
	}
	for _, l := range rows(table) {
		sb.WriteString(l + "\n")
	}
	for _, r := range fl.States {
		sb.WriteString("  state " + r.Entity + ": " + r.From + " -> " + r.To + "\n")
	}
	for _, a := range fl.Alts {
		line := "  alt " + a.Cond
		if a.Flow != "" {
			line += " -> " + a.Flow
		}
		if a.Text != "" {
			line += " " + quote(a.Text)
		}
		sb.WriteString(line + "\n")
	}
	if fl.Exit != "" {
		sb.WriteString("  exit " + quote(fl.Exit) + "\n")
	}
	return sb.String()
}

func formatJourney(j *Journey) string {
	line := "journey " + j.ID
	if j.Name != "" {
		line += " " + quote(j.Name)
	}
	return line + ": " + strings.Join(j.Flows, " > ") + "\n"
}
