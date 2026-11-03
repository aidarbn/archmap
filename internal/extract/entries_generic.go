package extract

import (
	"regexp"
	"strings"
)

// Entry points of languages on the generic tags parser. The parser gives
// annotations/attributes as Func.Decorators (Java annotations, Rust #[...]); the
// class a Java method belongs to is known, but its class-level @RequestMapping
// prefix is not (it would need class annotations from the parser), so Spring
// paths are those of the method annotation only.
var (
	springMap  = regexp.MustCompile(`^(Get|Post|Put|Patch|Delete|Request|Http(?:Get|Post|Put|Patch|Delete))(?:Mapping)?\b(?:\((.*)\))?$`)
	springTask = regexp.MustCompile(`^(Scheduled|KafkaListener|RabbitListener|JmsListener|SqsListener|EventListener|StreamListener)\b`)
	rustRoute  = regexp.MustCompile(`^(?:[\w]+::)*(get|post|put|patch|delete|head|options|route)\s*\((.*)\)$`)
	rustTask   = regexp.MustCompile(`^(?:[\w]+::)*(tokio::main|actix_web::main|actix_rt::main|async_std::main|rocket::main|launch)\b`)
	quoted     = regexp.MustCompile(`"([^"]*)"`)
	httpVerb   = regexp.MustCompile(`RequestMethod\.(\w+)`)
	// axum/actix-style registration in the body: .route("/x", get(handler))
	axumRoute = regexp.MustCompile(`\.route\(\s*"([^"]*)"\s*,\s*((?:(?:get|post|put|patch|delete|head|options)\s*\(\s*[\w:]+\s*\)\s*\.?\s*)+)`)
	axumVerb  = regexp.MustCompile(`(get|post|put|patch|delete|head|options)\s*\(\s*([\w:]+)\s*\)`)
	javaMain  = regexp.MustCompile(`\bstatic\b.*\b[Mm]ain\s*\(`)
	scalaMain = regexp.MustCompile(`^def main\s*\(`)
	railsSkip = regexp.MustCompile(`^(set_|authenticate|authorize|require_|check_|.*_params$|initialize$)`)
	rubyPriv  = regexp.MustCompile(`(?m)^\s*(private|protected)\s*$`)
)

// genericEntry marks entry points of Java/Rust/Ruby/C#/Scala/... functions that
// the shared rules (free `main`, python-style decorators) do not cover.
func genericEntry(e *entryMap, fid string, f *fn, src func(string) (string, bool)) {
	switch f.lang {
	case "java", "csharp", "groovy":
		if f.Cls != "" && javaMain.MatchString(f.Sig) && (f.Name == "main" || f.Name == "Main") {
			e.mark(fid, "main", f.Cls+".main")
		}
		for _, d := range f.Decorators {
			if m := springMap.FindStringSubmatch(d); m != nil {
				verb := strings.ToUpper(strings.TrimPrefix(m[1], "Http"))
				if verb == "REQUEST" {
					verb = "ANY"
					if v := httpVerb.FindStringSubmatch(m[2]); v != nil {
						verb = v[1]
					}
				}
				label := verb
				if q := quoted.FindStringSubmatch(m[2]); q != nil {
					label += " " + q[1]
				} else {
					label += " " + f.Cls + "." + f.Name
				}
				e.mark(fid, "http", label)
			} else if springTask.MatchString(d) {
				e.mark(fid, "task", strings.SplitN(d, "(", 2)[0])
			}
		}
	case "scala":
		if f.Cls != "" && f.Name == "main" && scalaMain.MatchString(f.Sig) {
			e.mark(fid, "main", f.Cls+".main")
		}
	case "rust":
		for _, d := range f.Decorators {
			if m := rustRoute.FindStringSubmatch(d); m != nil && (m[1] != "route" || quoted.MatchString(m[2])) {
				path := ""
				if q := quoted.FindStringSubmatch(m[2]); q != nil {
					path = q[1]
				}
				e.mark(fid, "http", strings.ToUpper(strings.Replace(m[1], "route", "ANY", 1))+" "+path)
			} else if rustTask.MatchString(d) && f.Name != "main" {
				e.mark(fid, "main", "main")
			}
		}
	case "ruby":
		// Rails: public instance methods of *Controller classes under controllers/.
		if !strings.HasSuffix(f.Cls, "Controller") || !strings.Contains(f.file, "controllers/") || railsSkip.MatchString(f.Name) {
			return
		}
		if s, ok := src(f.file); ok {
			if loc := rubyPriv.FindStringIndex(s); loc != nil && f.Start > strings.Count(s[:loc[0]], "\n")+1 {
				return
			}
		}
		e.mark(fid, "http", f.Cls+"#"+f.Name)
	}
}

// axumRoutes marks handlers registered with `.route("/x", get(handler))` in a
// Rust file (axum, also warp-like builders written this way).
func axumRoutes(e *entryMap, src string, byName map[string][]string, fns map[string]*fn) {
	for _, m := range axumRoute.FindAllStringSubmatch(src, -1) {
		for _, v := range axumVerb.FindAllStringSubmatch(m[2], -1) {
			name := v[2][strings.LastIndex(v[2], ":")+1:]
			var cands []string
			for _, fid := range byName[name] {
				if fns[fid].lang == "rust" && fns[fid].Cls == "" {
					cands = append(cands, fid)
				}
			}
			if len(cands) <= 3 {
				for _, fid := range cands {
					e.mark(fid, "http", strings.ToUpper(v[1])+" "+m[1])
				}
			}
		}
	}
}
