package autosem

import "github.com/aidarbn/archmap/internal/facts"

// Layers returns the layer of every module of f (see the Layer constants), the
// same classification the semantic model uses.
func Layers(f *facts.Facts) map[string]string {
	stats := collectStats(f)
	out := map[string]string{}
	for p := range f.Modules {
		out[p] = classify(p, stats[p])
	}
	for p := range stats {
		if _, ok := out[p]; !ok {
			out[p] = classify(p, stats[p])
		}
	}
	return out
}
