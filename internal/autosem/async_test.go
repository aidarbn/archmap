package autosem

import (
	"fmt"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

// queueFacts: Pay -> svc.Checkout -> svc.finish -> Schedule (queues Worker.Work).
// Checkout has plenty of tempting siblings (domain steps, repository reads) that
// would fill the step budget before the queue is reached; extra levels between
// finish and Schedule push the hand-off deeper.
func queueFacts(extraLevels int) (*facts.Facts, string) {
	f := &facts.Facts{Repo: "demo", Modules: map[string]facts.Module{}, Functions: map[string]facts.Function{}}
	const svc, repo, queue, work, http = "internal/services/order", "internal/adapters/out/db/repository", "internal/adapters/dbqueue", "internal/adapters/dbqueue/workers", "internal/adapters/in/http"
	add := func(mod, name string, callees ...string) string {
		fid := mod + "/f.go::" + name
		f.Modules[mod] = facts.Module{}
		f.Functions[fid] = facts.Function{Name: name, Start: 1, End: 30, Module: mod, File: mod + "/f.go", Lang: "go", Callees: callees}
		return fid
	}
	id := func(mod, name string) string { return mod + "/f.go::" + name }
	addAsync := func(fid string, to ...string) {
		fn := f.Functions[fid]
		fn.AsyncCallees = to
		f.Functions[fid] = fn
	}
	// the queue side
	add(work, "Work", id(svc, "process"))
	add(svc, "process", id(svc, "rules"), id(repo, "Update"))
	add(svc, "rules", id(svc, "inner"))
	add(svc, "inner")
	add(repo, "Update", id(repo, "exec"))
	add(repo, "exec")
	add(queue, "Schedule")
	addAsync(id(queue, "Schedule"), id(work, "Work"))
	// the way to it, extraLevels deep
	next := id(queue, "Schedule")
	for i := extraLevels; i > 0; i-- {
		next = add(svc, fmt.Sprintf("hop%d", i), next)
	}
	add(svc, "finish", next)
	// siblings that compete for the budget
	var sib []string
	for i := 0; i < 6; i++ {
		leaf := add(repo, fmt.Sprintf("Read%d", i))
		sib = append(sib, leaf)
		d := add(svc, fmt.Sprintf("step%d", i), add(svc, fmt.Sprintf("sub%d", i), leaf), leaf)
		sib = append(sib, d)
	}
	callees := append(append([]string{}, sib...), id(svc, "finish"))
	add(svc, "Checkout", callees...)
	add(http, "Pay", id(svc, "Checkout"))
	e := f.Functions[id(http, "Pay")]
	e.Entry = &facts.Entry{Kind: "http", Labels: []string{"POST /pay"}}
	f.Functions[id(http, "Pay")] = e
	return f, id(work, "Work")
}

func flowOf(t *testing.T, f *facts.Facts, name string) Flow {
	t.Helper()
	for _, fl := range Build(f).Flows {
		if fl.Name == name {
			return fl
		}
	}
	t.Fatalf("no flow %s", name)
	return Flow{}
}

func TestWalkReachesQueueHandoff(t *testing.T) {
	for _, extra := range []int{0, 2} {
		f, handler := queueFacts(extra)
		fl := flowOf(t, f, "POST /pay")
		at := -1
		for i, s := range fl.Steps {
			if s == handler {
				at = i
			}
		}
		if at < 0 {
			t.Fatalf("extra=%d: scenario must reach the queue handler: %v", extra, fl.Steps)
		}
		if len(fl.Steps) > MaxStepsAsync || len(fl.Steps) <= MaxSteps-3 {
			t.Errorf("extra=%d: %d steps, budget %d", extra, len(fl.Steps), MaxStepsAsync)
		}
		if at == len(fl.Steps)-1 {
			t.Errorf("extra=%d: the handler's story must continue: %v", extra, fl.Steps)
		}
		if next := fl.Steps[at+1]; next != "internal/services/order/f.go::process" {
			t.Errorf("extra=%d: step after the handler = %s", extra, next)
		}
		// the scheduler is right before the handler
		if fl.Steps[at-1] != "internal/adapters/dbqueue/f.go::Schedule" {
			t.Errorf("extra=%d: step before the handler = %s", extra, fl.Steps[at-1])
		}
	}
}

func TestWalkSkipsReadsOnTheWayToQueue(t *testing.T) {
	f, _ := queueFacts(0)
	for _, s := range flowOf(t, f, "POST /pay").Steps {
		if len(s) > 0 && s[:len("internal/adapters/out/db/repository")] == "internal/adapters/out/db/repository" {
			fn := f.Functions[s]
			if len(fn.Callees) == 0 {
				t.Errorf("a plain repository read %s takes a step of a queue scenario", s)
			}
		}
	}
}

func TestWalkIgnoresQueueBeyondDepth(t *testing.T) {
	f, handler := queueFacts(AsyncDepth) // too far: Pay+Checkout+finish+hops+Schedule+Work > AsyncDepth levels
	fl := flowOf(t, f, "POST /pay")
	if len(fl.Steps) > MaxSteps {
		t.Errorf("no forced queue path beyond AsyncDepth, got %d steps", len(fl.Steps))
	}
	for _, s := range fl.Steps {
		if s == handler {
			t.Errorf("handler out of reach must not be forced: %v", fl.Steps)
		}
	}
}

func TestWalkWithoutQueueKeepsBudget(t *testing.T) {
	f := synth()
	for _, fl := range Build(f).Flows {
		if len(fl.Steps) > MaxSteps {
			t.Errorf("%s: %d steps without a queue", fl.Name, len(fl.Steps))
		}
	}
}
