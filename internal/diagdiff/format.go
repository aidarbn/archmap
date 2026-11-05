package diagdiff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Languages of the report.
const (
	LangEN = "en"
	LangRU = "ru"
)

type words struct {
	title, none                                 string
	changed, added, removed, broken, oldBroken  string
	kind                                        map[string]string
	status, pageStatus                          map[string]string
	reason                                      map[string]string
	stepRemoved, stepAdded, stepText, stepOrder string
	steps, flows                                string
	flowRemoved, flowAdded                      string
	stateAdded, stateRemoved                    string
	trAdded, trRemoved, trFuncs                 string
	frames, frameAdded, frameRemoved, renamed   string
	inBase                                      string
	brokenHead                                  string
	brokenKind                                  map[string]string
}

var dict = map[string]words{
	LangEN: {
		title: "archmap diagrams", none: "Diagrams did not change.",
		changed: "changed", added: "added", removed: "gone", broken: "broken", oldBroken: "broken already in the base",
		kind:   map[string]string{KindFlow: "Scenario", KindJourney: "Journey", KindStates: "States", KindPage: "Page"},
		status: map[string]string{Added: "new", Removed: "gone", Changed: "changed"},
		reason: map[string]string{
			ReasonEntryDeleted:  "the entry-point function %s was deleted",
			ReasonEntryUnmarked: "arch:flow was taken off the entry point %s",
			ReasonNotEntry:      "%s is no longer an entry point",
			ReasonSpecRemoved:   "removed from arch/*.arch",
			ReasonUnmarked:      "the declaration was removed",
			ReasonFlowsGone:     "none of its scenarios exist any more",
			ReasonDeleted:       "the function was deleted",
			ReasonStepUnmark:    "arch:step was taken off",
			ReasonUnreachable:   "still marked, but the entry point no longer reaches it",
			ReasonWalk:          "the path no longer goes through it",
			ReasonFlowGone:      "the scenario no longer exists",
		},
		stepRemoved: "Step gone", stepAdded: "Step added", stepText: "Step caption changed", stepOrder: "Step order changed",
		steps: "Steps", flows: "Scenarios",
		flowRemoved: "Scenario left the journey", flowAdded: "Scenario joined the journey",
		stateAdded: "State added", stateRemoved: "State removed",
		trAdded: "Transition added", trRemoved: "Transition gone", trFuncs: "Transition is now made by",
		frames: "Frames", frameAdded: "Frame added", frameRemoved: "Frame gone", renamed: "Renamed",
		inBase:     "in the base",
		brokenHead: "Broken diagrams",
		brokenKind: map[string]string{
			BrokenJourneyFlow: "journey `%s` names the scenario `%s`, which does not exist",
			BrokenOnFlow:      "a branch leads to the scenario `%s`, which does not exist",
			BrokenNoEntry:     "scenario `%s` has steps but no entry point (no arch:flow): %s",
			BrokenNoFunction:  "scenario `%s`: step `%s` names no function",
			BrokenUnreachable: "scenario `%s`: step %s is not reachable from the entry point",
			BrokenPageRef:     "page `%s` shows `%s`, which the code does not have",
		},
	},
	LangRU: {
		title: "Диаграммы archmap", none: "Диаграммы не изменились.",
		changed: "изменено", added: "появилось", removed: "исчезло", broken: "битых", oldBroken: "битых уже в базе",
		kind:       map[string]string{KindFlow: "Сценарий", KindJourney: "Путь", KindStates: "Статусы", KindPage: "Страница"},
		status:     map[string]string{Added: "новый", Removed: "исчез", Changed: "изменён"},
		pageStatus: map[string]string{Added: "новая", Removed: "исчезла", Changed: "изменена"},
		reason: map[string]string{
			ReasonEntryDeleted:  "удалена функция точки входа %s",
			ReasonEntryUnmarked: "с точки входа %s снята пометка arch:flow",
			ReasonNotEntry:      "%s больше не точка входа",
			ReasonSpecRemoved:   "убран из arch/*.arch",
			ReasonUnmarked:      "объявление удалено",
			ReasonFlowsGone:     "ни одного его сценария больше нет",
			ReasonDeleted:       "функция удалена",
			ReasonStepUnmark:    "снята пометка arch:step",
			ReasonUnreachable:   "пометка есть, но точка входа до функции больше не доходит",
			ReasonWalk:          "путь больше не идёт через неё",
			ReasonFlowGone:      "сценария больше нет",
		},
		stepRemoved: "Шаг пропал", stepAdded: "Шаг добавлен", stepText: "Шаг переименован", stepOrder: "Порядок шагов изменился",
		steps: "Шаги", flows: "Сценарии",
		flowRemoved: "Сценарий убран из пути", flowAdded: "Сценарий добавлен в путь",
		stateAdded: "Статус добавлен", stateRemoved: "Статус удалён",
		trAdded: "Переход появился", trRemoved: "Переход исчез", trFuncs: "Переход теперь делает",
		frames: "Рамки", frameAdded: "Рамка добавлена", frameRemoved: "Рамка убрана", renamed: "Переименована",
		inBase:     "в базе",
		brokenHead: "Битые диаграммы",
		brokenKind: map[string]string{
			BrokenJourneyFlow: "путь `%s` ссылается на сценарий `%s`, которого нет",
			BrokenOnFlow:      "ветка ведёт в сценарий `%s`, которого нет",
			BrokenNoEntry:     "у сценария `%s` есть шаги, но нет точки входа (нет arch:flow): %s",
			BrokenNoFunction:  "сценарий `%s`: шаг `%s` не указывает ни на одну функцию",
			BrokenUnreachable: "сценарий `%s`: шаг %s недостижим из точки входа",
			BrokenPageRef:     "страница `%s` показывает `%s`, которого нет в коде",
		},
	},
}

func lang(l string) words {
	if w, ok := dict[l]; ok {
		return w
	}
	return dict[LangEN]
}

// printer renders with or without Markdown.
type printer struct {
	w  words
	md bool
}

func (p printer) code(s string) string {
	if p.md {
		return "`" + strings.ReplaceAll(s, "`", "'") + "`"
	}
	return s
}

func (p printer) fn(r *Ref) string {
	if r == nil {
		return ""
	}
	return p.code(r.Name)
}

func (p printer) fns(rs []Ref) string {
	var out []string
	for i := range rs {
		out = append(out, p.fn(&rs[i]))
	}
	return strings.Join(out, ", ")
}

func quote(s string) string { return "«" + s + "»" }

// status of a diagram; a page has its own words (Russian: «страница» is feminine)
func (p printer) status(d Diff) string {
	if s, ok := p.w.pageStatus[d.Status]; ok && d.Kind == KindPage {
		return s
	}
	return p.w.status[d.Status]
}

func (p printer) title(d Diff) string {
	name := d.Name
	if name == "" || name == d.ID {
		return fmt.Sprintf("%s %s", p.w.kind[d.Kind], p.code(d.ID))
	}
	return fmt.Sprintf("%s %s (%s)", p.w.kind[d.Kind], quote(name), p.code(d.ID))
}

func (p printer) reason(d Diff) string {
	t := p.w.reason[d.Reason]
	if strings.Contains(t, "%s") {
		e := ""
		if d.Entry != nil {
			e = fmt.Sprintf("%s (%s:%d %s)", p.code(d.Entry.Name), d.Entry.File, d.Entry.Line, p.w.inBase)
		}
		t = fmt.Sprintf(t, e)
	}
	return t
}

func (p printer) step(c Change) string {
	if c.Text != "" {
		return fmt.Sprintf("%s — %s", quote(c.Text), p.fn(c.Func))
	}
	return p.fn(c.Func)
}

func (p printer) change(c Change) string {
	switch c.Code {
	case StepRemoved:
		return fmt.Sprintf("%s: %s (%s)", p.w.stepRemoved, p.step(c), p.w.reason[c.Reason])
	case StepAdded:
		return fmt.Sprintf("%s: %s", p.w.stepAdded, p.step(c))
	case StepText:
		return fmt.Sprintf("%s: %s → %s (%s)", p.w.stepText, quote(c.Old), quote(c.Text), p.fn(c.Func))
	case StepOrder:
		return fmt.Sprintf("%s: %s", p.w.stepOrder, strings.Join(c.Order, " → "))
	case FlowRemoved:
		return fmt.Sprintf("%s: %s (%s)", p.w.flowRemoved, p.code(c.Flow), p.w.reason[c.Reason])
	case FlowAdded:
		return fmt.Sprintf("%s: %s", p.w.flowAdded, p.code(c.Flow))
	case StateAdded:
		return fmt.Sprintf("%s: %s", p.w.stateAdded, p.code(c.State))
	case StateRemoved:
		return fmt.Sprintf("%s: %s", p.w.stateRemoved, p.code(c.State))
	case TransitionAdded, TransitionRemoved:
		label := p.w.trAdded
		if c.Code == TransitionRemoved {
			label = p.w.trRemoved
		}
		s := fmt.Sprintf("%s: %s", label, p.code(transition(c)))
		if len(c.Funcs) > 0 {
			s += " (" + p.fns(c.Funcs) + ")"
		}
		return s
	case FrameAdded:
		return fmt.Sprintf("%s: %s", p.w.frameAdded, p.code(c.Text))
	case FrameRemoved:
		return fmt.Sprintf("%s: %s", p.w.frameRemoved, p.code(c.Text))
	case PageRenamed:
		return fmt.Sprintf("%s: %s → %s", p.w.renamed, quote(c.Old), quote(c.Text))
	case TransitionFuncs:
		return fmt.Sprintf("%s: %s %s (%s: %s)", p.code(transition(c)), p.w.trFuncs, p.fns(c.Funcs), p.w.inBase, p.fns(c.Before))
	}
	return c.Code
}

// maxListed is how many steps (flows) of a new diagram are named in its one line.
const maxListed = 8

// lines are the bullet lines of a diagram: a new flow or journey is one line
// with its steps (flows) in order, anything else one line per change.
func (p printer) lines(d Diff) []string {
	if d.Status == Added && d.Kind != KindStates {
		var names []string
		for _, c := range d.Changes {
			switch {
			case c.Code == FlowAdded:
				names = append(names, p.code(c.Flow))
			case c.Code == FrameAdded:
				names = append(names, p.code(c.Text))
			case c.Text != "":
				names = append(names, quote(c.Text))
			default:
				names = append(names, p.fn(c.Func))
			}
		}
		label := p.w.steps
		sep := " → "
		switch d.Kind {
		case KindJourney:
			label = p.w.flows
		case KindPage:
			label, sep = p.w.frames, "; "
		}
		more := ""
		if len(names) > maxListed {
			more = fmt.Sprintf(" … (+%d)", len(names)-maxListed)
			names = names[:maxListed]
		}
		return []string{fmt.Sprintf("%s (%d): %s%s", label, len(d.Changes), strings.Join(names, sep), more)}
	}
	var out []string
	for _, c := range d.Changes {
		out = append(out, p.change(c))
	}
	return out
}

func transition(c Change) string {
	from := c.From
	if from == "" {
		from = "?"
	}
	return from + " → " + c.To
}

func (p printer) broken(b Broken) string {
	t := p.w.brokenKind[b.Kind]
	var s string
	switch b.Kind {
	case BrokenJourneyFlow:
		s = fmt.Sprintf(t, b.Diagram, b.Ref)
	case BrokenOnFlow:
		s = fmt.Sprintf(t, b.Diagram)
	case BrokenNoEntry, BrokenUnreachable:
		s = fmt.Sprintf(t, b.Diagram, p.code(b.Func))
	default:
		s = fmt.Sprintf(t, b.Diagram, b.Ref)
	}
	if !p.md {
		s = strings.ReplaceAll(s, "`", "")
	}
	return fmt.Sprintf("%s (%s:%d)", s, b.File, b.Line)
}

func (p printer) summary(r Report) string {
	parts := []string{
		fmt.Sprintf("%s: %d", p.w.changed, r.Count(Changed)),
		fmt.Sprintf("%s: %d", p.w.added, r.Count(Added)),
		fmt.Sprintf("%s: %d", p.w.removed, r.Count(Removed)),
		fmt.Sprintf("%s: %d", p.w.broken, len(r.Broken)),
	}
	if r.OldBroken > 0 {
		parts = append(parts, fmt.Sprintf("%s: %d", p.w.oldBroken, r.OldBroken))
	}
	r0 := []rune(strings.Join(parts, " · "))
	return strings.ToUpper(string(r0[:1])) + string(r0[1:])
}

// Markdown renders the report for a pull request comment: a title, the
// summary, the new broken diagrams, then one folded block per diagram.
func Markdown(r Report, l string) string {
	p := printer{w: lang(l), md: true}
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", p.w.title)
	if r.Empty() {
		b.WriteString(p.w.none + "\n")
		if r.OldBroken > 0 {
			fmt.Fprintf(&b, "\n%s: %d\n", p.w.oldBroken, r.OldBroken)
		}
		return b.String()
	}
	b.WriteString(p.summary(r) + "\n")
	if len(r.Broken) > 0 {
		fmt.Fprintf(&b, "\n**%s**\n\n", p.w.brokenHead)
		for _, x := range r.Broken {
			b.WriteString("- " + p.broken(x) + "\n")
		}
	}
	for _, d := range r.Diffs {
		head := fmt.Sprintf("%s — %s", p.title(d), p.status(d))
		if d.Status == Removed && d.Reason != "" {
			head += ": " + p.reason(d)
		}
		if len(d.Changes) == 0 { // nothing to fold: one line
			fmt.Fprintf(&b, "\n%s\n", head)
			continue
		}
		fmt.Fprintf(&b, "\n<details><summary>%s</summary>\n\n", mdInline(head))
		for _, l := range p.lines(d) {
			b.WriteString("- " + l + "\n")
		}
		b.WriteString("\n</details>\n")
	}
	return b.String()
}

// mdInline turns `code` into <code> for HTML contexts (a <summary> does not
// render Markdown on GitHub).
func mdInline(s string) string {
	s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
	var out strings.Builder
	open := false
	for _, r := range s {
		if r == '`' {
			if open {
				out.WriteString("</code>")
			} else {
				out.WriteString("<code>")
			}
			open = !open
			continue
		}
		out.WriteRune(r)
	}
	if open {
		out.WriteString("</code>")
	}
	return out.String()
}

// Text renders the report as plain lines for a terminal.
func Text(r Report, l string) string {
	p := printer{w: lang(l)}
	var b strings.Builder
	b.WriteString(p.w.title + "\n")
	if r.Empty() {
		b.WriteString(p.w.none + "\n")
		if r.OldBroken > 0 {
			fmt.Fprintf(&b, "%s: %d\n", p.w.oldBroken, r.OldBroken)
		}
		return b.String()
	}
	b.WriteString(p.summary(r) + "\n")
	if len(r.Broken) > 0 {
		fmt.Fprintf(&b, "\n%s:\n", p.w.brokenHead)
		for _, x := range r.Broken {
			b.WriteString("  ! " + p.broken(x) + "\n")
		}
	}
	for _, d := range r.Diffs {
		head := fmt.Sprintf("%s — %s", p.title(d), p.status(d))
		if d.Status == Removed && d.Reason != "" {
			head += ": " + p.reason(d)
		}
		b.WriteString("\n" + head + "\n")
		for _, l := range p.lines(d) {
			b.WriteString("  - " + l + "\n")
		}
	}
	return b.String()
}

// JSON renders the report as indented JSON (no HTML escaping).
func JSON(r Report) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
