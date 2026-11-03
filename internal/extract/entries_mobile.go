package extract

import (
	"regexp"
	"strings"
)

var (
	swiftLifecycle = regexp.MustCompile(`^(application|scene)$|^(application|scene)(Did|Will)[A-Z]`)
	ktLifecycle    = map[string]bool{"onCreate": true, "onCreateView": true, "onStartCommand": true}
	ktComponents   = []string{"Activity", "Fragment", "Service", "Application"}
)

func hasMark(f *fn, pred func(string) bool) bool {
	for _, d := range f.Decorators {
		if pred(d) {
			return true
		}
	}
	return false
}

func isDelegate(d string) bool {
	return strings.HasPrefix(d, "conforms:") && strings.HasSuffix(d, "Delegate")
}

// swiftEntry marks lifecycle methods of application/scene delegates.
func swiftEntry(e *entryMap, fid string, f *fn) {
	if f.Cls != "" && swiftLifecycle.MatchString(f.Name) && hasMark(f, isDelegate) {
		e.mark(fid, "lifecycle", "iOS "+f.Cls+"."+f.Name)
	}
}

// swiftAppMain marks the entry of every @main / @UIApplicationMain / App type:
// its main(), else init, else didFinishLaunching, else the first function.
func swiftAppMain(e *entryMap, fns map[string]*fn, order []string) {
	type key struct{ file, cls string }
	groups := map[key][]string{}
	var keys []key
	for _, fid := range order {
		f := fns[fid]
		if f.lang != "swift" || f.Cls == "" {
			continue
		}
		if hasMark(f, func(d string) bool {
			return d == "type:main" || d == "type:UIApplicationMain" || d == "type:NSApplicationMain" || d == "conforms:App"
		}) {
			k := key{f.file, f.Cls}
			if groups[k] == nil {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], fid)
		}
	}
	for _, k := range keys {
		ids := groups[k]
		pick := ids[0]
	outer:
		for _, name := range []string{"main", "init", "applicationDidFinishLaunching"} {
			for _, id := range ids {
				if fns[id].Name == name {
					pick = id
					break outer
				}
			}
		}
		if fns[pick].Name == "application" { // prefer the didFinishLaunching overload
			for _, id := range ids {
				if fns[id].Name == "application" && strings.Contains(fns[id].Sig, "didFinishLaunching") {
					pick = id
					break
				}
			}
		}
		e.mark(pick, "main", "iOS "+k.cls+" main")
	}
}

// kotlinEntry marks Android components, Compose screens and ViewModel API.
func kotlinEntry(e *entryMap, fid string, f *fn) {
	if f.Cls != "" {
		if f.Ext {
			return
		}
		base := func(suffix string) bool {
			for _, b := range f.Bases {
				if strings.HasSuffix(b, suffix) {
					return true
				}
			}
			return false
		}
		if ktLifecycle[f.Name] {
			for _, c := range ktComponents {
				if base(c) {
					e.mark(fid, "lifecycle", "Android "+f.Cls+"."+f.Name)
					return
				}
			}
		}
		if f.Name == "onReceive" && base("BroadcastReceiver") {
			e.mark(fid, "lifecycle", "Android "+f.Cls+".onReceive")
			return
		}
		if base("ViewModel") && !strings.Contains(f.Sig, "private") && !strings.Contains(f.Sig, "protected") {
			e.mark(fid, "ui", "VM "+f.Cls+"."+f.Name)
		}
		return
	}
	if (strings.HasSuffix(f.Name, "Screen") || strings.HasSuffix(f.Name, "Route")) &&
		hasMark(f, func(d string) bool { return strings.Contains(d, "Composable") }) {
		e.mark(fid, "ui", "SCREEN "+f.Name)
	}
}
