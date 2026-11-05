package lang

import "testing"

func TestIsDirectiveLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"//nolint:gocyclo", true},
		{"// nolint", true},
		{"//go:generate mockgen", true},
		{"//lint:ignore SA1019 old API", true},
		{"// arch:flow pay \"Pay\"", true},
		{"# noqa: E501", true},
		{"# type: ignore", true},
		{"# pylint: disable=too-many-args", true},
		{"// eslint-disable-next-line no-console", true},
		{"// @ts-ignore", true},
		{"// swiftlint:disable line_length", true},
		{"// MARK: - Actions", true},
		{"//export Add", true},
		{"// Pay charges the cart.", false},
		{"Creates an order and returns its id.", false},
		{"// Pragmatic shortcut for the cache.", false},
		{"// Exports the report as CSV.", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsDirectiveLine(c.line); got != c.want {
			t.Errorf("IsDirectiveLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestSummary(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"short text is kept", "Языки (i18n)", "Языки (i18n)"},
		{"lines are joined", "Админский сервис управления\nрестораном и продуктами", "Админский сервис управления рестораном и продуктами"},
		{"directives dropped", "Получить меню филиала\nnolint:revive\narch:step x \"y\"", "Получить меню филиала"},
		{"first paragraph only", "Создать заказ.\n\nПодробности реализации.", "Создать заказ."},
		{"decoration only", "==========", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Summary(c.raw); got != c.want {
				t.Errorf("Summary(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}
