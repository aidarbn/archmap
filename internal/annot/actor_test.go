package annot

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestActorDescription(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		flowActor string
		docs      []facts.ActorDoc
		errs      int
	}{
		{"bare name", "arch:flow a \"A\"\narch:actor Планшет", "Планшет", nil, 0},
		{"name with spaces", "arch:flow a \"A\"\narch:actor Кабинет ресторана", "Кабинет ресторана", nil, 0},
		{"name and description", "arch:flow a \"A\"\narch:actor Планшет \"Планшет в зале ресторана, через него гость заказывает\"",
			"Планшет", []facts.ActorDoc{{Name: "Планшет", Desc: "Планшет в зале ресторана, через него гость заказывает", Line: 2}}, 0},
		{"spaces in name and description", "arch:flow a \"A\"\narch:actor Кабинет ресторана \"Веб-кабинет владельца\"",
			"Кабинет ресторана", []facts.ActorDoc{{Name: "Кабинет ресторана", Desc: "Веб-кабинет владельца", Line: 2}}, 0},
		{"quoted name and description", "arch:flow a \"A\"\narch:actor \"Гость\" “Посетитель ресторана”",
			"Гость", []facts.ActorDoc{{Name: "Гость", Desc: "Посетитель ресторана", Line: 2}}, 0},
		{"description alone, no flow", "arch:actor Cron \"Планировщик River\"",
			"", []facts.ActorDoc{{Name: "Cron", Desc: "Планировщик River", Line: 1}}, 0},
		{"name alone without a flow is an error", "arch:actor Cron", "", nil, 1},
		{"unterminated description", "arch:flow a \"A\"\narch:actor Гость \"Посетитель", "", nil, 1},
		{"text after the description", "arch:flow a \"A\"\narch:actor Гость \"Посетитель\" лишнее", "", nil, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, errs := ParseText(c.text, 1)
			if len(errs) != c.errs {
				t.Fatalf("errs = %v, want %d", errs, c.errs)
			}
			var actor string
			var docs []facts.ActorDoc
			if n != nil {
				if len(n.Flows) > 0 {
					actor = n.Flows[0].Actor
				}
				docs = n.Actors
			}
			if actor != c.flowActor {
				t.Errorf("flow actor = %q, want %q", actor, c.flowActor)
			}
			if !reflect.DeepEqual(docs, c.docs) {
				t.Errorf("docs = %+v, want %+v", docs, c.docs)
			}
		})
	}
}
