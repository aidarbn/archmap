package extract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Summary renders the short stdout summary printed by `archmap extract`
// (same text as the Python original).
func Summary(f *facts.Facts) string {
	ids := make([]string, 0, len(f.Functions))
	for id := range f.Functions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := f.Functions[ids[i]], f.Functions[ids[j]]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return ids[i] < ids[j]
	})
	byMod := map[string]int{}
	indeg := map[string]int{}
	var firstSeen []string
	edges := 0
	added, modified := 0, 0
	for _, id := range ids {
		fn := f.Functions[id]
		byMod[fn.Module]++
		for _, c := range fn.Callees {
			if indeg[c] == 0 {
				firstSeen = append(firstSeen, c)
			}
			indeg[c]++
			edges++
		}
		switch fn.Status {
		case facts.Added:
			added++
		case facts.Modified:
			modified++
		}
	}
	base := "—"
	if f.Base != nil {
		base = *f.Base
	}
	var langKeys []string
	for k := range f.Langs {
		langKeys = append(langKeys, k)
	}
	sort.Strings(langKeys)
	var lp []string
	for _, k := range langKeys {
		lp = append(lp, fmt.Sprintf("'%s': %d", k, f.Langs[k]))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "repo %s @ %s (%s), база: %s\n", f.Repo, f.Branch, f.Head, base)
	fmt.Fprintf(&b, "языки: {%s}; модулей %d, функций %d, рёбер вызовов %d\n", strings.Join(lp, ", "), len(f.Modules), len(f.Functions), edges)
	if len(f.Unparsed) > 0 {
		var ue []string
		n := 0
		for k, c := range f.Unparsed {
			ue = append(ue, k)
			n += c
		}
		sort.Slice(ue, func(i, j int) bool {
			if f.Unparsed[ue[i]] != f.Unparsed[ue[j]] {
				return f.Unparsed[ue[i]] > f.Unparsed[ue[j]]
			}
			return ue[i] < ue[j]
		})
		if len(ue) > 8 {
			ue = ue[:8]
		}
		fmt.Fprintf(&b, "без разбора: %d файлов (%s)\n", n, strings.Join(ue, ", "))
	}
	fmt.Fprintf(&b, "изменения: добавлено %d, изменено %d, удалено %d\n", added, modified, len(f.Deleted))
	b.WriteString("\nмодули (функций / импортирует):\n")
	mods := make([]string, 0, len(byMod))
	for m := range byMod {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		imps := f.Modules[m].Imports
		if len(imps) > 6 {
			imps = imps[:6]
		}
		fmt.Fprintf(&b, "  %s  %d  → %s\n", m, byMod[m], strings.Join(imps, ", "))
	}
	var ents []string
	for _, id := range ids {
		if f.Functions[id].Entry != nil {
			ents = append(ents, id)
		}
	}
	fmt.Fprintf(&b, "\nточки входа (%d):\n", len(ents))
	for i, id := range ents {
		if i == 60 {
			break
		}
		e := f.Functions[id].Entry
		labels := e.Labels
		if len(labels) > 2 {
			labels = labels[:2]
		}
		fmt.Fprintf(&b, "  [%s] %s  →  %s\n", e.Kind, strings.Join(labels, ", "), id)
	}
	b.WriteString("\nсамые вызываемые:\n")
	sort.SliceStable(firstSeen, func(i, j int) bool { return indeg[firstSeen[i]] > indeg[firstSeen[j]] })
	for i, id := range firstSeen {
		if i == 15 {
			break
		}
		fmt.Fprintf(&b, "  %3d  %s\n", indeg[id], id)
	}
	return strings.TrimRight(b.String(), "\n")
}
