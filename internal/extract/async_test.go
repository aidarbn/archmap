package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang/all"
)

func runFiles(t *testing.T, files map[string]string) *facts.Facts {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := Run(Options{Root: root, Parsers: all.Parsers()})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// asyncOf lists "from -> to" pairs (function names) of all async edges.
func asyncOf(f *facts.Facts) []string {
	var out []string
	for _, fn := range f.Functions {
		for _, c := range fn.AsyncCallees {
			to := f.Functions[c]
			name := to.Name
			if to.Cls != nil {
				name = *to.Cls + "." + name
			}
			from := fn.Name
			if fn.Cls != nil {
				from = *fn.Cls + "." + from
			}
			out = append(out, from+" -> "+name)
		}
	}
	return out
}

func wantEdges(t *testing.T, f *facts.Facts, want ...string) {
	t.Helper()
	got := strings.Join(asyncOf(f), "\n")
	for _, w := range want {
		if !strings.Contains("\n"+got+"\n", "\n"+w+"\n") {
			t.Errorf("missing async edge %q in:\n%s", w, got)
		}
	}
}

func TestAsyncGoRiver(t *testing.T) {
	f := runFiles(t, map[string]string{
		"go.mod": "module ex.com/m\n\ngo 1.22\n",
		"jobs/jobs.go": `package jobs

type SendArgs struct{ ID int64 }

func (SendArgs) Kind() string { return "send" }

type PingArgs struct{}
`,
		"workers/send.go": `package workers

import (
	"context"

	"github.com/riverqueue/river"
	"ex.com/m/jobs"
)

type SendWorker struct {
	river.WorkerDefaults[jobs.SendArgs]
}

func (w *SendWorker) Work(ctx context.Context, job *river.Job[jobs.SendArgs]) error { return nil }

type PingWorker struct {
	river.WorkerDefaults[jobs.PingArgs]
}

func (w *PingWorker) Work(ctx context.Context, job *river.Job[jobs.PingArgs]) error { return nil }
`,
		"queue/sched.go": `package queue

import (
	"context"

	"ex.com/m/jobs"
)

type Scheduler struct{ client Inserter }

// ScheduleSend queues a send job.
func (s *Scheduler) ScheduleSend(ctx context.Context, id int64) error {
	args := jobs.SendArgs{ID: id}
	_, err := s.client.Insert(ctx, args, nil)
	return err
}

func (s *Scheduler) ScheduleSendTx(ctx context.Context, tx Tx, id int64) error {
	_, err := s.client.InsertTx(ctx, tx, &jobs.SendArgs{ID: id}, nil)
	return err
}

// Build makes args but queues nothing: no async edge.
func (s *Scheduler) Build(id int64) jobs.SendArgs {
	return jobs.SendArgs{ID: id}
}
`,
	})
	wantEdges(t, f, "Scheduler.ScheduleSend -> SendWorker.Work", "Scheduler.ScheduleSendTx -> SendWorker.Work")
	for _, e := range asyncOf(f) {
		if strings.Contains(e, "Build") || strings.Contains(e, "PingWorker") {
			t.Errorf("unexpected async edge %q", e)
		}
	}
	if n := len(asyncOf(f)); n != 2 {
		t.Errorf("want 2 async edges, got %d: %v", n, asyncOf(f))
	}
}

func TestAsyncSidekiq(t *testing.T) {
	f := runFiles(t, map[string]string{
		"app/workers/hard_worker.rb": `class HardWorker
  include Sidekiq::Worker

  def perform(id)
    puts id
  end
end
`,
		"app/controllers/orders_controller.rb": `class OrdersController
  def create
    HardWorker.perform_async(1)
  end
end
`,
	})
	wantEdges(t, f, "OrdersController.create -> HardWorker.perform")
}

func TestAsyncCelery(t *testing.T) {
	f := runFiles(t, map[string]string{
		"app/tasks.py": `from celery import shared_task

@shared_task
def send_email(user_id):
    pass

def not_a_task(x):
    pass
`,
		"app/views.py": `from app.tasks import send_email, not_a_task

def signup(request):
    send_email.delay(1)
    not_a_task.delay(2)
`,
	})
	wantEdges(t, f, "signup -> send_email")
	if n := len(asyncOf(f)); n != 1 {
		t.Errorf("only decorated tasks are linked, got %v", asyncOf(f))
	}
}

func TestAsyncBullMQ(t *testing.T) {
	f := runFiles(t, map[string]string{
		"src/worker.js": `import { Worker } from 'bullmq'

async function processEmail(job) {
  return job.data
}

export function startWorkers(conn) {
  return new Worker('email', processEmail, { connection: conn })
}
`,
		"src/api.js": `export async function signup(queue, user) {
  await queue.add('email', { user })
}
`,
	})
	wantEdges(t, f, "signup -> processEmail")
}
