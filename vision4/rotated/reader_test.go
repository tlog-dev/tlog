package rotated

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextNameLogrotate(t *testing.T) {
	base := "file.log"

	fname := func(base string, i int) string {
		n := base
		if i != 0 {
			ext := filepath.Ext(base)
			n = strings.TrimSuffix(base, ext) + strconv.FormatInt(int64(i), 4) + ext
		}

		return n
	}

	testNextName(t, 5, base, fname)
}

func TestNextNameSubstNum(t *testing.T) {
	base := "file_@.log"

	fname := func(b string, i int) string {
		return strings.Replace(base, string(SubstituteSymbol), strconv.FormatInt(int64(i), 4), 1)
	}

	testNextName(t, 5, base, fname)
}

func TestNextNameSubstStr(t *testing.T) {
	base := "file_@.log"

	fname := func(b string, i int) string {
		return strings.Replace(base, string(SubstituteSymbol), fmt.Sprintf("%d:%d", i/4, i%4), 1)
	}

	testNextName(t, 5, base, fname)
}

func testNextName(t *testing.T, N int, base string, fname func(string, int) string) {
	d, err := ioutil.TempDir("", "rotated_")
	require.NoError(t, err)

	defer func() {
		err = os.RemoveAll(d)
		assert.NoError(t, err)
	}()

	tl = tlog.NewTestLogger(t, "", nil)

	for i := 0; i < N; i++ {
		n := fname(base, i)

		err = ioutil.WriteFile(filepath.Join(d, n), []byte(fmt.Sprintf("content_%d\n", i)), 0664)
		require.NoError(t, err)

		tl.Printf("file %v", n)
	}

	// forward
	f := NextName(true)

	var next string
	var ok bool
	for i := 0; i < N; i++ {
		next, ok, err = f(filepath.Join(d, base), next)
		require.NoError(t, err)
		assert.True(t, ok)

		want := fname(base, i)
		want = filepath.Join(d, want)

		assert.Equal(t, want, next)

		next = want
	}

	next, ok, err = f(base, next)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "", next)

	// backward
	f = NextName(false)

	for i := N - 1; i >= 0; i-- {
		next, ok, err = f(filepath.Join(d, base), next)
		require.NoError(t, err)
		assert.True(t, ok)

		want := fname(base, i)
		want = filepath.Join(d, want)

		assert.Equal(t, want, next)

		next = want
	}

	next, ok, err = f(base, next)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "", next)
}
