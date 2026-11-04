package extract

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

type pyClass struct {
	name, file string
	bases      []string
	start, end int // 1-based inclusive lines
	text       string
}

var pyClassRE = regexp.MustCompile(`(?m)^class\s+(\w+)\s*(?:\(([^)]*)\))?\s*:`)

// pyClasses indexes top-level Python classes by name.
func (c *apiCtx) pyClassIndex() map[string][]*pyClass {
	if c.pyClasses != nil {
		return c.pyClasses
	}
	c.pyClasses = map[string][]*pyClass{}
	for _, p := range c.paths {
		if path.Ext(p) != ".py" || isTestPath(p) {
			continue
		}
		src := c.read(p)
		ms := pyClassRE.FindAllStringSubmatchIndex(src, -1)
		for i, m := range ms {
			end := len(src)
			if rest := regexp.MustCompile(`(?m)^\S`).FindStringIndex(src[m[1]:]); rest != nil {
				end = m[1] + rest[0]
			}
			_ = i
			pc := &pyClass{name: src[m[2]:m[3]], file: p, start: lineAt(src, m[0]), text: src[m[0]:end]}
			pc.end = pc.start + strings.Count(pc.text, "\n")
			if m[4] >= 0 {
				for _, b := range strings.Split(src[m[4]:m[5]], ",") {
					b = strings.TrimSpace(b)
					if b != "" && !strings.Contains(b, "=") {
						pc.bases = append(pc.bases, b)
					}
				}
			}
			c.pyClasses[pc.name] = append(c.pyClasses[pc.name], pc)
		}
	}
	return c.pyClasses
}

func (c *apiCtx) pyClass(name, nearFile string) *pyClass {
	name = lastSeg(name)
	cs := c.pyClassIndex()[name]
	switch len(cs) {
	case 0:
		return nil
	case 1:
		return cs[0]
	}
	for _, x := range cs {
		if path.Dir(x.file) == path.Dir(nearFile) {
			return x
		}
	}
	return cs[0]
}

func lastSeg(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// chain returns the class and its project ancestors (nearest first) plus the names of framework bases.
func (c *apiCtx) pyChain(cl *pyClass) (classes []*pyClass, ext []string) {
	seen := map[*pyClass]bool{}
	var walkCl func(x *pyClass, d int)
	walkCl = func(x *pyClass, d int) {
		if x == nil || seen[x] || d > 5 {
			return
		}
		seen[x] = true
		classes = append(classes, x)
		for _, b := range x.bases {
			if pb := c.pyClass(b, x.file); pb != nil && pb != x {
				walkCl(pb, d+1)
			} else {
				ext = append(ext, lastSeg(b))
			}
		}
	}
	walkCl(cl, 0)
	return
}

var verbsOfBase = map[string][]string{
	"ListAPIView": {"GET"}, "CreateAPIView": {"POST"}, "RetrieveAPIView": {"GET"}, "UpdateAPIView": {"PUT", "PATCH"},
	"DestroyAPIView": {"DELETE"}, "ListCreateAPIView": {"GET", "POST"}, "RetrieveUpdateAPIView": {"GET", "PUT", "PATCH"},
	"RetrieveDestroyAPIView": {"GET", "DELETE"}, "RetrieveUpdateDestroyAPIView": {"GET", "PUT", "PATCH", "DELETE"},
}

var actionsOfBase = map[string][]string{
	"ModelViewSet":         {"list", "create", "retrieve", "update", "partial_update", "destroy"},
	"ReadOnlyModelViewSet": {"list", "retrieve"},
	"ListModelMixin":       {"list"}, "CreateModelMixin": {"create"}, "RetrieveModelMixin": {"retrieve"},
	"UpdateModelMixin": {"update", "partial_update"}, "DestroyModelMixin": {"destroy"},
}

var stdActionRoute = map[string]struct {
	method string
	detail bool
}{
	"list": {"GET", false}, "create": {"POST", false}, "retrieve": {"GET", true},
	"update": {"PUT", true}, "partial_update": {"PATCH", true}, "destroy": {"DELETE", true},
}

var permClassesRE = regexp.MustCompile(`permission_classes\s*=\s*[\[(]([^\])]*)[\])]`)

// permAuth maps a DRF permission_classes list to an auth value.
func permAuth(list string) string {
	if i := strings.Index(list, "("); i >= 0 && strings.HasPrefix(strings.TrimSpace(list), "permission_classes") {
		list = list[i+1:]
	}
	if strings.Trim(list, " []()") == "" {
		return "public"
	}
	var custom, auth, public bool
	var customName string
	for _, n := range regexp.MustCompile(`[\w.]+`).FindAllString(list, -1) {
		n = lastSeg(n)
		switch n {
		case "AllowAny":
			public = true
		case "IsAuthenticated", "IsAuthenticatedOrReadOnly", "DjangoModelPermissions", "DjangoModelPermissionsOrAnonReadOnly", "DjangoObjectPermissions":
			auth = true
		case "IsAdminUser":
			return "role:admin"
		default:
			custom = true
			if customName == "" {
				customName = n
			}
		}
	}
	switch {
	case public && !auth && !custom:
		return "public"
	case custom:
		return "role:" + customName
	case auth:
		return "authenticated"
	}
	return ""
}

func (c *apiCtx) defaultDjangoAuth() string {
	for _, p := range c.paths {
		if path.Ext(p) == ".py" && strings.Contains(p, "settings") {
			if m := regexp.MustCompile(`DEFAULT_PERMISSION_CLASSES['"]\s*:\s*[\[(]([^\])]*)[\])]`).FindStringSubmatch(c.read(p)); m != nil {
				return permAuth(m[1])
			}
		}
	}
	return ""
}

func (c *apiCtx) classAuth(cl *pyClass, def string) string {
	chain, _ := c.pyChain(cl)
	for _, x := range chain {
		if m := permClassesRE.FindStringSubmatch(x.text); m != nil {
			return permAuth(m[1])
		}
	}
	return def
}

// classMethod finds method name on the class or its project ancestors.
func (c *apiCtx) classMethod(cl *pyClass, name string) string {
	chain, _ := c.pyChain(cl)
	for _, x := range chain {
		for _, id := range c.byCls[x.name] {
			if f := c.fns[id]; f.Name == name && f.file == x.file {
				return id
			}
		}
	}
	return ""
}

func (c *apiCtx) classActions(cl *pyClass) map[string]bool {
	chain, ext := c.pyChain(cl)
	out := map[string]bool{}
	for _, x := range chain {
		for _, id := range c.byCls[x.name] {
			if f := c.fns[id]; f.file == x.file {
				if _, std := stdActionRoute[f.Name]; std {
					out[f.Name] = true
				}
			}
		}
	}
	for _, b := range ext {
		for _, a := range actionsOfBase[b] {
			out[a] = true
		}
	}
	return out
}

var fallbackFns = map[string][]string{
	"list": {"get_queryset", "list"}, "retrieve": {"get_object", "get_queryset", "retrieve"}, "create": {"perform_create", "create"},
	"update": {"perform_update", "update"}, "partial_update": {"perform_update", "partial_update"}, "destroy": {"perform_destroy", "destroy"},
}

var (
	djPathRE    = regexp.MustCompile(`\b(path|re_path|url)\(\s*r?(?:'([^']*)'|"([^"]*)")\s*,\s*(.+?)\s*$`)
	djIncludeRE = regexp.MustCompile(`include\(\s*(?:\(\s*)?(?:r?['"]([\w.]+)['"]|(\w+)\.urls)`)
	djRouterRE  = regexp.MustCompile(`(\w+)\s*=\s*(?:\w+\.)?(?:Default|Simple|Nested\w*)Router\(`)
	djRegRE     = regexp.MustCompile(`(\w+)\.register\(\s*(?:prefix\s*=\s*)?r?['"]([^'"]*)['"]\s*,\s*(?:viewset\s*=\s*)?([\w.]+)`)
	djRouterURL = regexp.MustCompile(`urlpatterns\s*(?:\+)?=\s*(?:\[[^\]]*\]\s*\+\s*)?(\w+)\.urls`)
	djAsViewRE  = regexp.MustCompile(`([\w.]+?)\.as_view\((.*)\)`)
	djParamRE   = regexp.MustCompile(`\(\?P<(\w+)>[^)]*\)`)
	djConvRE    = regexp.MustCompile(`<(?:\w+:)?(\w+)>`)
	djActionRE  = regexp.MustCompile(`^(?:\w+\.)?action\((.*)\)$`)
	djAPIView   = regexp.MustCompile(`^api_view\(\s*\[([^\]]*)\]`)
)

func djangoPath(p string) string {
	p = strings.TrimPrefix(p, "^")
	p = strings.TrimSuffix(p, "$")
	p = strings.ReplaceAll(p, "/?", "/")
	p = djParamRE.ReplaceAllString(p, "{$1}")
	p = djConvRE.ReplaceAllString(p, "{$1}")
	return p
}

type djEdge struct {
	file   string
	prefix string
	// either include of a file, a router's urls, or a view
	child  string
	router string
	view   string
	line   int
}

func (c *apiCtx) djangoEndpoints() []facts.Endpoint {
	urlFiles := c.pathsWhere(func(p string) bool {
		return path.Ext(p) == ".py" && !isTestPath(p) && (path.Base(p) == "urls.py" || strings.HasPrefix(path.Base(p), "urls_")) && strings.Contains(c.read(p), "urlpatterns")
	})
	if len(urlFiles) == 0 {
		return nil
	}
	edges := map[string][]djEdge{}
	routers := map[string]map[string][][2]string{} // file -> router var -> [(prefix, viewset)]
	included := map[string]bool{}
	selfRouter := map[string]string{} // file -> router var spliced into urlpatterns
	for _, uf := range urlFiles {
		src := c.read(uf)
		routers[uf] = map[string][][2]string{}
		for _, m := range djRegRE.FindAllStringSubmatch(src, -1) {
			routers[uf][m[1]] = append(routers[uf][m[1]], [2]string{m[2], m[3]})
		}
		if m := djRouterURL.FindStringSubmatch(src); m != nil {
			selfRouter[uf] = m[1]
		}
		for i, line := range strings.Split(src, "\n") {
			if isCodeComment(line) {
				continue
			}
			m := djPathRE.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			pre := djangoPath(m[2] + m[3])
			e := djEdge{file: uf, prefix: pre, line: i + 1}
			if im := djIncludeRE.FindStringSubmatch(m[4]); im != nil {
				if im[2] != "" {
					e.router = im[2]
				} else {
					suffix := strings.ReplaceAll(im[1], ".", "/") + ".py"
					for _, p := range urlFiles {
						if p == suffix || strings.HasSuffix(p, "/"+suffix) {
							e.child = p
							included[p] = true
						}
					}
					if e.child == "" {
						continue
					}
				}
			} else {
				e.view = m[4]
			}
			edges[uf] = append(edges[uf], e)
		}
	}
	def := c.defaultDjangoAuth()
	var eps []facts.Endpoint
	var visit func(uf, prefix string, depth int)
	emitRouter := func(uf, rv, prefix string) {
		for _, r := range routers[uf][rv] {
			eps = append(eps, c.viewsetRoutes(uf, joinPath(prefix, r[0]), r[1], def)...)
		}
	}
	visit = func(uf, prefix string, depth int) {
		if depth > 6 {
			return
		}
		if rv := selfRouter[uf]; rv != "" {
			emitRouter(uf, rv, prefix)
		}
		for _, e := range edges[uf] {
			full := joinPath(prefix, e.prefix)
			switch {
			case e.child != "":
				visit(e.child, full, depth+1)
			case e.router != "":
				emitRouter(uf, e.router, full)
			default:
				eps = append(eps, c.djangoView(uf, full, e.view, def)...)
			}
		}
	}
	for _, uf := range urlFiles {
		if !included[uf] {
			visit(uf, "", 0)
		}
	}
	for i := range eps {
		eps[i].Path = joinPath("", djangoPath(strings.ReplaceAll(eps[i].Path, "/^", "/")))
	}
	return eps
}

// viewsetRoutes expands router.register(prefix, ViewSet) into the standard and @action routes.
func (c *apiCtx) viewsetRoutes(uf, prefix, vs, def string) []facts.Endpoint {
	cl := c.pyClass(vs, uf)
	if cl == nil {
		return nil
	}
	auth := c.classAuth(cl, def)
	var eps []facts.Endpoint
	acts := c.classActions(cl)
	var names []string
	for a := range acts {
		names = append(names, a)
	}
	sort.Strings(names)
	for _, a := range names {
		r := stdActionRoute[a]
		pth := prefix
		if r.detail {
			pth = joinPath(prefix, "{pk}")
		}
		fid := c.classMethod(cl, a)
		if fid == "" {
			for _, fb := range fallbackFns[a] {
				if fid = c.classMethod(cl, fb); fid != "" {
					break
				}
			}
		}
		eps = append(eps, facts.Endpoint{Kind: "http", Service: cl.name, Name: a, Method: r.method, Path: pth + trailingSlash(pth), Fid: fid, Auth: auth, Source: "router"})
	}
	// @action methods
	chain, _ := c.pyChain(cl)
	for _, x := range chain {
		for _, id := range c.byCls[x.name] {
			f := c.fns[id]
			if f.file != x.file {
				continue
			}
			for _, d := range f.Decorators {
				m := djActionRE.FindStringSubmatch(d)
				if m == nil {
					continue
				}
				args := m[1]
				detail := regexp.MustCompile(`detail\s*=\s*True`).MatchString(args)
				urlPath := f.Name
				if um := regexp.MustCompile(`url_path\s*=\s*['"]([^'"]*)['"]`).FindStringSubmatch(args); um != nil {
					urlPath = um[1]
				}
				methods := []string{"GET"}
				if mm := pyMethodsRE.FindStringSubmatch(args); mm != nil {
					methods = nil
					for _, s := range regexp.MustCompile(`\w+`).FindAllString(mm[1], -1) {
						methods = append(methods, strings.ToUpper(s))
					}
				}
				pth := prefix
				if detail {
					pth = joinPath(pth, "{pk}")
				}
				pth = joinPath(pth, urlPath)
				a := auth
				if pm := permClassesRE.FindStringSubmatch(args); pm != nil {
					a = permAuth(pm[1])
				}
				for _, mth := range methods {
					eps = append(eps, facts.Endpoint{Kind: "http", Service: cl.name, Name: f.Name, Method: mth, Path: pth + trailingSlash(pth), Fid: id, Auth: a, Source: "router"})
				}
			}
		}
	}
	return eps
}

func trailingSlash(p string) string {
	if strings.HasSuffix(p, "/") {
		return ""
	}
	return "/"
}

// djangoView expands one path(...) entry into endpoints.
func (c *apiCtx) djangoView(uf, full, view, def string) []facts.Endpoint {
	view = strings.TrimSpace(view)
	if m := djAsViewRE.FindStringSubmatch(view); m != nil {
		cl := c.pyClass(m[1], uf)
		if cl == nil {
			return nil
		}
		auth := c.classAuth(cl, def)
		// viewset bound by hand: as_view({'get': 'list'})
		if mm := regexp.MustCompile(`['"](\w+)['"]\s*:\s*['"](\w+)['"]`).FindAllStringSubmatch(m[2], -1); len(mm) > 0 {
			var eps []facts.Endpoint
			for _, x := range mm {
				fid := c.classMethod(cl, x[2])
				if fid == "" {
					for _, fb := range fallbackFns[x[2]] {
						if fid = c.classMethod(cl, fb); fid != "" {
							break
						}
					}
				}
				eps = append(eps, facts.Endpoint{Kind: "http", Service: cl.name, Name: x[2], Method: strings.ToUpper(x[1]), Path: full, Fid: fid, Auth: auth, Source: "router"})
			}
			return eps
		}
		var eps []facts.Endpoint
		seen := map[string]bool{}
		for _, v := range []string{"get", "post", "put", "patch", "delete"} {
			if fid := c.classMethod(cl, v); fid != "" {
				seen[strings.ToUpper(v)] = true
				eps = append(eps, facts.Endpoint{Kind: "http", Service: cl.name, Name: v, Method: strings.ToUpper(v), Path: full, Fid: fid, Auth: auth, Source: "router"})
			}
		}
		_, ext := c.pyChain(cl)
		for _, b := range ext {
			for _, v := range verbsOfBase[b] {
				if seen[v] {
					continue
				}
				seen[v] = true
				act := map[string]string{"GET": "list", "POST": "create", "PUT": "update", "PATCH": "partial_update", "DELETE": "destroy"}[v]
				if v == "GET" && strings.HasPrefix(b, "Retrieve") {
					act = "retrieve"
				}
				fid := ""
				for _, fb := range fallbackFns[act] {
					if fid = c.classMethod(cl, fb); fid != "" {
						break
					}
				}
				eps = append(eps, facts.Endpoint{Kind: "http", Service: cl.name, Name: act, Method: v, Path: full, Fid: fid, Auth: auth, Source: "router"})
			}
		}
		return eps
	}
	// function view
	name := strings.Trim(strings.TrimSpace(strings.SplitN(view, ",", 2)[0]), "() ")
	if !regexp.MustCompile(`^[\w.]+$`).MatchString(name) {
		return nil
	}
	fid := c.djangoFuncView(uf, name)
	if fid == "" {
		return nil
	}
	f := c.fns[fid]
	var methods []string
	auth := ""
	for _, d := range f.Decorators {
		if m := djAPIView.FindStringSubmatch(d); m != nil {
			for _, s := range regexp.MustCompile(`\w+`).FindAllString(m[1], -1) {
				methods = append(methods, strings.ToUpper(s))
			}
		}
		if strings.HasPrefix(d, "login_required") {
			auth = "authenticated"
		}
		if strings.HasPrefix(d, "permission_classes") {
			auth = permAuth(d)
		}
	}
	if auth == "" && len(methods) > 0 {
		auth = def
	}
	if len(methods) == 0 {
		methods = []string{""}
	}
	var eps []facts.Endpoint
	for _, m := range methods {
		eps = append(eps, facts.Endpoint{Kind: "http", Name: f.Name, Method: m, Path: full, Fid: fid, Auth: auth, Source: "router"})
	}
	return eps
}

func (c *apiCtx) djangoFuncView(uf, expr string) string {
	n := lastSeg(expr)
	var cands []string
	for _, id := range c.byName[n] {
		if c.fns[id].Cls == "" && c.fns[id].lang == "python" {
			cands = append(cands, id)
		}
	}
	if len(cands) == 0 {
		return ""
	}
	dir := path.Dir(uf)
	for _, id := range cands {
		if path.Dir(c.fns[id].file) == dir {
			return id
		}
	}
	// "from app.views import x" / "from app import views"
	src := c.read(uf)
	for _, m := range regexp.MustCompile(`(?m)^\s*from\s+([\w.]+)\s+import\s+(.*)$`).FindAllStringSubmatch(src, -1) {
		mod := strings.ReplaceAll(strings.TrimLeft(m[1], "."), ".", "/")
		for _, id := range cands {
			fd := c.fns[id].file
			if strings.Contains(fd, mod+"/") || strings.Contains(strings.TrimSuffix(fd, ".py"), mod) {
				return id
			}
		}
	}
	if len(cands) == 1 {
		return cands[0]
	}
	return ""
}
