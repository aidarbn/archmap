package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang/all"
)

func apiRepo(t *testing.T, files map[string]string) *facts.Facts {
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

func findEP(f *facts.Facts, pred func(facts.Endpoint) bool) *facts.Endpoint {
	for i := range f.Endpoints {
		if pred(f.Endpoints[i]) {
			return &f.Endpoints[i]
		}
	}
	return nil
}

func hasAccess(fn facts.Function, table, op string) bool {
	for _, a := range fn.Data {
		if a.Table == table && a.Op == op {
			return true
		}
	}
	return false
}

const apiGoMod = "module example.com/app\n\ngo 1.22\n"

func TestGRPCProtoWithHTTPAndRBAC(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod": apiGoMod,
		"api/shop.proto": `syntax = "proto3";
package shop.v1;
service ShopService {
  // Get an order
  rpc GetOrder(GetOrderRequest) returns (GetOrderResponse) {
    option (google.api.http) = {get: "/v1/orders/{id}"};
  }
  rpc Pay(PayRequest) returns (PayResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
}`,
		"rbac/policy.csv": "p, admin, /shop.v1.ShopService/*, *\np, *, /shop.v1.ShopService/Login, *\np, cashier, /shop.v1.ShopService/Pay, *\n",
		"srv/server.go": `package srv

type ShopServer struct{}

func (s *ShopServer) GetOrder(ctx context.Context, req *shopv1.GetOrderRequest) (*shopv1.GetOrderResponse, error) {
	return nil, nil
}
func (s *ShopServer) Pay(ctx context.Context, req *shopv1.PayRequest) (*shopv1.PayResponse, error) {
	return nil, nil
}
`,
	})
	e := findEP(f, func(e facts.Endpoint) bool { return e.Name == "GetOrder" })
	if e == nil {
		t.Fatalf("no GetOrder endpoint: %+v", f.Endpoints)
	}
	if e.Kind != "grpc" || e.Method != "GET" || e.Path != "/v1/orders/{id}" || e.Request != "GetOrderRequest" || e.Response != "GetOrderResponse" || e.Source != "proto" {
		t.Errorf("bad endpoint %+v", *e)
	}
	if e.Fid != "srv/server.go::ShopServer.GetOrder" {
		t.Errorf("fid = %q", e.Fid)
	}
	if e.Auth != "role:admin" {
		t.Errorf("auth = %q", e.Auth)
	}
	pay := findEP(f, func(e facts.Endpoint) bool { return e.Name == "Pay" })
	if pay.Path != "/shop.v1.ShopService/Pay" || pay.Method != "" || pay.Auth != "role:admin,cashier" {
		t.Errorf("pay = %+v", *pay)
	}
	if login := findEP(f, func(e facts.Endpoint) bool { return e.Name == "Login" }); login.Auth != "public" {
		t.Errorf("login auth = %q", login.Auth)
	}
}

func TestGoRoutesGroupsAndAuth(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod": apiGoMod,
		"web/routes.go": `package web

import "github.com/gin-gonic/gin"

type Handler struct{}

func (h *Handler) List(c *gin.Context)   {}
func (h *Handler) Create(c *gin.Context) {}
func (h *Handler) Ping(c *gin.Context)   {}

func Register(r *gin.Engine, h *Handler) {
	r.GET("/ping", h.Ping)
	api := r.Group("/api")
	v1 := api.Group("/v1", AuthMiddleware())
	v1.GET("/orders", h.List)
	v1.POST("/orders", h.Create)
}
`,
	})
	ping := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/ping" })
	if ping == nil || ping.Method != "GET" || ping.Fid != "web/routes.go::Handler.Ping" || ping.Auth != "" {
		t.Errorf("ping = %+v", ping)
	}
	post := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/api/v1/orders" && e.Method == "POST" })
	if post == nil || post.Fid != "web/routes.go::Handler.Create" || post.Auth != "authenticated" || post.Source != "router" {
		t.Errorf("post = %+v", post)
	}
}

func TestExpressMountedRouter(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"src/app.js": `const express = require('express');
const users = require('./routes/users');
const app = express();
app.use('/api/users', users);
`,
		"src/routes/users.js": `const router = require('express').Router();
const { list, remove } = require('../ctrl');
router.get('/', list);
router.delete('/:id', requireRole('admin'), remove);
module.exports = router;
`,
		"src/ctrl.js": "function list(req, res) {}\nfunction remove(req, res) {}\nmodule.exports = { list, remove };\n",
	})
	e := findEP(f, func(e facts.Endpoint) bool { return e.Method == "DELETE" })
	if e == nil || e.Path != "/api/users/:id" || e.Auth != "role:admin" || e.Fid != "src/ctrl.js::remove" {
		t.Errorf("delete = %+v (all %+v)", e, f.Endpoints)
	}
}

func TestSQLSchemaFromMigrations(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"main.go": "package main\nfunc main() {}\n",
		"db/migrations/001_init.sql": `-- +goose Up
CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE orders (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  total numeric(10,2) NOT NULL,
  legacy text,
  CONSTRAINT fk_x FOREIGN KEY (user_id) REFERENCES users (id)
);
-- +goose Down
DROP TABLE orders;
DROP TABLE users;
`,
		"db/migrations/002_more.sql": `-- +goose Up
ALTER TABLE orders ADD COLUMN status text NOT NULL DEFAULT 'new';
ALTER TABLE orders DROP COLUMN legacy;
ALTER TABLE orders ADD COLUMN payment_id uuid;
ALTER TABLE orders ADD CONSTRAINT orders_payment_fk FOREIGN KEY (payment_id) REFERENCES payments(id);
CREATE TABLE payments (id uuid primary key);
CREATE TABLE tmp_stuff (id int);
DROP TABLE tmp_stuff;
-- +goose Down
ALTER TABLE orders DROP COLUMN status;
`,
	})
	if f.Data == nil {
		t.Fatal("no data model")
	}
	o := f.Data.Tables["orders"]
	if o == nil || o.Source != "migration" {
		t.Fatalf("orders = %+v", o)
	}
	var names []string
	for _, fl := range o.Fields {
		names = append(names, fl.Name)
	}
	if got := strings.Join(names, ","); got != "id,user_id,total,status,payment_id" {
		t.Errorf("fields = %s", got)
	}
	if o.Fields[2].Type != "numeric(10,2)" {
		t.Errorf("type = %q", o.Fields[2].Type)
	}
	refs := map[string]string{}
	for _, r := range o.Refs {
		refs[r.Field] = r.Table + "." + r.To
	}
	if refs["user_id"] != "users.id" || refs["payment_id"] != "payments.id" || len(o.Refs) != 2 {
		t.Errorf("refs = %+v", o.Refs)
	}
	if _, ok := f.Data.Tables["tmp_stuff"]; ok {
		t.Error("dropped table is still there")
	}
}

func TestSQLAccessInCode(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"db/migrations/001.sql": "CREATE TABLE orders (id int, status text, total int);\nCREATE TABLE users (id int, name text);\nCREATE TABLE audit (id int);",
		"repo/repo.go": "package repo\n\n" +
			"func Save() {\n\texec(`INSERT INTO orders (id, status) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET status = $2`)\n}\n\n" +
			"func Close() {\n\texec(\"UPDATE orders SET status = $1, total = 0 WHERE id = $2\")\n\texec(\"DELETE FROM audit WHERE id = 1\")\n}\n\n" +
			"func Load() {\n\tq := `SELECT o.id FROM orders o JOIN users u ON u.id = o.user_id`\n\t// SELECT * FROM audit\n}\n\n" +
			"func Prose() {\n\t_ = \"update the set of rows from other\"\n\t_ = \"insert into nowhere (a) values (1)\"\n}\n",
	})
	get := func(n string) facts.Function { return f.Functions["repo/repo.go::"+n] }
	if a := get("Save"); !hasAccess(a, "orders", "upsert") || a.Data[0].Fields[0] != "id" {
		t.Errorf("Save = %+v", a.Data)
	}
	c := get("Close")
	if !hasAccess(c, "orders", "update") || !hasAccess(c, "audit", "delete") {
		t.Errorf("Close = %+v", c.Data)
	}
	if c.Data[0].Fields[0] != "status" || c.Data[0].Fields[1] != "total" || c.Data[0].Line != 8 {
		t.Errorf("Close update = %+v", c.Data[0])
	}
	l := get("Load")
	if !hasAccess(l, "orders", "read") || !hasAccess(l, "users", "read") || hasAccess(l, "audit", "read") {
		t.Errorf("Load = %+v", l.Data)
	}
	if len(get("Prose").Data) != 0 {
		t.Errorf("Prose = %+v", get("Prose").Data)
	}
}

func TestSqlcAndJetAccess(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod":                "module example.com/app\n\ngo 1.22\n",
		"db/migrations/001.sql": "CREATE TABLE payments (id int, status text);\nCREATE TABLE orders (id int, status text);",
		"db/sqlc/sqlc.yaml":     "version: \"2\"\nsql:\n  - schema: ../migrations\n    queries: queries\n    gen:\n      go:\n        package: sqlcgen\n",
		"db/sqlc/queries/p.sql": "-- name: CreatePayment :one\nINSERT INTO payments (id, status) VALUES ($1, $2) RETURNING *;\n\n-- name: GetPayment :one\nSELECT * FROM payments WHERE id = $1;\n",
		"gen/jet/public/table/orders.go": `// Code generated by go-jet DO NOT EDIT.
package table

var Orders = newOrdersTable("public", "orders", "")

func newOrdersTable() {
	IDColumn := postgres.StringColumn("id")
	StatusColumn := postgres.StringColumn("status")
}
`,
		"repo/pay.go": `package repo

import "example.com/app/db/sqlcgen"

type PayRepo struct{ q *sqlcgen.Queries }

func (r *PayRepo) Create() {
	r.q.CreatePayment(ctx, nil)
}

func (r *PayRepo) Get() {
	r.q.GetPayment(ctx, 1)
}
`,
		"repo/ord.go": `package repo

import "example.com/app/gen/jet/public/table"

func SetStatus() {
	t := table.Orders
	stmt := t.UPDATE(t.Status).SET("x")
	_ = stmt
}

func Find() {
	t := table.Orders
	postgres.SELECT(t.AllColumns).FROM(t)
}
`,
	})
	if a := f.Functions["repo/pay.go::PayRepo.Create"]; !hasAccess(a, "payments", "insert") || a.Data[0].Line != 8 {
		t.Errorf("Create = %+v", a.Data)
	}
	if a := f.Functions["repo/pay.go::PayRepo.Get"]; !hasAccess(a, "payments", "read") {
		t.Errorf("Get = %+v", a.Data)
	}
	if a := f.Functions["repo/ord.go::SetStatus"]; !hasAccess(a, "orders", "update") || a.Data[0].Fields[0] != "status" {
		t.Errorf("SetStatus = %+v", a.Data)
	}
	if a := f.Functions["repo/ord.go::Find"]; !hasAccess(a, "orders", "read") {
		t.Errorf("Find = %+v", a.Data)
	}
}

func TestDjangoEndpointsModelsAndORM(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"manage.py": "",
		"config/urls.py": `from django.urls import path, include
urlpatterns = [
    path("api/", include("shop.urls")),
]
`,
		"shop/urls.py": `from django.urls import path, include
from rest_framework.routers import DefaultRouter
from . import views

router = DefaultRouter()
router.register(r"orders", views.OrderViewSet)

urlpatterns = [
    path("health/", views.health),
    path("items/<int:pk>/", views.ItemView.as_view()),
    path("", include(router.urls)),
]
`,
		"shop/models.py": `from django.db import models

class Customer(models.Model):
    name = models.CharField(max_length=10)

class Order(models.Model):
    customer = models.ForeignKey(
        Customer, on_delete=models.CASCADE
    )
    status = models.CharField(max_length=10)

class Item(models.Model):
    title = models.CharField(max_length=10)
`,
		"shop/views.py": `from rest_framework import viewsets
from rest_framework.decorators import action, api_view
from rest_framework.permissions import IsAuthenticated, AllowAny
from .models import Order, Item

@api_view(["GET"])
def health(request):
    return None

class ItemView(APIView):
    permission_classes = [AllowAny]

    def get(self, request, pk):
        return Item.objects.get(pk=pk)

class OrderViewSet(viewsets.ModelViewSet):
    permission_classes = [IsAuthenticated]

    def create(self, request):
        Order.objects.create(status="new")

    @action(detail=True, methods=["post"])
    def close(self, request, pk=None):
        Order.objects.filter(pk=pk).update(status="closed")
`,
	})
	chk := func(method, path string) *facts.Endpoint {
		e := findEP(f, func(e facts.Endpoint) bool { return e.Method == method && e.Path == path })
		if e == nil {
			t.Fatalf("no %s %s in %+v", method, path, f.Endpoints)
		}
		return e
	}
	if e := chk("GET", "/api/health/"); e.Fid != "shop/views.py::health" {
		t.Errorf("health = %+v", e)
	}
	if e := chk("GET", "/api/items/{pk}/"); e.Fid != "shop/views.py::ItemView.get" || e.Auth != "public" {
		t.Errorf("item = %+v", e)
	}
	if e := chk("POST", "/api/orders/"); e.Fid != "shop/views.py::OrderViewSet.create" || e.Auth != "authenticated" {
		t.Errorf("create = %+v", e)
	}
	if e := chk("POST", "/api/orders/{pk}/close/"); e.Fid != "shop/views.py::OrderViewSet.close" {
		t.Errorf("close = %+v", e)
	}
	chk("GET", "/api/orders/{pk}/")
	o := f.Data.Tables["shop_order"]
	if o == nil || o.Source != "orm" || len(o.Refs) != 1 || o.Refs[0].Table != "shop_customer" || o.Refs[0].Field != "customer_id" {
		t.Fatalf("order table = %+v", o)
	}
	if fn := f.Functions["shop/views.py::OrderViewSet.create"]; !hasAccess(fn, "shop_order", "insert") || fn.Data[0].Fields[0] != "status" {
		t.Errorf("create data = %+v", fn.Data)
	}
	if fn := f.Functions["shop/views.py::OrderViewSet.close"]; !hasAccess(fn, "shop_order", "update") {
		t.Errorf("close data = %+v", fn.Data)
	}
	if fn := f.Functions["shop/views.py::ItemView.get"]; !hasAccess(fn, "shop_item", "read") {
		t.Errorf("get data = %+v", fn.Data)
	}
}

func TestSpringNestAndNextRoutes(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"src/OrderController.java": `package x;

@RestController
@RequestMapping("/orders")
public class OrderController {
    @GetMapping("/{id}")
    @PreAuthorize("hasRole('ADMIN')")
    public Order get(Long id) { return null; }

    @PostMapping
    public Order create() { return null; }
}
`,
		"src/cats.controller.ts": `@Controller('cats')
export class CatsController {
  @Get(':id')
  @UseGuards(AuthGuard)
  findOne() {}

  @Post()
  create() {}
}
`,
		"pages/users/[id].tsx": "export default function UserPage() { return null }\n",
	})
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/orders/{id}" }); e == nil || e.Method != "GET" || e.Auth != "role:ADMIN" || e.Fid != "src/OrderController.java::OrderController.get" {
		t.Errorf("spring = %+v (%+v)", e, f.Endpoints)
	}
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/orders" && e.Method == "POST" }); e == nil {
		t.Errorf("spring create missing: %+v", f.Endpoints)
	}
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/cats/{id}" }); e == nil || e.Auth != "authenticated" || e.Method != "GET" {
		t.Errorf("nest = %+v", e)
	}
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/users/{id}" }); e == nil || e.Name != "page" {
		t.Errorf("next = %+v", e)
	}
}

func TestDataClosureDepthAndAsync(t *testing.T) {
	f := &facts.Facts{Functions: map[string]facts.Function{
		"a": {Callees: []string{"b"}, Data: []facts.Access{{Table: "t1", Op: "read"}}},
		"b": {Callees: []string{"c"}, AsyncCallees: []string{"w"}, Data: []facts.Access{{Table: "t1", Op: "update", Fields: []string{"x"}}}},
		"c": {Callees: []string{"a"}, Data: []facts.Access{{Table: "t2", Op: "insert"}}},
		"w": {Data: []facts.Access{{Table: "t3", Op: "delete"}}},
	}}
	got := map[string]DataUse{}
	for _, u := range DataClosure(f, "a", 0) {
		got[u.Table] = u
	}
	if len(got) != 3 {
		t.Fatalf("closure = %+v", got)
	}
	if u := got["t1"]; !u.Reads() || !u.Writes() || u.Depth != 0 || len(u.Fields) != 1 {
		t.Errorf("t1 = %+v", u)
	}
	if got["t3"].Depth != 2 || got["t3"].Reads() || !got["t3"].Writes() {
		t.Errorf("t3 = %+v", got["t3"])
	}
	if n := len(DataClosure(f, "a", 1)); n != 1 {
		t.Errorf("depth 1 should see only t1 (b is depth 1 and also touches t1), got %d tables", n)
	}
	f.Endpoints = []facts.Endpoint{{Kind: "http", Method: "GET", Path: "/x", Fid: "a"}}
	if m := endpointMatrix(f, 10); !strings.Contains(m, "GET /x") || !strings.Contains(m, "t1:RW") || !strings.Contains(m, "t3:W") {
		t.Errorf("matrix = %q", m)
	}
}

func TestRailsRoutes(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"config/routes.rb": `Rails.application.routes.draw do
  root to: 'home#index'
  namespace :api do
    resources :orders, only: [:index, :show] do
      member do
        post :close
      end
    end
    get 'ping', to: 'health#ping'
  end
end
`,
		"app/controllers/api/orders_controller.rb": "class Api::OrdersController\n  def index\n  end\n  def show\n  end\nend\n",
		"app/controllers/api/health_controller.rb": "class Api::HealthController\n  def ping\n  end\nend\n",
	})
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/api/orders/{id}" && e.Method == "GET" }); e == nil {
		t.Errorf("show missing: %+v", f.Endpoints)
	}
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/api/ping" }); e == nil || e.Fid == "" {
		t.Errorf("ping = %+v", e)
	}
	if e := findEP(f, func(e facts.Endpoint) bool { return e.Path == "/" && e.Name == "index" }); e == nil {
		t.Errorf("root missing: %+v", f.Endpoints)
	}
}
