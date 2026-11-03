package js

import (
	"reflect"
	"testing"
)

func TestQueueHints(t *testing.T) {
	f := parse(t, "q.js", `
export function producer(q, name) {
  q.add('email', {a: 1})
  q.add(name, {})
  q.add(`+"`dyn${name}`"+`, {})
}
export function boot() {
  new Worker('email', handler)
  new Worker('inline', async (job) => job)
}
`)
	if got, want := byName(f, "producer").Queues, []string{"add:email"}; !reflect.DeepEqual(got, want) {
		t.Errorf("producer Queues = %v, want %v", got, want)
	}
	if got, want := byName(f, "boot").Queues, []string{"worker:email:handler", "worker:inline"}; !reflect.DeepEqual(got, want) {
		t.Errorf("boot Queues = %v, want %v", got, want)
	}
}

func TestDocIgnoresTrailingComment(t *testing.T) {
	f := parse(t, "d.js", `
const limit = 10; // the page size

function load() {}

// Saves it.
function save() {}
`)
	if d := byName(f, "load").Doc; d != "" {
		t.Errorf("a comment trailing another statement is not a doc: %q", d)
	}
	if d := byName(f, "save").Doc; d != "Saves it." {
		t.Errorf("save doc = %q", d)
	}
}
