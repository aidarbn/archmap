package extract

import (
	"regexp"
	"strings"
)

// Asynchronous hand-offs through job queues: a function that enqueues a job is
// linked to the function that handles it (fn.async -> facts async_callees).
//
// The rules are a table by framework; each looks at one enqueuing function and
// returns the handlers it hands work to. A rule is purely syntactic: a handler
// is found by type, class or name, never by calling convention at runtime.
//
//   - Go River: a call Insert/InsertTx/InsertMany(Tx) in a body that builds a
//     value of type XArgs ({}, &{}) -> the Work method whose signature takes
//     *river.Job[XArgs] (also a Schedule* wrapper: it is the function with Insert).
//   - Ruby Sidekiq / ActiveJob: X.perform_async/perform_in/perform_at/perform_later
//     -> X#perform.
//   - Python Celery: x.delay(...) / x.apply_async(...) / x.s(...) -> the function
//     x decorated as a task (shared_task, app.task, celery.task...).
//   - JS BullMQ: queue.add('name', ...) -> the handler of new Worker('name', h)
//     (the function h by name, or the function that creates the Worker).

// maxAsync caps the handlers of one rule hit: a name shared by many functions
// is ambiguous and not linked.
const maxAsync = 3

type asyncIndex struct {
	fns     map[string]*fn
	byName  map[string][]string
	byCls   map[string]map[string][]string
	workers map[string][]string // river args type -> Work methods
	bull    map[string][]string // bullmq queue name -> handler functions
}

var (
	riverInsert  = setOf("Insert InsertTx InsertMany InsertManyTx InsertManyFast InsertFast")
	rubyEnqueue  = setOf("perform_async perform_in perform_at perform_later")
	celeryEnq    = setOf("delay apply_async s si signature")
	riverJobType = regexp.MustCompile(`river\.Job\[\s*(?:\*?\s*[\w.]*\.)?(\w+)\s*\]`)
)

func buildAsyncIndex(order []string, fns map[string]*fn, byName map[string][]string, byCls map[string]map[string][]string) *asyncIndex {
	ix := &asyncIndex{fns: fns, byName: byName, byCls: byCls, workers: map[string][]string{}, bull: map[string][]string{}}
	for _, fid := range order {
		f := fns[fid]
		switch f.lang {
		case "go":
			if f.Name == "Work" && f.Cls != "" {
				if m := riverJobType.FindStringSubmatch(f.Sig); m != nil {
					ix.workers[m[1]] = append(ix.workers[m[1]], fid)
				}
			}
		case "js":
			for _, q := range f.Queues {
				if !strings.HasPrefix(q, "worker:") {
					continue
				}
				name, handler, _ := strings.Cut(strings.TrimPrefix(q, "worker:"), ":")
				target := []string{fid}
				if handler != "" {
					if hs := byName[handler]; len(hs) > 0 && len(hs) <= maxAsync {
						target = hs
					}
				}
				ix.bull[name] = append(ix.bull[name], target...)
			}
		}
	}
	return ix
}

// handlers lists the queue handlers function f hands work to.
func (ix *asyncIndex) handlers(fid string, f *fn) []string {
	var out []string
	add := func(ids []string) {
		if len(ids) == 0 || len(ids) > maxAsync {
			return
		}
		for _, id := range ids {
			if id != fid && !contains(out, id) {
				out = append(out, id)
			}
		}
	}
	insert := false
	for _, c := range f.Calls {
		switch {
		case f.lang == "go" && riverInsert[c.Name]:
			insert = true
		case rubyEnqueue[c.Name]:
			recv := c.Recv
			if i := strings.LastIndex(recv, "::"); i >= 0 {
				recv = recv[i+2:]
			}
			if i := strings.LastIndex(recv, "."); i >= 0 {
				recv = recv[i+1:]
			}
			if recv != "" && recv != "?" {
				add(ix.byCls[recv]["perform"])
			}
		case f.lang == "python" && celeryEnq[c.Name] && c.Recv != "" && c.Recv != "?":
			name := c.Recv
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:]
			}
			var tasks []string
			for _, id := range ix.byName[name] {
				if t := ix.fns[id]; t.Cls == "" && isTask(t) {
					tasks = append(tasks, id)
				}
			}
			add(tasks)
		}
	}
	if insert {
		for _, t := range f.Types {
			add(ix.workers[t])
		}
	}
	for _, q := range f.Queues {
		if name, ok := strings.CutPrefix(q, "add:"); ok {
			add(ix.bull[name])
		}
	}
	return out
}

// isTask reports a Celery-style task function by its decorators.
func isTask(f *fn) bool {
	for _, d := range f.Decorators {
		l := strings.ToLower(d)
		if strings.Contains(l, "task") {
			return true
		}
	}
	return false
}
