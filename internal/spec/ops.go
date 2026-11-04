package spec

import (
	"fmt"
	"slices"
	"strings"
)

// Op is one edit of the specification, sent by the editor as JSON
// ({"op": "...", ...}). Fields that an operation does not use are ignored.
//
//	add_state          entity, state [, file]
//	remove_state       entity, state                   (also removes its transitions)
//	rename_state       entity, from, to
//	set_final          entity, state, final            (true/false)
//	add_transition     entity, from, to [, on, label, file]
//	remove_transition  entity, from, to [, on]
//	remove_states      entity                          (the whole machine)
//	add_flow           id [, name, actor, file]
//	remove_flow        flow
//	rename_flow        flow, to                        (also in journeys and alts)
//	set_name           flow | journey, name
//	set_actor          flow, actor
//	set_exit           flow, exit
//	add_step           flow, (ref | planned), [text, ext, index]   index -1 or absent: at the end
//	set_step           flow, index, ref, planned, text, ext        (replaces the step)
//	remove_step        flow, index
//	move_step          flow, index, to_index
//	add_alt            flow, cond [, target, text]
//	remove_alt         flow, index
//	add_journey        id [, name, flows, file]
//	set_journey_flows  journey, flows
//	remove_journey     journey
type Op struct {
	Op     string   `json:"op"`
	File   string   `json:"file,omitempty"`
	Entity string   `json:"entity,omitempty"`
	State  string   `json:"state,omitempty"`
	From   string   `json:"from,omitempty"`
	To     string   `json:"to,omitempty"`
	On     string   `json:"on,omitempty"`
	Label  string   `json:"label,omitempty"`
	Final  *bool    `json:"final,omitempty"`
	ID     string   `json:"id,omitempty"`
	Flow   string   `json:"flow,omitempty"`
	Name   *string  `json:"name,omitempty"`
	Actor  *string  `json:"actor,omitempty"`
	Exit   *string  `json:"exit,omitempty"`
	Ref    string   `json:"ref,omitempty"`
	Ext    string   `json:"ext,omitempty"`
	Text   string   `json:"text,omitempty"`
	Plan   bool     `json:"planned,omitempty"`
	Index  *int     `json:"index,omitempty"`
	ToIdx  *int     `json:"to_index,omitempty"`
	Cond   string   `json:"cond,omitempty"`
	Target string   `json:"target,omitempty"`
	Jrny   string   `json:"journey,omitempty"`
	Flows  []string `json:"flows,omitempty"`
}

// OpError is a rejected operation.
type OpError struct{ Msg string }

func (e *OpError) Error() string { return e.Msg }

func bad(format string, a ...any) error { return &OpError{fmt.Sprintf(format, a...)} }

// Apply performs ops on the Set in order. It returns the paths of the files that
// changed. On the first error nothing is kept: s stays as it was.
func (s *Set) Apply(ops []Op) ([]string, error) {
	if len(s.Errors) > 0 {
		return nil, bad("the spec has syntax errors (%s); fix them first", s.Errors[0])
	}
	work := s.Clone()
	touched := map[string]bool{}
	for i, op := range ops {
		files, err := work.apply(op)
		if err != nil {
			if len(ops) > 1 {
				return nil, bad("op %d (%s): %s", i+1, op.Op, err)
			}
			return nil, err
		}
		for _, f := range files {
			touched[f] = true
		}
	}
	*s = *work
	out := make([]string, 0, len(touched))
	for p := range touched {
		out = append(out, p)
	}
	slices.Sort(out)
	return out, nil
}

func (s *Set) findStates(entity string) (*File, *States) {
	for _, f := range s.Files {
		for _, st := range f.Doc.States {
			if st.Entity == entity {
				return f, st
			}
		}
	}
	return nil, nil
}

func (s *Set) findFlow(id string) (*File, *Flow) {
	for _, f := range s.Files {
		for _, fl := range f.Doc.Flows {
			if fl.ID == id {
				return f, fl
			}
		}
	}
	return nil, nil
}

func (s *Set) findJourney(id string) (*File, *Journey) {
	for _, f := range s.Files {
		for _, j := range f.Doc.Journeys {
			if j.ID == id {
				return f, j
			}
		}
	}
	return nil, nil
}

func ident(what, v string, star bool) error {
	if !isIdent(v, star) {
		return bad("%s %q: use letters (latin preferred), digits and _ - . only, no spaces", what, v)
	}
	return nil
}

func freeText(what, v string) error {
	if strings.ContainsAny(v, "\r") {
		return bad("%s: carriage return is not allowed", what)
	}
	return nil
}

func (s *Set) stateTarget(entity, file string) (*File, *States, error) {
	if err := ident("entity", entity, false); err != nil {
		return nil, nil, err
	}
	if f, st := s.findStates(entity); st != nil {
		return f, st, nil
	}
	p := file
	if p == "" {
		p = fileFor(entity)
	}
	f, err := s.ensure(p)
	if err != nil {
		return nil, nil, err
	}
	st := &States{Entity: entity}
	f.Doc.States = append(f.Doc.States, st)
	return f, st, nil
}

// tidy removes from States the names that are implied by transitions or final.
func (st *States) tidy() {
	used := map[string]bool{}
	for _, t := range st.Transitions {
		used[t.From], used[t.To] = true, true
	}
	for _, n := range st.Final {
		used[n] = true
	}
	st.States = slices.DeleteFunc(st.States, func(n string) bool { return used[n] })
}

func (st *States) has(name string) bool {
	if slices.Contains(st.States, name) || slices.Contains(st.Final, name) {
		return true
	}
	for _, t := range st.Transitions {
		if t.From == name || t.To == name {
			return true
		}
	}
	return false
}

func (s *Set) apply(op Op) ([]string, error) {
	switch op.Op {
	case "add_state":
		if err := ident("state", op.State, false); err != nil {
			return nil, err
		}
		f, st, err := s.stateTarget(op.Entity, op.File)
		if err != nil {
			return nil, err
		}
		if st.has(op.State) {
			return nil, bad("state %q already exists in %s", op.State, op.Entity)
		}
		st.States = append(st.States, op.State)
		return []string{f.Path}, nil
	case "remove_state":
		f, st := s.findStates(op.Entity)
		if st == nil || !st.has(op.State) {
			return nil, bad("no state %q in %s", op.State, op.Entity)
		}
		st.States = slices.DeleteFunc(st.States, func(n string) bool { return n == op.State })
		st.Final = slices.DeleteFunc(st.Final, func(n string) bool { return n == op.State })
		st.Transitions = slices.DeleteFunc(st.Transitions, func(t Transition) bool { return t.From == op.State || t.To == op.State })
		return []string{f.Path}, nil
	case "rename_state":
		if err := ident("state", op.To, false); err != nil {
			return nil, err
		}
		f, st := s.findStates(op.Entity)
		if st == nil || !st.has(op.From) {
			return nil, bad("no state %q in %s", op.From, op.Entity)
		}
		if st.has(op.To) {
			return nil, bad("state %q already exists in %s", op.To, op.Entity)
		}
		ren := func(n string) string {
			if n == op.From {
				return op.To
			}
			return n
		}
		for i := range st.States {
			st.States[i] = ren(st.States[i])
		}
		for i := range st.Final {
			st.Final[i] = ren(st.Final[i])
		}
		for i := range st.Transitions {
			st.Transitions[i].From, st.Transitions[i].To = ren(st.Transitions[i].From), ren(st.Transitions[i].To)
		}
		touched := []string{f.Path}
		for _, ff := range s.Files {
			for _, fl := range ff.Doc.Flows {
				for i := range fl.States {
					if r := &fl.States[i]; r.Entity == op.Entity {
						if r.From == op.From || r.To == op.From {
							r.From, r.To = ren(r.From), ren(r.To)
							touched = append(touched, ff.Path)
						}
					}
				}
			}
		}
		return touched, nil
	case "set_final":
		f, st := s.findStates(op.Entity)
		if st == nil || !st.has(op.State) {
			return nil, bad("no state %q in %s", op.State, op.Entity)
		}
		want := op.Final == nil || *op.Final
		st.Final = slices.DeleteFunc(st.Final, func(n string) bool { return n == op.State })
		if want {
			st.Final = append(st.Final, op.State)
		} else if !slices.Contains(st.States, op.State) {
			used := false
			for _, t := range st.Transitions {
				used = used || t.From == op.State || t.To == op.State
			}
			if !used {
				st.States = append(st.States, op.State)
			}
		}
		st.tidy()
		return []string{f.Path}, nil
	case "add_transition":
		if err := ident("from", op.From, true); err != nil {
			return nil, err
		}
		if err := ident("to", op.To, false); err != nil {
			return nil, err
		}
		if op.On != "" {
			if err := ident("on", op.On, false); err != nil {
				return nil, err
			}
		}
		if err := freeText("label", op.Label); err != nil {
			return nil, err
		}
		f, st, err := s.stateTarget(op.Entity, op.File)
		if err != nil {
			return nil, err
		}
		for _, t := range st.Transitions {
			if t.From == op.From && t.To == op.To && t.On == op.On {
				return nil, bad("transition %s -> %s already exists", op.From, op.To)
			}
		}
		st.Transitions = append(st.Transitions, Transition{From: op.From, To: op.To, On: op.On, Label: op.Label})
		st.tidy()
		return []string{f.Path}, nil
	case "remove_transition":
		f, st := s.findStates(op.Entity)
		if st == nil {
			return nil, bad("no states %q", op.Entity)
		}
		n := len(st.Transitions)
		st.Transitions = slices.DeleteFunc(st.Transitions, func(t Transition) bool {
			return t.From == op.From && t.To == op.To && (op.On == "" || t.On == op.On)
		})
		if len(st.Transitions) == n {
			return nil, bad("no transition %s -> %s in %s", op.From, op.To, op.Entity)
		}
		return []string{f.Path}, nil
	case "remove_states":
		f, st := s.findStates(op.Entity)
		if st == nil {
			return nil, bad("no states %q", op.Entity)
		}
		f.Doc.States = slices.DeleteFunc(f.Doc.States, func(x *States) bool { return x == st })
		return []string{f.Path}, nil

	case "add_flow":
		if err := ident("flow id", op.ID, false); err != nil {
			return nil, err
		}
		if _, fl := s.findFlow(op.ID); fl != nil {
			return nil, bad("flow %q already exists", op.ID)
		}
		p := op.File
		if p == "" {
			p = fileFor(op.ID)
		}
		f, err := s.ensure(p)
		if err != nil {
			return nil, err
		}
		fl := &Flow{ID: op.ID}
		if op.Name != nil {
			fl.Name = *op.Name
		}
		if op.Actor != nil {
			fl.Actor = *op.Actor
		}
		f.Doc.Flows = append(f.Doc.Flows, fl)
		return []string{f.Path}, nil
	case "remove_flow":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		f.Doc.Flows = slices.DeleteFunc(f.Doc.Flows, func(x *Flow) bool { return x == fl })
		return []string{f.Path}, nil
	case "rename_flow":
		if err := ident("flow id", op.To, false); err != nil {
			return nil, err
		}
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		if _, other := s.findFlow(op.To); other != nil {
			return nil, bad("flow %q already exists", op.To)
		}
		fl.ID = op.To
		touched := []string{f.Path}
		for _, ff := range s.Files {
			hit := false
			for _, x := range ff.Doc.Flows {
				for i := range x.Alts {
					if x.Alts[i].Flow == op.Flow {
						x.Alts[i].Flow, hit = op.To, true
					}
				}
			}
			for _, j := range ff.Doc.Journeys {
				for i := range j.Flows {
					if j.Flows[i] == op.Flow {
						j.Flows[i], hit = op.To, true
					}
				}
			}
			if hit {
				touched = append(touched, ff.Path)
			}
		}
		return touched, nil
	case "set_name":
		if op.Name == nil {
			return nil, bad("name is required")
		}
		if err := freeText("name", *op.Name); err != nil {
			return nil, err
		}
		if op.Jrny != "" {
			f, j := s.findJourney(op.Jrny)
			if j == nil {
				return nil, bad("no journey %q", op.Jrny)
			}
			j.Name = *op.Name
			return []string{f.Path}, nil
		}
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		fl.Name = *op.Name
		return []string{f.Path}, nil
	case "set_actor", "set_exit":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		v := op.Actor
		if op.Op == "set_exit" {
			v = op.Exit
		}
		if v == nil {
			return nil, bad("%s: value is required (empty string clears)", op.Op)
		}
		if err := freeText("value", *v); err != nil {
			return nil, err
		}
		if op.Op == "set_actor" {
			fl.Actor = strings.TrimSpace(*v)
		} else {
			fl.Exit = *v
		}
		return []string{f.Path}, nil

	case "add_step", "set_step":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		st := Step{Ref: op.Ref, Planned: op.Plan, Ext: op.Ext, Text: op.Text}
		if err := freeText("text", st.Text); err != nil {
			return nil, err
		}
		if st.Planned {
			st.Ref, st.Ext = "", ""
			if st.Text == "" {
				return nil, bad("a planned step needs text")
			}
		} else {
			if !isRef(st.Ref) || st.Ref == "plan" {
				return nil, bad("ref %q is not a function reference", st.Ref)
			}
			if st.Ext != "" {
				if err := ident("ext", st.Ext, false); err != nil {
					return nil, err
				}
			}
		}
		if op.Op == "set_step" {
			i, err := stepIndex(op.Index, len(fl.Steps))
			if err != nil {
				return nil, err
			}
			fl.Steps[i] = st
			return []string{f.Path}, nil
		}
		at := len(fl.Steps)
		if op.Index != nil && *op.Index >= 0 {
			if *op.Index > len(fl.Steps) {
				return nil, bad("index %d is out of range (0..%d)", *op.Index, len(fl.Steps))
			}
			at = *op.Index
		}
		fl.Steps = slices.Insert(fl.Steps, at, st)
		return []string{f.Path}, nil
	case "remove_step":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		i, err := stepIndex(op.Index, len(fl.Steps))
		if err != nil {
			return nil, err
		}
		fl.Steps = slices.Delete(fl.Steps, i, i+1)
		return []string{f.Path}, nil
	case "move_step":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		i, err := stepIndex(op.Index, len(fl.Steps))
		if err != nil {
			return nil, err
		}
		j, err := stepIndex(op.ToIdx, len(fl.Steps))
		if err != nil {
			return nil, fmt.Errorf("to_index: %w", err)
		}
		st := fl.Steps[i]
		fl.Steps = slices.Delete(fl.Steps, i, i+1)
		fl.Steps = slices.Insert(fl.Steps, j, st)
		return []string{f.Path}, nil

	case "add_alt":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		cond := strings.Join(strings.Fields(op.Cond), " ")
		if cond == "" || strings.ContainsAny(cond, "\"#") || strings.Contains(cond, "->") {
			return nil, bad("cond %q: words without quotes, # or ->", op.Cond)
		}
		if op.Target != "" {
			if err := ident("target flow", op.Target, false); err != nil {
				return nil, err
			}
		}
		if err := freeText("text", op.Text); err != nil {
			return nil, err
		}
		fl.Alts = append(fl.Alts, Alt{Cond: cond, Flow: op.Target, Text: op.Text})
		return []string{f.Path}, nil
	case "remove_alt":
		f, fl := s.findFlow(op.Flow)
		if fl == nil {
			return nil, bad("no flow %q", op.Flow)
		}
		i, err := stepIndex(op.Index, len(fl.Alts))
		if err != nil {
			return nil, err
		}
		fl.Alts = slices.Delete(fl.Alts, i, i+1)
		return []string{f.Path}, nil

	case "add_journey", "set_journey_flows":
		id := op.ID
		if op.Op == "set_journey_flows" {
			id = op.Jrny
		}
		if len(op.Flows) == 0 && op.Op == "set_journey_flows" {
			return nil, bad("flows is required")
		}
		for _, fid := range op.Flows {
			if err := ident("flow id", fid, false); err != nil {
				return nil, err
			}
		}
		if op.Op == "set_journey_flows" {
			f, j := s.findJourney(id)
			if j == nil {
				return nil, bad("no journey %q", id)
			}
			j.Flows = slices.Clone(op.Flows)
			return []string{f.Path}, nil
		}
		if err := ident("journey id", id, false); err != nil {
			return nil, err
		}
		if _, j := s.findJourney(id); j != nil {
			return nil, bad("journey %q already exists", id)
		}
		if len(op.Flows) == 0 {
			return nil, bad("a journey needs at least one flow")
		}
		p := op.File
		if p == "" {
			p = fileFor(id)
		}
		f, err := s.ensure(p)
		if err != nil {
			return nil, err
		}
		j := &Journey{ID: id, Flows: slices.Clone(op.Flows)}
		if op.Name != nil {
			j.Name = *op.Name
		}
		f.Doc.Journeys = append(f.Doc.Journeys, j)
		return []string{f.Path}, nil
	case "remove_journey":
		f, j := s.findJourney(op.Jrny)
		if j == nil {
			return nil, bad("no journey %q", op.Jrny)
		}
		f.Doc.Journeys = slices.DeleteFunc(f.Doc.Journeys, func(x *Journey) bool { return x == j })
		return []string{f.Path}, nil
	}
	return nil, bad("unknown op %q", op.Op)
}

func stepIndex(p *int, n int) (int, error) {
	if p == nil {
		return 0, bad("index is required")
	}
	if *p < 0 || *p >= n {
		return 0, bad("index %d is out of range (0..%d)", *p, n-1)
	}
	return *p, nil
}
