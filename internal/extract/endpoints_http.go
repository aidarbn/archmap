package extract

import (
	"path"
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

var (
	routeCallRE  = regexp.MustCompile("(\\w+(?:\\.\\w+)*)\\.(Get|Post|Put|Patch|Delete|Head|Options|GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|Handle|HandleFunc|Any|All|get|post|put|patch|delete|head|options|all)\\(\\s*(?:\"([^\"]*)\"|'([^']*)'|`([^`]*)`)\\s*(?:,\\s*(.*))?$")
	handleHTTPRE = regexp.MustCompile(`HandleHTTP\(\s*\w+\s*,\s*"([^"]*)"\s*,\s*(.*)$`)
	groupDeclRE  = regexp.MustCompile("(\\w+)\\s*:?=\\s*(\\w+)\\.(?:Group|PathPrefix)\\(\\s*[\"'`]([^\"'`]*)[\"'`]\\s*(?:,(.*))?")
	chiRouteRE   = regexp.MustCompile("(\\w+)\\.Route\\(\\s*\"([^\"]*)\"\\s*,\\s*func\\(\\s*(\\w+)")
	useCallRE    = regexp.MustCompile(`(\w+)\.(?:Use|use|With)\(([^)]*)\)`)
	jsMountRE    = regexp.MustCompile(`\.use\(\s*['"]([^'"]*)['"]\s*,\s*([\w.]+)`)
	jsImportRE   = regexp.MustCompile(`(?:import\s+(\w+)\s+from|(?:const|let|var)\s+(\w+)\s*=\s*require\()\s*['"](\.[^'"]*)['"]`)
	goRouterPkg  = regexp.MustCompile(`net/http|go-chi|gin-gonic|labstack/echo|gofiber|gorilla/mux|julienschmidt|cloudwego/hertz|kataras/iris|beego|go-zero|fasthttp`)
	jsRouterPkg  = regexp.MustCompile(`express|fastify|koa|hono|[rR]outer`)
	authWordsRE  = regexp.MustCompile(`(?i)\b(auth\w*|jwt\w*|passport\.authenticate|verify\w*token|require\w*auth|requireuser|is_?authenticated|protect\w*|guard\w*|ensure\w*logged\w*|login_?required|check\w*auth)\b`)
	roleArgRE    = regexp.MustCompile(`(?i)\b(?:require\w*role|has\w*role|roles?|authorize|permit)\w*\(\s*['"]([\w:\-]+)['"]`)
	jsExts       = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true}
	handlerTok   = regexp.MustCompile(`[A-Za-z_][\w.]*`)
)

func authFromText(s string) string {
	if m := roleArgRE.FindStringSubmatch(s); m != nil {
		return "role:" + m[1]
	}
	if authWordsRE.MatchString(s) {
		return "authenticated"
	}
	return ""
}

// handlerExpr extracts the handler identifier from the rest of a route call:
// the last dotted identifier that is not a function literal.
func handlerExpr(rest string) (expr string, inline bool) {
	if strings.Contains(rest, "func(") || strings.Contains(rest, "=>") || strings.Contains(rest, "function") {
		return "", true
	}
	rest = strings.TrimRight(strings.TrimSpace(rest), ");, {")
	toks := handlerTok.FindAllString(rest, -1)
	for i := len(toks) - 1; i >= 0; i-- {
		switch toks[i] {
		case "nil", "true", "false", "async", "await", "new", "http", "HandlerFunc":
			continue
		}
		return toks[i], false
	}
	return "", false
}

type routeFrame struct {
	depth  int
	v      string
	prefix string
	authed bool
}

// routerEndpoints finds Go (net/http, chi, gin, echo, fiber, gorilla) and
// JS/TS (express, fastify, koa) route registrations.
func (c *apiCtx) routerEndpoints() []facts.Endpoint {
	mounts := c.jsMounts()
	var eps []facts.Endpoint
	for _, p := range c.paths {
		ext := path.Ext(p)
		isGo := ext == ".go"
		if !isGo && !jsExts[ext] || isTestPath(p) {
			continue
		}
		src := c.read(p)
		if src == "" || (isGo && !goRouterPkg.MatchString(src)) || (!isGo && !jsRouterPkg.MatchString(src)) {
			continue
		}
		if strings.Contains(src, "@Controller(") {
			continue // Nest
		}
		varPrefix := map[string]string{}
		varAuth := map[string]bool{}
		var frames []routeFrame
		depth := 0
		filePrefix := mounts[p]
		lookup := func(v string) (string, bool) {
			for i := len(frames) - 1; i >= 0; i-- {
				if frames[i].v == v {
					return frames[i].prefix, frames[i].authed
				}
			}
			return varPrefix[v], varAuth[v]
		}
		for i, line := range strings.Split(src, "\n") {
			if isCodeComment(line) {
				continue
			}
			for len(frames) > 0 && depth < frames[len(frames)-1].depth {
				frames = frames[:len(frames)-1]
			}
			if m := groupDeclRE.FindStringSubmatch(line); m != nil {
				pp, pa := lookup(m[2])
				varPrefix[m[1]] = joinPath(pp, m[3])
				varAuth[m[1]] = pa || authFromText(m[4]) != ""
			}
			if m := chiRouteRE.FindStringSubmatch(line); m != nil {
				pp, pa := lookup(m[1])
				frames = append(frames, routeFrame{depth: depth + 1, v: m[3], prefix: joinPath(pp, m[2]), authed: pa})
			}
			if m := useCallRE.FindStringSubmatch(line); m != nil && authFromText(m[2]) != "" {
				v := m[1]
				placed := false
				for k := len(frames) - 1; k >= 0; k-- {
					if frames[k].v == v {
						frames[k].authed = true
						placed = true
						break
					}
				}
				if !placed {
					varAuth[v] = true
				}
			}
			m := routeCallRE.FindStringSubmatch(line)
			if m == nil {
				if hm := handleHTTPRE.FindStringSubmatch(line); hm != nil { // platform-go: api.HandleHTTP(app, "GET /x", h)
					m = []string{hm[0], "", "HandleFunc", hm[1], "", "", hm[2]}
				}
			}
			if m != nil {
				verb, pth := m[2], m[3]+m[4]+m[5]
				method := strings.ToUpper(verb)
				switch method {
				case "HANDLE", "HANDLEFUNC", "ANY", "ALL":
					method = ""
				}
				if sm := regexp.MustCompile(`^([A-Z]+)\s+(/.*)$`).FindStringSubmatch(pth); sm != nil {
					method, pth = sm[1], sm[2]
				}
				if !strings.HasPrefix(pth, "/") {
					continue
				}
				pp, pa := lookup(m[1])
				full := joinPath(filePrefix, joinPath(pp, pth))
				e := facts.Endpoint{Kind: "http", Method: method, Path: full, Source: "router"}
				expr, inline := handlerExpr(m[6])
				if !inline && expr != "" {
					e.Fid = c.handler(p, expr)
					e.Name = lastSeg(expr)
				}
				if a := authFromText(m[6]); a != "" {
					e.Auth = a
				} else if pa {
					e.Auth = "authenticated"
				}
				if e.Fid == "" && inline {
					e.Name = "inline"
				}
				if e.Fid == "" && e.Name == "" {
					continue
				}
				eps = append(eps, e)
				_ = i
			}
			depth += strings.Count(line, "{") - strings.Count(line, "}")
		}
	}
	return eps
}

// jsMounts maps a router file to the URL prefix it is mounted under (app.use('/x', router)).
func (c *apiCtx) jsMounts() map[string]string {
	type edge struct{ parent, child, prefix string }
	var edges []edge
	for _, p := range c.paths {
		if !jsExts[path.Ext(p)] || isTestPath(p) {
			continue
		}
		src := c.read(p)
		if !strings.Contains(src, ".use(") {
			continue
		}
		imports := map[string]string{}
		for _, m := range jsImportRE.FindAllStringSubmatch(src, -1) {
			name := m[1]
			if name == "" {
				name = m[2]
			}
			imports[name] = c.resolveRel(p, m[3])
		}
		for _, m := range jsMountRE.FindAllStringSubmatch(src, -1) {
			name := strings.Split(m[2], ".")[0]
			if f := imports[name]; f != "" {
				edges = append(edges, edge{p, f, m[1]})
			}
		}
	}
	res := map[string]string{}
	var prefixOf func(f string, depth int) string
	prefixOf = func(f string, depth int) string {
		if depth > 5 {
			return ""
		}
		for _, e := range edges {
			if e.child == f {
				return joinPath(prefixOf(e.parent, depth+1), e.prefix)
			}
		}
		return ""
	}
	for _, e := range edges {
		res[e.child] = prefixOf(e.child, 0)
	}
	return res
}

func (c *apiCtx) resolveRel(from, rel string) string {
	base := path.Join(path.Dir(from), rel)
	for _, cand := range []string{base, base + ".js", base + ".ts", base + ".mjs", base + "/index.js", base + "/index.ts"} {
		if c.read(cand) != "" {
			return cand
		}
	}
	return ""
}

var (
	pyDecoRouteRE = regexp.MustCompile(`^(\w+)\.(get|post|put|patch|delete|route|websocket|api_route)\(\s*['"]([^'"]*)['"](.*)$`)
	pyMethodsRE   = regexp.MustCompile(`methods\s*=\s*[\[(]([^\])]*)[\])]`)
	pyRouterDecl  = regexp.MustCompile(`(\w+)\s*=\s*(?:fastapi\.)?APIRouter\(([^)]*)\)`)
	pyPrefixRE    = regexp.MustCompile(`prefix\s*=\s*['"]([^'"]*)['"]`)
	pyIncludeRE   = regexp.MustCompile(`include_router\(\s*([\w.]+?)(?:\.router)?\s*,([^)]*)\)`)
	pyDependsAuth = regexp.MustCompile(`Depends\(\s*\w*(?:current_user|auth|token|verify|user)\w*`)
)

// decoratorEndpoints finds FastAPI/Flask style routes and DRF @api_view functions.
func (c *apiCtx) decoratorEndpoints() []facts.Endpoint {
	routerPrefix := map[string]string{}
	incl := map[string]string{}
	for _, p := range c.paths {
		if path.Ext(p) != ".py" || isTestPath(p) {
			continue
		}
		src := c.read(p)
		for _, m := range pyRouterDecl.FindAllStringSubmatch(src, -1) {
			if pm := pyPrefixRE.FindStringSubmatch(m[2]); pm != nil {
				routerPrefix[p+"::"+m[1]] = pm[1]
			}
		}
		for _, m := range pyIncludeRE.FindAllStringSubmatch(src, -1) {
			if pm := pyPrefixRE.FindStringSubmatch(m[2]); pm != nil {
				parts := strings.Split(m[1], ".")
				incl[parts[len(parts)-1]] = pm[1]
			}
		}
	}
	var eps []facts.Endpoint
	for _, fid := range c.order {
		f := c.fns[fid]
		if f.lang != "python" {
			continue
		}
		auth := ""
		if pyDependsAuth.MatchString(f.Sig) {
			auth = "authenticated"
		}
		for _, d := range f.Decorators {
			if strings.HasPrefix(d, "login_required") || strings.HasPrefix(d, "jwt_required") || strings.HasPrefix(d, "auth_required") {
				auth = "authenticated"
			}
		}
		for _, d := range f.Decorators {
			m := pyDecoRouteRE.FindStringSubmatch(d)
			if m == nil {
				continue
			}
			recv, verb, pth, rest := m[1], m[2], m[3], m[4]
			pre := routerPrefix[f.file+"::"+recv]
			if pre == "" {
				pre = incl[recv]
			} else if ip := incl[recv]; ip != "" {
				pre = joinPath(ip, pre)
			}
			full := joinPath(pre, pth)
			methods := []string{strings.ToUpper(verb)}
			switch verb {
			case "route", "api_route":
				methods = []string{"GET"}
				if mm := pyMethodsRE.FindStringSubmatch(rest); mm != nil {
					methods = nil
					for _, s := range regexp.MustCompile(`\w+`).FindAllString(mm[1], -1) {
						methods = append(methods, strings.ToUpper(s))
					}
				}
			case "websocket":
				eps = append(eps, facts.Endpoint{Kind: "ws", Path: full, Name: f.Name, Fid: fid, Source: "annotation", Auth: auth})
				continue
			}
			for _, mth := range methods {
				eps = append(eps, facts.Endpoint{Kind: "http", Method: mth, Path: full, Name: f.Name, Fid: fid, Source: "annotation", Auth: auth})
			}
		}
	}
	return eps
}
