package extract

import (
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/facts"
)

// buildEndpoints collects the externally reachable entries of the repository.
func (c *apiCtx) buildEndpoints() []facts.Endpoint {
	var eps []facts.Endpoint
	policy := c.loadPolicy()
	eps = append(eps, c.protoEndpoints(policy)...)
	eps = append(eps, c.routerEndpoints()...)
	eps = append(eps, c.decoratorEndpoints()...)
	eps = append(eps, c.djangoEndpoints()...)
	eps = append(eps, c.nestEndpoints()...)
	eps = append(eps, c.springEndpoints()...)
	eps = append(eps, c.nextEndpoints()...)
	eps = append(eps, c.railsEndpoints()...)
	eps = append(eps, c.taskEndpoints()...)

	// Entries the extractor marked as gRPC/HTTP handlers but that no router or proto explained.
	seenFid := map[string]bool{}
	for _, e := range eps {
		if e.Fid != "" {
			seenFid[e.Fid] = true
		}
	}
	for _, fid := range c.order {
		en := c.entries[fid]
		if en == nil || en.Kind != "http" || seenFid[fid] || len(en.Labels) == 0 {
			continue
		}
		f := c.fns[fid]
		l := en.Labels[0]
		switch {
		case strings.HasPrefix(l, "gRPC "):
			eps = append(eps, facts.Endpoint{Kind: "grpc", Service: f.Cls, Name: f.Name, Fid: fid, Source: "code"})
		case strings.HasPrefix(l, "HTTP "):
			eps = append(eps, facts.Endpoint{Kind: "http", Service: f.Cls, Name: f.Name, Fid: fid, Source: "code"})
		}
	}

	// dedupe, stable order
	seen := map[string]bool{}
	var out []facts.Endpoint
	for _, e := range eps {
		k := e.Kind + "|" + e.Method + "|" + e.Path + "|" + e.Service + "|" + e.Name + "|" + e.Fid
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		return a.Name < b.Name
	})
	return out
}

func (c *apiCtx) taskEndpoints() []facts.Endpoint {
	var eps []facts.Endpoint
	for _, fid := range c.order {
		en := c.entries[fid]
		if en == nil {
			continue
		}
		f := c.fns[fid]
		label := ""
		if len(en.Labels) > 0 {
			label = en.Labels[0]
		}
		switch {
		case en.Kind == "task":
			eps = append(eps, facts.Endpoint{Kind: "task", Service: f.Cls, Name: f.Name, Fid: fid, Source: "code", Path: label})
		case en.Kind == "main" && strings.HasPrefix(label, "manage.py "):
			eps = append(eps, facts.Endpoint{Kind: "cli", Name: strings.TrimPrefix(label, "manage.py "), Fid: fid, Source: "code", Path: label})
		}
	}
	return eps
}

// ---- proto + RBAC ----

type policyRule struct {
	sub string
	obj *regexp.Regexp
}

type policy []policyRule

var policyFileRE = regexp.MustCompile(`(^|/)(rbac|casbin|policies|policy)[^/]*(/[^/]*)?\.csv$`)

// loadPolicy reads casbin-style `p, subject, object, action` rules.
func (c *apiCtx) loadPolicy() policy {
	var out policy
	for _, p := range c.paths {
		if !policyFileRE.MatchString(p) {
			continue
		}
		for _, line := range strings.Split(c.read(p), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "p,") {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) < 3 {
				continue
			}
			sub, obj := strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
			if !strings.HasPrefix(obj, "/") {
				continue
			}
			re, err := regexp.Compile("^" + strings.ReplaceAll(regexp.QuoteMeta(obj), `\*`, `.*`) + "$")
			if err != nil {
				continue
			}
			out = append(out, policyRule{sub, re})
		}
	}
	return out
}

// auth returns "public", "role:a,b" or "" for a full gRPC method.
func (pl policy) auth(full string) string {
	roles := map[string]bool{}
	for _, r := range pl {
		if !r.obj.MatchString(full) {
			continue
		}
		if r.sub == "*" {
			return "public"
		}
		roles[r.sub] = true
	}
	if len(roles) == 0 {
		return ""
	}
	var rs []string
	for r := range roles {
		rs = append(rs, r)
	}
	sort.Strings(rs)
	return "role:" + strings.Join(rs, ",")
}

var (
	protoPkgRE  = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;`)
	protoSvcRE  = regexp.MustCompile(`\bservice\s+(\w+)\s*\{`)
	protoRPCRE  = regexp.MustCompile(`\brpc\s+(\w+)\s*\(\s*(?:stream\s+)?([\w.]+)\s*\)\s*returns\s*\(\s*(?:stream\s+)?([\w.]+)\s*\)\s*([{;])?`)
	protoHTTPRE = regexp.MustCompile(`google\.api\.http\)\s*=\s*\{`)
	protoVerbRE = regexp.MustCompile(`\b(get|post|put|patch|delete)\s*:\s*"([^"]+)"`)
)

func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s) - 1
}

func shortType(t string) string {
	if i := strings.LastIndex(t, "."); i >= 0 {
		return t[i+1:]
	}
	return t
}

func (c *apiCtx) protoEndpoints(pl policy) []facts.Endpoint {
	var eps []facts.Endpoint
	for _, p := range c.pathsWhere(func(p string) bool { return strings.HasSuffix(p, ".proto") }) {
		src := blankKeepLines(c.read(p))
		pkg := ""
		if m := protoPkgRE.FindStringSubmatch(src); m != nil {
			pkg = m[1]
		}
		for _, sm := range protoSvcRE.FindAllStringSubmatchIndex(src, -1) {
			svc := src[sm[2]:sm[3]]
			open := sm[1] - 1
			body := src[open : matchBrace(src, open)+1]
			for _, rm := range protoRPCRE.FindAllStringSubmatchIndex(body, -1) {
				name := body[rm[2]:rm[3]]
				req, resp := shortType(body[rm[4]:rm[5]]), shortType(body[rm[6]:rm[7]])
				full := "/" + svc + "/" + name
				if pkg != "" {
					full = "/" + pkg + "." + svc + "/" + name
				}
				e := facts.Endpoint{Kind: "grpc", Service: svc, Name: name, Path: full, Request: req, Response: resp, Source: "proto"}
				if rm[8] >= 0 && body[rm[8]:rm[9]] == "{" {
					blk := body[rm[9]-1 : matchBrace(body, rm[9]-1)+1]
					if hi := protoHTTPRE.FindStringIndex(blk); hi != nil {
						httpBlk := blk[hi[1]-1 : matchBrace(blk, hi[1]-1)+1]
						if vm := protoVerbRE.FindStringSubmatch(httpBlk); vm != nil {
							e.Method, e.Path = strings.ToUpper(vm[1]), vm[2]
						}
					}
				}
				e.Auth = pl.auth(full)
				e.Fid = c.grpcImpl(svc, name, req)
				eps = append(eps, e)
			}
		}
	}
	return eps
}

// grpcImpl finds the function that implements rpc name of service svc.
func (c *apiCtx) grpcImpl(svc, name, req string) string {
	var cands []string
	for _, id := range c.byName[name] {
		if f := c.fns[id]; f.Cls != "" {
			cands = append(cands, id)
		}
	}
	if len(cands) == 0 {
		return ""
	}
	var bySig []string
	for _, id := range cands {
		if strings.Contains(c.fns[id].Sig, req) {
			bySig = append(bySig, id)
		}
	}
	if len(bySig) == 1 {
		return bySig[0]
	}
	if len(bySig) > 1 {
		cands = bySig
	}
	short := strings.ToLower(strings.TrimSuffix(svc, "Service"))
	var bySvc []string
	for _, id := range cands {
		cl := strings.ToLower(c.fns[id].Cls)
		if short != "" && strings.Contains(cl, short) {
			bySvc = append(bySvc, id)
		}
	}
	if len(bySvc) == 1 {
		return bySvc[0]
	}
	if len(cands) == 1 && len(bySig) == 1 {
		return cands[0]
	}
	return ""
}
