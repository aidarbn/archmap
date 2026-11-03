package autosem

import (
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// Layers, shallow to deep.
const (
	Entry   = "entry"
	UI      = "ui"
	Domain  = "domain"
	Adapter = "adapter"
	Infra   = "infra"
	Test    = "test"
	Other   = "other"
)

// rank orders layers by call depth (entry -> domain -> adapter -> infra); -1 = unranked.
func rank(layer string) int {
	switch layer {
	case Entry, UI:
		return 0
	case Domain:
		return 1
	case Adapter:
		return 2
	case Infra:
		return 3
	}
	return -1
}

// compoundRules match a whole module path and win over per-segment rules.
// Order matters: first match wins.
var compoundRules = []struct {
	re    *regexp.Regexp
	layer string
}{
	{regexp.MustCompile(`(^|/)adapters?/(out|outbound|driven|secondary)(/|$)`), Adapter},
	{regexp.MustCompile(`(^|/)adapters?/(in|inbound|driving|primary)(/|$)`), Entry},
}

// segmentRules maps a lowercase path segment to a layer. Segments are checked
// from the last to the first; the first one that matches decides.
var segmentRules = map[string]string{}

func init() {
	add := func(layer string, words ...string) {
		for _, w := range words {
			segmentRules[w] = layer
		}
	}
	add(Test, "test", "tests", "testutil", "testutils", "testing", "testdata", "mock", "mocks", "fixture", "fixtures", "__tests__", "__mocks__", "e2e", "spec", "specs",
		"example", "examples", "bench", "benches", "benchmark", "benchmarks", "fuzz", "fuzz_targets")
	add(Entry, "cmd", "main", "handler", "handlers", "controller", "controllers", "grpc", "http", "httpgw", "api", "rest",
		"route", "routes", "router", "routers", "server", "servers", "transport", "endpoint", "endpoints",
		"worker", "workers", "job", "jobs", "consumer", "consumers", "cli", "command", "commands", "cron", "scheduler", "schedulers",
		"rpc", "graphql", "websocket", "ws", "webhook", "webhooks")
	add(Domain, "hook", "hooks", "context", "contexts", "store", "reducer", "reducers", "saga", "sagas", "service", "services", "usecase", "usecases", "use_cases", "domain", "core", "model", "models", "entity", "entities",
		"business", "logic", "biz", "application")
	add(Adapter, "adapter", "adapters", "repository", "repositories", "repo", "repos", "db", "database", "stores", "storage",
		"dao", "persistence", "client", "clients", "integration", "integrations", "external", "gateway", "gateways", "provider",
		"providers", "sdk", "queue", "dbqueue", "cache", "redis", "postgres", "sql")
	add(Infra, "config", "configs", "configuration", "settings", "pkg", "util", "utils", "lib", "libs", "common", "shared", "platform",
		"log", "logger", "logging", "metrics", "telemetry", "tracing", "middleware", "middlewares", "helper", "helpers", "i18n",
		"errors", "constants", "types")
	add(UI, "component", "components", "screen", "screens", "page", "pages", "ui", "widgets", "widget", "view", "views",
		"layout", "layouts", "fragments", "activity", "activities", "composables")
}

// frontendLang reports languages whose "views/pages" are screens, not HTTP views.
func frontendLang(l string) bool {
	switch l {
	case "js", "ts", "tsx", "jsx", "javascript", "typescript", "kotlin", "swift", "dart":
		return true
	}
	return false
}

// pathLayer classifies a module by its path only; Other when nothing matches.
// lang is the dominant language of the module; it disambiguates "views"/"api".
func pathLayer(mod, lang string) string {
	m := strings.ToLower(mod)
	for _, r := range compoundRules {
		if r.re.MatchString(m) {
			return r.layer
		}
	}
	segs := strings.Split(m, "/")
	if lang == "rust" {
		if i := cratePrefix(segs); i >= 0 {
			// Cargo layout <dir>/<crate>/src/<mods>: crate names are free-form
			// ("cli", "core", "printer") and say nothing about a layer; only
			// the module folders inside src do. Test folders still win.
			for _, s := range segs {
				if segmentRules[s] == Test {
					return Test
				}
			}
			segs = segs[i+1:]
		}
	}
	for i := len(segs) - 1; i >= 0; i-- {
		s := segs[i]
		if s == "internal" {
			if i == len(segs)-1 {
				return Infra
			}
			continue
		}
		l, ok := segmentRules[s]
		if !ok {
			continue
		}
		if frontendLang(lang) && s == "api" {
			return Adapter // client-side "api" folder = API client
		}
		if !frontendLang(lang) && (s == "view" || s == "views") {
			return Entry
		}
		return l
	}
	return Other
}

// cratePrefix returns the index of the "src" segment of a Cargo crate path
// (<dir>/<crate>/src/...), -1 when the module is not inside a crate's src.
func cratePrefix(segs []string) int {
	for i := 1; i < len(segs); i++ {
		if segs[i] == "src" {
			return i
		}
	}
	return -1
}

// moduleStats summarizes a module's functions for content-based heuristics.
type moduleStats struct {
	funcs, entries, uiEntries int
	hasMain                   bool
	lang                      string
}

func collectStats(f *facts.Facts) map[string]*moduleStats {
	st := map[string]*moduleStats{}
	langCount := map[string]map[string]int{}
	for _, fn := range f.Functions {
		s := st[fn.Module]
		if s == nil {
			s = &moduleStats{}
			st[fn.Module] = s
			langCount[fn.Module] = map[string]int{}
		}
		s.funcs++
		langCount[fn.Module][fn.Lang]++
		if fn.Entry != nil {
			switch fn.Entry.Kind {
			case "main":
				s.hasMain = true
				s.entries++
			case "http", "task":
				s.entries++
			case "ui":
				s.uiEntries++
			}
		}
	}
	for m, s := range st {
		best, n := "", 0
		for l, c := range langCount[m] {
			if c > n || (c == n && l < best) {
				best, n = l, c
			}
		}
		s.lang = best
	}
	return st
}

// classify decides the layer of a module from its path and content.
func classify(mod string, s *moduleStats) string {
	if s == nil {
		s = &moduleStats{}
	}
	l := pathLayer(mod, s.lang)
	if l == Test {
		return l
	}
	if s.hasMain {
		return Entry
	}
	if l == Other && s.funcs > 0 {
		switch {
		case s.entries*2 >= s.funcs:
			return Entry
		case s.uiEntries*2 >= s.funcs:
			return UI
		case s.lang == "rust" && cratePrefix(strings.Split(strings.ToLower(mod), "/")) >= 0:
			return Domain // a library crate holds logic; a binary crate was caught by main above
		}
	}
	return l
}
