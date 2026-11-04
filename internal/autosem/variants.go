package autosem

import (
	"strconv"
	"strings"
)

// MaxVariants is the most variants listed for one flow, the main one included.
const MaxVariants = 12

// Choice is a decision at a fork: the step and the condition taken there.
type Choice struct {
	At   string `json:"at"` // fid of the step with the `arch:on`
	Cond string `json:"cond"`
}

// VariantEnd is how a variant ends.
type VariantEnd struct {
	Kind string `json:"kind"` // "flow" (continues in another flow) | "exit" (the main path ends) | "text" (an outcome)
	Flow string `json:"flow,omitempty"`
	Text string `json:"text,omitempty"`
}

// Variant is one way through a declared flow: the steps up to a fork and the
// outcome chosen there. Every `arch:on` ends its variant, so a variant has at
// most one choice; the main variant has none and runs through all the steps.
// Variants of a flow that has more than MaxVariants outcomes are folded: those
// with the same end become one variant with Any set, whose choices are
// alternatives (any of them leads to that end).
type Variant struct {
	ID      string     `json:"id"` // "<flow id>#<n>"; #0 is the main variant
	Name    string     `json:"name"`
	Main    bool       `json:"main,omitempty"`
	Any     bool       `json:"any,omitempty"`
	Choices []Choice   `json:"choices"`
	Steps   []string   `json:"steps"`
	End     VariantEnd `json:"end"`
}

// buildVariants lists the variants of every declared flow with forks (`arch:on`).
// Flows without forks, and automatic flows, get none.
func buildVariants(flows []Flow) {
	names := map[string]string{}
	for _, fl := range flows {
		if fl.ID != "" {
			if _, ok := names[fl.ID]; !ok {
				names[fl.ID] = fl.Name
			}
		}
	}
	for i := range flows {
		fl := &flows[i]
		if len(fl.Branches) == 0 || len(fl.Steps) == 0 {
			continue
		}
		pos := map[string]int{}
		for k, s := range fl.Steps {
			pos[s] = k
		}
		main := Variant{ID: fl.ID + "#0", Name: "main path", Main: true, Choices: []Choice{}, Steps: fl.Steps, End: VariantEnd{Kind: "exit", Text: fl.Exit}}
		vs := []Variant{main}
		var rest []Variant
		// forks in step order, outcomes in markup order
		for _, s := range fl.Steps {
			for _, b := range fl.Branches {
				if b.At != s {
					continue
				}
				v := Variant{Choices: []Choice{{At: b.At, Cond: b.Cond}}, Steps: fl.Steps[:pos[s]+1]}
				target := b.Text
				if b.Flow != "" {
					v.End = VariantEnd{Kind: "flow", Flow: b.Flow, Text: b.Text}
					if target == "" {
						target = names[b.Flow]
						if target == "" {
							target = b.Flow
						}
					}
				} else {
					v.End = VariantEnd{Kind: "text", Text: b.Text}
				}
				v.Name = b.Cond + " → " + target
				rest = append(rest, v)
			}
		}
		if len(rest)+1 > MaxVariants {
			rest = foldVariants(rest)
		}
		vs = append(vs, rest...)
		if len(vs) > MaxVariants {
			fl.VariantsMore = len(vs) - MaxVariants
			vs = vs[:MaxVariants]
		}
		for k := range vs {
			vs[k].ID = fl.ID + "#" + strconv.Itoa(k)
		}
		fl.Variants = vs
	}
}

// foldVariants merges variants that end the same way into one with alternative choices.
func foldVariants(vs []Variant) []Variant {
	var out []Variant
	at := map[VariantEnd]int{}
	for _, v := range vs {
		k, ok := at[v.End]
		if !ok {
			at[v.End] = len(out)
			out = append(out, v)
			continue
		}
		o := &out[k]
		o.Any = true
		o.Choices = append(o.Choices, v.Choices...)
		o.Steps = longer(o.Steps, v.Steps)
	}
	for i := range out {
		if !out[i].Any {
			continue
		}
		conds := make([]string, len(out[i].Choices))
		for k, c := range out[i].Choices {
			conds[k] = c.Cond
		}
		_, target, _ := strings.Cut(out[i].Name, " → ")
		out[i].Name = strings.Join(conds, " | ") + " → " + target
	}
	return out
}

// longer returns the longer of two step lists, which share a prefix (both are
// prefixes of the same flow).
func longer(a, b []string) []string {
	if len(b) > len(a) {
		return b
	}
	return a
}
