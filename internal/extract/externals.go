package extract

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/aidarbn/archmap/internal/autosem"
	"github.com/aidarbn/archmap/internal/facts"
	"github.com/aidarbn/archmap/internal/lang"
)

// Kinds of external systems.
const (
	KindPayment   = "payment"
	KindPOS       = "pos"
	KindAuth      = "auth"
	KindDB        = "db"
	KindCache     = "cache"
	KindQueue     = "queue"
	KindStorage   = "storage"
	KindMessaging = "messaging"
	KindHTTPAPI   = "http_api"
	KindOther     = "other"
)

// libRule maps an import path to a system. A pattern starting with "=" matches
// the path exactly or as a prefix followed by "/" (package names that are too
// short for a substring match); any other pattern is a substring. The first
// matching rule wins, so specific rules come before general ones. A rule with
// an empty name is a generic database driver whose dialect is resolved later.
type libRule struct{ pat, name, kind string }

const genericDB = "Database"

var libRules = []libRule{
	// databases, specific dialects first
	{"jackc/pgx", "Postgres", KindDB}, {"lib/pq", "Postgres", KindDB}, {"jet/v2/postgres", "Postgres", KindDB},
	{"gorm.io/driver/postgres", "Postgres", KindDB}, {"pgdialect", "Postgres", KindDB}, {"=psycopg", "Postgres", KindDB},
	{"=psycopg2", "Postgres", KindDB}, {"=asyncpg", "Postgres", KindDB}, {"=pg", "Postgres", KindDB}, {"=postgres", "Postgres", KindDB},
	{"go-sql-driver/mysql", "MySQL", KindDB}, {"gorm.io/driver/mysql", "MySQL", KindDB}, {"=pymysql", "MySQL", KindDB},
	{"=MySQLdb", "MySQL", KindDB}, {"=mysql2", "MySQL", KindDB}, {"=mysql", "MySQL", KindDB},
	{"mattn/go-sqlite3", "SQLite", KindDB}, {"modernc.org/sqlite", "SQLite", KindDB}, {"gorm.io/driver/sqlite", "SQLite", KindDB},
	{"=sqlite3", "SQLite", KindDB}, {"=better-sqlite3", "SQLite", KindDB},
	{"mongo-driver", "MongoDB", KindDB}, {"=pymongo", "MongoDB", KindDB}, {"=mongoose", "MongoDB", KindDB}, {"=mongodb", "MongoDB", KindDB},
	{"clickhouse", "ClickHouse", KindDB},
	// generic database layers
	{"database/sql", genericDB, KindDB}, {"gorm.io/gorm", genericDB, KindDB}, {"jmoiron/sqlx", genericDB, KindDB},
	{"go-jet/jet", genericDB, KindDB}, {"uptrace/bun", genericDB, KindDB}, {"entgo.io/ent", genericDB, KindDB},
	{"=sqlalchemy", genericDB, KindDB}, {"=django.db", genericDB, KindDB}, {"=peewee", genericDB, KindDB},
	{"@prisma/client", genericDB, KindDB}, {"=typeorm", genericDB, KindDB}, {"=sequelize", genericDB, KindDB},
	{"=knex", genericDB, KindDB}, {"drizzle-orm", genericDB, KindDB},
	// queues
	{"riverqueue/river", "River", KindQueue}, {"hibiken/asynq", "Asynq", KindQueue}, {"=celery", "Celery", KindQueue},
	{"=sidekiq", "Sidekiq", KindQueue}, {"=bullmq", "BullMQ", KindQueue}, {"=bull", "BullMQ", KindQueue},
	{"kafka-go", "Kafka", KindQueue}, {"sarama", "Kafka", KindQueue}, {"confluent-kafka", "Kafka", KindQueue},
	{"=kafkajs", "Kafka", KindQueue}, {"=confluent_kafka", "Kafka", KindQueue}, {"=kafka", "Kafka", KindQueue},
	{"amqp091", "RabbitMQ", KindQueue}, {"streadway/amqp", "RabbitMQ", KindQueue}, {"=amqplib", "RabbitMQ", KindQueue}, {"=pika", "RabbitMQ", KindQueue},
	{"nats-io", "NATS", KindQueue}, {"=nats", "NATS", KindQueue},
	// cache
	{"go-redis", "Redis", KindCache}, {"gomodule/redigo", "Redis", KindCache}, {"rueidis", "Redis", KindCache},
	{"=redis", "Redis", KindCache}, {"=ioredis", "Redis", KindCache},
	{"memcache", "Memcached", KindCache},
	// storage
	{"minio-go", "MinIO", KindStorage}, {"=minio", "MinIO", KindStorage}, {"service/s3", "S3", KindStorage},
	{"@aws-sdk/client-s3", "S3", KindStorage}, {"=boto3", "S3", KindStorage},
	// vendors with an SDK
	{"gocloak", "Keycloak", KindAuth}, {"=keycloak-js", "Keycloak", KindAuth}, {"=keycloak-connect", "Keycloak", KindAuth},
	{"=auth0", "Auth0", KindAuth}, {"stripe", "Stripe", KindPayment}, {"firebase.google.com/go", "Firebase", KindMessaging},
	{"=firebase-admin", "Firebase", KindMessaging}, {"twilio", "Twilio", KindMessaging}, {"sendgrid", "SendGrid", KindMessaging},
	{"telegram-bot-api", "Telegram", KindMessaging}, {"go-telegram", "Telegram", KindMessaging},
}

// vendor is a well-known system recognised by a path segment of an adapter
// module or by the host of a base URL.
type vendor struct{ name, kind string }

var vendors = map[string]vendor{
	"kaspi": {"Kaspi", KindPayment}, "halyk": {"Halyk", KindPayment}, "stripe": {"Stripe", KindPayment},
	"paypal": {"PayPal", KindPayment}, "cloudpayments": {"CloudPayments", KindPayment}, "tiptoppay": {"TipTopPay", KindPayment},
	"freedompay": {"FreedomPay", KindPayment}, "yookassa": {"YooKassa", KindPayment}, "payme": {"Payme", KindPayment},
	"iiko": {"iiko", KindPOS}, "poster": {"Poster", KindPOS}, "rkeeper": {"r-keeper", KindPOS}, "r-keeper": {"r-keeper", KindPOS},
	"zebra": {"Zebra", KindOther}, "egov": {"eGov", KindHTTPAPI}, "1c": {"1C", KindOther}, "onec": {"1C", KindOther},
	"keycloak": {"Keycloak", KindAuth}, "auth0": {"Auth0", KindAuth}, "okta": {"Okta", KindAuth},
	"firebase": {"Firebase", KindMessaging}, "fcm": {"Firebase", KindMessaging}, "apns": {"APNs", KindMessaging},
	"twilio": {"Twilio", KindMessaging}, "sendgrid": {"SendGrid", KindMessaging}, "telegram": {"Telegram", KindMessaging},
	"whatsapp": {"WhatsApp", KindMessaging}, "greenapi": {"Green API", KindMessaging}, "smsc": {"SMSC", KindMessaging},
	"mailgun": {"Mailgun", KindMessaging}, "smtp": {"SMTP", KindMessaging},
	"redis": {"Redis", KindCache}, "memcached": {"Memcached", KindCache},
	"s3": {"S3", KindStorage}, "minio": {"MinIO", KindStorage},
	"postgres": {"Postgres", KindDB}, "postgresql": {"Postgres", KindDB}, "mysql": {"MySQL", KindDB},
	"mongodb": {"MongoDB", KindDB}, "kafka": {"Kafka", KindQueue}, "rabbitmq": {"RabbitMQ", KindQueue}, "nats": {"NATS", KindQueue},
}

// notSystems are adapter sub-folders that are not an outside system.
var notSystems = setOf(`db database dbs repo repos repository repositories store stores storage sql pg queue queues cache
http https grpc rest api apis common shared util utils lib libs pkg internal mock mocks model models dto dtos domain
in out inbound outbound driven driving primary secondary v1 v2 v3 base client clients httpclient casbin config
metrics logging log middleware persistence sqlcgen sqlc migrations testdata`)

// systemParents are folders whose sub-folders are separate outside systems.
var systemParents = setOf(`out outbound driven secondary client clients integration integrations external gateway
gateways provider providers sdk thirdparty third_party third-party vendors`)

// extInput is everything the externals pass needs.
type extInput struct {
	Facts   *facts.Facts
	Layers  map[string]string
	Imports map[string][]lang.Import // by file
	Calls   map[string][]lang.Call   // by fid
	Text    func(file string) string
}

type extAcc struct {
	name, kind string
	modules    map[string]bool
	direct     map[string]bool // functions that use the system themselves
}

type extSet struct {
	by map[string]*extAcc
}

func (s *extSet) get(name, kind string) *extAcc {
	k := strings.ToLower(name)
	a := s.by[k]
	if a == nil {
		a = &extAcc{name: name, kind: kind, modules: map[string]bool{}, direct: map[string]bool{}}
		s.by[k] = a
	}
	return a
}

// matchLib returns the first rule that matches an import path.
func matchLib(p string) (libRule, bool) {
	for _, r := range libRules {
		if strings.HasPrefix(r.pat, "=") {
			e := r.pat[1:]
			if p == e || strings.HasPrefix(p, e+"/") || strings.HasPrefix(p, e+".") {
				return r, true
			}
			continue
		}
		if strings.Contains(p, r.pat) {
			return r, true
		}
	}
	return libRule{}, false
}

var majorSeg = regexp.MustCompile(`^v\d+$`)

// localName is the name an import is used by in code.
func localName(imp lang.Import, language string) string {
	if imp.Alias != "" && imp.Alias != "_" && imp.Alias != "." {
		return imp.Alias
	}
	switch language {
	case "python":
		return strings.SplitN(imp.Path, ".", 2)[0]
	case "go":
		segs := strings.Split(imp.Path, "/")
		for i := len(segs) - 1; i >= 0; i-- {
			if !majorSeg.MatchString(segs[i]) {
				return strings.TrimPrefix(strings.TrimPrefix(segs[i], "go-"), "go_")
			}
		}
	}
	return ""
}

var baseURLRe = regexp.MustCompile("(?i)(?:base_?url|api_?url|api_?host|endpoint|host|url)\\w*[\"']?\\s*(?::=|=|:)\\s*[\"'`](https?://[^\"'`\\s]+)")

var envURLRe = regexp.MustCompile(`(?:process\.env|import\.meta\.env)\.([A-Z][A-Z0-9_]*(?:URL|HOST|ENDPOINT))\b`)

// envAPIName turns NEXT_PUBLIC_API_URL into "Backend API", NEXT_PUBLIC_MEDIA_URL
// into "Media API"; "" for variables that do not name an API.
func envAPIName(env string) string {
	s := env
	for _, p := range []string{"NEXT_PUBLIC_", "VITE_", "REACT_APP_", "EXPO_PUBLIC_", "PUBLIC_"} {
		s = strings.TrimPrefix(s, p)
	}
	for _, x := range []string{"_URL", "_HOST", "_ENDPOINT", "URL", "HOST"} {
		s = strings.TrimSuffix(s, x)
	}
	switch {
	case s == "" || s == "API" || s == "BACKEND" || s == "SERVER" || s == "BASE" || s == "BACKEND_API":
		return "Backend API"
	case strings.ContainsAny(s, "0123456789") || strings.Contains(s, "SENTRY") || strings.Contains(s, "SITE") ||
		strings.Contains(s, "ANALYTICS") || strings.Contains(s, "GTM") || strings.Contains(s, "APP"):
		return ""
	}
	return titleName(strings.ToLower(strings.TrimSuffix(s, "_API"))) + " API"
}

// vendorOfSegment recognises a vendor in a folder name: "kaspi", "twilio_service",
// "iiko-client". Only whole words count.
func vendorOfSegment(seg string) (vendor, bool) {
	if v, ok := vendors[seg]; ok {
		return v, true
	}
	for _, tok := range strings.FieldsFunc(seg, func(r rune) bool { return r == '_' || r == '-' || r == '.' }) {
		if v, ok := vendors[tok]; ok {
			return v, true
		}
	}
	return vendor{}, false
}

func vendorByHost(host string) (vendor, bool) {
	h := strings.ToLower(host)
	for k, v := range vendors {
		if len(k) >= 4 && strings.Contains(h, k) {
			return v, true
		}
	}
	return vendor{}, false
}

func registrable(host string) string {
	ls := strings.Split(host, ".")
	if len(ls) > 2 {
		ls = ls[len(ls)-2:]
	}
	return strings.Join(ls, ".")
}

func hostOK(h string) bool {
	h = strings.ToLower(h)
	if h == "" || !strings.Contains(h, ".") {
		return false
	}
	for _, bad := range []string{"localhost", "example.", "w3.org", "schema", "127.", "0.0.0.0", "github.com", "golang.org", "apache.org", "xmlsoap", "purl.org", "json-schema"} {
		if strings.Contains(h, bad) {
			return false
		}
	}
	return !strings.HasPrefix(h, "10.") && !strings.HasPrefix(h, "192.168.")
}

var httpImports = []string{"net/http", "=requests", "=httpx", "=axios", "=urllib", "=ky", "=got", "=node-fetch", "resty"}

func importsHTTP(imps []lang.Import) bool {
	for _, i := range imps {
		for _, h := range httpImports {
			if strings.HasPrefix(h, "=") {
				if i.Path == h[1:] || strings.HasPrefix(i.Path, h[1:]+"/") {
					return true
				}
			} else if strings.Contains(i.Path, h) {
				return true
			}
		}
	}
	return false
}

func titleName(s string) string {
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// externalRank orders kinds on the system diagram.
var externalRank = map[string]int{KindPayment: 0, KindPOS: 1, KindAuth: 2, KindMessaging: 3, KindHTTPAPI: 4, KindOther: 5,
	KindDB: 6, KindCache: 7, KindQueue: 8, KindStorage: 9}

// detectExternals finds the outside systems of a repository with plain tables:
// SDK and driver imports, adapter modules named after a system, base URLs.
func detectExternals(in extInput) []facts.External {
	f := in.Facts
	set := &extSet{by: map[string]*extAcc{}}
	attributed := map[string]bool{} // modules already explained by a name or an import
	skip := func(mod string) bool { return in.Layers[mod] == autosem.Test }

	// functions by file, to find direct users of a library
	byFile := map[string][]string{}
	for fid, fn := range f.Functions {
		byFile[fn.File] = append(byFile[fn.File], fid)
	}

	mods := make([]string, 0, len(f.Modules))
	for m := range f.Modules {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	// 1. imports of drivers and SDKs
	for _, m := range mods {
		if skip(m) {
			continue
		}
		layer := in.Layers[m]
		for _, file := range f.Modules[m].Files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			language := ""
			for _, fid := range byFile[file] {
				language = f.Functions[fid].Lang
				break
			}
			for _, imp := range in.Imports[file] {
				r, ok := matchLib(imp.Path)
				if !ok {
					continue
				}
				if layer == autosem.Adapter || layer == autosem.Infra || layer == autosem.Other {
					set.get(r.name, r.kind).modules[m] = true
					attributed[m] = true
					continue
				}
				// a service or handler: only when its own code calls the library
				ln := localName(imp, language)
				if ln == "" {
					continue
				}
				for _, fid := range byFile[file] {
					for _, c := range in.Calls[fid] {
						if strings.SplitN(c.Recv, ".", 2)[0] == ln {
							a := set.get(r.name, r.kind)
							a.modules[m] = true
							a.direct[fid] = true
						}
					}
				}
			}
		}
	}

	// 2. adapter modules named after a system
	for _, m := range mods {
		layer := in.Layers[m]
		if layer != autosem.Adapter && layer != autosem.Other && layer != autosem.Infra {
			continue
		}
		segs := strings.Split(strings.ToLower(m), "/")
		for i, seg := range segs {
			if v, ok := vendorOfSegment(seg); ok {
				set.get(v.name, v.kind).modules[m] = true
				attributed[m] = true
				break
			}
			if layer == autosem.Adapter && i > 0 && systemParents[segs[i-1]] && !notSystems[seg] && len(seg) > 1 {
				kind := KindOther
				var imps []lang.Import
				for _, file := range f.Modules[m].Files {
					imps = append(imps, in.Imports[file]...)
				}
				if importsHTTP(imps) {
					kind = KindHTTPAPI
				}
				set.get(titleName(seg), kind).modules[m] = true
				attributed[m] = true
				break
			}
		}
	}

	// 3. base URLs in adapter modules nothing else explained
	for _, m := range mods {
		if in.Layers[m] != autosem.Adapter || attributed[m] {
			continue
		}
		for _, file := range f.Modules[m].Files {
			if strings.HasSuffix(file, "_test.go") || in.Text == nil {
				continue
			}
			text := in.Text(file)
			for _, mm := range baseURLRe.FindAllStringSubmatch(text, -1) {
				u, err := url.Parse(mm[1])
				if err != nil || !hostOK(u.Hostname()) {
					continue
				}
				if v, ok := vendorByHost(u.Hostname()); ok {
					set.get(v.name, v.kind).modules[m] = true
				} else {
					set.get(registrable(u.Hostname()), KindHTTPAPI).modules[m] = true
				}
				break
			}
			// a client-side API folder takes its base URL from the environment
			for _, mm := range envURLRe.FindAllStringSubmatch(text, -1) {
				if name := envAPIName(mm[1]); name != "" {
					set.get(name, KindHTTPAPI).modules[m] = true
				}
			}
		}
	}

	resolveDialect(set, in)

	// callers from outside a system's modules
	var out []facts.External
	for _, a := range set.by {
		if len(a.modules) == 0 {
			continue
		}
		ex := facts.External{Name: a.name, Kind: a.kind, Modules: keys(a.modules)}
		callers := map[string]bool{}
		for fid := range a.direct {
			callers[fid] = true
		}
		for fid, fn := range f.Functions {
			if a.modules[fn.Module] || skip(fn.Module) || strings.HasSuffix(fn.File, "_test.go") {
				continue
			}
			for _, c := range append(append([]string(nil), fn.Callees...), fn.AsyncCallees...) {
				if cf, ok := f.Functions[c]; ok && a.modules[cf.Module] {
					callers[fid] = true
					break
				}
			}
		}
		ex.Functions = keys(callers)
		out = append(out, ex)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := externalRank[out[i].Kind], externalRank[out[j].Kind]
		if ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var dialects = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Postgres", regexp.MustCompile(`(?i)postgres|pgx|psycopg`)},
	{"MySQL", regexp.MustCompile(`(?i)mysql|mariadb`)},
	{"SQLite", regexp.MustCompile(`(?i)sqlite`)},
	{"MongoDB", regexp.MustCompile(`(?i)mongo`)},
}

// resolveDialect gives the generic "Database" the name of the one dialect the
// repository uses: a specific driver found elsewhere, or a dialect named in the
// files that use the generic layer (a connection string, a Django ENGINE, a
// Prisma provider). Several dialects or none leave it generic.
func resolveDialect(set *extSet, in extInput) {
	g := set.by[strings.ToLower(genericDB)]
	if g == nil {
		return
	}
	var specific []*extAcc
	for _, a := range set.by {
		if a.kind == KindDB && a != g {
			specific = append(specific, a)
		}
	}
	var target *extAcc
	if len(specific) == 1 {
		target = specific[0]
	} else if len(specific) == 0 && in.Text != nil {
		found := map[string]bool{}
		files := map[string]bool{}
		for m := range g.modules {
			for _, f := range in.Facts.Modules[m].Files {
				files[f] = true
			}
		}
		for _, p := range []string{"prisma/schema.prisma", "schema.prisma"} {
			files[p] = true
		}
		for f := range files {
			t := in.Text(f)
			for _, d := range dialects {
				if t != "" && d.re.MatchString(t) {
					found[d.name] = true
				}
			}
		}
		if len(found) == 1 {
			for n := range found {
				target = set.get(n, KindDB)
			}
		}
	}
	if target == nil {
		return
	}
	for m := range g.modules {
		target.modules[m] = true
	}
	for fid := range g.direct {
		target.direct[fid] = true
	}
	delete(set.by, strings.ToLower(genericDB))
}

// addSystemContext fills Externals and States of the facts.
func addSystemContext(out *facts.Facts, fns map[string]*fn, order []string, rawImports map[string]struct {
	lang string
	imps []lang.Import
}, texts map[string]string, queries map[string]string) {
	layers := autosem.Layers(out)
	in := extInput{
		Facts:   out,
		Layers:  layers,
		Imports: map[string][]lang.Import{},
		Calls:   map[string][]lang.Call{},
		Text:    func(file string) string { return texts[file] },
	}
	for p, ri := range rawImports {
		in.Imports[p] = ri.imps
	}
	var funcs []stFunc
	for _, fid := range order {
		f := fns[fid]
		in.Calls[fid] = f.Calls
		if layers[f.module] != autosem.Test {
			funcs = append(funcs, stFunc{Fid: fid, File: f.file, Start: f.Start, End: f.End})
		}
	}
	out.Externals = detectExternals(in)
	var sfiles []stFile
	for p, t := range texts {
		m := facts.ModuleOf(p)
		if layers[m] != autosem.Test {
			sfiles = append(sfiles, stFile{Path: p, Module: m, Lang: rawImports[p].lang, Text: t})
		}
	}
	sort.Slice(sfiles, func(i, j int) bool { return sfiles[i].Path < sfiles[j].Path })
	out.States = detectStates(sfiles, funcs, queries)
}
