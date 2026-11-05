package autosem

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/spec"
)

func specSet(t *testing.T, text string) *spec.Set {
	t.Helper()
	s := spec.FromTexts(map[string]string{"arch/t.arch": text})
	if len(s.Errors) > 0 {
		t.Fatal(s.Errors)
	}
	return s
}

func TestMergeSpecIntoCodeFlow(t *testing.T) {
	f := declFacts()
	base := Build(f)
	baseSteps := flowByID(base, "pay").Steps
	if len(baseSteps) < 3 {
		t.Fatalf("setup: %v", baseSteps)
	}
	m := BuildWith(f, specSet(t, `flow pay "Оплата по спецификации"
  actor Касса
  step save   "Сохраняем"
  step plan "Проверить лимит" in internal/x
  step requestQR   ext Kaspi
  alt declined -> retry
  exit "Оплачено"

flow retry "Повтор"
  step plan "Показать ошибку"

journey j-new "Новый путь": pay > retry
`))
	fl := flowByID(m, "pay")
	if fl == nil || !fl.Spec || fl.Name != "Оплата по спецификации" || fl.Actor != "Касса" || fl.Exit != "Оплачено" {
		t.Fatalf("merged flow: %+v", fl)
	}
	id := func(n string) string { return "internal/x/f.go::" + n }
	// the spec lists save before requestQR; the code calls requestQR first: the graph wins
	if !reflect.DeepEqual(fl.Steps, []string{id("Pay"), id("requestQR"), id("save"), id("Poll")}) {
		t.Fatalf("call-graph order must win over the spec order: %v", fl.Steps)
	}
	if fl.Labels[id("save")] != "Сохраняем" || fl.Labels[id("requestQR")] == "" {
		t.Fatalf("labels: %v", fl.Labels)
	}
	// planned steps live in the plan layer, never among the steps
	if !reflect.DeepEqual(fl.Plan, []PlanStep{{ID: PlannedID("pay", 1), Text: "Проверить лимит", Module: "internal/x", After: id("save")}}) || fl.Ext[id("requestQR")] != "Kaspi" {
		t.Fatalf("plan/ext: %+v %v", fl.Plan, fl.Ext)
	}
	if len(fl.Unspecified) == 0 || fl.Unspecified[id("save")] {
		t.Fatalf("code steps without spec must be marked: %v", fl.Unspecified)
	}
	for _, s := range baseSteps {
		found := false
		for _, x := range fl.Steps {
			found = found || x == s
		}
		if !found {
			t.Errorf("code step %s lost", s)
		}
	}
	var declined bool
	for _, b := range fl.Branches {
		declined = declined || (b.Cond == "declined" && b.Flow == "retry")
	}
	if !declined {
		t.Fatalf("alt: %+v", fl.Branches)
	}

	re := flowByID(m, "retry")
	if re == nil || !re.Spec || re.Kind != KindDeclared || len(re.Steps) != 0 || len(re.Plan) != 1 || re.Name != "Повтор" {
		t.Fatalf("spec-only flow: %+v", re)
	}
	// spec-only flows sit among the declared ones, before the automatic ones
	seenAuto := false
	for _, x := range m.Flows {
		if x.Kind != KindDeclared {
			seenAuto = true
		} else if seenAuto {
			t.Fatal("declared flow after an automatic one")
		}
	}
	if m.Journeys[0].ID != "j-new" || !m.Journeys[0].Declared || !reflect.DeepEqual(m.Journeys[0].Flows, []string{"pay", "retry"}) || len(m.Journeys[0].Links) != 1 {
		t.Fatalf("journeys: %+v", m.Journeys)
	}
	if m.Spec == nil || len(m.Spec.Flows) != 2 || m.Spec.Flows[1].Planned != 1 {
		t.Fatalf("spec model: %+v", m.Spec)
	}
	b, err := JSONWith(f, specSet(t, "flow retry\n  step plan \"x\"\n"))
	if err != nil || !strings.Contains(string(b), `"plan":[{"id":"plan:retry:0"`) || !strings.Contains(string(b), `"spec":`) {
		t.Fatalf("json: %v %s", err, b)
	}
}

func TestBuildWithoutSpecUnchanged(t *testing.T) {
	f := declFacts()
	a, b := Build(f), BuildWith(f, &spec.Set{})
	if !reflect.DeepEqual(a, b) || b.Spec != nil {
		t.Fatal("empty spec must not change the model")
	}
}
