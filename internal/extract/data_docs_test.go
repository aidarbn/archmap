package extract

import (
	"strings"
	"testing"

	"github.com/aidarbn/archmap/internal/facts"
)

func fieldOf(t *facts.Table, name string) *facts.Field {
	if t == nil {
		return nil
	}
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

// want: "table" → table description, "table.col" → column description ("" = none).
func checkDocs(t *testing.T, f *facts.Facts, want map[string]string) {
	t.Helper()
	if f.Data == nil {
		t.Fatal("no data model")
	}
	for k, w := range want {
		tbl, col := k, ""
		for i := range k {
			if k[i] == '.' {
				tbl, col = k[:i], k[i+1:]
			}
		}
		tb := f.Data.Tables[tbl]
		if tb == nil {
			t.Errorf("%s: no table %q", k, tbl)
			continue
		}
		got := tb.Desc
		if col != "" {
			fl := fieldOf(tb, col)
			if fl == nil {
				t.Errorf("%s: no column", k)
				continue
			}
			got = fl.Desc
		}
		if got != w {
			t.Errorf("%s: desc = %q, want %q", k, got, w)
		}
	}
}

func TestSQLColumnDocs(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  map[string]string
	}{
		{
			name: "comment on, last wins, down ignored",
			files: map[string]string{
				"db/migrations/001_init.sql": `-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
  id uuid PRIMARY KEY,
  total numeric(10,2) NOT NULL,
  note text
);
-- +goose StatementEnd
COMMENT ON TABLE orders IS 'Customer orders';
COMMENT ON COLUMN orders.total IS
    'Sum, tenge; it''s with VAT (incl. delivery)';
COMMENT ON COLUMN public.orders.note IS 'old note';
-- +goose Down
COMMENT ON COLUMN orders.total IS 'down part';
DROP TABLE orders;
`,
				"db/migrations/002_note.sql": `-- +goose Up
COMMENT ON COLUMN orders.note IS 'A note from the cashier';
-- +goose Down
COMMENT ON COLUMN orders.note IS NULL;
`,
			},
			want: map[string]string{
				"orders":       "Customer orders",
				"orders.total": "Sum, tenge; it's with VAT (incl. delivery)",
				"orders.note":  "A note from the cashier",
				"orders.id":    "",
			},
		},
		{
			name: "line comments in create table",
			files: map[string]string{
				"migrations/001.sql": `-- +goose Up

-- Restaurants of the network
-- (one row per legal entity)
CREATE TABLE restaurants (
    id   UUID PRIMARY KEY, -- the id
    -- shown in the admin
    -- and on receipts
    name VARCHAR(255) NOT NULL,
    url  TEXT DEFAULT 'http://x--y', -- site; may be empty
    -- a check below is not a column
    CONSTRAINT name_ok CHECK (name <> ''),
    deleted_at TIMESTAMPTZ -- soft delete
);

-- this comment is followed by a blank line

CREATE TABLE plain (id int, k int, UNIQUE(id, k));
COMMENT ON COLUMN plain.k IS 'Kinds:
   - a: first
   - b: second';
CREATE TABLE inline ( -- a table described on the paren line
  a int,
  b int
);
`,
			},
			want: map[string]string{
				"restaurants":            "Restaurants of the network (one row per legal entity)",
				"restaurants.id":         "the id",
				"restaurants.name":       "shown in the admin and on receipts",
				"restaurants.url":        "site; may be empty",
				"restaurants.deleted_at": "soft delete",
				"plain":                  "",
				"plain.k":                "Kinds:\n- a: first\n- b: second",
				"inline":                 "a table described on the paren line",
				"inline.a":               "",
			},
		},
		{
			name: "alter table add column",
			files: map[string]string{
				"db/migrations/001.sql": `CREATE TABLE payments (id uuid primary key);
ALTER TABLE payments ADD COLUMN error_message TEXT; -- last error from the bank
-- the operator who confirmed it
ALTER TABLE payments ADD COLUMN IF NOT EXISTS operator_id uuid;
ALTER TABLE payments
    -- amount in tiyn
    ADD COLUMN amount bigint NOT NULL DEFAULT 0,
    ADD COLUMN currency text; -- ISO code
ALTER TABLE payments ADD COLUMN plain int;
ALTER TABLE payments ALTER COLUMN plain SET NOT NULL;
`,
				"db/migrations/002.sql": `-- error text from the provider
ALTER TABLE payments ALTER COLUMN error_message TYPE varchar(500);
COMMENT ON COLUMN payments.operator_id IS 'Who confirmed the payment';
`,
			},
			want: map[string]string{
				"payments.error_message": "last error from the bank",
				"payments.operator_id":   "Who confirmed the payment",
				"payments.amount":        "amount in tiyn",
				"payments.currency":      "ISO code",
				"payments.plain":         "",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"main.go": "package main\nfunc main() {}\n"}
			for k, v := range tc.files {
				files[k] = v
			}
			f := apiRepo(t, files)
			checkDocs(t, f, tc.want)
			for name, tb := range f.Data.Tables {
				for _, fl := range tb.Fields {
					if strings.ContainsAny(fl.Name, "(,") {
						t.Errorf("%s: a constraint read as the column %q", name, fl.Name)
					}
				}
			}
		})
	}
}

func TestSQLNotNull(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"main.go": "package main\nfunc main() {}\n",
		"db/migrations/001.sql": `CREATE TABLE t (id uuid primary key, a text not null, b text, c text);
ALTER TABLE t ALTER COLUMN b SET NOT NULL;
ALTER TABLE t ALTER COLUMN a DROP NOT NULL;
`,
	})
	tb := f.Data.Tables["t"]
	for col, want := range map[string]bool{"id": true, "a": false, "b": true, "c": false} {
		if got := fieldOf(tb, col).NotNull; got != want {
			t.Errorf("%s: notNull = %v, want %v", col, got, want)
		}
	}
}

func TestGoStructColumnDocs(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"go.mod": apiGoMod,
		"db/migrations/001.sql": `CREATE TABLE orders (id uuid primary key, user_id uuid, status text, total numeric);
COMMENT ON COLUMN orders.total IS 'From SQL';
CREATE TABLE audit_log (id bigint, actor text, action text, at timestamptz);
CREATE TABLE categories (id int, title text);
`,
		"internal/db/models.go": `package db

// Order is a customer order.
type Order struct {
	ID     string ` + "`json:\"id\"`" + `
	// UserID is the buyer.
	UserID string ` + "`json:\"user_id\"`" + `
	Status string ` + "`json:\"status\"`" + ` // new, paid or cancelled
	Total  string ` + "`json:\"total\"`" + ` // from Go
}

// Category — a menu section.
type Category struct {
	ID    int
	Title string // shown on the menu
}

// a struct with db tags matched by its columns
type logEntry struct {
	ID     int64  ` + "`db:\"id\"`" + `
	Actor  string ` + "`db:\"actor\"`" + ` // who did it
	Action string ` + "`db:\"action\"`" + `
	Skip   string ` + "`db:\"-\"`" + ` // not a column
}

// DTO has json tags but no matching name: not a table.
type OrderDTO2 struct {
	Status string ` + "`json:\"status\"`" + ` // dto status
}
`,
	})
	checkDocs(t, f, map[string]string{
		"orders":           "Order is a customer order.",
		"orders.user_id":   "UserID is the buyer.",
		"orders.status":    "new, paid or cancelled",
		"orders.total":     "From SQL",
		"orders.id":        "",
		"categories":       "a menu section.",
		"categories.title": "shown on the menu",
		"audit_log.actor":  "who did it",
		"audit_log.action": "",
	})
}

func TestDjangoFieldDocs(t *testing.T) {
	f := apiRepo(t, map[string]string{
		"manage.py": "import django\n",
		"shop/models.py": `from django.db import models
from django.utils.translation import gettext_lazy as _


class Product(models.Model):
    """A thing we sell."""
    title = models.CharField(max_length=100, help_text=_("Shown in the catalogue"))
    sku = models.CharField(max_length=20, verbose_name="Stock keeping unit")
    price = models.DecimalField(max_digits=8, decimal_places=2)
    shop = models.ForeignKey("Shop", on_delete=models.CASCADE, help_text='Owner shop')


class Shop(models.Model):
    name = models.CharField(max_length=50)
`,
	})
	checkDocs(t, f, map[string]string{
		"shop_product":         "A thing we sell.",
		"shop_product.title":   "Shown in the catalogue",
		"shop_product.sku":     "Stock keeping unit",
		"shop_product.price":   "",
		"shop_product.shop_id": "Owner shop",
	})
}
