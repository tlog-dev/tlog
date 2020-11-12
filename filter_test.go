package tlog

import (
	"testing"

	"github.com/nikandfor/tlog/loc"
	"github.com/stretchr/testify/assert"
)

func TestFilterMatchTopics(t *testing.T) {
	var ff filter

	assert.True(t, ff.matchTopics("a", []string{"a", "b", "c"}))
	assert.True(t, ff.matchTopics("a+b", []string{"a", "b", "c"}))
	assert.True(t, ff.matchTopics("c+d", []string{"a", "b", "c"}))

	assert.True(t, ff.matchTopics("*", []string{"a", "b", "c"}))
	assert.True(t, ff.matchTopics("a+*+c", []string{"a", "b", "c"}))

	assert.False(t, ff.matchTopics("d", []string{"a", "b", "c"}))
	assert.False(t, ff.matchTopics("d+e", []string{"a", "b", "c"}))
}

func TestFilterMatchPath(t *testing.T) {
	var ff filter

	for _, p := range []string{
		"path",
		"path/",
		"path/*",
		"long/*",
		"long/path",
		"long/path/",
		"long/path/*",
	} {
		assert.True(t, ff.matchPath(p, "long/path/file.go"), "%v", p)
	}

	for _, p := range []string{
		"ath",
		"long",
		"long/",
	} {
		assert.False(t, ff.matchPath(p, "long/path/file.go"), "%v", p)
	}
}

func TestFilterMatchType(t *testing.T) {
	var ff filter

	for _, p := range []string{
		"(*Type).Func",
		"*Type.Func",
		"(Type).Func",
		"Type.Func",
		"(*Type)",
		"Type",
		"*Type",
		"*",
		"(*Type).*",
		"Type.*",
		"*Type.*",
	} {
		for _, path := range []string{"", "path."} {
			assert.True(t, ff.matchType(path+p, "path.(*Type).Func"), "%v", path+p)
		}
	}

	for _, p := range []string{
		"(Type).Func",
		"Type.Func",
	} {
		for _, path := range []string{"", "path."} {
			for _, tp := range []string{
				"path.(Type).Func",
			} {
				assert.True(t, ff.matchType(path+p, tp), "%v == %v", path+p, tp)
			}
		}
	}

	for _, p := range []string{
		"(*Type).Func*",
		"*Type.Func*",
		"(Type).Func*",
		"Type.Func*",
	} {
		for _, path := range []string{"", "path."} {
			for _, tp := range []string{
				"Func",
				"Func.func1",
				"Func.func1.func2",
			} {
				assert.True(t, ff.matchType(path+p, "path.(*Type)."+tp), "%v == %v", path+p, tp)
			}
		}
	}

	for _, p := range []string{
		"unc",
		"Fun",
		"(*Type).Fun",
		"Type.Fun",
		"path/(*Type).Func",
		"path/Type.Func",
	} {
		assert.False(t, ff.matchType(p, "long/path.(*Type).Func"), "%v", p)
	}

	assert.True(t, ff.matchType("path", "path.(*Type).Func"))

	assert.False(t, ff.matchType("(*Type).Func", "long/path.(Type).Func"))

	assert.False(t, ff.matchType("%$^", "long/path.(Type).Func"))
}

func TestFilterMatchFilter(t *testing.T) {
	assert.True(t, newFilter("a,b").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("filter_test.go").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("tlog").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("tlog=a").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("tlog=a+b").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("=a").matchFilter(loc.Caller(0), "a"))
	assert.False(t, newFilter("tlog=b").matchFilter(loc.Caller(0), "a"))
	assert.False(t, newFilter("tlog=b,").matchFilter(loc.Caller(0), "a"))
	assert.False(t, newFilter("=a").matchFilter(loc.Caller(0), "b"))

	assert.True(t, newFilter("TestFilterMatchFilter").matchFilter(loc.Caller(0), "a"))
	assert.False(t, newFilter("TestFilterMatchType").matchFilter(loc.Caller(0), "a"))

	// include/exclude
	assert.False(t, newFilter("a,b,!a").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("a,b,!another_file.go=a").matchFilter(loc.Caller(0), "a,b"))
	assert.False(t, newFilter("a,b,c,!filter_test.go=a").matchFilter(loc.Caller(0), "a,c,d"))
	assert.False(t, newFilter("!a").matchFilter(loc.Caller(0), "a"))
	assert.True(t, newFilter("!a").matchFilter(loc.Caller(0), "b"))
}

func TestFilterMatchBase(t *testing.T) {
	assert.False(t, newFilter("").match("a", 0))

	assert.True(t, newFilter("*").match("", 0))

	assert.True(t, newFilter("a,*,b=c").match("q", loc.Caller(0)))

	assert.False(t, newFilter("*,!a").match("a", loc.Caller(0)))
}

func BenchmarkMatchFilter(b *testing.B) {
	b.ReportAllocs()

	f := newFilter("a,b,!another_file.go=a")

	c := loc.Caller(0)

	for i := 0; i < b.N; i++ {
		f.matchFilter(c, "a,b")
	}
}
