package extract

import (
	"path"
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

var (
	annoClassRE   = regexp.MustCompile(`((?:@[\w.]+(?:\((?:[^()]|\([^()]*\))*\))?\s*)+)(?:export\s+)?(?:default\s+)?(?:public\s+|open\s+|internal\s+)?(?:abstract\s+)?class\s+(\w+)`)
	annoRE        = regexp.MustCompile(`@([\w.]+)(\((?:[^()]|\([^()]*\))*\))?`)
	firstStrRE    = regexp.MustCompile(`['"]([^'"]*)['"]`)
	springVerbs   = map[string]string{"GetMapping": "GET", "PostMapping": "POST", "PutMapping": "PUT", "PatchMapping": "PATCH", "DeleteMapping": "DELETE", "RequestMapping": ""}
	nestVerbs     = map[string]string{"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH", "Delete": "DELETE", "All": "", "Head": "HEAD", "Options": "OPTIONS"}
	springRoleRE  = regexp.MustCompile(`has(?:Any)?(?:Role|Authority)\(\s*'?"?([\w:\-]+)`)
	pathVarNormRE = regexp.MustCompile(`:(\w+)`)
)

// annotationsAbove returns the annotation texts attached to function fid.
func (c *apiCtx) annotationsAbove(fid string) string {
	f := c.fns[fid]
	// Parsers may leak the decorators of a neighbouring method, so read the
	// annotation lines directly above the definition; fall back to the parsed list.
	lines := strings.Split(c.read(f.file), "\n")
	var above []string
	open := 0
	for i := f.Start - 2; i >= 0 && i < len(lines) && len(above) < 12; i-- {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "@") || open > 0 {
			above = append([]string{t}, above...)
			open += strings.Count(t, ")") - strings.Count(t, "(")
			if open < 0 {
				open = 0
			}
			continue
		}
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") {
			continue
		}
		break
	}
	if len(above) > 0 {
		return strings.Join(above, "\n")
	}
	return "@" + strings.Join(f.Decorators, "\n@")
}

type annoClass struct {
	cls    string
	annos  string
	prefix string
}

func (c *apiCtx) annotatedClasses(p string) map[string]annoClass {
	src := c.read(p)
	out := map[string]annoClass{}
	for _, m := range annoClassRE.FindAllStringSubmatch(src, -1) {
		out[m[2]] = annoClass{cls: m[2], annos: m[1]}
	}
	return out
}

// ---- NestJS ----

func (c *apiCtx) nestEndpoints() []facts.Endpoint {
	var eps []facts.Endpoint
	for _, p := range c.paths {
		if !jsExts[path.Ext(p)] || isTestPath(p) {
			continue
		}
		if src := c.read(p); !strings.Contains(src, "@Controller(") {
			continue
		}
		for cls, ac := range c.annotatedClasses(p) {
			var prefix string
			ok := false
			for _, m := range annoRE.FindAllStringSubmatch(ac.annos, -1) {
				if m[1] == "Controller" {
					ok = true
					if s := firstStrRE.FindStringSubmatch(m[2]); s != nil {
						prefix = s[1]
					}
				}
			}
			if !ok {
				continue
			}
			clsAuth := nestAuth(ac.annos)
			for _, id := range c.byCls[cls] {
				f := c.fns[id]
				if f.file != p {
					continue
				}
				annos := c.annotationsAbove(id)
				for _, m := range annoRE.FindAllStringSubmatch(annos, -1) {
					verb, ok := nestVerbs[m[1]]
					if !ok {
						continue
					}
					sub := ""
					if s := firstStrRE.FindStringSubmatch(m[2]); s != nil {
						sub = s[1]
					}
					a := nestAuth(annos)
					if a == "" {
						a = clsAuth
					}
					eps = append(eps, facts.Endpoint{Kind: "http", Service: cls, Name: f.Name, Method: verb,
						Path: pathVarNormRE.ReplaceAllString(joinPath(prefix, sub), "{$1}"), Fid: id, Auth: a, Source: "annotation"})
				}
			}
		}
	}
	return eps
}

func nestAuth(annos string) string {
	if strings.Contains(annos, "@Public(") {
		return "public"
	}
	for _, m := range annoRE.FindAllStringSubmatch(annos, -1) {
		if m[1] == "Roles" {
			if s := firstStrRE.FindStringSubmatch(m[2]); s != nil {
				return "role:" + s[1]
			}
		}
	}
	if strings.Contains(annos, "@UseGuards(") {
		return "authenticated"
	}
	return ""
}

// ---- Spring ----

func (c *apiCtx) springEndpoints() []facts.Endpoint {
	var eps []facts.Endpoint
	for _, p := range c.paths {
		if ext := path.Ext(p); (ext != ".java" && ext != ".kt") || isTestPath(p) {
			continue
		}
		src := c.read(p)
		if !strings.Contains(src, "Controller") {
			continue
		}
		for cls, ac := range c.annotatedClasses(p) {
			if !strings.Contains(ac.annos, "@RestController") && !strings.Contains(ac.annos, "@Controller") {
				continue
			}
			prefix := ""
			for _, m := range annoRE.FindAllStringSubmatch(ac.annos, -1) {
				if m[1] == "RequestMapping" {
					prefix = springPath(m[2])
				}
			}
			clsAuth := springAuth(ac.annos)
			for _, id := range c.byCls[cls] {
				f := c.fns[id]
				if f.file != p {
					continue
				}
				annos := c.annotationsAbove(id)
				for _, m := range annoRE.FindAllStringSubmatch(annos, -1) {
					verb, ok := springVerbs[m[1]]
					if !ok {
						continue
					}
					methods := []string{verb}
					if m[1] == "RequestMapping" {
						methods = nil
						for _, mm := range regexp.MustCompile(`RequestMethod\.(\w+)`).FindAllStringSubmatch(m[2], -1) {
							methods = append(methods, mm[1])
						}
						if len(methods) == 0 {
							methods = []string{""}
						}
					}
					a := springAuth(annos)
					if a == "" {
						a = clsAuth
					}
					for _, mth := range methods {
						eps = append(eps, facts.Endpoint{Kind: "http", Service: cls, Name: f.Name, Method: mth,
							Path: joinPath(prefix, springPath(m[2])), Fid: id, Auth: a, Source: "annotation"})
					}
				}
			}
		}
	}
	return eps
}

func springPath(args string) string {
	if s := regexp.MustCompile(`(?:value|path)?\s*=?\s*\{?\s*"([^"]*)"`).FindStringSubmatch(args); s != nil {
		return s[1]
	}
	return ""
}

func springAuth(annos string) string {
	for _, m := range annoRE.FindAllStringSubmatch(annos, -1) {
		switch m[1] {
		case "PreAuthorize", "Secured", "RolesAllowed":
			switch {
			case strings.Contains(m[2], "permitAll"):
				return "public"
			case springRoleRE.MatchString(m[2]):
				r := springRoleRE.FindStringSubmatch(m[2])[1]
				return "role:" + strings.TrimPrefix(r, "ROLE_")
			case strings.Contains(m[2], "isAuthenticated"):
				return "authenticated"
			case m[1] != "PreAuthorize":
				if s := firstStrRE.FindStringSubmatch(m[2]); s != nil {
					return "role:" + strings.TrimPrefix(s[1], "ROLE_")
				}
			}
		}
	}
	return ""
}

// ---- Next.js ----

func normNext(route string) string {
	segs := strings.Split(route, "/")
	var out []string
	for _, s := range segs {
		switch {
		case strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"), strings.HasPrefix(s, "@"):
			continue
		case strings.HasPrefix(s, "[[...") && strings.HasSuffix(s, "]]"):
			s = "{" + s[5:len(s)-2] + "}"
		case strings.HasPrefix(s, "[...") && strings.HasSuffix(s, "]"):
			s = "{" + s[4:len(s)-1] + "}"
		case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
			s = "{" + s[1:len(s)-1] + "}"
		}
		out = append(out, s)
	}
	r := strings.Join(out, "/")
	if r == "" {
		return "/"
	}
	return r
}

func (c *apiCtx) nextEndpoints() []facts.Endpoint {
	var eps []facts.Endpoint
	for _, fid := range c.order {
		en := c.entries[fid]
		f := c.fns[fid]
		if en == nil || en.Kind != "http" || len(en.Labels) == 0 || (!nextPage.MatchString(f.file) && !nextApp.MatchString(f.file)) {
			continue
		}
		for _, l := range en.Labels {
			kind, route, ok := strings.Cut(l, " ")
			if !ok || !strings.HasPrefix(route, "/") {
				continue
			}
			e := facts.Endpoint{Kind: "http", Path: normNext(route), Fid: fid, Source: "router"}
			switch kind {
			case "PAGE":
				e.Method, e.Name = "GET", "page"
			case "API":
				e.Name = f.Name
			default:
				e.Method, e.Name = kind, f.Name
			}
			eps = append(eps, e)
		}
	}
	return eps
}

// ---- Rails ----

var (
	railsVerbRE = regexp.MustCompile(`^(get|post|put|patch|delete)\s+['"]?([\w/:\-.*]+)['"]?\s*(?:,\s*to:\s*|=>\s*)['"](\w+(?:/\w+)*)#(\w+)['"]`)
	railsResRE  = regexp.MustCompile(`^(resources|resource)\s+:(\w+)(.*)$`)
	railsNsRE   = regexp.MustCompile(`^(namespace|scope)\s+(?::(\w+)|['"]([^'"]*)['"])(.*)\bdo\b`)
	railsRootRE = regexp.MustCompile(`^root\s+(?:to:\s*)?['"](\w+(?:/\w+)*)#(\w+)['"]`)
)

type railsFrame struct{ path, mod string }

func (c *apiCtx) railsEndpoints() []facts.Endpoint {
	var eps []facts.Endpoint
	for _, p := range c.paths {
		if path.Base(p) != "routes.rb" || !strings.Contains(p, "config") {
			continue
		}
		stack := []railsFrame{{}}
		cur := func() (string, string) {
			var pa, mo string
			for _, f := range stack {
				pa = joinPath(pa, f.path)
				if f.mod != "" {
					mo += f.mod + "/"
				}
			}
			return strings.TrimRight(pa, "/"), mo
		}
		ctrl := func(mod, name string) string {
			parts := strings.Split(strings.Trim(mod+name, "/"), "/")
			for i, s := range parts {
				parts[i] = camel(s)
			}
			return strings.Join(parts, "::") + "Controller"
		}
		emit := func(method, pth, mod, name, action string) {
			cls := ctrl(mod, name)
			fid := ""
			for _, id := range c.byCls[cls] {
				if c.fns[id].Name == action {
					fid = id
				}
			}
			if fid == "" {
				short := cls[strings.LastIndex(cls, ":")+1:]
				for _, id := range c.byCls[short] {
					if c.fns[id].Name == action {
						fid = id
					}
				}
			}
			if pth == "" {
				pth = "/"
			}
			eps = append(eps, facts.Endpoint{Kind: "http", Service: cls, Name: action, Method: method, Path: pth, Fid: fid, Source: "router"})
		}
		for _, raw := range strings.Split(c.read(p), "\n") {
			line := strings.TrimSpace(raw)
			if i := strings.Index(line, " #"); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			pa, mo := cur()
			switch {
			case line == "end":
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
			case railsRootRE.MatchString(line):
				m := railsRootRE.FindStringSubmatch(line)
				emit("GET", pa+"/", mo, m[1], m[2])
			case railsNsRE.MatchString(line):
				m := railsNsRE.FindStringSubmatch(line)
				fr := railsFrame{}
				name := m[2] + m[3]
				if m[1] == "namespace" {
					fr = railsFrame{path: "/" + name, mod: name}
				} else if strings.Contains(m[4], "module:") {
					if mm := regexp.MustCompile(`module:\s*['":]*(\w+)`).FindStringSubmatch(m[4]); mm != nil {
						fr.mod = mm[1]
					}
				} else {
					fr.path = "/" + strings.Trim(name, "/")
				}
				stack = append(stack, fr)
			case railsResRE.MatchString(line):
				m := railsResRE.FindStringSubmatch(line)
				kind, name, rest := m[1], m[2], m[3]
				acts := []string{"index", "create", "show", "update", "destroy"}
				if kind == "resource" {
					acts = []string{"show", "create", "update", "destroy"}
				}
				if on := regexp.MustCompile(`only:\s*(?:\[([^\]]*)\]|:(\w+))`).FindStringSubmatch(rest); on != nil {
					acts = nil
					for _, s := range regexp.MustCompile(`\w+`).FindAllString(on[1]+on[2], -1) {
						acts = append(acts, s)
					}
				} else if ex := regexp.MustCompile(`except:\s*\[([^\]]*)\]`).FindStringSubmatch(rest); ex != nil {
					skip := ex[1]
					var keep []string
					for _, a := range acts {
						if !regexp.MustCompile(`\b` + a + `\b`).MatchString(skip) {
							keep = append(keep, a)
						}
					}
					acts = keep
				}
				base := pa + "/" + name
				for _, a := range acts {
					pth, mth := base, "GET"
					switch a {
					case "create":
						mth = "POST"
					case "show":
						if kind == "resources" {
							pth += "/{id}"
						}
					case "update":
						mth = "PATCH"
						if kind == "resources" {
							pth += "/{id}"
						}
					case "destroy":
						mth = "DELETE"
						if kind == "resources" {
							pth += "/{id}"
						}
					}
					emit(mth, pth, mo, name, a)
				}
				if regexp.MustCompile(`\bdo\s*(\|.*\|)?$`).MatchString(rest) {
					seg := name
					if kind == "resources" {
						seg = name + "/{" + strings.TrimSuffix(name, "s") + "_id}"
					}
					stack = append(stack, railsFrame{path: "/" + seg})
				}
			case railsVerbRE.MatchString(line):
				m := railsVerbRE.FindStringSubmatch(line)
				emit(strings.ToUpper(m[1]), pa+"/"+strings.TrimLeft(pathVarNormRE.ReplaceAllString(m[2], "{$1}"), "/"), mo, m[3], m[4])
			case regexp.MustCompile(`\bdo\b\s*(\|[^|]*\|)?\s*$`).MatchString(line):
				stack = append(stack, railsFrame{})
			}
		}
	}
	return eps
}

func camel(s string) string {
	parts := strings.Split(s, "_")
	for i, x := range parts {
		if x != "" {
			parts[i] = strings.ToUpper(x[:1]) + x[1:]
		}
	}
	return strings.Join(parts, "")
}
