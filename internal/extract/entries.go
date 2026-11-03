package extract

import (
	"path"
	"regexp"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

var routeREs = []*regexp.Regexp{
	// Go: r.Get("/x", h.Fn) / mux.HandleFunc("/x", fn) / e.POST("/x", fn)
	regexp.MustCompile(`\.\s*(Get|Post|Put|Patch|Delete|Head|Options|Handle|HandleFunc|GET|POST|PUT|PATCH|DELETE|Any|Group)\s*\(\s*"([^"]*)"\s*,\s*(?:[\w.]+\()?\s*([\w.]+)`),
	// Django: path("x/", views.fn) / re_path(r"...", views.Cls.as_view())
	regexp.MustCompile(`\b(path|re_path|url)\s*\(\s*r?['"]([^'"]*)['"]\s*,\s*([\w.]+)`),
	// DRF: router.register("x", ViewSet) / register(prefix='x', viewset=ViewSet)
	regexp.MustCompile(`\.(register)\(\s*(?:prefix\s*=\s*)?r?['"]([^'"]*)['"]\s*,\s*(?:viewset\s*=\s*)?([\w.]+)`),
}

var (
	pyRouteDeco  = regexp.MustCompile(`^(?:\w+\.)*(get|post|put|patch|delete|route|websocket|api_route)\s*\(\s*['"]([^'"]*)['"]`)
	pyTaskDeco   = regexp.MustCompile(`(shared_task|\.task\b|^task\b|periodic_task|receiver|celery|job\b)`)
	pyViewBases  = regexp.MustCompile(`(View|ViewSet|APIView|Mixin|GenericAPIView)$`)
	goHTTPSig    = regexp.MustCompile(`http\.ResponseWriter,\s*\w+\s+\*http\.Request|\*gin\.Context|echo\.Context|\*fiber\.Ctx|\*fasthttp\.RequestCtx`)
	goGRPCSig    = regexp.MustCompile(`\(\s*\w+\s+context\.Context,\s*\w+\s+\*[\w.]*(?:Request|Req)\)\s*\(\*?[\w.]+,\s*error\)|\w+_\w+Server\)\s*error\s*\{?$|\w+\s+\*[\w.]*(?:Request|Req),\s*\w+\s+\w*[\w.]*Server\)\s*error`)
	goTaskSig    = regexp.MustCompile(`\*river\.Job\[|\*asynq\.Task|\*machinery\.|\*work\.Job`)
	nextPage     = regexp.MustCompile(`(^|/)(?:src/)?pages/(.+)\.(?:[jt]sx?)$`)
	nextApp      = regexp.MustCompile(`(^|/)(?:src/)?app/(.*?)(?:/)?(page|route)\.(?:[jt]sx?)$`)
	nextIndex    = regexp.MustCompile(`(^|/)index$`)
	exportDef    = regexp.MustCompile(`export\s+default\s+(?:async\s+)?(?:function\s+)?(\w+)`)
	httpMethods  = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "list": true, "create": true, "retrieve": true, "update": true, "partial_update": true, "destroy": true, "dispatch": true}
	plainVerbs   = map[string]bool{"path": true, "re_path": true, "url": true, "handle": true, "handlefunc": true, "register": true}
	djangoVerbs  = map[string]bool{"path": true, "re_path": true, "url": true}
	nextRouteFns = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
)

type entryMap struct {
	m map[string]*facts.Entry
}

func (e *entryMap) mark(fid, kind, label string) {
	en, ok := e.m[fid]
	if !ok {
		en = &facts.Entry{Kind: kind, Labels: []string{}}
		e.m[fid] = en
	}
	if label == "" {
		return
	}
	for _, l := range en.Labels {
		if l == label {
			return
		}
	}
	en.Labels = append(en.Labels, label)
}

// findEntries detects entry points: HTTP routes, gRPC handlers, tasks, main and
// Next.js pages. order is the deterministic iteration order of functions.
func findEntries(files []string, read func(string) (string, bool), fns map[string]*fn, order []string, byName map[string][]string, defaultExport map[string]string, langOf func(string) string) map[string]*facts.Entry {
	e := &entryMap{m: map[string]*facts.Entry{}}
	swiftAppMain(e, fns, order) // first: kind "main" wins over "lifecycle" on the same func
	for _, fid := range order {
		f := fns[fid]
		if f.Name == "main" && f.Cls == "" {
			e.mark(fid, "main", "main")
		}
		for _, d := range f.Decorators {
			if m := pyRouteDeco.FindStringSubmatch(d); m != nil {
				e.mark(fid, "http", strings.ToUpper(m[1])+" "+m[2])
			} else if strings.HasPrefix(d, "action") || strings.Contains(d, ".action(") {
				e.mark(fid, "http", "ACTION "+f.Name)
			} else if pyTaskDeco.MatchString(d) {
				e.mark(fid, "task", strings.SplitN(d, "(", 2)[0])
			}
		}
		if f.Cls != "" && httpMethods[f.Name] {
			for _, b := range f.Bases {
				if pyViewBases.MatchString(b) {
					e.mark(fid, "http", f.Cls+"."+f.Name)
					break
				}
			}
		}
		genericEntry(e, fid, f, read)
		switch f.lang {
		case "swift":
			swiftEntry(e, fid, f)
		case "kotlin":
			kotlinEntry(e, fid, f)
		}
		if f.lang == "python" && f.Cls == "Command" && f.Name == "handle" && strings.Contains(f.file, "/management/commands/") {
			stem := strings.TrimSuffix(path.Base(f.file), path.Ext(f.file))
			e.mark(fid, "main", "manage.py "+stem)
		}
		if f.lang == "go" && f.Cls != "" && f.Name != "" && f.Name[0] >= 'A' && f.Name[0] <= 'Z' {
			switch {
			case goHTTPSig.MatchString(f.Sig):
				e.mark(fid, "http", "HTTP "+f.Cls+"."+f.Name)
			case goGRPCSig.MatchString(f.Sig):
				e.mark(fid, "http", "gRPC "+f.Cls+"."+f.Name)
			case goTaskSig.MatchString(f.Sig) && (f.Name == "Work" || f.Name == "ProcessTask" || f.Name == "Handle" || f.Name == "Perform"):
				e.mark(fid, "task", f.Cls+"."+f.Name)
			}
		}
	}
	byFile := map[string][]string{}
	for _, fid := range order {
		byFile[fns[fid].file] = append(byFile[fns[fid].file], fid)
	}
	for _, p := range files {
		src, ok := read(p)
		if !ok {
			continue
		}
		if langOf(p) == "js" {
			if m := nextPage.FindStringSubmatch(p); m != nil {
				route := "/" + nextIndex.ReplaceAllString(m[2], "")
				api := strings.HasPrefix(route, "/api")
				base := path.Base(p)
				if !strings.HasPrefix(base, "_") || strings.HasPrefix(base, "_app") {
					def := ""
					if d := exportDef.FindStringSubmatch(src); d != nil {
						def = d[1]
					} else {
						def = defaultExport[p]
					}
					for _, fid := range byFile[p] {
						if def != "" && fns[fid].Name == def && fns[fid].Cls == "" {
							kind := "PAGE "
							if api {
								kind = "API "
							}
							e.mark(fid, "http", kind+route)
						}
					}
				}
			}
			if m := nextApp.FindStringSubmatch(p); m != nil {
				route := "/" + m[2]
				for _, fid := range byFile[p] {
					n := fns[fid].Name
					if m[3] == "route" && nextRouteFns[n] {
						e.mark(fid, "http", n+" "+route)
					} else if m[3] == "page" && regexp.MustCompile(`export\s+default\s+(?:async\s+)?(?:function\s+)?`+regexp.QuoteMeta(n)+`\b`).MatchString(src) {
						e.mark(fid, "http", "PAGE "+route)
					}
				}
			}
			continue
		}
		if langOf(p) == "rust" {
			axumRoutes(e, src, byName, fns)
		}
		for _, rx := range routeREs {
			for _, m := range rx.FindAllStringSubmatch(src, -1) {
				verb, route, target := m[1], m[2], m[3]
				if verb == "Group" {
					continue
				}
				parts := strings.Split(target, ".")
				name := parts[len(parts)-1]
				if name == "as_view" {
					name = ""
					if len(parts) > 1 {
						name = parts[len(parts)-2]
					}
				}
				if name == "as_view" || name == "" {
					continue
				}
				cands := byName[name]
				var clsMethods []string
				for _, fid := range order {
					if f := fns[fid]; f.Cls == name && httpMethods[f.Name] {
						clsMethods = append(clsMethods, fid)
					}
				}
				lv := strings.ToLower(verb)
				label := route
				if !plainVerbs[lv] {
					label = strings.ToUpper(verb) + " " + route
				}
				plain := cands
				if djangoVerbs[lv] {
					plain = nil
					for _, c := range cands {
						if fns[c].Cls == "" {
							plain = append(plain, c)
						}
					}
				}
				var targets []string
				if len(plain) <= 3 {
					targets = append(targets, plain...)
				}
				targets = append(targets, clsMethods...)
				for _, fid := range targets {
					e.mark(fid, "http", strings.TrimSpace(label))
				}
			}
		}
	}
	return e.m
}
