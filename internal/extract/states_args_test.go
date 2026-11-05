package extract

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// goFuncs cuts every top-level `func` of a Go file into an stFunc, named by the function (methods too).
func goFuncs(f stFile) []stFunc {
	var out []stFunc
	lines := strings.Split(f.Text, "\n")
	for i := 0; i < len(lines); i++ {
		m := goSigRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		end := i
		for end < len(lines) && lines[end] != "}" && !(end == i && strings.HasSuffix(lines[end], "}")) {
			end++
		}
		out = append(out, stFunc{Fid: m[1], File: f.Path, Start: i + 1, End: end + 1})
		i = end
	}
	return out
}

func trList(t *testing.T, src string, q map[string]string) []string {
	t.Helper()
	f := goFile(src)
	st := detectStates([]stFile{f}, goFuncs(f), q)["OrderStatus"]
	if st == nil {
		return nil
	}
	var got []string
	for _, x := range st.Transitions {
		s := x.Fid + ":" + x.From + ">" + x.To
		if x.Via != "" {
			s += " via " + x.Via
		}
		if x.Insert {
			s += " new"
		}
		got = append(got, s)
	}
	sort.Strings(got)
	return got
}

const argHead = `package app
type OrderStatus string
const (
	OrderStatusNew      OrderStatus = "new"
	OrderStatusPaid     OrderStatus = "paid"
	OrderStatusFailed   OrderStatus = "failed"
	OrderStatusCanceled OrderStatus = "canceled"
)
`

func TestStatusPassedAsArgument(t *testing.T) {
	q := map[string]string{
		"CompleteOrder":      "-- name: CompleteOrder :exec\n-- WHERE on status keeps it atomic\nUPDATE orders SET status = $2 WHERE id = $1",
		"CreateOrder":        "INSERT INTO orders (name, status, created_at) VALUES ($1, 'new', NOW())",
		"ActivateOrder":      "-- name: ActivateOrder :one\n-- WHERE status guards the race\nUPDATE orders\nSET status = 'paid'\nWHERE id = $1 AND status = 'new'",
		"ListOrdersByStatus": "SELECT * FROM orders WHERE status = $1",
	}
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"constant passed to a repo setter (Params{Status: string(status)})", `
func (r *Repo) Complete(ctx context.Context, id int64, status OrderStatus) error {
	return r.q.CompleteOrder(ctx, CompleteOrderParams{ID: id, Status: string(status)})
}
func (s *Svc) Fail(ctx context.Context, id int64) error {
	return s.repo.Complete(ctx, id, OrderStatusFailed)
}`, []string{"Fail:>failed via Complete"}},
		{"through a helper that hands the status on", `
func (r *Repo) Complete(ctx context.Context, id int64, status OrderStatus) error {
	return r.q.CompleteOrder(ctx, CompleteOrderParams{ID: id, Status: string(status)})
}
func (s *Svc) finish(ctx context.Context, id int64, newStatus OrderStatus) error {
	return s.repo.Complete(ctx, id, newStatus)
}
func (s *Svc) Cancel(ctx context.Context, id int64) error {
	return s.finish(ctx, id, OrderStatusCanceled)
}`, []string{"Cancel:>canceled via finish"}},
		{"a mapper result handed to a setter: every status it returns, no source", `
func MapStatus(code string) OrderStatus {
	switch code {
	case "ok":
		return OrderStatusPaid
	default:
		return OrderStatusFailed
	}
}
func (r *Repo) Complete(ctx context.Context, id int64, status OrderStatus) error {
	return r.q.CompleteOrder(ctx, CompleteOrderParams{ID: id, Status: string(status)})
}
func (s *Svc) Poll(ctx context.Context, id int64, code string) error {
	st := MapStatus(code)
	if st == OrderStatusPaid {
		log.Print("paid")
	}
	return s.repo.Complete(ctx, id, st)
}`, []string{"Poll:>failed via Complete", "Poll:>paid via Complete"}},
		{"INSERT … VALUES with a status literal is a creation", `
func (r *Repo) Create(ctx context.Context, name string) error {
	return r.q.CreateOrder(ctx, name)
}`, []string{"Create:>new new"}},
		{"a WHERE in an SQL comment does not hide the SET", `
func (r *Repo) Activate(ctx context.Context, id int64) error {
	return r.q.ActivateOrder(ctx, id)
}`, []string{"Activate:new>paid"}},
		{"a function named like the query is not a use of it", `
func (s *Server) ActivateOrder(ctx context.Context, id int64) error {
	return s.svc.Do(ctx, id)
}`, nil},
		{"a filter is a read, not a write", `
func (r *Repo) Paid(ctx context.Context) error {
	return r.q.ListOrdersByStatus(ctx, ListOrdersParams{Status: string(OrderStatusPaid)})
}`, nil},
		{"a status argument of a function that is not a setter is nothing", `
func (s *Svc) Log(ctx context.Context, status OrderStatus) { log.Print(status) }
func (s *Svc) Touch(ctx context.Context) { s.Log(ctx, OrderStatusPaid) }`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := trList(t, argHead+c.body+"\n", q)
			if !reflect.DeepEqual(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestTableNames(t *testing.T) {
	cases := []struct {
		table, key string
		want       bool
	}{
		{"orders", "OrderStatus", true},
		{"kaspi_certificates", "KaspiCertStatus", true},
		{"kaspi_certificates", "RestaurantStatus", false},
		{"sync_runs", "SyncRunStatus", true},
		{"sync_run_meta", "SyncRunStatus", false},
	}
	for _, c := range cases {
		if got := tableNames(c.table, c.key); got != c.want {
			t.Errorf("tableNames(%q, %q) = %v, want %v", c.table, c.key, got, c.want)
		}
	}
}
