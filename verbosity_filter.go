package tlog

import (
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"tlog.app/go/loc"
)

type (
	filter struct {
		f string

		mu sync.RWMutex  `deep:"-"`
		c  map[fkey]bool `deep:"-"`
	}

	fkey struct {
		pc     loc.PC
		topics string
	}
)

func Verbosity() string {
	return DefaultLogger.Verbosity()
}

func SetVerbosity(vfilter string) {
	DefaultLogger.SetVerbosity(vfilter)
}

func (l *Logger) Verbosity() string {
	f := l.getfilter()
	if f == nil {
		return ""
	}

	return f.f
}

func (l *Logger) SetVerbosity(vfilter string) {
	var f *filter

	if vfilter != "" {
		f = &filter{
			f: vfilter,
			c: make(map[fkey]bool),
		}
	}

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter)), unsafe.Pointer(f))
}

func V(topics string) *Logger {
	if DefaultLogger.ifv(0, topics) {
		return DefaultLogger
	}

	return nil
}

func If(topics string) bool {
	return DefaultLogger.ifv(0, topics)
}

func IfDepth(d int, topics string) bool {
	return DefaultLogger.ifv(d, topics)
}

func (l *Logger) V(topics string) *Logger {
	if l.ifv(0, topics) {
		return l
	}

	return nil
}

func (l *Logger) If(topics string) bool {
	return l.ifv(0, topics)
}

func (l *Logger) IfDepth(d int, topics string) bool {
	return l.ifv(d, topics)
}

func (s Span) V(topics string) Span {
	if s.Logger.ifv(0, topics) {
		return s
	}

	return Span{}
}

func (s Span) If(topics string) bool {
	return s.Logger.ifv(0, topics)
}

func (s Span) IfDepth(d int, topics string) bool {
	return s.Logger.ifv(d, topics)
}

func (l *Logger) ifv(d int, topics string) bool {
	if l == nil {
		return false
	}

	f := l.getfilter()
	if f == nil {
		return false
	}

	if f.f == "*" {
		return true
	}

	var pc loc.PC
	caller1(2+d, &pc, 1, 1)

	return f.match(pc, topics)
}

func (l *Logger) getfilter() *filter {
	return (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
}

func (f *filter) match(pc loc.PC, topics string) (r bool) {
	k := fkey{pc: pc, topics: topics}

	f.mu.RLock()
	r, ok := f.c[k]
	f.mu.RUnlock()

	if ok {
		return r
	}

	defer f.mu.Unlock()
	f.mu.Lock()

	r, ok = f.c[k]
	if ok {
		return r
	}

	name, file, _ := pc.NameFileLine()
	r = f.matchPattern(name, file, topics)

	f.c[k] = r

	return r
}

func (f *filter) matchPattern(name, file, topics string) (r bool) {
	ts := strings.Split(topics, ",")

	if f.f != "" && f.f[0] == '!' {
		r = true
	}

	for _, ff := range strings.Split(f.f, ",") {
		if ff == "" {
			continue
		}

		set := ff[0] != '!'
		ff = strings.TrimPrefix(ff, "!")

		p := strings.IndexByte(ff, '=')

		if p != -1 && ff[:p] != "" {
			if !f.matchPath(ff[:p], file) && !f.matchType(ff[:p], name) {
				continue
			}
		}

		if !f.matchTopics(ff[p+1:], ts) {
			continue
		}

		r = set
	}

	return r
}

func (f *filter) matchTopics(filt string, ts []string) bool {
	for _, ff := range strings.Split(filt, "+") {
		if ff == "" {
			continue
		}
		if ff == "*" {
			return true
		}

		for _, t := range ts {
			if ff == t {
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
	for i, n := range strings.Split(pt, ".") {
		if i != 0 {
			b.WriteByte('.')
		}

		switch {
		case n == "*":
			b.WriteString(`[\w\.]+`)
		case regexp.MustCompile(`\(\*?\w+\)`).MatchString(n):
			n = regexp.QuoteMeta(n)
			b.WriteString(n)
		case regexp.MustCompile(`[\w\*]+`).MatchString(n):
			n = strings.ReplaceAll(n, "*", `.*`)
			b.WriteString(n)
		default:
			return false
		}
	}

	re := regexp.MustCompile(`(^|\.)` + b.String() + "\\b")

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
