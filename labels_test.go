package tlog

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabels(t *testing.T) {
	var ll Labels

	ll.Set("key", "value")
	assert.ElementsMatch(t, Labels{"key=value"}, ll)

	ll.Set("key2", "val2")
	assert.ElementsMatch(t, Labels{"key=value", "key2=val2"}, ll)

	ll.Set("key", "pelupe")
	assert.ElementsMatch(t, Labels{"key=pelupe", "key2=val2"}, ll)

	ll.Del("key")
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Del("key2")
	assert.ElementsMatch(t, Labels{"=key", "=key2"}, ll)

	ll.Set("key", "value")
	assert.ElementsMatch(t, Labels{"key=value", "=key2"}, ll)

	ll.Set("key2", "")
	assert.ElementsMatch(t, Labels{"key=value", "key2"}, ll)

	ll.Merge(Labels{"", "key2=val2", "=key"})
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Del("key")
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Set("flag", "")

	v, ok := ll.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, "val2", v)

	_, ok = ll.Get("key")
	assert.False(t, ok)

	v, ok = ll.Get("flag")
	assert.True(t, ok)
	assert.Equal(t, "", v)
}

func TestDumpLabelsWithDefault(t *testing.T) {
	assert.Equal(t, Labels{"a", "b", "c"}, FillLabelsWithDefaults("a", "b", "c"))

	assert.Equal(t, Labels{"a=b", "f"}, FillLabelsWithDefaults("a=b", "f"))

	assert.Equal(t, Labels{"_hostname=myhost", "_pid=mypid", "_md5=mymd5", "_sha1=mysha1", "_project=myname"},
		FillLabelsWithDefaults("_hostname=myhost", "_pid=mypid", "_md5=mymd5", "_sha1=mysha1", "_project=myname"))

	ll := FillLabelsWithDefaults("_hostname", "_pid", "_md5", "_sha1", "_project")

	t.Logf("%v", ll)

	re := regexp.MustCompile(`_hostname=[\w-]+`)
	assert.True(t, re.MatchString(ll[0]), "%s is not %s ", ll[0], re)

	re = regexp.MustCompile(`_pid=\d+`)
	assert.True(t, re.MatchString(ll[1]), "%s is not %s ", ll[1], re)

	re = regexp.MustCompile(`^_md5=[0-9a-z]{32}$`)
	assert.True(t, re.MatchString(ll[2]), "%s is not %s ", ll[2], re)

	re = regexp.MustCompile(`^_sha1=[0-9a-z]{40}$`)
	assert.True(t, re.MatchString(ll[3]), "%s is not %s ", ll[3], re)

	re = regexp.MustCompile(`_project=tlog.test`)
	assert.True(t, re.MatchString(ll[4]), "%s is not %s ", ll[4], re)
}
