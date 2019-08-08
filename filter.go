package tlog

import (
	"path"
	"regexp"
	"strings"
	"sync"
)

type filter struct {
	f string

	mu sync.Mutex
	c  map[Location]bool
}

func newFilter(f string) *filter {
	for _, q := range strings.Split(f, ",") {
		if q == "*" {
			f = q
			break
		}
	}

	return &filter{
		f: f,
		c: make(map[Location]bool),
	}
}

func (f *filter) match(t string) bool {
	if f == nil {
		return false
	}

	if f.f == "*" {
		return true
	}

	loc := Caller(2)

	defer f.mu.Unlock()
	f.mu.Lock()

	en, ok := f.c[loc]
	if !ok {
		en = f.matchFilter(loc, t)
		f.c[loc] = en
	}

	return en
}

func (f *filter) matchFilter(loc Location, t string) bool {
	topics := strings.Split(t, ",")
	name, file, _ := loc.NameFileLine()

	for _, ft := range strings.Split(f.f, ",") {
		if ft == "" {
			continue
		}

		lr := strings.SplitN(ft, "=", 2)

		var pt string
		if len(lr) == 1 {
			if f.matchTopics(ft, topics) {
				return true
			}
			pt = ft
		} else {
			if !f.matchTopics(lr[1], topics) {
				continue
			}
			pt = lr[0]
		}

		if f.matchPath(pt, file) {
			return true
		}

		if f.matchType(pt, name) {
			return true
		}
	}

	return false
}

func (f *filter) matchTopics(filt string, topics []string) bool {
	for _, f := range strings.Split(filt, "+") {
		if f == "*" {
			return true
		}
		for _, t := range topics {
			if f == t {
				return true
			}
		}
	}

	return false
}

func (f *filter) matchPath(pt, file string) bool {
	var b strings.Builder
	for i, seg := range strings.Split(pt, "/") {
		if seg == "" {
			continue
		}

		if i != 0 {
			b.WriteByte('/')
		}

		if seg == "*" {
			b.WriteString(`.*`)
		} else {
			b.WriteString(regexp.QuoteMeta(seg))
		}
	}

	//	Printf("file %v <- %v", b.String(), pattern)

	re := regexp.MustCompile("(^|/)" + b.String() + "$")

	return re.MatchString(file) || re.MatchString(path.Dir(file))
}

func (f *filter) matchType(pt, name string) bool {
	tp := path.Base(name)

	var b strings.Builder
	end := "$"
	for i, n := range strings.Split(pt, ".") {
		if i != 0 {
			b.WriteByte('.')
		}

		switch {
		case n == "*":
			b.WriteString(`[\w\.]+`)
			end = ""
		case regexp.MustCompile(`\(\*?\w+\)`).MatchString(n):
			n = regexp.QuoteMeta(n)
			b.WriteString(n)
			end = ""
		case regexp.MustCompile(`[\w\*]+`).MatchString(n):
			n = strings.ReplaceAll(n, "*", `.*`)
			b.WriteString(n)
			end = "$"
		default:
			return false
		}
	}

	re := regexp.MustCompile(`(^|\.)` + b.String() + end)

	if re.MatchString(tp) {
		return true
	}

	s := regexp.MustCompile(`(\w+)(\.\((\*?)(\w+)\))?\.((\w+)(\.\w+)*)`).FindStringSubmatch(tp)
	s = s[1:]

	if pt == s[0] { // pkg
		return true
	}

	if s[1] == "" { // no (*Type) (It's function)
		return false
	}

	if re.MatchString(s[0] + "." + s[2] + s[3]) { // pkg.*Type
		return true
	}

	if re.MatchString(s[0] + "." + s[3]) { // pkg.*Type
		return true
	}

	if re.MatchString(s[0] + "." + s[2] + s[3] + "." + s[4]) { // pkg.*Type.Func...
		return true
	}

	if s[2] == "" { // *
		return false
	}

	if re.MatchString(s[0] + "." + s[3] + "." + s[4]) { // Type
		return true
	}

	if re.MatchString(s[0] + ".(" + s[3] + ")." + s[4]) { // (Type)
		return true
	}

	//	Printf("type %q <- %v  %v", s, tp, pt)

	return false
}
