package autosem

import (
	"strings"
	"unicode"
)

// acronyms are rendered in their canonical spelling inside role names.
var acronyms = map[string]string{
	"grpc": "gRPC", "http": "HTTP", "https": "HTTPS", "db": "DB", "api": "API", "sql": "SQL",
	"ui": "UI", "url": "URL", "id": "ID", "cli": "CLI", "jwt": "JWT", "rpc": "RPC", "s3": "S3",
	"rest": "REST", "json": "JSON", "pg": "Postgres", "qr": "QR", "crm": "CRM", "pos": "POS", "ws": "WebSocket",
}

// splitWords splits snake_case, kebab-case, camelCase and dotted names.
func splitWords(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case r == '_' || r == '-' || r == '.' || r == ' ' || r == '@' || r == '[' || r == ']' || r == '(' || r == ')':
			flush()
		case unicode.IsUpper(r):
			if len(cur) > 0 {
				prev := cur[len(cur)-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) ||
					(unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
					flush()
				}
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// humanize turns an identifier or path segment into a readable phrase.
func humanize(s string) string {
	ws := splitWords(s)
	if len(ws) == 0 {
		return ""
	}
	for i, w := range ws {
		lw := strings.ToLower(w)
		if a, ok := acronyms[lw]; ok {
			ws[i] = a
		} else {
			ws[i] = lw
		}
	}
	if r := []rune(ws[0]); unicode.IsLower(r[0]) && ws[0] == strings.ToLower(ws[0]) && !isAcronymSpelling(ws[0]) {
		r[0] = unicode.ToUpper(r[0])
		ws[0] = string(r)
	}
	return strings.Join(ws, " ")
}

func isAcronymSpelling(w string) bool {
	for _, a := range acronyms {
		if a == w {
			return true
		}
	}
	return false
}

// genericSegments are folder names that do not name a service on their own.
var genericSegments = map[string]bool{
	"client": true, "clients": true, "httpclient": true, "http": true, "api": true, "sdk": true, "out": true, "adapter": true,
	"adapters": true, "integration": true, "integrations": true, "external": true, "internal": true, "pkg": true, "src": true,
	"gateway": true, "gateways": true, "provider": true, "providers": true, "v1": true, "v2": true, "v3": true,
}

var serviceParents = map[string]bool{
	"out": true, "outbound": true, "clients": true, "client": true, "integrations": true, "integration": true,
	"external": true, "gateways": true, "providers": true, "sdk": true,
}

var dbSegments = map[string]bool{
	"db": true, "database": true, "repository": true, "repositories": true, "repo": true, "repos": true, "store": true, "stores": true,
	"storage": true, "dao": true, "persistence": true, "sql": true, "postgres": true, "redis": true, "cache": true, "queue": true, "dbqueue": true,
	"casbin": true,
}

// externalService returns the name of the external service an adapter module
// talks to ("" when it is not a service client): the folder under
// out/clients/integrations, or a folder named <svc>client.
func externalService(mod string) string {
	segs := strings.Split(mod, "/")
	for i := len(segs) - 2; i >= 0; i-- {
		next := strings.ToLower(segs[i+1])
		if serviceParents[strings.ToLower(segs[i])] && !genericSegments[next] && !dbSegments[next] {
			return humanize(segs[i+1])
		}
	}
	last := strings.ToLower(segs[len(segs)-1])
	if strings.HasSuffix(last, "client") && last != "client" && last != "httpclient" {
		return humanize(segs[len(segs)-1][:len(last)-len("client")])
	}
	if len(segs) >= 2 && genericSegments[last] && !dbSegments[last] && last != "out" {
		p := strings.ToLower(segs[len(segs)-2])
		if !genericSegments[p] && !dbSegments[p] {
			return humanize(segs[len(segs)-2])
		}
	}
	return ""
}

// roleOf builds the human-readable module name.
func roleOf(mod, layer string) string {
	if mod == "." || mod == "" {
		return "Project root"
	}
	segs := strings.Split(mod, "/")
	last := segs[len(segs)-1]
	if layer == Adapter {
		if svc := externalService(mod); svc != "" {
			return svc + " client"
		}
	}
	// version-like or "internal" last segments read better with their parent
	if len(segs) > 1 && last == "src" { // <crate>/src is the crate itself
		last = segs[len(segs)-2]
	} else if len(segs) > 1 && (last == "internal" || last == "src" || last == "lib" || last == "app" || last == "pkg" || (len(last) <= 3 && strings.HasPrefix(strings.ToLower(last), "v"))) {
		last = segs[len(segs)-2] + " " + last
	}
	return humanize(last)
}
