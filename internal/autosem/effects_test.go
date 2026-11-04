package autosem

import (
	"reflect"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func TestFlowEffects(t *testing.T) {
	f := declFacts()
	id := func(n string) string { return "internal/x/f.go::" + n }
	save := f.Functions[id("save")]
	save.Data = []facts.Access{
		{Table: "kaspi_payments", Op: "insert"},
		{Table: "orders", Op: "update", Fields: []string{"status"}},
		{Table: "users", Op: "read"},
	}
	f.Functions[id("save")] = save
	poll := f.Functions[id("Poll")]
	poll.Arch.State = []facts.StateNote{{Entity: "orders.status", From: "payment_requested", To: "paid"}}
	f.Functions[id("Poll")] = poll

	m := Build(f)
	check := func(fl Flow) {
		if want := []string{"INSERT kaspi_payments", "UPDATE orders (status)"}; fl.Effects[id("save")] != nil && !reflect.DeepEqual(fl.Effects[id("save")], want) {
			t.Fatalf("save effects %v", fl.Effects[id("save")])
		}
	}
	d := m.Flows[0]
	if d.ID != "pay" {
		t.Fatal("declared flow not first")
	}
	check(d)
	if want := []string{"INSERT kaspi_payments", "UPDATE orders (status)"}; !reflect.DeepEqual(d.Effects[id("save")], want) {
		t.Fatalf("declared save effects %v", d.Effects)
	}
	if want := []string{"orders.status ← paid"}; !reflect.DeepEqual(d.Effects[id("Poll")], want) {
		t.Fatalf("Poll effects %v", d.Effects[id("Poll")])
	}
	if _, ok := d.Effects[id("Pay")]; ok {
		t.Fatal("step without effects must not be listed")
	}
	// automatic flow from the other entry point also gets effects
	for _, fl := range m.Flows {
		if fl.Kind != KindDeclared && len(fl.Steps) > 0 && fl.Steps[0] == id("Other") {
			if len(fl.Effects[id("save")]) != 2 {
				t.Fatalf("auto flow effects %v", fl.Effects)
			}
			return
		}
	}
	t.Fatal("auto flow for Other not found")
}
