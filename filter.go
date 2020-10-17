package tlog

import (
	"path"
	"regexp"
	"strings"
)

type (
	filter struct {
		f string

		c map[filterkey]bool
	}

	filterkey struct {
		l  PC
		tp string
	}
)

func newFilter(f string) *filter {
	if f == "" {
		return nil
	}

	return &filter{
		f: f,
		c: make(map[filterkey]bool),
	}
}

func (f *filter) match(t string) bool {
	if f == nil || f.f == "" {
		return false
	}

	if f.f == "*" {
		return true
	}

	loc := Caller(3)

	k := filterkey{
		l:  loc,
		tp: t,
	}

	en, ok := f.c[k]
	if !ok {
		en = f.matchFilter(loc, t)
		f.c[k] = en
	}

	return en
}

func (f *filter) matchFilter(loc PC, t string) bool {
	topics := strings.Split(t, ",")
	name, file, _ := loc.NameFileLine()

	var ok bool

	for i, ft := range strings.Split(f.f, ",") {
		if ft == "" {
			continue
		}
		set := true
		if ft[0] == '!' {
			if i == 0 {
				ok = true
			}
			set = false
			ft = ft[1:]
		}

		lr := strings.SplitN(ft, "=", 2)

		var pt string
		if len(lr) == 1 {
			if f.matchTopics(ft, topics) {
				ok = set
				continue
			}
			pt = ft
		} else {
			if !f.matchTopics(lr[1], topics) {
				continue
			}
			pt = lr[0]
		}

		if pt == "" {
			ok = set
			continue
		}

		if f.matchPath(pt, file) {
			ok = set
			continue
		}

		if f.matchType(pt, name) {
			ok = set
			continue
		}
	}

	return ok
}

func (f *filter) matchTopics(filt string, topics []string) bool {
	ff := strings.Split(filt, "+")
	for i := 0; i < len(ff); i++ {
		if ff[i] == "*" {
			return true
		}

		for _, t := range topics {
			if ff[i] == t {
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
